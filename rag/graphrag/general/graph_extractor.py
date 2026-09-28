# Copyright (c) 2024 Microsoft Corporation.
# Licensed under the MIT License

"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
"""

import re
from typing import Any
from dataclasses import dataclass
import tiktoken

from rag.graphrag.general.extractor import Extractor, ENTITY_EXTRACTION_MAX_GLEANINGS
from rag.graphrag.general.graph_prompt import GRAPH_EXTRACTION_PROMPT, CONTINUE_PROMPT, LOOP_PROMPT
from rag.graphrag.utils import ErrorHandlerFn, perform_variable_replacements, chat_limiter, split_string_by_multi_markers
from rag.llm.chat_model import Base as CompletionLLM
import networkx as nx
from common.token_utils import num_tokens_from_string

DEFAULT_TUPLE_DELIMITER = "<|>"
DEFAULT_RECORD_DELIMITER = "##"
DEFAULT_COMPLETION_DELIMITER = "<|COMPLETE|>"


# ---------------------------------------------------------------------------
# Loop-argument constants
# ---------------------------------------------------------------------------
# GraphRAG's continuation prompt is "Y"/"N" and RAGFlow uses cl100k_base's
# first tokens for both as a logit_bias to force the LLM's next-token choice.
# Yes, that is a constant: the BPE table and the strings are fixed for the
# lifetime of the process. Computing the encodings on every GraphExtractor
# instantiation loads the rank table and re-runs encode() for no semantic
# change. Cache at module scope; populate lazily on first use so optional /
# unusual-tiktoken environments can still import this module.
_LOOP_LOGIT_BIAS: dict[int, int] | None = None
_EXTRACTION_PROMPT_TOKEN_COUNT: int | None = None


def _build_loop_logit_bias() -> dict[int, int]:
    """Return the YES/NO logit_bias keyed on cl100k_base first tokens."""
    global _LOOP_LOGIT_BIAS
    if _LOOP_LOGIT_BIAS is None:
        encoding = tiktoken.get_encoding("cl100k_base")
        _LOOP_LOGIT_BIAS = {
            encoding.encode("YES")[0]: 100,
            encoding.encode("NO")[0]: 100,
        }
    return _LOOP_LOGIT_BIAS


def _loop_args() -> dict[str, Any]:
    return {"logit_bias": _build_loop_logit_bias(), "max_tokens": 1}


def _extraction_prompt_token_count(prompt: str) -> int:
    """Return the cl100k_base token count for ``prompt``.

    For the default extraction prompt the prompt is a module constant, so we
    cache the first computed value; for any other prompt (e.g. a custom one
    passed at construction) we always recompute.
    """
    global _EXTRACTION_PROMPT_TOKEN_COUNT
    if prompt is GRAPH_EXTRACTION_PROMPT and _EXTRACTION_PROMPT_TOKEN_COUNT is not None:
        return _EXTRACTION_PROMPT_TOKEN_COUNT
    count = num_tokens_from_string(prompt)
    if prompt is GRAPH_EXTRACTION_PROMPT:
        _EXTRACTION_PROMPT_TOKEN_COUNT = count
    return count


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
        tuple_delimiter_key: str | None = None,
        record_delimiter_key: str | None = None,
        input_text_key: str | None = None,
        entity_types_key: str | None = None,
        completion_delimiter_key: str | None = None,
        join_descriptions=True,
        max_gleanings: int | None = None,
        on_error: ErrorHandlerFn | None = None,
    ):
        super().__init__(llm_invoker, language, entity_types)
        """Init method definition."""
        # TODO: streamline construction
        self._llm = llm_invoker
        self._join_descriptions = join_descriptions
        self._input_text_key = input_text_key or "input_text"
        self._tuple_delimiter_key = tuple_delimiter_key or "tuple_delimiter"
        self._record_delimiter_key = record_delimiter_key or "record_delimiter"
        self._completion_delimiter_key = completion_delimiter_key or "completion_delimiter"
        self._entity_types_key = entity_types_key or "entity_types"
        self._extraction_prompt = GRAPH_EXTRACTION_PROMPT
        self._max_gleanings = max_gleanings if max_gleanings is not None else ENTITY_EXTRACTION_MAX_GLEANINGS
        self._on_error = on_error or (lambda _e, _s, _d: None)
        self.prompt_token_count = _extraction_prompt_token_count(self._extraction_prompt)

        # Construct the looping arguments
        self._loop_args = _loop_args()

        # Wire defaults into the prompt variables
        self._prompt_variables = {
            self._tuple_delimiter_key: DEFAULT_TUPLE_DELIMITER,
            self._record_delimiter_key: DEFAULT_RECORD_DELIMITER,
            self._completion_delimiter_key: DEFAULT_COMPLETION_DELIMITER,
            self._entity_types_key: ",".join(entity_types),
        }

    async def _process_single_content(self, chunk_key_dp: tuple[str, str], chunk_seq: int, num_chunks: int, out_results, task_id=""):
        token_count = 0
        chunk_key = chunk_key_dp[0]
        content = chunk_key_dp[1]
        variables = {
            **self._prompt_variables,
            self._input_text_key: content,
        }
        hint_prompt = perform_variable_replacements(self._extraction_prompt, variables=variables)
        async with chat_limiter:
            response = await self._async_chat(hint_prompt, [{"role": "user", "content": "Output:"}], {}, task_id)
        token_count += num_tokens_from_string(hint_prompt + response)

        results = response or ""
        history = [{"role": "system", "content": hint_prompt}, {"role": "user", "content": response}]

        # Repeat to ensure we maximize entity count
        for i in range(self._max_gleanings):
            history.append({"role": "user", "content": CONTINUE_PROMPT})
            async with chat_limiter:
                response = await self._async_chat("", history, {}, task_id)
            token_count += num_tokens_from_string("\n".join([m["content"] for m in history]) + response)
            results += response or ""

            # if this is the final glean, don't bother updating the continuation flag
            if i >= self._max_gleanings - 1:
                break
            history.append({"role": "assistant", "content": response})
            history.append({"role": "user", "content": LOOP_PROMPT})
            async with chat_limiter:
                continuation = await self._async_chat("", history, {}, task_id)
            token_count += num_tokens_from_string("\n".join([m["content"] for m in history]) + response)
            if continuation != "Y":
                break
            history.append({"role": "assistant", "content": "Y"})

        records = split_string_by_multi_markers(
            results,
            [self._prompt_variables[self._record_delimiter_key], self._prompt_variables[self._completion_delimiter_key]],
        )
        rcds = []
        for record in records:
            record = re.search(r"\((.*)\)", record)
            if record is None:
                continue
            rcds.append(record.group(1))
        records = rcds
        maybe_nodes, maybe_edges = self._entities_and_relations(chunk_key, records, self._prompt_variables[self._tuple_delimiter_key])
        out_results.append((maybe_nodes, maybe_edges, token_count))
        if self.callback:
            self.callback(
                0.5 + 0.1 * len(out_results) / num_chunks,
                msg=f"Entities extraction of chunk {chunk_seq + 1} {len(out_results)}/{num_chunks} done, {len(maybe_nodes)} nodes, {len(maybe_edges)} edges, {token_count} tokens.",
            )
