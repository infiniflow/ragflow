"""LightGraph — spaCy-based lightweight graph extraction.

Entity extraction: MGranRAG ``ner_all_keywords`` + spaCy NER union.
Relation extraction: ``DepRelationExtractor`` typed relations + LinearRAG co-occurrence.
Incremental update: entity-level, no full-graph rebuilds, no n_hop/PageRank precomputed.
"""

import asyncio
import json
import logging
from typing import AsyncIterator, Callable, Dict, List, Optional, Tuple

import xxhash
import spacy
from spacy.language import Language

from common.exceptions import TaskCanceledException
from common.misc_utils import thread_pool_exec
from common.token_utils import num_tokens_from_string
from common import settings
from rag.nlp import rag_tokenizer, search

# ── spaCy model routing ─────────────────────────────────────────────
# Map RAGFlow language labels (e.g. "Chinese", "English") to spaCy
# model identifiers.  Keys are lowercased during lookup.
_LANG_TO_MODEL = {
    "en": "en_core_web_sm",
    "english": "en_core_web_sm",
    "zh": "zh_core_web_sm",
    "chinese": "zh_core_web_sm",
    "zh-cn": "zh_core_web_sm",
    "zh_cn": "zh_core_web_sm",
    "de": "de_core_news_sm",
    "german": "de_core_news_sm",
    "fr": "fr_core_news_sm",
    "french": "fr_core_news_sm",
    "es": "es_core_news_sm",
    "spanish": "es_core_news_sm",
    "pt": "pt_core_news_sm",
    "portuguese": "pt_core_news_sm",
    "ja": "ja_core_news_sm",
    "japanese": "ja_core_news_sm",
}

_nlp_cache: Dict[str, Language] = {}


def _resolve_spacy_model(language: str) -> str:
    """Map RAGFlow language label (e.g. 'Chinese', 'English', 'zh', 'en')
    to a spaCy model name.  Falls back to ``en_core_web_sm``."""
    key = (language or "en").strip().lower()
    for k, v in _LANG_TO_MODEL.items():
        if k == key or (len(k) <= 3 and key.startswith(k)):
            return v
    return "en_core_web_sm"


def _load_model(language: str) -> Language:
    model_name = _resolve_spacy_model(language)
    if model_name not in _nlp_cache:
        try:
            nlp = spacy.load(model_name)
        except OSError:
            from spacy.cli import download
            download(model_name)
            nlp = spacy.load(model_name)
        _nlp_cache[model_name] = nlp
    return _nlp_cache[model_name]


# ── ID minting ──────────────────────────────────────────────────────


def _entity_row_id(entity_name: str, kb_id: str) -> str:
    return xxhash.xxh64(
        f"lightgraph:entity:{entity_name.upper()}:{kb_id}".encode("utf-8", "surrogatepass")
    ).hexdigest()


def _relation_row_id(from_ent: str, to_ent: str, rel_type: str, kb_id: str) -> str:
    return xxhash.xxh64(
        f"lightgraph:relation:{from_ent.upper()}:{to_ent.upper()}:{rel_type}:{kb_id}".encode(
            "utf-8", "surrogatepass"
        )
    ).hexdigest()


# ── Entity extraction: MGranRAG + NER ───────────────────────────────


def _extract_ner(doc) -> List[dict]:
    """Extract spaCy NER entities, return list of dicts with text/label/pos."""
    _SKIP = {"ORDINAL", "CARDINAL"}
    entities = []
    seen = set()
    for ent in doc.ents:
        if ent.label_ in _SKIP:
            continue
        key = (ent.text.strip().lower(), ent.start_char)
        if key in seen:
            continue
        seen.add(key)
        entities.append({
            "text": ent.text.strip(),
            "type": ent.label_,
            "start_char": ent.start_char,
            "end_char": ent.end_char,
        })
    return entities


def _mgrank_keywords(doc) -> set:
    """MGranRAG 3-pass keyword extraction — module-level function from graph_extractor."""
    from rag.graphrag.ner.graph_extractor import extract_keywords, get_ner
    keywords = extract_keywords(doc)
    ner_dict = get_ner(doc)
    return keywords.union(ner_dict.keys())


def _normalize_entity_type(ent_type: str, name: str, from_spacy_ner: bool) -> str:
    """Return the best entity type, normalizing MGranRAG-inferred types.

    - spaCy NER types are preserved as-is.
    - MGranRAG single-word uppercase → ``TOPIC`` (technical term,
      not a proper name)
    - MGranRAG multi-word uppercase → ``ORG`` (likely a named concept)
    """
    if from_spacy_ner:
        return ent_type
    words = name.split()
    if len(words) >= 2:
        return "ORG"
    return "TOPIC"


