# Copyright (c) 2024 Microsoft Corporation.
# Licensed under the MIT License
"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
"""
import logging
import re
from typing import Any, Callable
from dataclasses import dataclass
from graphrag.general.extractor import Extractor, ENTITY_EXTRACTION_MAX_GLEANINGS
from graphrag.light.graph_prompt import PROMPTS
from graphrag.utils import pack_user_ass_to_openai_messages, split_string_by_multi_markers
from rag.llm.chat_model import Base as CompletionLLM
import networkx as nx
from rag.utils import num_tokens_from_string


@dataclass
class GraphExtractionResult:
    """Unipartite graph extraction result class definition."""

    output: nx.Graph
    source_docs: dict[Any, Any]


class GraphExtractor(Extractor):

    _max_gleanings: int

    def __init__(
        self,
        llm_invoker: CompletionLLM,
        language: str | None = "English",
        entity_types: list[str] | None = None,
        get_entity: Callable | None = None,
        set_entity: Callable | None = None,
        get_relation: Callable | None = None,
        set_relation: Callable | None = None,
        example_number: int = 2,
        max_gleanings: int | None = None,
    ):
        super().__init__(llm_invoker, language, entity_types, get_entity, set_entity, get_relation, set_relation)
        """Init method definition."""
        self._max_gleanings = (
            max_gleanings
            if max_gleanings is not None
            else ENTITY_EXTRACTION_MAX_GLEANINGS
        )
        self._example_number = example_number
        examples = "\n".join(
                PROMPTS["entity_extraction_examples"][: int(self._example_number)]
            )

        example_context_base = dict(
            tuple_delimiter=PROMPTS["DEFAULT_TUPLE_DELIMITER"],
            record_delimiter=PROMPTS["DEFAULT_RECORD_DELIMITER"],
            completion_delimiter=PROMPTS["DEFAULT_COMPLETION_DELIMITER"],
            entity_types=",".join(self._entity_types),
            language=self._language,
        )
        # add example's format
        examples = examples.format(**example_context_base)

        self._entity_extract_prompt = PROMPTS["entity_extraction"]
        self._context_base = dict(
            tuple_delimiter=PROMPTS["DEFAULT_TUPLE_DELIMITER"],
            record_delimiter=PROMPTS["DEFAULT_RECORD_DELIMITER"],
            completion_delimiter=PROMPTS["DEFAULT_COMPLETION_DELIMITER"],
            entity_types=",".join(self._entity_types),
            examples=examples,
            language=self._language,
        )

        self._continue_prompt = PROMPTS["entiti_continue_extraction"]
        self._if_loop_prompt = PROMPTS["entiti_if_loop_extraction"]

        self._left_token_count = llm_invoker.max_length - num_tokens_from_string(
            self._entity_extract_prompt.format(
                **self._context_base, input_text="{input_text}"
            ).format(**self._context_base, input_text="")
        )
        self._left_token_count = max(llm_invoker.max_length * 0.6, self._left_token_count)

    def _process_single_content(self, chunk_key_dp: tuple[str, str]):
        token_count = 0
        chunk_key = chunk_key_dp[0]
        content = chunk_key_dp[1]
        hint_prompt = self._entity_extract_prompt.format(
            **self._context_base, input_text="{input_text}"
        ).format(**self._context_base, input_text=content)

        try:
            gen_conf = {"temperature": 0.8}
            final_result = self._chat(hint_prompt, [{"role": "user", "content": "Output:"}], gen_conf)
            token_count += num_tokens_from_string(hint_prompt + final_result)
            history = pack_user_ass_to_openai_messages("Output:", final_result, self._continue_prompt)
            for now_glean_index in range(self._max_gleanings):
                glean_result = self._chat(hint_prompt, history, gen_conf)
                history.extend([{"role": "assistant", "content": glean_result}, {"role": "user", "content": self._continue_prompt}])
                token_count += num_tokens_from_string("\n".join([m["content"] for m in history]) + hint_prompt + self._continue_prompt)
                final_result += glean_result
                if now_glean_index == self._max_gleanings - 1:
                    break

                if_loop_result = self._chat(self._if_loop_prompt, history, gen_conf)
                token_count += num_tokens_from_string("\n".join([m["content"] for m in history]) + if_loop_result + self._if_loop_prompt)
                if_loop_result = if_loop_result.strip().strip('"').strip("'").lower()
                if if_loop_result != "yes":
                    break

            records = split_string_by_multi_markers(
                final_result,
                [self._context_base["record_delimiter"], self._context_base["completion_delimiter"]],
            )
            rcds = []
            for record in records:
                record = re.search(r"\((.*)\)", record)
                if record is None:
                    continue
                rcds.append(record.group(1))
            records = rcds
            maybe_nodes, maybe_edges = self._entities_and_relations(chunk_key, records, self._context_base["tuple_delimiter"])
            return maybe_nodes, maybe_edges, token_count
        except Exception as e:
            logging.exception("error extracting graph")
            return e, None, None
