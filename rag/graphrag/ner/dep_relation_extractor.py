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

from typing import Dict, List, Optional

from .types import Entity, Relation

# Language-specific dependency label mappings
# Keys: pass_subj, subj, agent, dobj, prep_obj — each maps to a dep label
# or a tuple (dep, child_dep) for compound patterns.
# None = no standard mapping (language uses different structure)
_LANG_DEP_RULES: Dict[str, Dict[str, object]] = {
    "en": {"pass_subj": "nsubjpass", "subj": "nsubj", "agent": ("agent", "pobj"), "dobj": "dobj", "prep_obj": ("prep", "pobj")},
    "de": {"subj": "sb", "agent": ("sbp", "nk"), "prep_obj": ("mo", "nk"), "root_verb_child": "oc"},  # German ROOT is aux, real verb is "oc"
    "fr": {"pass_subj": "nsubj:pass", "subj": "nsubj", "agent": "obl:agent", "dobj": "obj", "prep_obj": ("case", "obl")},
    "es": {"subj": "nsubj", "agent": "obj", "prep_obj": ("case", "obl")},
    "pt": {"pass_subj": "nsubj:pass", "subj": "nsubj", "agent": "obl:agent", "dobj": "obj", "prep_obj": ("case", "obl")},
    "zh": {
        "subj": "nsubj",
        "agent": ("nmod:prep", None, "由"),  # case "由" marks agent
        "prep_obj": ("case", "nmod"),
    },
    "ja": {
        "subj": "nsubj",
        "agent": ("obl", None, "によって"),  # "によって" marks agent
        "prep_obj": ("case", "obl"),
    },
}

# Multi-hop inference rules: if A rel1 B and B rel2 C then A rel3 C
_MULTI_HOP: Dict[str, Dict[str, str]] = {
    "ceo_of": {"is_subsidiary_of": "works_for", "located_in": "works_for"},
    "works_for": {"is_subsidiary_of": "works_for"},
    "founded_by": {"is_subsidiary_of": "founded_by"},
}