def _is_valid_entity(name: str) -> bool:
    """Reject entity-name noise regardless of extraction source.

    Filters out: short names, markdown/HTML artifacts, truncations.
    Does NOT filter based on entity type — that's left to
    ``_normalize_entity_type`` so the graph retains density.
    """
    name = name.strip()
    if len(name) < 2:
        return False
    if "#" in name or "<" in name or ">" in name:
        return False
    if "\r" in name or "\n" in name:
        return False
    if name.endswith("-"):
        return False
    # Single non-letter char (e.g. ``#``, ``1``)
    if len(name) == 1 and not name.isalpha():
        return False
    return True


def _extract_entities_merged(doc, language: str) -> List[dict]:
    """Merge NER entities + MGranRAG keyword entities, return list of dicts.

    ``_is_valid_entity`` is applied to BOTH sources so that spaCy NER
    noise (e.g. ``#`` tagged as MONEY, ``END-`` tagged as PERSON) is
    also filtered out.
    """
    # 1. NER entities (preserve spaCy types, filter only name-shape noise)
    ner_ents = []
    ner_names = set()
    for e in _extract_ner(doc):
        name = e["text"].strip()
        upper = name.upper()
        if _is_valid_entity(name):
            e["text"] = upper
            ner_ents.append(e)
            ner_names.add(upper)

    # 2. MGranRAG keyword entities (normalize type, filter name-shape noise)
    kw_names = _mgrank_keywords(doc)
    for name in kw_names:
        upper = name.strip().upper()
        if not upper or upper in ner_names:
            continue
        if _is_valid_entity(name):
            etype = _normalize_entity_type(_infer_type_from_pos(doc, name), upper, from_spacy_ner=False)
            ner_ents.append({"text": upper, "type": etype, "start_char": -1, "end_char": -1})
            ner_names.add(upper)

    return ner_ents


def _infer_type_from_pos(doc, keyword: str) -> str:
    """Infer app-level type from POS when keyword is not a NER entity.

    Only ``PROPN`` and ``NUM`` produce FIRST-CLASS entity types;
    everything else (noun, adjective, etc.) returns ``OTHER`` which
    ``_is_valid_entity`` then rejects unless it is a multi-word phrase.
    """
    keyword_upper = keyword.upper()
    for token in doc:
        if token.text.upper() == keyword_upper or (len(keyword.split()) > 1 and token.text.upper() == keyword_upper.split()[0]):
            if token.pos_ == "PROPN":
                return "PERSON"
            if token.pos_ == "NUM":
                return "DATE"
            break
    # Multi-word capitalized → likely a named concept
    if len(keyword.split()) >= 2 and any(c.isupper() for c in keyword):
        return "ORG"
    return "OTHER"


# ── Relation extraction: typed + co-occurrence ──────────────────────


def _extract_relations_merged(text: str, entities: List[dict], doc, language: str) -> List[dict]:
    """Extract typed relations + co-occurrence relations.

    Each relation dict:
        from_entity: str (uppercased)
        to_entity: str (uppercased)
        type: str — "founded_by" | "works_for" | "related_to" | ...
        confidence: float
        weight: int (2 for typed, 1 for co-occurrence)
    """
    from rag.graphrag.ner.dep_relation_extractor import DepRelationExtractor, Entity

    # Build Entity objects for DepRelationExtractor (need positional info)
    ner_entities = [
        Entity(text=e["text"], label=e["type"],
               start_char=e.get("start_char", 0) if e.get("start_char", -1) >= 0 else 0,
               end_char=e.get("end_char", 0) if e.get("end_char", -1) >= 0 else 0)
        for i, e in enumerate(entities)
        if e.get("start_char", -1) >= 0 and e.get("end_char", -1) >= 0
    ]
    name_set = {e["text"].upper() for e in entities}

    relations: List[dict] = []
    seen_pairs: set = set()

    # 1. Typed relations via DepRelationExtractor (only for positional entities)
    if len(ner_entities) >= 2:
        dep_ext = DepRelationExtractor(language=language, confidence_threshold=0.3)
        typed = dep_ext.extract(text, ner_entities, doc=doc)
        for r in typed:
            subj = r.subject.text.strip().upper()
            obj = r.obj.text.strip().upper()
            rel_type = r.predicate  # "founded_by", "works_for", "related_to", etc.
            pair = tuple(sorted([subj, obj]))
            if pair in seen_pairs:
                continue
            seen_pairs.add(pair)
            relations.append({
                "from_entity": subj,
                "to_entity": obj,
                "type": rel_type,
                "confidence": r.confidence,
                "weight": 2 if rel_type != "related_to" else 1,
            })

    # 2. Co-occurrence relations (all entity pairs co-occurring in the same sentence)
    # MGranRAG keywords lack character positions, so match by name text.
    sent_lower = [sent.text.lower() for sent in doc.sents] if len(entities) >= 2 else []
    if sent_lower:
        for si, sent in enumerate(doc.sents):
            ents_in = []
            for e in entities:
                start = e.get("start_char", -1)
                if start >= 0:
                    if start >= sent.start_char and e.get("end_char", -1) <= sent.end_char:
                        ents_in.append(e)
                else:
                    if e["text"].lower() in sent_lower[si]:
                        ents_in.append(e)
            for i in range(len(ents_in)):
                for j in range(i + 1, len(ents_in)):
                    pair = tuple(sorted([ents_in[i]["text"].upper(), ents_in[j]["text"].upper()]))
                    if pair in seen_pairs:
                        continue
                    seen_pairs.add(pair)
                    relations.append({
                        "from_entity": pair[0],
                        "to_entity": pair[1],
                        "type": "related_to",
                        "confidence": 0.4,
                        "weight": 1,
                    })

    return relations


