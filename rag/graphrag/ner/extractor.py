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
Combined entity and relation extractor — orchestrates NerExtractor + RelationExtractor.

This is the primary entry point for the rag/graphrag/ner package.
"""
from typing import Any, Dict, List, Optional

from .spacy_ner import NerExtractor
from .relation_extractor import RelationExtractor
from .types import Entity, ExtractionResult, Relation


class Extractor:
    """
    Combined entity and relation extractor.

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
            spacy_model = {"en": "en_core_web_sm", "zh": "zh_core_web_sm"}.get(
                language, "en_core_web_sm"
            )
        self._ner = NerExtractor(
            model_name=spacy_model,
            confidence_threshold=ner_confidence_threshold,
        )
        self._rel = RelationExtractor(
            language=language,
            confidence_threshold=relation_confidence_threshold,
        )
        self.language = language

    def extract(self, text: str, extract_relations: bool = True) -> ExtractionResult:
        """
        Extract entities (and optionally relations) from text.
        """
        entities = self._ner.extract(text)

        relations = []
        if extract_relations and len(entities) >= 2:
            relations = self._rel.extract(text, entities)

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
