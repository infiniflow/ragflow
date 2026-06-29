#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
"""
Relation extractor — inspired by semantica's RelationExtractor design.

Extracts typed relations between entities using:
1. Language-specific regex patterns (pattern method)
2. Sentence co-occurrence fallback (cooccurrence method)
"""
import logging
import re
from collections import defaultdict
from typing import Any, Dict, List, Optional

from .types import Entity, Relation, MULTILANG_RELATION_PATTERNS


class RelationExtractor:
    """
    Extract typed relations between entities.

    Usage:

        extractor = RelationExtractor(language="en")
        relations = extractor.extract(text, entities)
    """

    def __init__(
        self,
        language: str = "en",
        confidence_threshold: float = 0.5,
        max_distance: int = 100,
    ):
        self.language = language
        self.confidence_threshold = confidence_threshold
        self.max_distance = max_distance
        self._patterns = self._compile_patterns(language)

    @staticmethod
    def _compile_patterns(language: str) -> List:
        """Compile regex patterns for a language."""
        patterns = MULTILANG_RELATION_PATTERNS.get(language, MULTILANG_RELATION_PATTERNS["en"])
        compiled = []
        for pattern_str, pred in patterns:
            try:
                # Entity groups must match case-sensitively (only [A-Z] words).
                # Relation keywords are matched via (?i) inline in the pattern.
                compiled.append((re.compile(pattern_str), pred))
            except re.error:
                logging.warning("Bad relation pattern for %s: %s", pred, pattern_str)
        return compiled

    def extract(
        self,
        text: str,
        entities: List[Entity],
        **options,
    ) -> List[Relation]:
        """
        Extract relations between entities.

        Strategy:
        1. Try language-specific regex patterns on text → typed relations
        2. Fall back to sentence co-occurrence → "related_to" relations
        """
        relations = []

        # ---- Phase 1: Pattern matching ----
        if self._patterns:
            pattern_rels = self._extract_with_patterns(text, entities)
            relations.extend(pattern_rels)

        # ---- Phase 2: Co-occurrence ----
        co_rels = self._extract_cooccurrence(text, entities)
        relations.extend(co_rels)

        return self._deduplicate(relations)

    def _extract_with_patterns(self, text: str, entities: List[Entity]) -> List[Relation]:
        """Extract typed relations via regex patterns."""
        relations = []
        entity_map = self._build_entity_map(entities)

        def _normalize(t: str) -> str:
            """Strip trailing punctuation for entity matching."""
            t = t.strip()
            while t and t[-1] in ".,;:!?":
                t = t[:-1].strip()
            return t

        def _find_entity(text: str, entity_map) -> Entity | None:
            """Find entity by text, trying exact and partial match."""
            text = _normalize(text)
            key = text.lower()
            if key in entity_map:
                return entity_map[key]
            # Try removing trailing "and ..." (e.g. "Larry Page and Sergey Brin" → "Larry Page")
            for sep in (" and ", " or ", ", "):
                if sep in key:
                    candidate = key[:key.index(sep)]
                    if candidate in entity_map:
                        return entity_map[candidate]
            return None

        for pattern, predicate in self._patterns:
            for m in pattern.finditer(text):
                subj_text = m.group(1).strip()
                obj_text = m.group(2).strip()
                subj = _find_entity(subj_text, entity_map)
                obj = _find_entity(obj_text, entity_map)
                if subj is None or obj is None:
                    continue
                ctx_start = max(0, m.start() - 30)
                ctx_end = min(len(text), m.end() + 30)
                relations.append(Relation(
                    subject=subj,
                    predicate=predicate,
                    obj=obj,
                    confidence=0.8,
                    context=text[ctx_start:ctx_end],
                    metadata={"method": "pattern"},
                ))
        return relations

    def _extract_cooccurrence(self, text: str, entities: List[Entity]) -> List[Relation]:
        """Sentence co-occurrence fallback (LinearRAG-style)."""
        relations = []
        if len(entities) < 2:
            return relations

        for i in range(len(entities)):
            for j in range(i + 1, len(entities)):
                e1, e2 = entities[i], entities[j]
                distance = abs(e2.start_char - e1.end_char)
                if distance > self.max_distance:
                    continue
                ctx_start = max(0, min(e1.start_char, e2.start_char) - 20)
                ctx_end = min(
                    len(text), max(e1.end_char, e2.end_char) + 20
                )
                relations.append(Relation(
                    subject=e1,
                    predicate="related_to",
                    obj=e2,
                    confidence=0.4,
                    context=text[ctx_start:ctx_end],
                    metadata={"method": "cooccurrence", "distance": distance},
                ))
        return relations

    @staticmethod
    def _build_entity_map(entities: List[Entity]) -> Dict[str, Entity]:
        # Build map with both raw and punctuation-stripped keys
        result = {}
        for e in entities:
            result[e.text.lower()] = e
            # Also add a punctuation-stripped version
            cleaned = e.text.rstrip(".,;:!?").strip().lower()
            if cleaned != e.text.lower():
                result[cleaned] = e
        return result

    @staticmethod
    def _deduplicate(relations: List[Relation]) -> List[Relation]:
        seen = set()
        result = []
        for r in relations:
            key = (
                r.subject.text.lower(),
                r.predicate,
                r.obj.text.lower(),
            )
            if key in seen:
                continue
            seen.add(key)
            result.append(r)
        return result