# ── ES field helpers ────────────────────────────────────────────────
# Infinity/ES schema: source_doc_ids / source_chunk_ids are varchar
# with whitespace-# analyzer, so store as space-joined strings.


def _ids_to_str(ids: list) -> str:
    return " ".join(str(i).strip() for i in ids if i and str(i).strip())


def _str_to_ids(s) -> list:
    """Parse a space-joined ID string back into a list.  Accepts str or list
    (the latter for Elasticsearch which stores arrays natively)."""
    if isinstance(s, list):
        return [str(x).strip() for x in s if x and str(x).strip()]
    if isinstance(s, str) and s.strip():
        return s.strip().split()
    return []


# ── ES doc builders ─────────────────────────────────────────────────


def _entity_to_es_doc(entity: dict, doc_id: str, chunk_id: str,
                      kb_id: str, vec: list) -> dict:
    name = entity["text"].upper().strip()
    ent_type = entity.get("type", "OTHER")
    row_id = _entity_row_id(name, kb_id)
    desc = f"{name} ({ent_type})"
    ltks = rag_tokenizer.tokenize(desc)
    sm_ltks = rag_tokenizer.fine_grained_tokenize(ltks)

    return {
        "id": row_id,
        "compile_kwd": "lightgraph",
        "knowledge_graph_kwd": "entity",
        "entity_kwd": name,
        "entity_type_kwd": ent_type,
        "content_with_weight": json.dumps({"name": name, "type": ent_type}),
        "content_ltks": ltks,
        "content_sm_ltks": sm_ltks,
        "q_{}_vec".format(len(vec)): vec,
        "source_doc_ids": _ids_to_str([doc_id]),
        "source_chunk_ids": _ids_to_str([chunk_id]) if chunk_id else "",
        "doc_count_int": 1,
        "kb_id": kb_id,
        "available_int": 1,
    }


def _relation_to_es_doc(rel: dict, doc_id: str, chunk_id: str,
                        kb_id: str, vec: list) -> dict:
    frm = rel["from_entity"].strip().upper()
    to_ = rel["to_entity"].strip().upper()
    rel_type = rel.get("type", "related_to")
    weight = rel.get("weight", 1)
    row_id = _relation_row_id(frm, to_, rel_type, kb_id)
    desc = f"{frm} {rel_type} {to_}"
    ltks = rag_tokenizer.tokenize(desc)
    sm_ltks = rag_tokenizer.fine_grained_tokenize(ltks)

    return {
        "id": row_id,
        "compile_kwd": "lightgraph",
        "knowledge_graph_kwd": "relation",
        "from_entity_kwd": frm,
        "to_entity_kwd": to_,
        "entity_type_kwd": rel_type,
        "content_with_weight": json.dumps({"type": rel_type, "source": frm, "target": to_}),
        "content_ltks": ltks,
        "content_sm_ltks": sm_ltks,
        "q_{}_vec".format(len(vec)): vec,
        "weight_int": weight,
        "source_doc_ids": _ids_to_str([doc_id]),
        "source_chunk_ids": _ids_to_str([chunk_id]) if chunk_id else "",
        "kb_id": kb_id,
        "available_int": 1,
    }


# ── Entity-level incremental merge ──────────────────────────────────


