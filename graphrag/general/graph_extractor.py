# Copyright (c) 2024 Microsoft Corporation.
# Licensed under the MIT License
"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
"""

import re
from typing import Any, Callable
from dataclasses import dataclass
import tiktoken
import trio

from graphrag.general.extractor import Extractor, ENTITY_EXTRACTION_MAX_GLEANINGS, DEFAULT_ENTITY_TYPES
from graphrag.general.graph_prompt import GRAPH_EXTRACTION_PROMPT, CONTINUE_PROMPT, LOOP_PROMPT
from graphrag.utils import ErrorHandlerFn, perform_variable_replacements, chat_limiter
from rag.llm.chat_model import Base as CompletionLLM
import networkx as nx
from rag.utils import num_tokens_from_string

DEFAULT_TUPLE_DELIMITER = "<|>"
DEFAULT_RECORD_DELIMITER = "##"
DEFAULT_COMPLETION_DELIMITER = "<|COMPLETE|>"


@dataclass
class GraphExtractionResult:
    """Unipartite graph extraction result class definition."""

    output: nx.Graph
    source_docs: dict[Any, Any]


class GraphExtractor(Extractor):
    """Unipartite graph extractor class definition."""

    _join_descriptions: bool
    _tuple_delimiter_key: str
    _record_delimiter_key: str
    _entity_types_key: str
    _input_text_key: str
    _completion_delimiter_key: str
    _entity_name_key: str
    _input_descriptions_key: str
    _extraction_prompt: str
    _summarization_prompt: str
    _loop_args: dict[str, Any]
    _max_gleanings: int
    _on_error: ErrorHandlerFn

    def __init__(
        self,
        llm_invoker: CompletionLLM,
        language: str | None = "English",
        entity_types: list[str] | None = None,
        get_entity: Callable | None = None,
        set_entity: Callable | None = None,
        get_relation: Callable | None = None,
        set_relation: Callable | None = None,
        tuple_delimiter_key: str | None = None,
        record_delimiter_key: str | None = None,
        input_text_key: str | None = None,
        entity_types_key: str | None = None,
        completion_delimiter_key: str | None = None,
        join_descriptions=True,
        max_gleanings: int | None = None,
        on_error: ErrorHandlerFn | None = None,
    ):
        super().__init__(llm_invoker, language, entity_types, get_entity, set_entity, get_relation, set_relation)
        """Init method definition."""
        # TODO: streamline construction
        self._llm = llm_invoker
        self._join_descriptions = join_descriptions
        self._input_text_key = input_text_key or "input_text"
        self._tuple_delimiter_key = tuple_delimiter_key or "tuple_delimiter"
        self._record_delimiter_key = record_delimiter_key or "record_delimiter"
        self._completion_delimiter_key = (
            completion_delimiter_key or "completion_delimiter"
        )
        self._entity_types_key = entity_types_key or "entity_types"
        self._extraction_prompt = GRAPH_EXTRACTION_PROMPT
        self._max_gleanings = (
            max_gleanings
            if max_gleanings is not None
            else ENTITY_EXTRACTION_MAX_GLEANINGS
        )
        self._on_error = on_error or (lambda _e, _s, _d: None)
        self.prompt_token_count = num_tokens_from_string(self._extraction_prompt)

        # Construct the looping arguments
        encoding = tiktoken.get_encoding("cl100k_base")
        yes = encoding.encode("YES")
        no = encoding.encode("NO")
        self._loop_args = {"logit_bias": {yes[0]: 100, no[0]: 100}, "max_tokens": 1}

        # Wire defaults into the prompt variables
        self._prompt_variables = {
            "entity_types": entity_types,
            self._tuple_delimiter_key: DEFAULT_TUPLE_DELIMITER,
            self._record_delimiter_key: DEFAULT_RECORD_DELIMITER,
            self._completion_delimiter_key: DEFAULT_COMPLETION_DELIMITER,
            self._entity_types_key: ",".join(DEFAULT_ENTITY_TYPES),
        }

    async def _process_single_content(self, chunk_key_dp: tuple[str, str], chunk_seq: int, num_chunks: int, out_results):
        token_count = 0
        chunk_key = chunk_key_dp[0]
        content = chunk_key_dp[1]
        variables = {
            **self._prompt_variables,
            self._input_text_key: content,
        }
        gen_conf = {"temperature": 0.3}
        hint_prompt = perform_variable_replacements(self._extraction_prompt, variables=variables)
        async with chat_limiter:
            response = await trio.to_thread.run_sync(lambda: self._chat(hint_prompt, [{"role": "user", "content": "Output:"}], gen_conf))
        token_count += num_tokens_from_string(hint_prompt + response)

        results = response or ""
        history = [{"role": "system", "content": hint_prompt}, {"role": "user", "content": response}]

        # Repeat to ensure we maximize entity count
        for i in range(self._max_gleanings):
            text = perform_variable_replacements(CONTINUE_PROMPT, history=history, variables=variables)
            history.append({"role": "user", "content": text})
            async with chat_limiter:
                response = await trio.to_thread.run_sync(lambda: self._chat("", history, gen_conf))
            token_count += num_tokens_from_string("\n".join([m["content"] for m in history]) + response)
            results += response or ""

            # if this is the final glean, don't bother updating the continuation flag
            if i >= self._max_gleanings - 1:
                break
            history.append({"role": "assistant", "content": response})
            history.append({"role": "user", "content": LOOP_PROMPT})
            async with chat_limiter:
                continuation = await trio.to_thread.run_sync(lambda: self._chat("", history, {"temperature": 0.8}))
            token_count += num_tokens_from_string("\n".join([m["content"] for m in history]) + response)
            if continuation != "YES":
                break
        record_delimiter = variables.get(self._record_delimiter_key, DEFAULT_RECORD_DELIMITER)
        tuple_delimiter = variables.get(self._tuple_delimiter_key, DEFAULT_TUPLE_DELIMITER)
        records = [re.sub(r"^\(|\)$", "", r.strip()) for r in results.split(record_delimiter)]
        records = [r for r in records if r.strip()]
        maybe_nodes, maybe_edges = self._entities_and_relations(chunk_key, records, tuple_delimiter)
        out_results.append((maybe_nodes, maybe_edges, token_count))
        if self.callback:
            self.callback(0.5+0.1*len(out_results)/num_chunks, msg = f"Entities extraction of chunk {chunk_seq} {len(out_results)}/{num_chunks} done, {len(maybe_nodes)} nodes, {len(maybe_edges)} edges, {token_count} tokens.")
