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
spaCy-based entity and relationship extractor for GraphRAG.

Combines techniques from **LinearRAG** and **MGranRAG**:

* **Entity extraction** uses MGranRAG's multi-pass stacking algorithm
  (hyphen/apostrophe merging → capitalised-word merging → continuous
  noun/number merging) combined with spaCy NER, then deduplicated via
  ``ner_all_keywords``.
* **Relationship inference** follows LinearRAG's *relation-free* approach:
  entities co-occurring in the same sentence (or nearby sentences) are
  linked by implicit semantic edges whose description is the shared
  sentence text (semantic bridging).  Edge weights are optionally TF-
  normalised.

No LLM calls are needed for the extraction step itself.  The LLM is only
used downstream (inherited from ``Extractor``) for merging / summarising
duplicate entity descriptions when the same entity appears in multiple
chunks.
"""

import logging
from collections import defaultdict

from rag.graphrag.general.extractor import Extractor
from rag.llm.chat_model import Base as CompletionLLM

# ---------------------------------------------------------------------------
# spaCy model loading (lazy, module-level singleton)
# ---------------------------------------------------------------------------
_nlp = None
_nlp_model_name = ""


def _load_spacy_model(model_name: str = "en_core_web_sm"):
    """Load (or return cached) spaCy language model.

    Automatically downloads the model if it is not yet installed.
    """
    global _nlp, _nlp_model_name
    if _nlp is not None and _nlp_model_name == model_name:
        return _nlp
    try:
        import spacy
    except ImportError:
        raise ImportError(
            "spaCy is required for the spacy GraphRAG method. "
            "Install it with:  pip install spacy  &&  python -m spacy download en_core_web_sm"
        )
    try:
        _nlp = spacy.load(model_name)
        logging.info("Loaded spaCy model '%s'", model_name)
    except OSError:
        logging.warning(
            "spaCy model '%s' not found; downloading automatically …", model_name
        )
        from spacy.cli import download as spacy_download
        spacy_download(model_name)
        _nlp = spacy.load(model_name)
        logging.info("Downloaded and loaded spaCy model '%s'", model_name)
    _nlp_model_name = model_name
    return _nlp


# ---------------------------------------------------------------------------
# spaCy ↔ application entity-type mapping
# ---------------------------------------------------------------------------
# spaCy's built-in entity labels → the application-level types used by
# ``DEFAULT_ENTITY_TYPES``.  Labels not listed here fall through to
# ``"category"``.
SPACY_TO_APP_ENTITY_TYPE: dict[str, str] = {
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
}

# Labels to skip entirely (from LinearRAG: ordinals / cardinals are rarely
# useful as graph nodes).
_SKIP_SPACY_LABELS = {"ORDINAL", "CARDINAL"}


# ---------------------------------------------------------------------------
# MGranRAG-style multi-pass keyword extraction
# ---------------------------------------------------------------------------

def _has_uppercase(text: str) -> bool:
    return any(c.isupper() for c in text)


def _replace_word(word: str) -> str:
    """Normalise spaces around hyphens and apostrophes (from MGranRAG)."""
    return (
        word.replace(" - ", "-")
        .replace(" -", "-")
        .replace("- ", "-")
        .replace(" 's", "'s")
        .replace(" 'S", "'S")
    )


def extract_keywords(spacy_doc) -> set[str]:
    """MGranRAG-style 3-pass stacking keyword extraction.

    Phase 1 — Hyphen / apostrophe merging:
        Tokens connected by ``-`` or ``'s`` are merged into a single
        phrase labelled ``NP`` (e.g. ``New-York``, ``cat's``).

    Phase 2 — Capitalised-word merging:
        Consecutive tokens whose ``shape_`` contains ``X`` (i.e. start
        with an uppercase letter) are merged.  Function words (ADP, CCONJ,
        DET, PART) between them are absorbed as well, producing phrases
        like ``King of England``.  Merged results are labelled ``NX``
        unless already ``PROPN``.

    Phase 3 — Continuous noun / number merging:
        Consecutive tokens with POS in ``[PROPN, NOUN, NUM, NX, NP]``
        are merged and labelled ``NNN`` (unless already ``PROPN``).

    Finally, results with a trailing lowercase non-noun word are
    truncated, and coordinating conjunctions (``and``, ``or``) inside a
    merged phrase cause it to be split so that each proper noun is
    extracted individually (e.g. ``Bob and Lucy`` → ``Bob``, ``Lucy``).
    """
    # ── Phase 1: hyphen / apostrophe ──────────────────────────────────
    f1_word: list[str] = []
    f1_shape: list[str] = []
    f1_pos: list[str] = []
    f1_pos_list: list[list[str]] = []
    f1_word_list: list[list[str]] = []

    is_right = False
    for token in spacy_doc:
        if token.shape_ in ("'x", "-") and token.pos_ in ("PUNCT", "PART"):
            if token.shape_ == "-":
                is_right = True
            if f1_word:
                f1_word[-1] += token.text
                f1_pos[-1] = "NP"
                f1_pos_list[-1].append(token.pos_)
                f1_word_list[-1].append(token.text)
        elif is_right:
            is_right = False
            if f1_word:
                f1_word[-1] += token.text
                f1_pos[-1] = "NP"
                f1_pos_list[-1].append(token.pos_)
                f1_word_list[-1].append(token.text)
        else:
            f1_word.append(token.text)
            f1_shape.append(token.shape_)
            f1_pos.append(token.pos_)
            f1_pos_list.append([token.pos_])
            f1_word_list.append([token.text])

    # ── Phase 2: capitalised-word merging ───────────────────────────
    f2_word: list[str] = []
    f2_shape: list[str] = []
    f2_pos: list[str] = []
    f2_pos_list: list[list[str]] = []
    f2_word_list: list[list[str]] = []

    for cur in range(len(f1_word)):
        cw = f1_word[cur]
        cs = f1_shape[cur]
        cp = f1_pos[cur]
        cpl = f1_pos_list[cur]
        cwl = f1_word_list[cur]

        if "X" in cs or cp in ("ADP", "CCONJ", "DET", "PART"):
            if f2_word and "X" in f2_shape[-1]:
                # Merge with previous capitalised token.
                f2_word[-1] += " " + cw
                f2_shape[-1] += "X"
                if f2_pos[-1] != "PROPN":
                    f2_pos[-1] = "NX"
                f2_pos_list[-1].extend(cpl)
                f2_word_list[-1].extend(cwl)
            else:
                f2_word.append(cw)
                f2_shape.append(cs + "Start" if "X" in cs else cs)
                f2_pos.append(cp)
                f2_pos_list.append(cpl)
                f2_word_list.append(cwl)
        else:
            f2_word.append(cw)
            f2_shape.append(cs)
            f2_pos.append(cp)
            f2_pos_list.append(cpl)
            f2_word_list.append(cwl)

    # ── Phase 3: continuous noun / number merging ───────────────────
    f3_word: list[str] = []
    f3_shape: list[str] = []
    f3_pos: list[str] = []
    f3_pos_list: list[list[str]] = []
    f3_word_list: list[list[str]] = []

    _noun_pos = {"PROPN", "NOUN", "NUM", "NX", "NP"}
    _noun_pos_ext = _noun_pos | {"NNN"}

    for cur in range(len(f2_word)):
        cw = f2_word[cur]
        cs = f2_shape[cur]
        cp = f2_pos[cur]
        cpl = f2_pos_list[cur]
        cwl = f2_word_list[cur]

        if cp in _noun_pos:
            if f3_word and f3_pos[-1] in _noun_pos_ext:
                f3_word[-1] += " " + cw
                f3_shape[-1] += "X"
                if f3_pos[-1] != "PROPN":
                    f3_pos[-1] = "NNN"
                f3_pos_list[-1].extend(cpl)
                f3_word_list[-1].extend(cwl)
            else:
                f3_word.append(cw)
                f3_shape.append(cs)
                f3_pos.append(cp)
                f3_pos_list.append(cpl)
                f3_word_list.append(cwl)
        else:
            f3_word.append(cw)
            f3_shape.append(cs)
            f3_pos.append(cp)
            f3_pos_list.append(cpl)
            f3_word_list.append(cwl)

    # ── Final keyword collection ────────────────────────────────────
    keywords: set[str] = set()
    for cur in range(len(f3_word)):
        cw = f3_word[cur]
        cp = f3_pos[cur]
        cpl = f3_pos_list[cur]
        cwl = f3_word_list[cur]

        if cp not in _noun_pos_ext:
            continue

        # Truncate trailing lowercase non-noun / non-number words.
        if cwl and not _has_uppercase(cwl[-1]) and cpl[-1] not in (
            "PROPN",
            "NOUN",
            "NUM",
            "PART",
        ):
            for i in range(len(cpl) - 1, 0, -1):
                if cpl[i] in ("PROPN", "NOUN", "NUM", "PART") or _has_uppercase(
                    cwl[i]
                ):
                    break
            word = _replace_word(" ".join(cwl[: i + 1]))
            keywords.add(word)
        else:
            word = _replace_word(cw)
            keywords.add(word)

        # Split on coordinating conjunctions (and/or) inside merged
        # phrases so that individual proper nouns are also extracted
        # (e.g. ``Bob and Lucy`` → ``Bob``, ``Lucy``).
        if any(p in ("PROPN", "NOUN", "NUM") for p in cpl):
            cur_kws: list[str] = []
            for pidx, pos in enumerate(cpl):
                if pos == "CCONJ" and cwl[pidx] and cwl[pidx][0].islower():
                    if cur_kws:
                        keywords.add(_replace_word(" ".join(cur_kws)))
                    cur_kws = []
                else:
                    cur_kws.append(cwl[pidx])
            if cur_kws:
                keywords.add(_replace_word(" ".join(cur_kws)))

    return keywords


def get_ner(spacy_doc) -> dict[str, str]:
    """Return ``{entity_text: spaCy_label}`` for all NER entities."""
    entities_dict: dict[str, str] = {}
    for ent in spacy_doc.ents:
        if ent.label_ in _SKIP_SPACY_LABELS:
            continue
        text = ent.text.strip()
        for t in text.split("\n"):
            t = t.strip()
            if t:
                entities_dict[t] = ent.label_
    return entities_dict


def ner_all_keywords(spacy_doc) -> set[str]:
    """Combine rule-based keyword extraction with spaCy NER (MGranRAG).

    Returns the union of:
    - keywords from the 3-pass stacking algorithm (``extract_keywords``)
    - entity texts from spaCy NER (``get_ner``)
    """
    keywords = extract_keywords(spacy_doc)
    ner_dict = get_ner(spacy_doc)
    return keywords.union(ner_dict.keys())


# ---------------------------------------------------------------------------
# Main extractor class
# ---------------------------------------------------------------------------

class GraphExtractor(Extractor):
    """Extract entities and relationships using spaCy (no LLM calls).

    Entity extraction
        MGranRAG's ``ner_all_keywords`` combines a 3-pass stacking
        keyword algorithm with spaCy NER, yielding broader coverage than
        NER alone (e.g. it catches compound nouns, hyphenated terms, and
        multi-word proper nouns that NER might miss).

    Relationship inference
        LinearRAG's *relation-free* semantic bridging: entities
        co-occurring in the same sentence (or within
        ``max_sentence_distance`` sentences) are linked by an implicit
        edge.  The edge description is the shared sentence text, which
        provides natural language context without requiring an LLM.

        Optionally, edge weights are TF-normalised (LinearRAG):
        ``weight = count(entity_in_chunk) / sum(all_entity_counts_in_chunk)``.

    The ``llm_invoker`` is only used downstream for merging / summarising
    duplicate descriptions (inherited from ``Extractor``).

    Parameters
    ----------
    llm_invoker : CompletionLLM
        LLM handle (used only for description summarisation, not extraction).
    language : str
        Language hint.
    entity_types : list[str] | None
        Application-level entity types to keep.  Entities whose mapped
        type is not in this list are discarded.
    spacy_model : str
        Name of the spaCy model to load (default ``en_core_web_sm``).
    max_sentence_distance : int
        When inferring relationships, pair entities that co-occur within
        the same sentence.  If > 1, also pair entities in sentences whose
        indices differ by at most this value.
    relationship_strength : int
        Default weight assigned to every inferred relationship when
        ``use_tf_weight`` is ``False``.
    use_tf_weight : bool
        If ``True``, use TF-normalised weighting (LinearRAG-style) for
        edge weights instead of the fixed ``relationship_strength``.
    """

    def __init__(
        self,
        llm_invoker: CompletionLLM,
        language: str | None = "English",
        entity_types: list[str] | None = None,
        spacy_model: str = "en_core_web_sm",
        max_sentence_distance: int = 1,
        relationship_strength: int = 1,
        use_tf_weight: bool = False,
    ):
        super().__init__(llm_invoker, language, entity_types)
        self._spacy_model_name = spacy_model
        self._max_sentence_distance = max_sentence_distance
        self._relationship_strength = relationship_strength
        self._use_tf_weight = use_tf_weight
        # Eagerly load the model so import errors surface early.
        self._nlp = _load_spacy_model(spacy_model)

    # ------------------------------------------------------------------
    # Public interface – called by ``Extractor.__call__``
    # ------------------------------------------------------------------

    async def _process_single_content(
        self,
        chunk_key_dp: tuple[str, str],
        chunk_seq: int,
        num_chunks: int,
        out_results,
        task_id="",
    ):
        """Process one chunk through spaCy NER + keyword stacking + co-occurrence."""
        chunk_key = chunk_key_dp[0]
        content = chunk_key_dp[1]
        doc = self._nlp(content)

        # ── 1. Entity extraction (MGranRAG: ner_all_keywords) ────────
        # Build a mapping from keyword text → spaCy label (if available).
        ner_label_map: dict[str, str] = get_ner(doc)
        all_keywords = ner_all_keywords(doc)

        # For each keyword, determine its app-level entity type.
        # - If the keyword matches a NER entity, use that label.
        # - Otherwise, infer from POS heuristics.
        ent_records: dict[str, dict] = {}  # entity_name_upper → record
        ent_by_sent: dict[int, list[dict]] = defaultdict(list)

        for kw in all_keywords:
            kw_upper = kw.strip().upper()
            if not kw_upper:
                continue

            # Determine entity type.
            spacy_label = ner_label_map.get(kw)
            if spacy_label:
                app_type = SPACY_TO_APP_ENTITY_TYPE.get(spacy_label, "category")
            else:
                app_type = self._infer_type_from_pos(doc, kw)

            if app_type not in self._entity_types_set:
                continue

            # Determine which sentence this keyword belongs to.
            sent_idx = self._keyword_sent_idx(doc, kw)

            # Description: use the containing sentence (LinearRAG semantic bridging).
            #sent_text = self._keyword_sent_text(doc, kw)

            ent_record = dict(
                entity_name=kw_upper,
                entity_type=app_type.upper(),
                description="", #sent_text or kw,
                source_id=chunk_key,
            )
            # A keyword may appear multiple times; keep the first.
            if kw_upper not in ent_records:
                ent_records[kw_upper] = ent_record
            ent_by_sent[sent_idx].append(ent_record)

        maybe_nodes: dict[str, list[dict]] = defaultdict(list)
        for name, rec in ent_records.items():
            maybe_nodes[name].append(rec)

        # ── 2. Relationship inference (LinearRAG: sentence co-occurrence) ─
        maybe_edges: dict[tuple, list[dict]] = defaultdict(list)

        # Pre-compute TF weights if needed (LinearRAG).
        entity_tf: dict[str, float] = {}
        if self._use_tf_weight:
            total_count = sum(
                content.upper().count(name) for name in ent_records
            )
            for name in ent_records:
                count = content.upper().count(name)
                entity_tf[name] = count / total_count if total_count > 0 else 0.0

        seen_pairs: set[tuple[str, str]] = set()
        for si in sorted(ent_by_sent.keys()):
            ents_in_range = list(ent_by_sent[si])
            # Expand with nearby sentences.
            for offset in range(1, self._max_sentence_distance + 1):
                for nb_si in (si + offset, si - offset):
                    if nb_si in ent_by_sent:
                        ents_in_range.extend(ent_by_sent[nb_si])
            # Deduplicate by entity name.
            unique: dict[str, dict] = {}
            for e in ents_in_range:
                unique[e["entity_name"]] = e
            ent_list = list(unique.values())

            for a_idx in range(len(ent_list)):
                for b_idx in range(a_idx + 1, len(ent_list)):
                    ea, eb = ent_list[a_idx], ent_list[b_idx]
                    pair = tuple(sorted([ea["entity_name"], eb["entity_name"]]))
                    if pair in seen_pairs:
                        continue
                    seen_pairs.add(pair)

                    # Relationship description: shared sentence text
                    # (LinearRAG semantic bridging — the sentence is the
                    # semantic bridge between entities).
                    #desc = self._cooccurrence_description(doc, ea["entity_name"], eb["entity_name"])

                    # Edge weight: TF-normalised (LinearRAG) or fixed.
                    if self._use_tf_weight:
                        w = (entity_tf.get(ea["entity_name"], 0.0)
                             + entity_tf.get(eb["entity_name"], 0.0))
                        weight = max(w, 0.01)
                    else:
                        weight = self._relationship_strength

                    # Keywords for the edge: the two entity names.
                    edge_record = dict(
                        src_id=pair[0],
                        tgt_id=pair[1],
                        weight=weight,
                        description="", #desc,
                        keywords=[ea["entity_name"], eb["entity_name"]],
                        source_id=chunk_key,
                    )
                    maybe_edges[pair].append(edge_record)

        token_count = len(doc)
        out_results.append((dict(maybe_nodes), dict(maybe_edges), token_count))
        if self.callback:
            self.callback(
                0.5 + 0.1 * len(out_results) / num_chunks,
                msg=f"[spacy] Entities extraction of chunk {chunk_seq} "
                f"{len(out_results)}/{num_chunks} done, "
                f"{len(maybe_nodes)} nodes, {len(maybe_edges)} edges, "
                f"{token_count} tokens.",
            )

    # ------------------------------------------------------------------
    # Helpers
    # ------------------------------------------------------------------

    @property
    def _entity_types_set(self) -> set[str]:
        return {t.lower() for t in self._entity_types}

    @staticmethod
    def _infer_type_from_pos(doc, keyword: str) -> str:
        """Infer an application-level entity type from POS tags when the
        keyword was found by the stacking algorithm but not by NER."""
        kw_upper = keyword.upper()
        for token in doc:
            if token.text.upper() == kw_upper or token.text.upper().startswith(kw_upper.split()[0]):
                if token.pos_ == "PROPN":
                    return "person"
                if token.pos_ == "NOUN":
                    return "category"
                if token.pos_ == "NUM":
                    return "event"
                break
        # Fallback: check for uppercase → likely a named entity.
        if _has_uppercase(keyword):
            return "person"
        return "category"

    @staticmethod
    def _keyword_sent_idx(doc, keyword: str) -> int:
        """Return the sentence index that contains *keyword*."""
        kw_lower = keyword.lower()
        for i, sent in enumerate(doc.sents):
            if kw_lower in sent.text.lower():
                return i
        return 0

    @staticmethod
    def _keyword_sent_text(doc, keyword: str) -> str | None:
        """Return the sentence text containing *keyword* (LinearRAG semantic bridging)."""
        kw_lower = keyword.lower()
        for sent in doc.sents:
            if kw_lower in sent.text.lower():
                return sent.text.strip()
        return None

    @staticmethod
    def _cooccurrence_description(doc, head_name: str, tail_name: str) -> str:
        """Derive a relationship description using sentence co-occurrence
        (LinearRAG) with dependency-path enhancement as fallback.

        If both entities appear in the same sentence, that sentence is
        used as the description (semantic bridging).  Otherwise, try to
        find a lowest common ancestor in the dependency tree.  As a last
        resort, return a generic statement.
        """
        head_lower = head_name.lower()
        tail_lower = tail_name.lower()

        # Primary: shared sentence text (LinearRAG semantic bridging).
        for sent in doc.sents:
            sent_lower = sent.text.lower()
            if head_lower in sent_lower and tail_lower in sent_lower:
                return sent.text.strip()

        # Fallback: dependency path via LCA.
        head_tok = GraphExtractor._find_token_by_text(doc, head_name)
        tail_tok = GraphExtractor._find_token_by_text(doc, tail_name)
        if head_tok is not None and tail_tok is not None:
            path_head = list(GraphExtractor._ancestor_path(head_tok))
            path_tail = list(GraphExtractor._ancestor_path(tail_tok))
            lca = None
            for h in path_head:
                for t in path_tail:
                    if h == t:
                        lca = h
                        break
                if lca is not None:
                    break
            if lca is not None and lca is not head_tok and lca is not tail_tok:
                return f"{head_name} is related to {tail_name} via '{lca.lemma_}'"

        # Final fallback: nearby sentences.
        head_sent = GraphExtractor._find_sent_for_text(doc, head_lower)
        if head_sent is not None:
            return head_sent.text.strip()

        return f"{head_name} is related to {tail_name}"

    @staticmethod
    def _find_token_by_text(doc, ent_name: str):
        """Return the head token of the first spaCy entity matching *ent_name*."""
        target = ent_name.upper()
        for ent in doc.ents:
            if ent.text.strip().upper() == target:
                return ent.root
        # Fallback: token-level match for keywords not in doc.ents.
        for token in doc:
            if token.text.strip().upper() == target:
                return token
        return None

    @staticmethod
    def _find_sent_for_text(doc, text_lower: str):
        """Return the first ``Span`` whose text contains *text_lower*."""
        for sent in doc.sents:
            if text_lower in sent.text.lower():
                return sent
        return None

    @staticmethod
    def _ancestor_path(token):
        """Yield *token* then each ancestor up to the root."""
        yield token
        for anc in token.ancestors:
            yield anc