async def _entity_level_merge(
    entity_docs: List[dict], tenant_id: str, kb_id: str, doc_id: str,
    progress_cb: Callable, cancel_check: Callable,
):
    """Entity-level incremental update.

    For each entity: lookup by stable row_id.  If exists, append
    source_doc_ids / source_chunk_ids and increment doc_count_int.
    Otherwise insert.

    Infinity stores source_doc_ids/source_chunk_ids as space-joined
    varchar; Elasticsearch stores them as native lists.  The helper
    functions ``_str_to_ids`` / ``_ids_to_str`` handle both.
    """
    index = search.index_name(tenant_id)
    inserted = updated = 0

    for edoc in entity_docs:
        if cancel_check():
            raise TaskCanceledException("LightGraph entity merge cancelled")
        row_id = edoc["id"]

        existing = await thread_pool_exec(
            settings.docStoreConn.get, row_id, index, [kb_id])
        if existing:
            existing_ids = _str_to_ids(existing.get("source_doc_ids"))
            existing_chunks = _str_to_ids(existing.get("source_chunk_ids"))
            incoming_ids = _str_to_ids(edoc.get("source_doc_ids"))
            incoming_chunks = _str_to_ids(edoc.get("source_chunk_ids"))

            merged_ids = list(dict.fromkeys(existing_ids + incoming_ids))
            merged_chunks = list(dict.fromkeys(existing_chunks + incoming_chunks))

            await thread_pool_exec(
                settings.docStoreConn.update,
                {"id": row_id},
                {
                    "source_doc_ids": _ids_to_str(merged_ids),
                    "source_chunk_ids": _ids_to_str(merged_chunks),
                    "doc_count_int": len(merged_ids),
                },
                index, kb_id,
            )
            updated += 1
        else:
            await thread_pool_exec(
                settings.docStoreConn.insert, [edoc], index, kb_id)
            inserted += 1

    if progress_cb:
        progress_cb(msg=f"LightGraph entity merge: {inserted} inserted, {updated} updated")


async def _relation_level_merge(
    rel_docs: List[dict], tenant_id: str, kb_id: str, doc_id: str,
    progress_cb: Callable, cancel_check: Callable,
):
    """Relation-level incremental update.

    For each relation: lookup by stable row_id.  If exists, increment
    weight_int and append source_doc_ids.  Otherwise insert.
    """
    index = search.index_name(tenant_id)
    inserted = updated = 0

    for rdoc in rel_docs:
        if cancel_check():
            raise TaskCanceledException("LightGraph relation merge cancelled")
        row_id = rdoc["id"]

        existing = await thread_pool_exec(
            settings.docStoreConn.get, row_id, index, [kb_id])
        if existing:
            existing_ids = _str_to_ids(existing.get("source_doc_ids"))
            existing_chunks = _str_to_ids(existing.get("source_chunk_ids"))
            incoming_ids = _str_to_ids(rdoc.get("source_doc_ids"))
            incoming_chunks = _str_to_ids(rdoc.get("source_chunk_ids"))
            old_weight = existing.get("weight_int", 1)

            merged_ids = list(dict.fromkeys(existing_ids + incoming_ids))
            merged_chunks = list(dict.fromkeys(existing_chunks + incoming_chunks))
            new_weight = old_weight + (1 if doc_id not in existing_ids else 0)

            await thread_pool_exec(
                settings.docStoreConn.update,
                {"id": row_id},
                {
                    "weight_int": new_weight,
                    "source_doc_ids": _ids_to_str(merged_ids),
                    "source_chunk_ids": _ids_to_str(merged_chunks),
                },
                index, kb_id,
            )
            updated += 1
        else:
            await thread_pool_exec(
                settings.docStoreConn.insert, [rdoc], index, kb_id)
            inserted += 1

    if progress_cb:
        progress_cb(msg=f"LightGraph relation merge: {inserted} inserted, {updated} updated")


# ── Embedding helper ────────────────────────────────────────────────


async def _encode_texts(texts: List[str], embd_mdl) -> List[List[float]]:
    if not texts:
        return []
    embeddings, _ = await thread_pool_exec(embd_mdl.encode, texts)
    vecs = []
    for emb in embeddings:
        if hasattr(emb, "tolist"):
            vecs.append(emb.tolist())
        elif isinstance(emb, list):
            vecs.append(emb)
        else:
            vecs.append(list(emb))
    return vecs


# ── Main entry point ────────────────────────────────────────────────


