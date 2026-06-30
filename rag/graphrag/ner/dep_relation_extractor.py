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
Dependency-based relation extractor — full semantica alignment.

Extracts typed relations using spaCy dependency parse with:
- Multi-hop inference (A→B→C transitivity)
- Negation filtering
- Dynamic confidence scoring
- Multi-occurrence entity matching
"""
import logging
from typing import Dict, List, Optional

from .types import Entity, Relation

# Language-specific dependency label mappings
# For each language: which dep labels to use for extraction
# Based on actual spaCy output analysis (see docs)
_LANG_DEP_RULES: Dict[str, Dict[str, str]] = {
    "en": {"nsubjpass": "pass_subj", "agent_pobj": "agent",
           "nsubj": "subj", "dobj": "dobj",
           "prep_pobj": "prep_obj"},
    "de": {"sb": "pass_subj", "ROOT_oc": "root_verb",  # ROOT is aux, oc = main verb
           "sbp_nk": "agent", "sb": "subj",
           "mo_nk": "prep_obj",  # prepositional modifier → noun kernel
          },
    "fr": {"nsubj:pass": "pass_subj", "obl:agent": "agent",
           "nsubj": "subj", "obj": "dobj",
           "obl": "prep_obj",  # oblique (includes prep objects)
          },
    "es": {"nsubj": "subj", "obj": "agent",  # Spanish: no passive distinction, agent uses obj
           "obl": "prep_obj"},
    "pt": {"nsubj:pass": "pass_subj", "obl:agent": "agent",
           "nsubj": "subj", "obj": "dobj",
           "obl": "prep_obj"},
    "zh": {"nsubj": "subj", "nmod:prep": "agent",  # "由" = case marker for agent
           "obl": "prep_obj", "dobj": "dobj"},
    "ja": {"nsubj": "subj", "obl": "agent",
           "obl": "prep_obj", "dobj": "dobj"},
}

# Multi-hop inference rules: if A rel1 B and B rel2 C then A rel3 C
_MULTI_HOP: Dict[str, Dict[str, str]] = {
    "ceo_of": {"is_subsidiary_of": "works_for", "located_in": "works_for"},
    "works_for": {"is_subsidiary_of": "works_for"},
    "founded_by": {"is_subsidiary_of": "founded_by"},
}

_VERB_RELATIONS: Dict[str, str] = {
    # English
    "found+by": "founded_by", "co-found+by": "founded_by",
    "establish+by": "founded_by", "create+by": "founded_by",
    "set+up": "founded_by", "start+by": "founded_by",
    "work+for": "works_for", "employ+by": "works_for",
    "hire+by": "works_for", "join": "works_for",
    "lead+by": "works_for", "manage+by": "works_for",
    "head+by": "works_for", "run+by": "works_for",
    "own+by": "owns", "develop+by": "develops",
    "write+by": "wrote", "publish+by": "published",
    "invest+in": "invests_in", "partner+with": "partners_with",
    "collaborate+with": "collaborates_with",
    "merge+with": "merged_with", "subsidiar+y": "is_subsidiary_of",
    "base+in": "located_in", "locate+in": "located_in",
    "situate+in": "located_in", "headquarter+in": "located_in",
    "bear+in": "born_in", "bear+on": "born_in",
    "acquire+by": "acquired", "buy+by": "acquired",
    # German (de): spaCy lemmas
    "gründen+von": "founded_by", "errichten+von": "founded_by",
    "arbeiten+für": "works_for", "beschäftigen+bei": "works_for",
    "anstellen+bei": "works_for", "sich+befinden": "located_in",
    "liegen+in": "located_in", "sitzen+in": "located_in",
    "gebären+in": "born_in", "gebären+am": "born_in",
    "erwerben+durch": "acquired", "kaufen+durch": "acquired",
    "übernehmen+durch": "acquired",
    # French (fr): spaCy lemmas
    "fonder+par": "founded_by", "créer+par": "founded_by",
    "établir+par": "founded_by",
    "travailler+pour": "works_for", "employer+par": "works_for",
    "embaucher+par": "works_for",
    "situer+à": "located_in", "baser+à": "located_in",
    "implanter+à": "located_in",
    "naître+à": "born_in",
    "acquérir+par": "acquired", "racheter+par": "acquired",
    # Spanish (es): spaCy lemmas
    "fundar+por": "founded_by", "crear+por": "founded_by",
    "establecer+por": "founded_by",
    "trabajar+para": "works_for", "emplear+por": "works_for",
    "contratar+por": "works_for",
    "ubicar+en": "located_in", "situar+en": "located_in",
    "tener+sede": "located_in",
    "nacer+en": "born_in",
    "adquirir+por": "acquired", "comprar+por": "acquired",
    # Portuguese (pt): spaCy lemmas
    "fundar+por": "founded_by", "criar+por": "founded_by",
    "estabelecer+por": "founded_by",
    "trabalhar+para": "works_for", "empregar+por": "works_for",
    "contratar+por": "works_for",
    "localizar+em": "located_in", "situar+em": "located_in",
    "sediar+em": "located_in",
    "nascer+em": "born_in",
    "adquirir+por": "acquired", "comprar+por": "acquired",
    # Note: non-English languages (de/fr/es/pt/zh/ja) use different spaCy
    # dependency labels. Language-specific dep rules needed — future work.
}

_COPULA_TITLE_MAP: Dict[str, List[str]] = {
    "ceo": ["ceo_of", "works_for"], "cto": ["works_for"],
    "cfo": ["works_for"], "coo": ["works_for"],
    "vp": ["works_for"], "director": ["works_for"],
    "manager": ["works_for"], "engineer": ["works_for"],
    "employee": ["works_for"],
    "founder": ["founded_by"], "co-founder": ["founded_by"],
}


class DepRelationExtractor:
    """Extract typed relations using dependency parse — semantica-aligned."""

    def __init__(self, language: str = "en",
                 confidence_threshold: float = 0.3,
                 max_distance: int = 100):
        self.language = language
        self.confidence_threshold = confidence_threshold
        self.max_distance = max_distance

    def extract(self, text: str, entities: List[Entity],
                doc=None, **options) -> List[Relation]:
        semantica_rels = []
        if doc is not None:
            semantica_rels = self._extract_with_dep(text, doc, entities)
        semantica_rels.extend(self._extract_cooccurrence(text, entities))
        semantica_rels = self._infer_multi_hop(semantica_rels)
        semantica_rels = self._deduplicate(semantica_rels)
        return [r for r in semantica_rels if r.confidence >= self.confidence_threshold]

    # ------------------------------------------------------------------
    # Multi-hop inference (属性传递)
    # ------------------------------------------------------------------

    @staticmethod
    def _infer_multi_hop(relations: List[Relation]) -> List[Relation]:
        """Infer transitive relations: A→B→C ⇒ A→C."""
        by_subj: Dict[str, List[Relation]] = {}
        for r in relations:
            if r.predicate == "related_to":
                continue
            by_subj.setdefault(r.subject.text.lower(), []).append(r)

        inferred = []
        for r in relations:
            if r.predicate == "related_to":
                continue
            obj_key = r.obj.text.lower()
            if obj_key in by_subj:
                for r2 in by_subj[obj_key]:
                    if r2.predicate in _MULTI_HOP.get(r.predicate, {}):
                        inferred_rel = _MULTI_HOP[r.predicate][r2.predicate]
                        if inferred_rel:
                            inferred.append(Relation(
                                subject=r.subject, predicate=inferred_rel,
                                obj=r2.obj, confidence=min(r.confidence, r2.confidence) * 0.9,
                                metadata={"method": "multi_hop",
                                          "via": f"{r.predicate}→{r2.predicate}"},
                            ))
        return relations + inferred

    # ------------------------------------------------------------------
    # Dependency extraction
    # ------------------------------------------------------------------

    def _extract_with_dep(self, text, doc, entities) -> List[Relation]:
        relations = []
        # Build better entity map: keep ALL occurrences (not overwrite)
        entity_map = self._build_entity_map_multi(entities)
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

        # Check negation
        is_negated = any(c.dep_ == "neg" for c in root.children)
        if is_negated:
            return relations

        nsubj = self._get_child_entity(root, "nsubj", entity_map)
        nsubjpass = self._get_child_entity(root, "nsubjpass", entity_map)
        dobj = self._get_child_entity(root, "dobj", entity_map)
        agent_pobj = self._get_agent_pobj(root, entity_map)
        prep_list = self._get_prep_objs(root, entity_map)
        have_agent = any(c.dep_ == "agent" for c in root.children)

        # Passive
        if nsubjpass and agent_pobj and have_agent:
            rel_type = self._lookup(verb_lemma, "by")
            if rel_type:
                conf = 0.90
                if rel_type in ("founded_by", "acquired"):
                    subj, obj = nsubjpass, agent_pobj
                else:
                    subj, obj = agent_pobj, nsubjpass
                relations.append(Relation(
                    subject=subj, predicate=rel_type, obj=obj,
                    confidence=conf, context=text,
                    metadata={"method": "passive", "verb": verb_lemma},
                ))

        # Active
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

        # Passive with prep ("is based in")
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
                        confidence=0.88, context=text,
                        metadata={"method": "copula", "title": title_lemma},
                    ))
                break
        return relations

    # ------------------------------------------------------------------
    # Better entity map: multi-occurrence aware
    # ------------------------------------------------------------------

    @staticmethod
    def _build_entity_map_multi(entities: List[Entity]) -> Dict[str, List[Entity]]:
        """Build entity map that keeps ALL occurrences per name."""
        result: Dict[str, List[Entity]] = {}
        for e in entities:
            key = e.text.lower()
            result.setdefault(key, []).append(e)
            cleaned = e.text.rstrip(".,;:!?").strip().lower()
            if cleaned != key:
                result.setdefault(cleaned, []).append(e)
        return result

    @staticmethod
    def _find_best_entity(key: str, entity_map: Dict[str, List[Entity]],
                          fallback_text: str = "") -> Optional[Entity]:
        """Find the best entity match. If multiple, prefer the one whose
        text is an exact match for fallback_text, or the first one."""
        entries = entity_map.get(key.lower(), [])
        if not entries:
            return None
        if len(entries) == 1:
            return entries[0]
        # Prefer exact text match
        for e in entries:
            if e.text.lower() == fallback_text.lower():
                return e
        return entries[0]

    # ------------------------------------------------------------------
    # Argument extraction helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _get_child_entity(token, dep, entity_map):
        for c in token.children:
            if c.dep_ == dep:
                return DepRelationExtractor._entity_from_subtree(c, entity_map)
        return None

    @staticmethod
    def _get_agent_pobj(root, entity_map):
        for c in root.children:
            if c.dep_ == "agent":
                for gc in c.children:
                    if gc.dep_ == "pobj":
                        return DepRelationExtractor._entity_from_subtree(gc, entity_map)
        return None

    @staticmethod
    def _get_prep_objs(root, entity_map):
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
        """Match token's subtree against entity map. Uses character positions
        for conjunction handling."""
        min_char = token.idx
        max_char = token.idx + len(token.text)
        for t in token.subtree:
            if t.dep_ not in ("prep", "punct", "det", "aux", "auxpass", "cc", "conj"):
                if t.idx < min_char:
                    min_char = t.idx
                end = t.idx + len(t.text)
                if end > max_char:
                    max_char = end
        text = token.doc.text[min_char:max_char].strip()
        key = text.lower()
        # Try multi-map lookup
        entries = entity_map.get(key, [])
        if not entries:
            for sep in (" and ", " or ", ", "):
                if sep in key:
                    entries = entity_map.get(key.split(sep)[0].strip(), [])
                    if entries:
                        break
        if not entries:
            for ek, ev in entity_map.items():
                if ek in key or key in ek:
                    entries = ev
                    break
        if entries:
            return entries[0]
        return None

    @staticmethod
    def _lookup(verb: str, prep: Optional[str] = None) -> Optional[str]:
        if prep:
            key = f"{verb}+{prep}"
            return _VERB_RELATIONS.get(key)
        return _VERB_RELATIONS.get(verb)

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
