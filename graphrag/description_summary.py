# Copyright (c) 2024 Microsoft Corporation.
# Licensed under the MIT License
"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
"""

import json
from dataclasses import dataclass

from graphrag.extractor import Extractor
from graphrag.utils import ErrorHandlerFn, perform_variable_replacements
from rag.llm.chat_model import Base as CompletionLLM

from rag.utils import num_tokens_from_string

SUMMARIZE_PROMPT = """
You are a helpful assistant responsible for generating a comprehensive summary of the data provided below.
Given one or two entities, and a list of descriptions, all related to the same entity or group of entities.
Please concatenate all of these into a single, comprehensive description. Make sure to include information collected from all the descriptions.
If the provided descriptions are contradictory, please resolve the contradictions and provide a single, coherent summary.
Make sure it is written in third person, and include the entity names so we the have full context.

#######
-Data-
Entities: {entity_name}
Description List: {description_list}
#######
Output:
"""

# Max token size for input prompts
DEFAULT_MAX_INPUT_TOKENS = 4_000
# Max token count for LLM answers
DEFAULT_MAX_SUMMARY_LENGTH = 128


@dataclass
class SummarizationResult:
    """Unipartite graph extraction result class definition."""

    items: str | tuple[str, str]
    description: str


class SummarizeExtractor(Extractor):
    """Unipartite graph extractor class definition."""

    _entity_name_key: str
    _input_descriptions_key: str
    _summarization_prompt: str
    _on_error: ErrorHandlerFn
    _max_summary_length: int
    _max_input_tokens: int

    def __init__(
        self,
        llm_invoker: CompletionLLM,
        entity_name_key: str | None = None,
        input_descriptions_key: str | None = None,
        summarization_prompt: str | None = None,
        on_error: ErrorHandlerFn | None = None,
        max_summary_length: int | None = None,
        max_input_tokens: int | None = None,
    ):
        """Init method definition."""
        # TODO: streamline construction
        self._llm = llm_invoker
        self._entity_name_key = entity_name_key or "entity_name"
        self._input_descriptions_key = input_descriptions_key or "description_list"

        self._summarization_prompt = summarization_prompt or SUMMARIZE_PROMPT
        self._on_error = on_error or (lambda _e, _s, _d: None)
        self._max_summary_length = max_summary_length or DEFAULT_MAX_SUMMARY_LENGTH
        self._max_input_tokens = max_input_tokens or DEFAULT_MAX_INPUT_TOKENS

    def __call__(
        self,
        items: str | tuple[str, str],
        descriptions: list[str],
    ) -> SummarizationResult:
        """Call method definition."""
        result = ""
        if len(descriptions) == 0:
            result = ""
        if len(descriptions) == 1:
            result = descriptions[0]
        else:
            result = self._summarize_descriptions(items, descriptions)

        return SummarizationResult(
            items=items,
            description=result or "",
        )

    def _summarize_descriptions(
        self, items: str | tuple[str, str], descriptions: list[str]
    ) -> str:
        """Summarize descriptions into a single description."""
        sorted_items = sorted(items) if isinstance(items, list) else items

        # Safety check, should always be a list
        if not isinstance(descriptions, list):
            descriptions = [descriptions]

            # Iterate over descriptions, adding all until the max input tokens is reached
        usable_tokens = self._max_input_tokens - num_tokens_from_string(
            self._summarization_prompt
        )
        descriptions_collected = []
        result = ""

        for i, description in enumerate(descriptions):
            usable_tokens -= num_tokens_from_string(description)
            descriptions_collected.append(description)

            # If buffer is full, or all descriptions have been added, summarize
            if (usable_tokens < 0 and len(descriptions_collected) > 1) or (
                i == len(descriptions) - 1
            ):
                # Calculate result (final or partial)
                result = await self._summarize_descriptions_with_llm(
                    sorted_items, descriptions_collected
                )

                # If we go for another loop, reset values to new
                if i != len(descriptions) - 1:
                    descriptions_collected = [result]
                    usable_tokens = (
                        self._max_input_tokens
                        - num_tokens_from_string(self._summarization_prompt)
                        - num_tokens_from_string(result)
                    )

        return result

    def _summarize_descriptions_with_llm(
        self, items: str | tuple[str, str] | list[str], descriptions: list[str]
    ):
        """Summarize descriptions using the LLM."""
        variables = {
                        self._entity_name_key: json.dumps(items),
                        self._input_descriptions_key: json.dumps(sorted(descriptions)),
                    }
        text = perform_variable_replacements(self._summarization_prompt, variables=variables)
        return self._chat("", [{"role": "user", "content": text}])
