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
NERExtractor — semantica-style full pipeline extraction.

Pipeline: tokenize → tag(POS) → parse(dep) → NER → typed relations

All components share a single spaCy `doc` object (one forward pass).

Output includes:
- Entities (from NER, enriched with POS/dep)
- Typed relations (from dependency patterns)
- Dependency tree (heads + labels per token)
- POS tags per token

Supports 7 languages: en, zh, de, fr, es, pt, ja
"""

import logging
from typing import Any, Dict, List, Optional

import spacy
from spacy import Language

from .dep_relation_extractor import DepRelationExtractor
from .types import Entity, ExtractionResult

# Language → spaCy model
_MODEL_MAP = {
    "en": "en_core_web_sm",
    "zh": "zh_core_web_sm",
    "de": "de_core_news_sm",
    "fr": "fr_core_news_sm",
    "es": "es_core_news_sm",
    "pt": "pt_core_news_sm",
    "ja": "ja_core_news_sm",
}

# SpaCy labels to skip from NER output
_SKIP_LABELS = {"ORDINAL", "CARDINAL"}

# Labels by confidence tier (for NER confidence scoring)
_HIGH_CONF = {"PERSON", "ORG", "GPE", "LOC", "DATE"}
_MED_CONF = {"PRODUCT", "EVENT", "WORK_OF_ART", "LAW", "LANGUAGE", "NORP", "MONEY", "TIME", "PERCENT", "FAC", "QUANTITY"}


class NERExtractor:
    """
    Full semantic extraction pipeline (NER + tagger + parser + relations).

    Usage:
        ext = NERExtractor(language="en")
        result = ext.extract("Apple Inc. was founded by Steve Jobs.")

        # result.entities → [Entity]
        # result.relations → [Relation]
        # result.tokens → [TokenInfo] (text, head, dep, tag, index)
    """

    # Model cache: language → nlp (shared singleton per process)
    _nlp_cache: Dict[str, Language] = {}

    def __init__(
        self,
        language: str = "en",
        spacy_model: Optional[str] = None,
        confidence_threshold: float = 0.3,
    ):
        if language not in _MODEL_MAP and spacy_model is None:
            language = "en"
        self.language = language
        self.model_name = spacy_model or _MODEL_MAP.get(language, "en_core_web_sm")
        self.confidence_threshold = confidence_threshold
        self._nlp: Optional[Language] = None

    # ------------------------------------------------------------------
    # Model lifecycle
    # ------------------------------------------------------------------

    def _ensure_model(self):
        """Lazy-load shared spaCy model. Keeps ALL pipes needed for
        dependency parsing (tagger, parser, ner, lemmatizer, attribute_ruler)."""
        if self.model_name in self._nlp_cache:
            self._nlp = self._nlp_cache[self.model_name]
            return
        try:
            nlp = spacy.load(self.model_name)
            self._nlp_cache[self.model_name] = nlp
            self._nlp = nlp
        except Exception as e:
            logging.error("Failed to load spaCy model '%s': %s", self.model_name, e)
            raise

    # ------------------------------------------------------------------
    # Main extraction
    # ------------------------------------------------------------------

    def extract(
        self,
        text: str,
        extract_relations: bool = True,
        include_tokens: bool = True,
    ) -> ExtractionResult:
        """Run full pipeline on text."""

        # 1. Single forward pass through spaCy
        self._ensure_model()
        doc = self._nlp(text)

        # 2. Extract entities from NER
        entities = self._extract_entities(doc)

        # 3. Build token list (with POS, dep)
        tokens = self._build_tokens(doc) if include_tokens else []

        # 4. Extract typed relations using dependency parse
        relations = []
        if extract_relations and len(entities) >= 2:
            dep_ext = DepRelationExtractor(
                language=self.language,
                confidence_threshold=self.confidence_threshold,
            )
            relations = dep_ext.extract(text, entities, doc=doc)

        # 5. Build result
        result = ExtractionResult(
            entities=entities,
            relations=relations,
            language=self.language,
        )
        result.metadata = {
            "model": self.model_name,
            "n_tokens": len(doc),
            "n_entities": len(entities),
            "n_relations": len([r for r in relations if r.predicate != "related_to"]),
        }
        if include_tokens:
            result.metadata["tokens"] = tokens

        return result

    def extract_batch(
        self,
        texts: List[str],
        extract_relations: bool = True,
        include_tokens: bool = False,
        batch_size: int = 32,
    ) -> List[ExtractionResult]:
        """Batch extraction using spaCy's nlp.pipe() for efficiency."""
        self._ensure_model()
        results = []
        for doc in self._nlp.pipe(texts, batch_size=batch_size):
            entities = self._extract_entities(doc)
            tokens = self._build_tokens(doc) if include_tokens else []
            relations = []
            if extract_relations and len(entities) >= 2:
                dep_ext = DepRelationExtractor(
                    language=self.language,
                    confidence_threshold=self.confidence_threshold,
                )
                relations = dep_ext.extract(doc.text, entities, doc=doc)
            result = ExtractionResult(
                entities=entities,
                relations=relations,
                language=self.language,
            )
            if include_tokens:
                result.metadata = {"tokens": tokens}
            results.append(result)
        return results

    # ------------------------------------------------------------------
    # Helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _label_confidence(label: str) -> float:
        if label in _HIGH_CONF:
            return 0.85
        if label in _MED_CONF:
            return 0.65
        return 0.50

    def _extract_entities(self, doc) -> List[Entity]:
        """Extract NER entities from spaCy doc, enriched with POS."""
        entities = []
        seen = set()
        for ent in doc.ents:
            if ent.label_ in _SKIP_LABELS:
                continue
            confidence = self._label_confidence(ent.label_)
            if confidence < self.confidence_threshold:
                continue
            key = (ent.text.lower(), ent.start_char)
            if key in seen:
                continue
            seen.add(key)
            entities.append(
                Entity(
                    text=ent.text,
                    label=ent.label_,
                    start_char=ent.start_char,
                    end_char=ent.end_char,
                    confidence=confidence,
                    metadata={"source": "spacy"},
                )
            )
        return entities

    @staticmethod
    def _build_tokens(doc) -> List[Dict[str, Any]]:
        """Build token list with POS tags and dependency info."""
        return [
            {
                "text": t.text,
                "tag": t.tag_,
                "dep": t.dep_,
                "head": t.head.i,
                "index": i,
                "lemma": t.lemma_,
                "pos": t.pos_,
            }
            for i, t in enumerate(doc)
        ]

    @staticmethod
    def clear_cache():
        """Clear the NLP model cache (e.g., for testing)."""
        NERExtractor._nlp_cache.clear()


# Patch ExtractionResult to support metadata