_VERB_RELATIONS: Dict[str, str] = {
    # English
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
    "lead+by": "works_for",
    "manage+by": "works_for",
    "head+by": "works_for",
    "run+by": "works_for",
    "own+by": "owns",
    "develop+by": "develops",
    "write+by": "wrote",
    "publish+by": "published",
    "invest+in": "invests_in",
    "partner+with": "partners_with",
    "collaborate+with": "collaborates_with",
    "merge+with": "merged_with",
    "subsidiar+y": "is_subsidiary_of",
    "base+in": "located_in",
    "locate+in": "located_in",
    "situate+in": "located_in",
    "headquarter+in": "located_in",
    "bear+in": "born_in",
    "bear+on": "born_in",
    "acquire+by": "acquired",
    "buy+by": "acquired",
    # German (de): spaCy lemmas
    "gründen+von": "founded_by",
    "errichten+von": "founded_by",
    "arbeiten+für": "works_for",
    "beschäftigen+bei": "works_for",
    "anstellen+bei": "works_for",
    "sich+befinden": "located_in",
    "liegen+in": "located_in",
    "sitzen+in": "located_in",
    "gebären+in": "born_in",
    "gebären+am": "born_in",
    "erwerben+durch": "acquired",
    "kaufen+durch": "acquired",
    "übernehmen+durch": "acquired",
    # French (fr): spaCy lemmas
    "fonder+par": "founded_by",
    "créer+par": "founded_by",
    "établir+par": "founded_by",
    "travailler+pour": "works_for",
    "employer+par": "works_for",
    "embaucher+par": "works_for",
    "situer+à": "located_in",
    "baser+à": "located_in",
    "implanter+à": "located_in",
    "naître+à": "born_in",
    "acquérir+par": "acquired",
    "racheter+par": "acquired",
    # Spanish + Portuguese (shared lemmas, no duplicate keys)
    "fundar+por": "founded_by",
    "crear+por": "founded_by",
    "criar+por": "founded_by",
    "establecer+por": "founded_by",
    "estabelecer+por": "founded_by",
    "trabajar+para": "works_for",
    "trabalhar+para": "works_for",
    "emplear+por": "works_for",
    "empregar+por": "works_for",
    "contratar+por": "works_for",
    "ubicar+en": "located_in",
    "situar+en": "located_in",
    "localizar+em": "located_in",
    "situar+em": "located_in",
    "sediar+em": "located_in",
    "tener+sede": "located_in",
    "nacer+en": "born_in",
    "nascer+em": "born_in",
    "adquirir+por": "acquired",
    "comprar+por": "acquired",
    # Chinese: verb + "由" (agent marker) or "被" (passive)
    "创立+由": "founded_by",
    "创建+由": "founded_by",
    "成立+由": "founded_by",
    "创办+由": "founded_by",
    "设立+由": "founded_by",
    "任职+于": "works_for",
    "就职+于": "works_for",
    "工作+在": "works_for",
    "位于+在": "located_in",
    "坐落+在": "located_in",
    "总部设+在": "located_in",
    "出生+在": "born_in",
    "出生+于": "born_in",
    "收购+由": "acquired",
    "并购+由": "acquired",
    # Japanese: verb + "によって" (agent marker)
    "設立+によって": "founded_by",
    "創立+によって": "founded_by",
    "勤務+で": "works_for",
    "在籍+で": "works_for",
    "位置+に": "located_in",
    "所在+に": "located_in",
    "本社+を": "located_in",
    "出生+に": "born_in",
    "買収+によって": "acquired",
}

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
    """Extract typed relations using dependency parse — semantica-aligned."""

    def __init__(self, language: str = "en", confidence_threshold: float = 0.3, max_distance: int = 100):
        self.language = language
        self.confidence_threshold = confidence_threshold
        self.max_distance = max_distance

    def extract(self, text: str, entities: List[Entity], doc=None, **options) -> List[Relation]:
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
                            inferred.append(
                                Relation(
                                    subject=r.subject,
                                    predicate=inferred_rel,
                                    obj=r2.obj,
                                    confidence=min(r.confidence, r2.confidence) * 0.9,
                                    metadata={"method": "multi_hop", "via": f"{r.predicate}→{r2.predicate}"},
                                )
                            )
        return relations + inferred

    # ------------------------------------------------------------------
    # Dependency extraction
    # ------------------------------------------------------------------

    # ------------------------------------------------------------------
    # Language-aware role mapping
    # ------------------------------------------------------------------

    def _roles(self) -> Dict[str, str]:
        """Get role → dep label mapping for current language."""
        return _LANG_DEP_RULES.get(self.language, _LANG_DEP_RULES["en"])

    def _get_by_role(self, root, role: str, entity_map) -> list:
        """Get entities for a semantic role (language-aware). Returns [(Entity, prep?)]"""
        rule = self._roles().get(role)
        if rule is None:
            return []
        results = []

        for c in root.children:
            dep = c.dep_
            if isinstance(rule, str):
                if dep == rule:
                    ent = self._entity_from_subtree(c, entity_map)
                    if ent:
                        results.append((ent, None))
            elif isinstance(rule, tuple):
                parent_dep, child_dep = rule[0], rule[1]
                # Check optional case marker (e.g., "由" for zh, "によって" for ja)
                case_marker = rule[2] if len(rule) > 2 else None
                if dep == parent_dep:
                    if case_marker:
                        # Check if any child has the expected case lemma
                        has_case = any(gc.lemma_ == case_marker or gc.text == case_marker for gc in c.subtree)
                        if not has_case:
                            continue
                    if child_dep is None:
                        ent = self._entity_from_subtree(c, entity_map)
                        if ent:
                            results.append((ent, c.lemma_.lower() if role == "prep_obj" else None))
                    else:
                        for gc in c.children:
                            if gc.dep_ == child_dep:
                                ent = self._entity_from_subtree(gc, entity_map)
                                if ent:
                                    prep = c.lemma_.lower() if role == "prep_obj" else None
                                    results.append((ent, prep))
                                break
        return results

    def _extract_with_dep(self, text, doc, entities) -> List[Relation]:
        relations = []
        entity_map = self._build_entity_map_multi(entities)
        is_de = self.language == "de"

        for sent in doc.sents:
            for token in sent:
                # German: ROOT is aux verb, real verb is "oc" child
                if is_de:
                    if token.dep_ != "ROOT":
                        continue
                    for c in token.children:
                        if c.dep_ == "oc":
                            # German: args attach to aux (ROOT), not main verb (oc)
                            # Pass both: root aux for args, oc for verb lemma
                            relations.extend(self._extract_from_root(text, c, entity_map, aux_root=token))
                    continue

                if token.dep_ != "ROOT":
                    continue
                relations.extend(self._extract_from_root(text, token, entity_map))
                if token.lemma_ == "be":
                    relations.extend(self._extract_copula(text, token, entity_map))

        return relations

    def _extract_from_root(self, text, root, entity_map, aux_root=None) -> List[Relation]:
        relations = []
        # Fall back to text when lemma is empty (zh, ja don't have lemmatizers)
        verb_lemma = (root.lemma_ or root.text).lower()
        # For languages like German where args attach to aux verb
        check = root if aux_root is None else aux_root

        # Negation
        if any(c.dep_ in ("neg", "advmod:neg") for c in check.children):
            return relations

        # Extract roles (check both the main verb and optional aux parent)
        def first(lst):
            return lst[0][0] if lst else None

        def get_roles(token):
            return (
                first(self._get_by_role(token, "subj", entity_map)),
                first(self._get_by_role(token, "pass_subj", entity_map)),
                first(self._get_by_role(token, "dobj", entity_map)),
                first(self._get_by_role(token, "agent", entity_map)),
                self._get_by_role(token, "prep_obj", entity_map),
                any(c.dep_ == "aux" for c in token.children),
            )

        s1, sp1, d1, a1, p1, h1 = get_roles(root)
        s2, sp2, d2, a2, p2, h2 = (None, None, None, None, [], False)
        if aux_root:
            s2, sp2, d2, a2, p2, h2 = get_roles(aux_root)

        # Merge: prefer found roles from aux if main verb lacks them
        nsubj = s1 or s2
        nsubjpass = sp1 or sp2
        dobj = d1 or d2
        agent_entity = a1 or a2
        prep_list = p1 + p2
        has_aux = h1 or h2 or aux_root is not None
        has_explicit_agent = agent_entity is not None

        # Detect passive:
        # - explicit pass_subj (en, fr, pt)
        # - subj + agent + aux (Spanish-style)
        # - subj + agent for languages with agent marker (zh, ja)
        is_passive_candidate = has_explicit_agent and (has_aux or self.language in ("zh", "ja"))

        effective_nsubjpass = nsubjpass or (nsubj if is_passive_candidate else None)
        effective_nsubj = nsubj if not is_passive_candidate else None

        # Passive: X was founded/acquired by Y
        if effective_nsubjpass and agent_entity:
            prep = ""
            # Try language-appropriate prepositions/case markers
            candidates = ("by", "von", "par", "por", "durch", "由", "によって")
            for candidate in candidates:
                if self._lookup(verb_lemma, candidate):
                    prep = candidate
                    break
            rel_type = self._lookup(verb_lemma, prep) if prep else None
            if rel_type:
                if rel_type in ("founded_by", "acquired"):
                    subj, obj = effective_nsubjpass, agent_entity
                else:
                    subj, obj = agent_entity, effective_nsubjpass
                relations.append(self._make_rel(subj, rel_type, obj, 0.90, "passive", verb_lemma))

        # Active: X VERB Y or X VERB prep Y
        if effective_nsubj:
            if dobj:
                rt = self._lookup(verb_lemma, None)
                if rt:
                    relations.append(self._make_rel(effective_nsubj, rt, dobj, 0.85, "active", verb_lemma))
            for prep_entity, prep_l in prep_list:
                rt = self._lookup(verb_lemma, prep_l)
                if rt:
                    relations.append(self._make_rel(effective_nsubj, rt, prep_entity, 0.85, "active_prep", verb_lemma, prep=prep_l))

        # Passive with prep ("is based in")
        if effective_nsubjpass and prep_list and not agent_entity:
            for prep_entity, prep_l in prep_list:
                rt = self._lookup(verb_lemma, prep_l)
                if not rt:
                    rt = self._lookup("be+" + verb_lemma, prep_l)
                if rt:
                    relations.append(self._make_rel(effective_nsubjpass, rt, prep_entity, 0.85, "passive_prep", verb_lemma, prep=prep_l))

        return relations

    @staticmethod
    def _make_rel(subj, pred, obj, conf, method, verb, prep=""):
        m = {"method": method, "verb": verb}
        if prep:
            m["prep"] = prep
        return Relation(subject=subj, predicate=pred, obj=obj, confidence=conf, metadata=m)

    @staticmethod
    def _already_has(rels, subj, pred, obj) -> bool:
        for r in rels:
            if r.subject.text == subj.text and r.predicate == pred and r.obj.text == obj.text:
                return True
        return False

    def _extract_copula(self, text, root, entity_map) -> List[Relation]:
        relations = []
        # Get subject using language-specific rules
        subjs = self._get_by_role(root, "subj", entity_map)
        subj = subjs[0][0] if subjs else None
        if not subj:
            return relations

        title_lemma = None
        prep_obj = None
        deps_to_check = ["attr", "pred"]  # attr=en, pred=de
        for c in root.children:
            if c.dep_ not in deps_to_check:
                continue
            for cc in c.children:
                prep_deps = {"prep", "mo", "case"}  # en=prep, de=mo, fr/case
                if cc.dep_ not in prep_deps:
                    continue
                for gc in cc.children:
                    pobj_deps = {"pobj", "nk", "obl"}
                    if gc.dep_ in pobj_deps or True:  # accept any child as object
                        prep_obj = self._entity_from_subtree(gc, entity_map)
                        if prep_obj:
                            title_lemma = c.lemma_.lower()
                        break

        if not title_lemma or not prep_obj:
            return relations
        for keyword, rel_types in _COPULA_TITLE_MAP.items():
            if keyword in title_lemma:
                for rt in rel_types:
                    relations.append(
                        Relation(
                            subject=subj,
                            predicate=rt,
                            obj=prep_obj,
                            confidence=0.88,
                            context=text,
                            metadata={"method": "copula", "title": title_lemma},
                        )
                    )
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
    def _find_best_entity(key: str, entity_map: Dict[str, List[Entity]], fallback_text: str = "") -> Optional[Entity]:
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

        spans = [(m.start(), m.end()) for m in _re.finditer(r"[^.!?]+(?:[.!?](?=\s|$))+", text)]

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
                rels.append(
                    Relation(
                        subject=e1,
                        predicate="related_to",
                        obj=e2,
                        confidence=0.4,
                        context=text[cs:ce],
                        metadata={"method": "cooccurrence"},
                    )
                )
        return rels
