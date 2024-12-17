# Copyright (c) 2024 Microsoft Corporation.
# Licensed under the MIT License
"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
"""

import logging
import argparse
import json
import re
import traceback
from dataclasses import dataclass
from typing import Any

import tiktoken

from graphrag.claim_prompt import CLAIM_EXTRACTION_PROMPT, CONTINUE_PROMPT, LOOP_PROMPT
from graphrag.extractor import Extractor
from rag.llm.chat_model import Base as CompletionLLM
from graphrag.utils import ErrorHandlerFn, perform_variable_replacements

DEFAULT_TUPLE_DELIMITER = "<|>"
DEFAULT_RECORD_DELIMITER = "##"
DEFAULT_COMPLETION_DELIMITER = "<|COMPLETE|>"
CLAIM_MAX_GLEANINGS = 1


@dataclass
class ClaimExtractorResult:
    """Claim extractor result class definition."""

    output: list[dict]
    source_docs: dict[str, Any]


class ClaimExtractor(Extractor):
    """Claim extractor class definition."""

    _extraction_prompt: str
    _summary_prompt: str
    _output_formatter_prompt: str
    _input_text_key: str
    _input_entity_spec_key: str
    _input_claim_description_key: str
    _tuple_delimiter_key: str
    _record_delimiter_key: str
    _completion_delimiter_key: str
    _max_gleanings: int
    _on_error: ErrorHandlerFn

    def __init__(
        self,
        llm_invoker: CompletionLLM,
        extraction_prompt: str | None = None,
        input_text_key: str | None = None,
        input_entity_spec_key: str | None = None,
        input_claim_description_key: str | None = None,
        input_resolved_entities_key: str | None = None,
        tuple_delimiter_key: str | None = None,
        record_delimiter_key: str | None = None,
        completion_delimiter_key: str | None = None,
        encoding_model: str | None = None,
        max_gleanings: int | None = None,
        on_error: ErrorHandlerFn | None = None,
    ):
        """Init method definition."""
        self._llm = llm_invoker
        self._extraction_prompt = extraction_prompt or CLAIM_EXTRACTION_PROMPT
        self._input_text_key = input_text_key or "input_text"
        self._input_entity_spec_key = input_entity_spec_key or "entity_specs"
        self._tuple_delimiter_key = tuple_delimiter_key or "tuple_delimiter"
        self._record_delimiter_key = record_delimiter_key or "record_delimiter"
        self._completion_delimiter_key = (
            completion_delimiter_key or "completion_delimiter"
        )
        self._input_claim_description_key = (
            input_claim_description_key or "claim_description"
        )
        self._input_resolved_entities_key = (
            input_resolved_entities_key or "resolved_entities"
        )
        self._max_gleanings = (
            max_gleanings if max_gleanings is not None else CLAIM_MAX_GLEANINGS
        )
        self._on_error = on_error or (lambda _e, _s, _d: None)

        # Construct the looping arguments
        encoding = tiktoken.get_encoding(encoding_model or "cl100k_base")
        yes = encoding.encode("YES")
        no = encoding.encode("NO")
        self._loop_args = {"logit_bias": {yes[0]: 100, no[0]: 100}, "max_tokens": 1}

    def __call__(
        self, inputs: dict[str, Any], prompt_variables: dict | None = None
    ) -> ClaimExtractorResult:
        """Call method definition."""
        if prompt_variables is None:
            prompt_variables = {}
        texts = inputs[self._input_text_key]
        entity_spec = str(inputs[self._input_entity_spec_key])
        claim_description = inputs[self._input_claim_description_key]
        resolved_entities = inputs.get(self._input_resolved_entities_key, {})
        source_doc_map = {}

        prompt_args = {
            self._input_entity_spec_key: entity_spec,
            self._input_claim_description_key: claim_description,
            self._tuple_delimiter_key: prompt_variables.get(self._tuple_delimiter_key)
            or DEFAULT_TUPLE_DELIMITER,
            self._record_delimiter_key: prompt_variables.get(self._record_delimiter_key)
            or DEFAULT_RECORD_DELIMITER,
            self._completion_delimiter_key: prompt_variables.get(
                self._completion_delimiter_key
            )
            or DEFAULT_COMPLETION_DELIMITER,
        }

        all_claims: list[dict] = []
        for doc_index, text in enumerate(texts):
            document_id = f"d{doc_index}"
            try:
                claims = self._process_document(prompt_args, text, doc_index)
                all_claims += [
                    self._clean_claim(c, document_id, resolved_entities) for c in claims
                ]
                source_doc_map[document_id] = text
            except Exception as e:
                logging.exception("error extracting claim")
                self._on_error(
                    e,
                    traceback.format_exc(),
                    {"doc_index": doc_index, "text": text},
                )
                continue

        return ClaimExtractorResult(
            output=all_claims,
            source_docs=source_doc_map,
        )

    def _clean_claim(
        self, claim: dict, document_id: str, resolved_entities: dict
    ) -> dict:
        # clean the parsed claims to remove any claims with status = False
        obj = claim.get("object_id", claim.get("object"))
        subject = claim.get("subject_id", claim.get("subject"))

        # If subject or object in resolved entities, then replace with resolved entity
        obj = resolved_entities.get(obj, obj)
        subject = resolved_entities.get(subject, subject)
        claim["object_id"] = obj
        claim["subject_id"] = subject
        claim["doc_id"] = document_id
        return claim

    def _process_document(
        self, prompt_args: dict, doc, doc_index: int
    ) -> list[dict]:
        record_delimiter = prompt_args.get(
            self._record_delimiter_key, DEFAULT_RECORD_DELIMITER
        )
        completion_delimiter = prompt_args.get(
            self._completion_delimiter_key, DEFAULT_COMPLETION_DELIMITER
        )
        variables = {
                        self._input_text_key: doc,
                        **prompt_args,
                    }
        text = perform_variable_replacements(self._extraction_prompt, variables=variables)
        gen_conf = {"temperature": 0.5}
        results = self._chat(text, [{"role": "user", "content": "Output:"}], gen_conf)
        claims = results.strip().removesuffix(completion_delimiter)
        history = [{"role": "system", "content": text}, {"role": "assistant", "content": results}]

        # Repeat to ensure we maximize entity count
        for i in range(self._max_gleanings):
            text = perform_variable_replacements(CONTINUE_PROMPT, history=history, variables=variables)
            history.append({"role": "user", "content": text})
            extension = self._chat("", history, gen_conf)
            claims += record_delimiter + extension.strip().removesuffix(
                completion_delimiter
            )

            # If this isn't the last loop, check to see if we should continue
            if i >= self._max_gleanings - 1:
                break

            history.append({"role": "assistant", "content": extension})
            history.append({"role": "user", "content": LOOP_PROMPT})
            continuation = self._chat("", history, self._loop_args)
            if continuation != "YES":
                break

        result = self._parse_claim_tuples(claims, prompt_args)
        for r in result:
            r["doc_id"] = f"{doc_index}"
        return result

    def _parse_claim_tuples(
        self, claims: str, prompt_variables: dict
    ) -> list[dict[str, Any]]:
        """Parse claim tuples."""
        record_delimiter = prompt_variables.get(
            self._record_delimiter_key, DEFAULT_RECORD_DELIMITER
        )
        completion_delimiter = prompt_variables.get(
            self._completion_delimiter_key, DEFAULT_COMPLETION_DELIMITER
        )
        tuple_delimiter = prompt_variables.get(
            self._tuple_delimiter_key, DEFAULT_TUPLE_DELIMITER
        )

        def pull_field(index: int, fields: list[str]) -> str | None:
            return fields[index].strip() if len(fields) > index else None

        result: list[dict[str, Any]] = []
        claims_values = (
            claims.strip().removesuffix(completion_delimiter).split(record_delimiter)
        )
        for claim in claims_values:
            claim = claim.strip().removeprefix("(").removesuffix(")")
            claim = re.sub(r".*Output:", "", claim)

            # Ignore the completion delimiter
            if claim == completion_delimiter:
                continue

            claim_fields = claim.split(tuple_delimiter)
            o = {
                "subject_id": pull_field(0, claim_fields),
                "object_id": pull_field(1, claim_fields),
                "type": pull_field(2, claim_fields),
                "status": pull_field(3, claim_fields),
                "start_date": pull_field(4, claim_fields),
                "end_date": pull_field(5, claim_fields),
                "description": pull_field(6, claim_fields),
                "source_text": pull_field(7, claim_fields),
                "doc_id": pull_field(8, claim_fields),
            }
            if any([not o["subject_id"], not o["object_id"], o["subject_id"].lower() == "none", o["object_id"] == "none"]):
                continue
            result.append(o)
        return result


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument('-t', '--tenant_id', default=False, help="Tenant ID", action='store', required=True)
    parser.add_argument('-d', '--doc_id', default=False, help="Document ID", action='store', required=True)
    args = parser.parse_args()

    from api.db import LLMType
    from api.db.services.llm_service import LLMBundle
    from api import settings
    from api.db.services.knowledgebase_service import KnowledgebaseService

    kb_ids = KnowledgebaseService.get_kb_ids(args.tenant_id)

    ex = ClaimExtractor(LLMBundle(args.tenant_id, LLMType.CHAT))
    docs = [d["content_with_weight"] for d in settings.retrievaler.chunk_list(args.doc_id, args.tenant_id, kb_ids, max_count=12, fields=["content_with_weight"])]
    info = {
        "input_text": docs,
        "entity_specs": "organization, person",
        "claim_description": ""
    }
    claim = ex(info)
    logging.info(json.dumps(claim.output, ensure_ascii=False, indent=2))
