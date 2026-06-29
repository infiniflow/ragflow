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
Data types for entity and relation extraction.
"""
from dataclasses import dataclass, field
from typing import Any, Dict, List


@dataclass
class Entity:
    """Extracted entity."""
    text: str
    label: str  # spaCy NER label: PERSON, ORG, GPE, ...
    start_char: int
    end_char: int
    confidence: float = 1.0
    metadata: Dict[str, Any] = field(default_factory=dict)


@dataclass
class Relation:
    """Extracted relation between two entities."""
    subject: Entity
    predicate: str   # relation type: "founded_by", "works_for", ...
    obj: Entity
    confidence: float = 1.0
    context: str = ""  # surrounding text
    metadata: Dict[str, Any] = field(default_factory=dict)


@dataclass
class ExtractionResult:
    """Result of a full extraction pass."""
    entities: List[Entity] = field(default_factory=list)
    relations: List[Relation] = field(default_factory=list)
    language: str = "en"


SPACY_TO_APP_ENTITY_TYPE: Dict[str, str] = {
    "PERSON": "person",
    "ORG": "organization",
    "GPE": "geo",
    "LOC": "geo",
    "FAC": "geo",
    "EVENT": "event",
    "PRODUCT": "category",
    "WORK_OF_ART": "category",
    "LAW": "category",
    "LANGUAGE": "category",
    "NORP": "category",
    "MONEY": "category",
    "QUANTITY": "category",
    "TIME": "event",
    "DATE": "event",
    "PERCENT": "category",
    "CARDINAL": "category",
    "ORDINAL": "category",
}

SKIP_SPACY_LABELS = {"ORDINAL", "CARDINAL"}

# Multilingual relation patterns — extends semantica's approach
# Key: language code, Value: list of (predicate, regex_pattern)
# Helper: entity capture group — case-sensitive [A-Z], non-greedy *?
# Entity names always start with uppercase in NER output.
# Entity pattern: allow periods only between uppercase initials (U.S., J.K.)
# otherwise period acts as word boundary. Use | for literal in patterns.
_ENT_WORD = r"[A-Za-z][\w']*(?:\.[A-Za-z][\w']*)*"
_REL_ENTITY = r"(" + _ENT_WORD + r"(?:\s+" + _ENT_WORD + r")*?)"
# Second entity: limited to 2 words; period allowed inside (U.S.)
_REL_ENTITY2 = r"(" + _ENT_WORD + r"(?:\s+" + _ENT_WORD + r"){0,1})"

MULTILANG_RELATION_PATTERNS: Dict[str, List] = {
    "en": [
        # founded_by — (?i) makes only the keyword part case-insensitive
        (_REL_ENTITY + r"\s+(?i:was)\s+(?i:founded)\s+(?i:by)\s+" + _REL_ENTITY2, "founded_by"),
        (_REL_ENTITY + r"\s+(?i:is)\s+(?i:an?\s+)?(?i:co-)?(?i:founder)\s+(?i:of)\s+" + _REL_ENTITY2, "founded_by"),
        # works_for
        (_REL_ENTITY + r"\s+(?i:works)\s+(?i:for)\s+" + _REL_ENTITY2, "works_for"),
        (_REL_ENTITY + r"\s+(?i:is)\s+(?i:an?\s+)?(?i:employee)\s+(?i:of)\s+" + _REL_ENTITY2, "works_for"),
        (_REL_ENTITY + r"\s+(?i:joined)\s+" + _REL_ENTITY2, "works_for"),
        (_REL_ENTITY + r"\s+(?i:is)\s+(?i:the\s+)?(?:CEO|CTO|CFO|VP|(?i:director|manager|engineer))\s+(?i:of|at)\s+" + _REL_ENTITY2, "works_for"),
        # located_in
        (_REL_ENTITY + r"\s+(?i:is)\s+(?i:located|based|headquartered|situated)\s+(?i:in)\s+" + _REL_ENTITY2, "located_in"),
        # born_in
        (_REL_ENTITY + r"\s+(?i:was)\s+(?i:born)\s+(?i:in|on)\s+" + _REL_ENTITY2, "born_in"),
        (_REL_ENTITY + r"\s+(?i:born)\s+(?i:in|on)\s+" + _REL_ENTITY2, "born_in"),
        # acquired
        (_REL_ENTITY + r"\s+(?i:was)\s+(?i:acquired)\s+(?i:by)\s+" + _REL_ENTITY2, "acquired"),
        (_REL_ENTITY + r"\s+(?i:acquired)\s+" + _REL_ENTITY2, "acquired"),
        # ceo_of
        (_REL_ENTITY + r"\s+(?i:is)\s+(?i:the\s+)?(?i:CEO)\s+(?i:of)\s+" + _REL_ENTITY2, "ceo_of"),
    ],
    "zh": [
        # founded_by
        (r"([\u4e00-\u9fff\w]{2,6})\s*由\s*([\u4e00-\u9fff\w]{2,4})\s*(?:创立|创建|成立|创办)", "founded_by"),
        (r"([\u4e00-\u9fff\w]{2,4})\s*(?:创立|创建|成立|创办)(?:\s*了\s*)?([\u4e00-\u9fff\w]{2,10})", "founded_by"),
        (r"([\u4e00-\u9fff\w]{2,4})\s*(?:是\s*)?([\u4e00-\u9fff\w]{2,10})\s*(?:创始人|联合创始人)", "founded_by"),
        # works_for
        (r"([\u4e00-\u9fff\w]{2,4})\s*(?:任职于|供职于|工作于|就职于)\s*([\u4e00-\u9fff\w]{2,10})", "works_for"),
        (r"([\u4e00-\u9fff\w]{2,4})\s*(?:是\s*)?([\u4e00-\u9fff\w]{2,10})\s*(?:的员工|的雇员)", "works_for"),
        # located_in
        (r"([\u4e00-\u9fff\w]{2,10})\s*(?:位于|坐落于|总部设在|总部位于)\s*([\u4e00-\u9fff\w]{2,6})", "located_in"),
        (r"([\u4e00-\u9fff\w]{2,10})\s*在\s*([\u4e00-\u9fff\w]{2,6})", "located_in"),
        # born_in
        (r"([\u4e00-\u9fff\w]{2,4})\s*(?:出生于|生于)\s*([\u4e00-\u9fff\w]{2,6})", "born_in"),
        # acquired
        (r"([\u4e00-\u9fff\w]{2,10})\s*(?:收购|并购)\s*([\u4e00-\u9fff\w]{2,10})", "acquired"),
        (r"([\u4e00-\u9fff\w]{2,10})\s*被\s*([\u4e00-\u9fff\w]{2,10})\s*(?:收购|并购)", "acquired"),
    ],
}
