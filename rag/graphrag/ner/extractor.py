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
Combined entity and relation extractor — orchestrates NerExtractor + relation extraction.

This is the primary entry point for the rag/graphrag/ner package.
"""
import logging
from typing import List, Optional

import spacy

from .spacy_ner import NerExtractor
from .dep_relation_extractor import DepRelationExtractor
from .types import ExtractionResult


class Extractor:
    """
    Combined entity and relation extractor.

    Uses spaCy's dependency parse for multilingual relation extraction
    (replacing the previous regex-based approach).

    Usage:

        extractor = Extractor(language="en", spacy_model="en_core_web_sm")
        result = extractor.extract("Apple Inc. was founded by Steve Jobs.")
        # result.entities → [...]
        # result.relations → [...]
    """

    def __init__(
        self,
        language: str = "en",
        spacy_model: Optional[str] = None,
        ner_confidence_threshold: float = 0.3,
        relation_confidence_threshold: float = 0.5,
    ):
        if spacy_model is None:
            _models = {
                "en": "en_core_web_sm", "zh": "zh_core_web_sm",
                "de": "de_core_news_sm", "fr": "fr_core_news_sm",
                "es": "es_core_news_sm", "pt": "pt_core_news_sm",
                "ja": "ja_core_news_sm",
            }
            spacy_model = _models.get(language, "en_core_web_sm")
        self._ner = NerExtractor(
            model_name=spacy_model,
            confidence_threshold=ner_confidence_threshold,
        )
        self._rel = DepRelationExtractor(
            language=language,
            confidence_threshold=relation_confidence_threshold,
        )
        self.language = language
        self._model_name = spacy_model
        self._nlp = None

    def _ensure_model(self):
        """Lazy-load spaCy model with parser for dependency extraction."""
        if self._nlp is None:
            try:
                self._nlp = spacy.load(self._model_name)
            except Exception:
                logging.warning("Failed to load spaCy '%s' for dep parsing",
                                self._model_name)

    def extract(self, text: str, extract_relations: bool = True,
                **options) -> ExtractionResult:
        """
        Extract entities (and optionally relations) from text.
        Uses spaCy dependency parse for relation extraction (over regex).
        """
        entities = self._ner.extract(text)

        relations = []
        if extract_relations and len(entities) >= 2:
            self._ensure_model()
            doc = self._nlp(text) if self._nlp else None
            relations = self._rel.extract(text, entities, doc=doc)

        return ExtractionResult(
            entities=entities,
            relations=relations,
            language=self.language,
        )

    def extract_batch(
        self, texts: List[str], extract_relations: bool = True
    ) -> List[ExtractionResult]:
        """Batch extraction."""
        return [self.extract(t, extract_relations) for t in texts]
