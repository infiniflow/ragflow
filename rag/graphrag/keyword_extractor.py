#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

import asyncio
import logging
import os
import re
from collections import Counter
from typing import Callable

from common.exceptions import TaskCanceledException

DEFAULT_ENTITY_TYPES = ["organization", "person", "geo", "event", "category"]
YIELD_EVERY = 50
logger = logging.getLogger(__name__)
_STOPWORDS = {
    "about",
    "above",
    "after",
    "again",
    "against",
    "also",
    "among",
    "because",
    "before",
    "being",
    "between",
    "could",
    "during",
    "from",
    "have",
    "into",
    "more",
    "only",
    "other",
    "over",
    "same",
    "some",
    "such",
    "than",
    "that",
    "their",
    "there",
    "these",
    "they",
    "this",
    "through",
    "under",
    "using",
    "when",
    "where",
    "which",
    "while",
    "with",
    "would",
}

_WORD_PATTERN = re.compile(r"[A-Za-z][A-Za-z0-9_-]{2,}")
_PHRASE_PATTERN = re.compile(r"\b(?:[A-Z][A-Za-z0-9_-]{2,})(?:\s+[A-Z][A-Za-z0-9_-]{2,}){0,4}\b")


class KeywordGraphExtractor:
    """Deterministic GraphRAG extractor that does not call an LLM.

    It extracts repeated keywords and capitalized phrases, then links terms that
    co-occur in the same chunk. This gives users a low-cost ingestion mode for
    large datasets or environments where LLM extraction is too expensive.
    """

    def __init__(
        self,
        llm_invoker,
        language: str | None = "English",
        entity_types: list[str] | None = None,
    ):
        del llm_invoker
        self._language = language
        self._entity_types = entity_types or DEFAULT_ENTITY_TYPES
        self._max_terms_per_chunk = max(2, int(os.environ.get("GRAPHRAG_KEYWORD_MAX_TERMS_PER_CHUNK", 12)))
        self._max_edges_per_chunk = max(1, int(os.environ.get("GRAPHRAG_KEYWORD_MAX_EDGES_PER_CHUNK", 36)))
        self._min_term_frequency = max(1, int(os.environ.get("GRAPHRAG_KEYWORD_MIN_TERM_FREQUENCY", 2)))

    def _entity_type(self) -> str:
        types = self._entity_types or DEFAULT_ENTITY_TYPES
        lowered = {t.lower(): t for t in types}
        return lowered.get("category") or lowered.get("organization") or types[0]

    @staticmethod
    def _clean_term(term: str) -> str:
        return re.sub(r"\s+", " ", term.strip(" -_.,;:()[]{}\"'")).strip()

    def _candidate_terms(self, text: str) -> list[str]:
        terms: list[str] = []
        for phrase in _PHRASE_PATTERN.findall(text):
            cleaned = self._clean_term(phrase)
            if cleaned and cleaned.lower() not in _STOPWORDS:
                terms.append(cleaned)
        for word in _WORD_PATTERN.findall(text):
            cleaned = self._clean_term(word)
            if not cleaned:
                continue
            if cleaned[0].isupper() or cleaned.isupper():
                continue
            lowered = cleaned.lower()
            if lowered in _STOPWORDS or lowered.isdigit():
                continue
            terms.append(lowered)
        return terms

    async def __call__(self, doc_id: str, chunks: list[str], callback: Callable | None = None, task_id: str = ""):
        start_ts = asyncio.get_running_loop().time()
        entity_type = self._entity_type()
        global_counts = Counter()
        chunk_term_counts: list[Counter] = []
        per_chunk_terms: list[list[str]] = []
        has_canceled = None

        logger.info("Keyword graph extraction started for doc %s with %d chunks.", doc_id, len(chunks))
        if task_id:
            from api.db.services.task_service import has_canceled as task_has_canceled

            has_canceled = task_has_canceled

        async def yield_and_check_cancel(processed: int, *, force: bool = False) -> None:
            if not force and processed % YIELD_EVERY != 0:
                return
            await asyncio.sleep(0)
            if has_canceled and has_canceled(task_id):
                logger.warning("Keyword graph extraction cancelled for task %s, doc %s.", task_id, doc_id)
                raise TaskCanceledException(f"Task {task_id} was cancelled during keyword graph extraction")

        for chunk_idx, chunk in enumerate(chunks, start=1):
            counts = Counter(self._candidate_terms(chunk))
            chunk_term_counts.append(counts)
            global_counts.update(counts)
            await yield_and_check_cancel(chunk_idx)
        await yield_and_check_cancel(len(chunk_term_counts), force=True)

        for chunk_idx, counts in enumerate(chunk_term_counts, start=1):
            ranked_terms = [
                term
                for term, _count in counts.most_common(self._max_terms_per_chunk)
                if global_counts[term] >= self._min_term_frequency or len(term.split()) > 1
            ]
            per_chunk_terms.append(ranked_terms)
            await yield_and_check_cancel(chunk_idx)
        await yield_and_check_cancel(len(per_chunk_terms), force=True)

        entities = []
        valid_entities = set()
        for term_idx, (term, count) in enumerate(global_counts.most_common(), start=1):
            if count < self._min_term_frequency and len(term.split()) == 1:
                continue
            valid_entities.add(term)
            entities.append(
                {
                    "entity_name": term,
                    "entity_type": entity_type,
                    "description": f"{term} appears {count} times in the source text.",
                    "source_id": [],
                }
            )
            await yield_and_check_cancel(term_idx)
        await yield_and_check_cancel(len(global_counts), force=True)

        edge_counts = Counter()
        for chunk_idx, terms in enumerate(per_chunk_terms, start=1):
            uniq_terms = list(dict.fromkeys(terms))
            edge_num = 0
            for i, src in enumerate(uniq_terms):
                for tgt in uniq_terms[i + 1 :]:
                    edge_counts[tuple(sorted((src, tgt)))] += 1
                    edge_num += 1
                    if edge_num >= self._max_edges_per_chunk:
                        break
                if edge_num >= self._max_edges_per_chunk:
                    break
            await yield_and_check_cancel(chunk_idx)
        await yield_and_check_cancel(len(per_chunk_terms), force=True)

        relations = []
        for edge_idx, ((src, tgt), weight) in enumerate(edge_counts.items(), start=1):
            if src in valid_entities and tgt in valid_entities:
                relations.append(
                    {
                        "src_id": src,
                        "tgt_id": tgt,
                        "description": f"{src} and {tgt} co-occur in {weight} chunk(s).",
                        "keywords": ["co-occurrence", "keyword"],
                        "weight": float(weight),
                        "source_id": [],
                    }
                )
            await yield_and_check_cancel(edge_idx)
        await yield_and_check_cancel(len(edge_counts), force=True)

        now = asyncio.get_running_loop().time()
        msg = (
            f"Keyword graph extraction done, {len(entities)} nodes, "
            f"{len(relations)} edges, 0 tokens, {now - start_ts:.2f}s."
        )
        logger.info(
            "Keyword graph extraction finished for doc %s: %d entities, %d relations, %.2fs.",
            doc_id,
            len(entities),
            len(relations),
            now - start_ts,
        )
        if callback:
            callback(msg=msg)

        return entities, relations
