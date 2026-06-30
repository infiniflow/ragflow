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
Dependency-based relation extractor — replaces regex-based approach.

Uses spaCy dependency parse for multilingual typed relation extraction.
"""

import logging
from typing import Dict, List, Optional

from .types import Entity, Relation

# Verb → relation mapping. All relations use conventional direction:
#   founded_by: ORG → PERSON  ("Apple founded_by Steve Jobs")
#   works_for:  PERSON → ORG  ("John works_for Microsoft")
#   located_in: ORG/LOC → GPE ("Microsoft located_in Redmond")
#   born_in:    PERSON → GPE  ("Einstein born_in Germany")
#   ceo_of:     PERSON → ORG  ("Sundar Pichai ceo_of Google")
#   acquired:   ORG → ORG     ("Facebook acquired Instagram")
_VERB_RELATIONS: Dict[str, str] = {
    "found+by": "founded_by",
    "co-found+by": "founded_by",
    "establish+by": "founded_by",
    "create+by": "founded_by",
    "set+up": "founded_by",
    "start+by": "founded_by",
    "work+for": "works_for",
    "employ+by": "works_for",
    "hire+by": "works_for",
    "join": "works_for",
    "base+in": "located_in",
    "locate+in": "located_in",
    "situate+in": "located_in",
    "bear+in": "born_in",
    "bear+on": "born_in",
    "acquire+by": "acquired",
    "merge+with": "acquired",
    "buy+by": "acquired",
}

# Copula: "X is [title] of Y" → relation (both ceo_of AND works_for)
_COPULA_TITLE_MAP: Dict[str, List[str]] = {
    "ceo": ["ceo_of", "works_for"],
    "cto": ["works_for"],
    "cfo": ["works_for"],
    "coo": ["works_for"],
    "vp": ["works_for"],
    "director": ["works_for"],
    "manager": ["works_for"],
    "engineer": ["works_for"],
    "employee": ["works_for"],
    "founder": ["founded_by"],
    "co-founder": ["founded_by"],
}


class DepRelationExtractor:
    """Extract typed relations using dependency parse."""

    def __init__(self, language: str = "en",
                 confidence_threshold: float = 0.3,
                 max_distance: int = 100):
        self.language = language
        self.confidence_threshold = confidence_threshold
        self.max_distance = max_distance

    def extract(self, text: str, entities: List[Entity],
                doc=None, **options) -> List[Relation]:
        relations = []
        if doc is not None:
            relations.extend(self._extract_with_dep(text, doc, entities))
        relations.extend(self._extract_cooccurrence(text, entities))
        relations = self._deduplicate(relations)
        return [r for r in relations if r.confidence >= self.confidence_threshold]

    # ------------------------------------------------------------------
    # Dependency extraction
    # ------------------------------------------------------------------

    def _extract_with_dep(self, text, doc, entities) -> List[Relation]:
        relations = []
        entity_map = self._build_entity_map(entities)
        for sent in doc.sents:
            for token in sent:
                if token.dep_ != "ROOT":
                    continue
                relations.extend(self._extract_from_root(text, token, entity_map))
                if token.lemma_ == "be":
                    relations.extend(self._extract_copula(text, token, entity_map))
        return relations

    def _extract_from_root(self, text, root, entity_map) -> List[Relation]:
        relations = []
        verb_lemma = root.lemma_.lower()

        # Collect arguments
        nsubj = self._get_child_entity(root, "nsubj", entity_map)
        nsubjpass = self._get_child_entity(root, "nsubjpass", entity_map)
        dobj = self._get_child_entity(root, "dobj", entity_map)
        agent_pobj = self._get_agent_pobj(root, entity_map)
        prep_list = self._get_prep_objs(root, entity_map)
        have_agent = any(c.dep_ == "agent" for c in root.children)

        # Passive: "X was founded/acquired by Y"
        # Relations use conventional direction
        if nsubjpass and agent_pobj and have_agent:
            rel_type = self._lookup(verb_lemma, "by")
            if rel_type:
                # founded_by: ORG → PERSON. Passive "X was founded by Y"
                # means subject=Y_founder, but we want subject=X_org
                if rel_type in ("founded_by", "acquired"):
                    subj, obj = nsubjpass, agent_pobj  # direction: org→person
                else:
                    subj, obj = agent_pobj, nsubjpass
                relations.append(Relation(
                    subject=subj, predicate=rel_type, obj=obj,
                    confidence=0.85, context=text,
                    metadata={"method": "passive", "verb": verb_lemma},
                ))

        # Active: "X VERB Y"
        if nsubj:
            if dobj:
                rel_type = self._lookup(verb_lemma, None)
                if rel_type:
                    relations.append(Relation(
                        subject=nsubj, predicate=rel_type, obj=dobj,
                        confidence=0.85, context=text,
                        metadata={"method": "active", "verb": verb_lemma},
                    ))
            for prep_lemma, prep_entity in prep_list:
                rel_type = self._lookup(verb_lemma, prep_lemma)
                if rel_type:
                    relations.append(Relation(
                        subject=nsubj, predicate=rel_type, obj=prep_entity,
                        confidence=0.85, context=text,
                        metadata={"method": "active_prep", "verb": verb_lemma,
                                  "prep": prep_lemma},
                    ))

        # Passive with prep: "X is based/located/situated in Y"
        if nsubjpass and prep_list:
            for prep_lemma, prep_entity in prep_list:
                rel_type = self._lookup(verb_lemma, prep_lemma)
                if not rel_type:
                    rel_type = self._lookup("be+" + verb_lemma, prep_lemma)
                if rel_type:
                    relations.append(Relation(
                        subject=nsubjpass, predicate=rel_type, obj=prep_entity,
                        confidence=0.85, context=text,
                        metadata={"method": "passive_prep", "verb": verb_lemma,
                                  "prep": prep_lemma},
                    ))
        return relations

    def _extract_copula(self, text, root, entity_map) -> List[Relation]:
        relations = []
        subj = self._get_child_entity(root, "nsubj", entity_map)
        if not subj:
            return relations
        # Find the attr token that has "of Y" as a prepositional child
        title_lemma = None
        prep_obj = None
        for c in root.children:
            if c.dep_ != "attr":
                continue
            for cc in c.children:
                if cc.dep_ != "prep" or cc.lemma_ != "of":
                    continue
                for gc in cc.children:
                    if gc.dep_ == "pobj":
                        prep_obj = self._entity_from_subtree(gc, entity_map)
                        if prep_obj:
                            title_lemma = c.lemma_.lower()
                        break

        if not title_lemma or not prep_obj:
            return relations

        for keyword, rel_types in _COPULA_TITLE_MAP.items():
            if keyword in title_lemma:
                for rt in rel_types:
                    relations.append(Relation(
                        subject=subj, predicate=rt, obj=prep_obj,
                        confidence=0.85, context=text,
                        metadata={"method": "copula", "title": title_lemma},
                    ))
                break
        return relations

    # ------------------------------------------------------------------
    # Argument extraction helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _get_child_entity(token, dep, entity_map):
        """Find first child with dep, get entity from its subtree."""
        for c in token.children:
            if c.dep_ == dep:
                return DepRelationExtractor._entity_from_subtree(c, entity_map)
        return None

    @staticmethod
    def _get_agent_pobj(root, entity_map):
        """Get entity from agent→pobj chain."""
        for c in root.children:
            if c.dep_ == "agent":
                for gc in c.children:
                    if gc.dep_ == "pobj":
                        return DepRelationExtractor._entity_from_subtree(gc, entity_map)
        return None

    @staticmethod
    def _get_prep_objs(root, entity_map):
        """Get list of (prep_lemma, entity) from prepositional children."""
        results = []
        for c in root.children:
            if c.dep_ == "prep":
                prep_lemma = c.lemma_.lower()
                for gc in c.children:
                    if gc.dep_ == "pobj":
                        ent = DepRelationExtractor._entity_from_subtree(gc, entity_map)
                        if ent:
                            results.append((prep_lemma, ent))
        return results

    @staticmethod
    def _entity_from_subtree(token, entity_map) -> Optional[Entity]:
        """Walk a token's subtree (excluding structure words), then try to
        match against entity_map. Uses character positions to handle
        conjunctions: gets the contiguous span from start of first dependent
        to end of last non-conjunct token."""
        # Get min/max char positions in the subtree
        min_char = token.idx
        max_char = token.idx + len(token.text)
        for t in token.subtree:
            if t.dep_ not in ("prep", "punct", "det", "aux", "auxpass",
                              "cc", "conj"):
                if t.idx < min_char:
                    min_char = t.idx
                end = t.idx + len(t.text)
                if end > max_char:
                    max_char = end

        # Build candidate by extracting text substring between min/max
        text = token.doc.text[min_char:max_char].strip()
        key = text.lower()
        if key in entity_map:
            return entity_map[key]

        # Try splitting on "and"/"or"/"," and take first part
        for sep in (" and ", " or ", ", "):
            if sep in key:
                candidate = key.split(sep)[0].strip()
                if candidate in entity_map:
                    return entity_map[candidate]

        # Try entities that are substrings of this span
        for ent_key, ent in entity_map.items():
            if ent_key in key or key in ent_key:
                return ent

        return None

    @staticmethod
    def _lookup(verb: str, prep: Optional[str] = None) -> Optional[str]:
        if prep:
            key = f"{verb}+{prep}"
            return _VERB_RELATIONS.get(key)
        return _VERB_RELATIONS.get(verb)

    @staticmethod
    def _build_entity_map(entities: List[Entity]) -> Dict[str, Entity]:
        result = {}
        for e in entities:
            result[e.text.lower()] = e
            cleaned = e.text.rstrip(".,;:!?").strip().lower()
            if cleaned != e.text.lower():
                result[cleaned] = e
        return result

    @staticmethod
    def _deduplicate(relations: List[Relation]) -> List[Relation]:
        seen = set()
        result = []
        for r in relations:
            key = (r.subject.text.lower(), r.predicate, r.obj.text.lower())
            rev = (r.obj.text.lower(), r.predicate, r.subject.text.lower())
            if key in seen or rev in seen:
                continue
            seen.add(key)
            result.append(r)
        return result

    # ------------------------------------------------------------------
    # Co-occurrence
    # ------------------------------------------------------------------

    def _extract_cooccurrence(self, text, entities) -> List[Relation]:
        if len(entities) < 2:
            return []
        import re as _re
        spans = [(m.start(), m.end())
                 for m in _re.finditer(r'[^.!?]+(?:[.!?](?=\s|$))+', text)]

        def same_sent(c1, c2):
            return any(ss <= c1 < se and ss <= c2 < se for ss, se in spans)

        rels = []
        for i in range(len(entities)):
            for j in range(i + 1, len(entities)):
                e1, e2 = entities[i], entities[j]
                if not same_sent(e1.start_char, e2.start_char):
                    continue
                if abs(e2.start_char - e1.end_char) > self.max_distance:
                    continue
                cs = max(0, min(e1.start_char, e2.start_char) - 20)
                ce = min(len(text), max(e1.end_char, e2.end_char) + 20)
                rels.append(Relation(
                    subject=e1, predicate="related_to", obj=e2,
                    confidence=0.4, context=text[cs:ce],
                    metadata={"method": "cooccurrence"},
                ))
        return rels