async def run_lightgraph_for_doc(handler, ctx, embedding_model) -> None:
    """Run LightGraph extraction + entity-level merge for a single document.

    Called from ``run_document_structure_compile()`` inside
    ``run_document_post_chunking_if_last()`` — the last-chunk-task
    gating is already done by the caller.

    Args:
        handler: TaskHandler instance (provides _load_chunks_for_doc)
        ctx: TaskContext with doc_id, kb_id, tenant_id, language, etc.
        embedding_model: LLMBundle for generating embeddings.
    """
    tenant_id, kb_id, doc_id = ctx.tenant_id, ctx.kb_id, ctx.doc_id
    language = ctx.language or "en"
    progress_cb = ctx.progress_cb
    cancel_check = lambda: ctx.has_canceled_func(ctx.id)

    progress_cb(msg="LightGraph: loading spaCy model ...")
    try:
        nlp = _load_model(language)
    except Exception as e:
        logging.error(f"LightGraph: failed to load spaCy model for {language}: {e}")
        progress_cb(msg=f"LightGraph: spaCy model unavailable for {language}, skipping")
        return

    progress_cb(msg="LightGraph: extracting entities and relations ...")

    # Collect all texts for batch embedding later
    all_entity_texts: List[str] = []
    all_entity_docs: List[dict] = []
    all_rel_docs: List[dict] = []

    batch_no = 0
    async for batch in handler._load_chunks_for_doc(
        tenant_id, kb_id, doc_id, batch_size=4,
    ):
        if cancel_check():
            raise TaskCanceledException(f"LightGraph task {ctx.id} cancelled")

        batch_no += 1
        texts = [c.get("content_with_weight", "") for c in batch if c.get("content_with_weight")]
        chunk_ids = [c.get("id", "") for c in batch if c.get("content_with_weight")]

        if not texts:
            continue

        # ── spaCy batch inference ────────────────────────────────────
        for spacy_doc in nlp.pipe(texts, batch_size=4):
            ci = len(all_entity_docs)  # which chunk in this batch
            if ci >= len(chunk_ids):
                break
            chunk_id = chunk_ids[ci]
            text = texts[ci]

            # Entity extraction
            entities = _extract_entities_merged(spacy_doc, language)
            if not entities:
                continue

            # Relation extraction
            relations = _extract_relations_merged(text, entities, spacy_doc, language)

            all_entity_texts.append("\n".join(e["text"] for e in entities))

            for ent in entities:
                ent["chunk_id"] = chunk_id
                all_entity_docs.append(ent)
            for rel in relations:
                rel["chunk_id"] = chunk_id
                all_rel_docs.append(rel)

        if batch_no % 10 == 0:
            progress_cb(msg=f"LightGraph: processed {batch_no} batches, "
                            f"{len(all_entity_docs)} entities, {len(all_rel_docs)} relations")

    if not all_entity_docs:
        progress_cb(msg="LightGraph: no entities found, skipping merge")
        return

    # ── Generate embeddings in bulk ──────────────────────────────────
    progress_cb(msg=f"LightGraph: generating embeddings for {len(all_entity_docs)} entities "
                    f"and {len(all_rel_docs)} relations ...")

    all_texts = all_entity_texts[:]
    for r in all_rel_docs:
        all_texts.append(f"{r['from_entity']} {r['type']} {r['to_entity']}")

    try:
        vecs = await _encode_texts(all_texts, embedding_model)
    except Exception as e:
        logging.error(f"LightGraph: embedding failed: {e}")
        progress_cb(msg=f"LightGraph: embedding failed, skipping")
        return

    # Build ES docs
    entity_docs = []
    entity_vecs = vecs[:len(all_entity_docs)]
    for ent, vec in zip(all_entity_docs, entity_vecs):
        entity_docs.append(_entity_to_es_doc(
            ent, doc_id, ent.pop("chunk_id", ""), kb_id, vec))

    rel_docs = []
    rel_vecs = vecs[len(all_entity_docs):]
    for rel, vec in zip(all_rel_docs, rel_vecs):
        rel_docs.append(_relation_to_es_doc(
            rel, doc_id, rel.pop("chunk_id", ""), kb_id, vec))

    # ── Entity-level merge ───────────────────────────────────────────
    progress_cb(msg=f"LightGraph: merging {len(entity_docs)} entities, {len(rel_docs)} relations ...")
    await _entity_level_merge(entity_docs, tenant_id, kb_id, doc_id,
                              progress_cb, cancel_check)
    await _relation_level_merge(rel_docs, tenant_id, kb_id, doc_id,
                                progress_cb, cancel_check)

    progress_cb(msg=f"LightGraph: done for doc {doc_id}")
