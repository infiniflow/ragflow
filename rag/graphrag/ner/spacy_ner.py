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
spaCy-based NER extractor — inspired by semantica's NERExtractor design.

Wraps spaCy NER with:
- Lazy model loading (shared singleton)
- Entity type mapping (spaCy labels → app-level types)
- Multi-language model support
- Fallback to regex-based extraction when spaCy model absent
- Confidence scoring
"""
import logging
import re
from typing import Any, Dict, List, Optional, Union

from .types import Entity, SPACY_TO_APP_ENTITY_TYPE, SKIP_SPACY_LABELS

# ---------------------------------------------------------------------------
# spaCy model singleton
# ---------------------------------------------------------------------------
_nlp = None
_nlp_model_name = ""


def _load_spacy_model(model_name: str = "en_core_web_sm"):
    """Load (or return cached) spaCy language model.
    Does NOT auto-download — model must be pre-installed via:
        python -m spacy download <model_name>
    """
    global _nlp, _nlp_model_name
    if _nlp is not None and _nlp_model_name == model_name:
        return _nlp
    try:
        import spacy
    except ImportError:
        raise ImportError(
            "spaCy is required. Install with: pip install spacy && "
            "python -m spacy download en_core_web_sm"
        )
    _nlp = spacy.load(model_name)
    _nlp_model_name = model_name
    return _nlp


# ---------------------------------------------------------------------------
# Regex fallback patterns (English only; used when spaCy model unavailable)
# ---------------------------------------------------------------------------
_FALLBACK_NER_PATTERNS = {
    "PERSON": re.compile(r"\b[A-Z][a-z]+(?:\s+[A-Z][a-z]+)+\b"),
    "ORG": re.compile(
        r"\b[A-Z][a-zA-Z]+(?:\s+[A-Z][a-zA-Z]+)*\s+(?:Inc|Corp|LLC|Ltd|Company|"
        r"University|Institute|Group|Bank|Fund)\b"
    ),
    "GPE": re.compile(r"\b[A-Z][a-z]+(?:\s+[A-Z][a-z]+)*"),
    "DATE": re.compile(r"\b\d{1,2}[/-]\d{1,2}[/-]\d{2,4}|\b\d{4}\b"),
    "MONEY": re.compile(r"\$\d+(?:,\d{3})*(?:\.\d{2})?"),
}


class NerExtractor:
    """
    NER extractor wrapping spaCy with fallback chain.

    Usage:

        extractor = NerExtractor(model_name="en_core_web_sm")
        entities = extractor.extract("Apple Inc. was founded by Steve Jobs.")
    """

    def __init__(
        self,
        model_name: str = "en_core_web_sm",
        confidence_threshold: float = 0.3,
        entity_type_map: Optional[Dict[str, str]] = None,
    ):
        self.model_name = model_name
        self.confidence_threshold = confidence_threshold
        self.entity_type_map = entity_type_map or SPACY_TO_APP_ENTITY_TYPE
        self._nlp = None
        self._load_failed = False

    def _ensure_model(self):
        """Lazy-load spaCy model on first call."""
        if self._nlp is None and not self._load_failed:
            try:
                self._nlp = _load_spacy_model(self.model_name)
            except Exception:
                self._load_failed = True
                logging.warning(
                    "Failed to load spaCy model '%s'; falling back to regex",
                    self.model_name, exc_info=True,
                )

    def extract(self, text: str, **options) -> List[Entity]:
        """
        Extract named entities from text.

        Uses spaCy NER first; falls back to regex patterns if unavailable.
        Chinese fallback returns [] since regex fallback is English-only.
        """
        self._ensure_model()
        if self._nlp is not None:
            return self._extract_with_spacy(text, options)
        if self.model_name.startswith("zh"):
            return []  # no Chinese-compatible fallback
        return self._extract_fallback(text)

    def extract_batch(self, texts: List[str], **options) -> List[List[Entity]]:
        """Batch extraction."""
        return [self.extract(t, **options) for t in texts]

    # ------------------------------------------------------------------
    # spaCy path
    # ------------------------------------------------------------------
    def _extract_with_spacy(self, text: str, options) -> List[Entity]:
        doc = self._nlp(text)
        entities = []
        seen = set()
        for ent in doc.ents:
            if ent.label_ in SKIP_SPACY_LABELS:
                continue

            # confidence: spaCy doesn't expose per-entity confidence
            # for small models; use label heuristic
            confidence = self._label_confidence(ent.label_)

            if confidence < self.confidence_threshold:
                continue

            key = (ent.text.lower(), ent.start_char)
            if key in seen:
                continue
            seen.add(key)

            app_type = self.entity_type_map.get(ent.label_, ent.label_.lower())
            entities.append(Entity(
                text=ent.text,
                label=ent.label_,
                start_char=ent.start_char,
                end_char=ent.end_char,
                confidence=confidence,
                metadata={"app_type": app_type, "source": "spacy"},
            ))
        return entities

    @staticmethod
    def _label_confidence(label: str) -> float:
        # spaCy small model: PERSON/ORG/GPE are most reliable
        high = {"PERSON", "ORG", "GPE", "LOC", "DATE"}
        medium = {"PRODUCT", "EVENT", "WORK_OF_ART", "LAW", "LANGUAGE", "NORP"}
        if label in high:
            return 0.85
        if label in medium:
            return 0.65
        return 0.50

    # ------------------------------------------------------------------
    # Regex fallback path
    # ------------------------------------------------------------------
    def _extract_fallback(self, text: str) -> List[Entity]:
        entities = []
        seen = set()
        for label, pattern in _FALLBACK_NER_PATTERNS.items():
            for m in pattern.finditer(text):
                key = (m.group().lower(), m.start())
                if key in seen:
                    continue
                seen.add(key)
                app_type = self.entity_type_map.get(label, label.lower())
                entities.append(Entity(
                    text=m.group(),
                    label=label,
                    start_char=m.start(),
                    end_char=m.end(),
                    confidence=0.6,
                    metadata={"app_type": app_type, "source": "regex_fallback"},
                ))
        return entities
