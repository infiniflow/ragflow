#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
import datetime
import json
import logging
from typing import Callable, Tuple

import xxhash

from common.exceptions import TaskCanceledException
from common.misc_utils import thread_pool_exec
from common.token_utils import num_tokens_from_string
from rag.prompts.generator import gen_json

from ._common import (
    build_chunk_batches as _build_chunk_batches,
    encode as _encode,
    find_vec_field as _find_vec_field,
    stable_row_id as _stable_row_id,
    tokenize_for_search as _tokenize_for_search,
    union_ordered as _union_ordered,
    run_chunked_pipeline as _run_chunked_pipeline,
)


_STRUCT_TYPES = ("list", "set", "hypergraph")


def _struct_normalize_kind(kind) -> str:
    if not isinstance(kind, str):
        return ""
    normalized = kind.strip().lower().replace("-", "_")
    if normalized in {"pageindex", "page_index", "knowledge_graph"}:
        return "timeline"
    return normalized


def _struct_localize(value, language: str = "en") -> str:
    """Render multilingual values to a single string (mirrors loader._localize_data)."""
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    if isinstance(value, list):
        return "\n".join(f"{i + 1}. {item}" for i, item in enumerate(value))
    if isinstance(value, dict):
        v = value.get(language)
        if v is None and language != "en":
            v = value.get("en")
        if isinstance(v, str):
            return v
        if isinstance(v, list):
            return "\n".join(f"{i + 1}. {item}" for i, item in enumerate(v))
    return ""


def _struct_get(cfg: dict, *keys, default=None):
    """Case-insensitive lookup against the first matching key."""
    if not isinstance(cfg, dict):
        return default
    for k in keys:
        if k in cfg:
            return cfg[k]
        kl = k.lower()
        for ck in cfg.keys():
            if isinstance(ck, str) and ck.lower() == kl:
                return cfg[ck]
    return default


def _struct_infer_type(parser_config: dict) -> str:
    explicit = _struct_get(parser_config, "compile_type")
    normalized_explicit = _struct_normalize_kind(explicit)
    if normalized_explicit in _STRUCT_TYPES:
        return normalized_explicit
    kind = _struct_get(parser_config, "kind")
    normalized_kind = _struct_normalize_kind(kind)
    if normalized_kind:
        return normalized_kind
    output = _struct_get(parser_config, "output", default={}) or {}
    if _struct_get(output, "entities") and _struct_get(output, "relations"):
        return "hypergraph"
    return "list"


def _struct_supported_type(parser_config: dict, autotype: str) -> bool:
    if autotype in _STRUCT_TYPES:
        return True
    kind = _struct_get(parser_config, "kind")
    return _struct_normalize_kind(kind) == autotype


def _struct_render_fields(fields: list, language: str) -> Tuple[str, str]:
    """Return (bulleted field descriptions, JSON skeleton for one item)."""
    lines = []
    skeleton_parts = []
    for f in fields or []:
        name = f.get("name", "")
        ftype = f.get("type", "str")
        desc = _struct_localize(f.get("description", ""), language)
        required = f.get("required")
        req_label = "optional" if required is False else "required"
        lines.append(f"- {name} ({ftype}, {req_label}): {desc}")
        if ftype == "list":
            placeholder = "[<string>, ...]"
        elif ftype == "int":
            placeholder = "<int>"
        elif ftype == "float":
            placeholder = "<float>"
        elif ftype == "bool":
            placeholder = "<true|false>"
        else:
            placeholder = "<string>"
        skeleton_parts.append(f'"{name}": {placeholder}')
    return "\n".join(lines), "{ " + ", ".join(skeleton_parts) + " }"


def _struct_render_type_fields(fields: list, language: str, *, kind: str) -> Tuple[str, str]:
    """Render the new compilation-template field shape.

    New templates define allowed item ``type`` values with descriptions/rules,
    rather than arbitrary output field names. The extraction output keeps a
    stable shape so downstream merge logic can compare concrete items instead
    of collapsing every item into the template type.
    """
    lines: list[str] = []
    type_values: list[str] = []
    for f in fields or []:
        if not isinstance(f, dict):
            continue
        typ = f.get("type")
        typ = typ.strip() if isinstance(typ, str) else ""
        if not typ:
            continue
        type_values.append(typ)
        lines.append(f"- type: {typ}")
        desc = _struct_localize(f.get("description"), language)
        rule = _struct_localize(f.get("rule"), language)
        if desc:
            lines.append(f"  description: {desc}")
        if rule:
            lines.append(f"  rule: {rule}")

    if not type_values:
        type_values.append("other")
        lines.append("- type: other")

    if kind == "relation":
        skeleton = '{ "type": "<one of: ' + "|".join(type_values) + '>", "source": "<known entity name>", "target": "<known entity name>", "description": "<evidence or relation description>" }'
    else:
        skeleton = '{ "type": "<one of: ' + "|".join(type_values) + '>", "name": "<exact extracted item text>", "description": "<evidence, definition, or detail from the source>" }'
    return "\n".join(lines), skeleton


def _struct_hypergraph_prompts(parser_config: dict, language: str = "en") -> Tuple[str, str]:
    autotype = _struct_infer_type(parser_config)
    guideline = _struct_get(parser_config, "guideline", default={}) or {}
    output = _struct_get(parser_config, "output", default={}) or {}
    options = _struct_get(parser_config, "options", default={}) or {}
    uses_template_shape = bool(_struct_get(parser_config, "entity") or _struct_get(parser_config, "relation"))

    target = _struct_localize(_struct_get(guideline, "target"), language)
    rules_e = _struct_localize(_struct_get(guideline, "rules_for_entities"), language)
    rules_r = _struct_localize(_struct_get(guideline, "rules_for_relations"), language)
    rules_t = _struct_localize(_struct_get(guideline, "rules_for_time"), language)
    global_rules = _struct_localize(_struct_get(parser_config, "global_rules"), language)

    observation_time = _struct_get(options, "observation_time") or datetime.date.today().isoformat()
    if rules_t and "{observation_time}" in rules_t:
        rules_t = rules_t.replace("{observation_time}", observation_time)

    entities_cfg = _struct_get(parser_config, "entity", default={}) or {} if uses_template_shape else _struct_get(output, "entities", default={}) or {}
    relations_cfg = _struct_get(parser_config, "relation", default={}) or {} if uses_template_shape else _struct_get(output, "relations", default={}) or {}
    ent_desc = _struct_localize(_struct_get(entities_cfg, "description"), language)
    rel_desc = _struct_localize(_struct_get(relations_cfg, "description"), language)
    ent_fields = _struct_get(entities_cfg, "fields", default=[]) or []
    rel_fields = _struct_get(relations_cfg, "fields", default=[]) or []
    if uses_template_shape:
        ent_fields_text, ent_skel = _struct_render_type_fields(ent_fields, language, kind="entity")
        rel_fields_text, rel_skel = _struct_render_type_fields(rel_fields, language, kind="relation")
    else:
        ent_fields_text, ent_skel = _struct_render_fields(ent_fields, language)
        rel_fields_text, rel_skel = _struct_render_fields(rel_fields, language)

    node_parts = [f"# Role and Task:\n{target}"] if target else []
    if global_rules:
        node_parts.append(f"## Global Rules:\n{global_rules}")
    if rules_e:
        node_parts.append(f"## Entity Extraction Rules:\n{rules_e}")
    if ent_desc:
        node_parts.append(f"## Entity Description:\n{ent_desc}")
    node_parts.append(f"## Entity Fields:\n{ent_fields_text}")
    node_parts.append(
        "## Response Format:\n"
        "Reply with a single JSON object of the form: "
        f'{{"items": [{ent_skel}, ...]}}.\n'
        f'Auto-type: "{_struct_infer_type(parser_config)}". ' + ("Items must be unique. " if autotype == "set" else "") + "Return JSON only, no commentary."
    )
    node_prompt = "\n\n".join(node_parts)

    if not relations_cfg:
        return node_prompt, ""

    edge_parts = [f"# Role and Task:\n{target}"] if target else []
    if global_rules:
        edge_parts.append(f"## Global Rules:\n{global_rules}")
    if rules_r:
        edge_parts.append(f"## Relation Extraction Rules:\n{rules_r}")
    if rules_t:
        edge_parts.append(f"## Time Rules:\n{rules_t}")
    if rel_desc:
        edge_parts.append(f"## Relation Description:\n{rel_desc}")
    edge_parts.append(f"## Relation Fields:\n{rel_fields_text}")
    edge_parts.append("## Known Entities:\n{known_nodes}")
    edge_parts.append(
        "## Response Format:\n"
        "Reply with a single JSON object of the form: "
        f'{{"items": [{rel_skel}, ...]}}.\n'
        "Only create relations between entities listed in 'Known Entities'. "
        "Return JSON only, no commentary."
    )
    edge_prompt = "\n\n".join(edge_parts)

    return node_prompt, edge_prompt


def _struct_entity_id_field(parser_config: dict) -> str:
    if _struct_get(parser_config, "entity"):
        return "name"
    identifiers = _struct_get(parser_config, "identifiers", default={}) or {}
    entity_id = _struct_get(identifiers, "entity_id")
    if isinstance(entity_id, str) and "{" not in entity_id and entity_id.strip():
        return entity_id.strip()
    entities_cfg = _struct_get(_struct_get(parser_config, "output", default={}) or {}, "entities", default={}) or {}
    for f in _struct_get(entities_cfg, "fields", default=[]) or []:
        if f.get("required") is not False:
            return f.get("name", "name")
    return "name"


def _struct_unwrap_items(res) -> list:
    if res is None:
        return []
    if isinstance(res, dict):
        items = res.get("items")
        if isinstance(items, list):
            return [it for it in items if isinstance(it, dict)]
        return []
    if isinstance(res, list):
        return [it for it in res if isinstance(it, dict)]
    return []


async def _struct_extract_hypergraph(text: str, parser_config: dict, chat_mdl, language: str) -> Tuple[list[dict], list[dict]]:
    node_prompt, edge_prompt_template = _struct_hypergraph_prompts(parser_config, language)

    user_prompt = f"## Source Text:\n{text}\n\n## Output (JSON only):"
    node_res = await gen_json(node_prompt, user_prompt, chat_mdl, gen_conf={"temperature": 0.1})
    nodes = _struct_unwrap_items(node_res)

    id_field = _struct_entity_id_field(parser_config)
    known_keys = []
    for n in nodes:
        v = n.get(id_field)
        if v is None:
            continue
        v_str = str(v).strip()
        if v_str and v_str not in known_keys:
            known_keys.append(v_str)
    known_str = "- " + "\n- ".join(known_keys) if known_keys else "(none)"

    if not edge_prompt_template:
        return nodes, []

    edge_prompt = edge_prompt_template.replace("{known_nodes}", known_str)
    edge_res = await gen_json(edge_prompt, user_prompt, chat_mdl, gen_conf={"temperature": 0.1})
    edges = _struct_unwrap_items(edge_res)

    return nodes, edges


# Backwards-compat alias for the shared helper. New code should use
# ``_common.encode`` directly; kept here so existing references inside this
# module keep working without a wider rename.
_struct_embed = _encode


def _struct_payload_description(payload: dict) -> str:
    """Concat string values of every non-description field (lists flattened)."""
    parts: list[str] = []
    for k, v in payload.items():
        if isinstance(v, (list, tuple)):
            for item in v:
                if item is None:
                    continue
                s = str(item).strip()
                if s:
                    parts.append(s)
        else:
            s = str(v).strip()
            if s:
                parts.append(s)
    return " ".join(parts)


def _struct_load_payload(doc: dict) -> dict:
    try:
        payload = json.loads(doc.get("content_with_weight") or "{}")
    except Exception:
        return {}
    return payload if isinstance(payload, dict) else {}


def _struct_graph_entity(payload: dict, source_chunk_ids: list | None = None) -> dict | None:
    name = payload.get("name") or payload.get("text") or payload.get("term") or payload.get("title")
    name = str(name).strip() if name is not None else ""
    if not name:
        return None
    typ = payload.get("type") or "other"
    typ = str(typ).strip() if typ is not None else "other"
    aliases = payload.get("aliases")
    if isinstance(aliases, str):
        aliases = [aliases]
    if not isinstance(aliases, list):
        aliases = []
    aliases = [str(a).strip() for a in aliases if str(a).strip()]
    description = payload.get("description") or payload.get("discription") or payload.get("definition_excerpt") or ""
    if isinstance(source_chunk_ids, str):
        source_chunk_ids = [source_chunk_ids]
    source_chunk_ids = _struct_union_chunk_ids(source_chunk_ids)
    return {
        "aliases": aliases,
        "mention_count": 1,
        "name": name,
        "source_chunk_ids": source_chunk_ids,
        "type": typ or "other",
        "discription": str(description).strip() if description is not None else "",
    }


def _struct_graph_relation(payload: dict) -> dict | None:
    src = payload.get("source") or payload.get("src") or payload.get("from")
    tgt = payload.get("target") or payload.get("tgt") or payload.get("to")
    src = str(src).strip() if src is not None else ""
    tgt = str(tgt).strip() if tgt is not None else ""
    if not src or not tgt:
        return None
    typ = payload.get("type") or "related"
    return {
        "from": src,
        "to": tgt,
        "type": str(typ).strip() if typ is not None else "related",
    }


def _struct_merge_graph_entities(entities: list[dict]) -> list[dict]:
    merged: dict[tuple[str, str], dict] = {}
    order: list[tuple[str, str]] = []
    for entity in entities:
        key = (entity["name"], entity.get("type") or "other")
        if key not in merged:
            merged[key] = entity
            order.append(key)
            continue
        target = merged[key]
        target["mention_count"] = int(target.get("mention_count") or 0) + int(entity.get("mention_count") or 1)
        aliases = target.setdefault("aliases", [])
        for alias in entity.get("aliases") or []:
            if alias not in aliases:
                aliases.append(alias)
        if not target.get("discription") and entity.get("discription"):
            target["discription"] = entity["discription"]
        target["source_chunk_ids"] = _struct_union_chunk_ids(
            target.get("source_chunk_ids"),
            entity.get("source_chunk_ids"),
        )
    return [merged[key] for key in order]


def _struct_relation_member_fields(parser_config: dict) -> Tuple:
    """Return (source_field, target_field) for relation docs, or (None, None).

    Looks at ``identifiers.relation_members`` first (dict form for graph-style
    configs, e.g. ``{source: source, target: target}``); falls back to the
    conventional ``source`` / ``target`` field names if both appear in the
    relation schema.
    """
    identifiers = _struct_get(parser_config, "identifiers", default={}) or {}
    members = _struct_get(identifiers, "relation_members")
    if isinstance(members, dict):
        src = members.get("source") or members.get("src")
        tgt = members.get("target") or members.get("tgt")
        if src or tgt:
            return src, tgt

    if _struct_get(parser_config, "relation"):
        return "source", "target"

    relations_cfg = (
        _struct_get(
            _struct_get(parser_config, "output", default={}) or {},
            "relations",
            default={},
        )
        or {}
    )
    field_names = {f.get("name") for f in (_struct_get(relations_cfg, "fields", default=[]) or []) if isinstance(f, dict)}
    if "source" in field_names and "target" in field_names:
        return "source", "target"
    return None, None


def _struct_to_es_doc(
    payload: dict,
    compile_kwd: str,
    doc_id: str,
    chunk_ids: list[str],
    vec,
    kind: str,
    src_field: str | None = None,
    target_field: str | None = None,
    compilation_template_id: str | None = None,
    compilation_template_kind: str | None = None,
) -> dict:
    """Build one ES doc for an extracted entity or relation.

    Args:
        kind: ``"entity"`` or ``"relation"`` — written to ``knowledge_graph_kwd``.
        src_field / target_field: when ``kind == "relation"`` and these field
            names exist on the payload, the resolved values are written to
            ``from_entity_kwd`` / ``to_entity_kwd``.
        compilation_template_id / compilation_template_kind: stamped onto
            every row so the document-structure endpoint can group by
            template id and the UI can render one tab per template. The
            id is stored as a single-element list under
            ``compilation_template_ids`` because the same logical entity
            *could* later be claimed by multiple templates during a
            cross-template merge (rare, but the schema is forward-compat).
    """
    content_with_weight = json.dumps(payload, ensure_ascii=False)
    if hasattr(vec, "tolist"):
        vec_list = vec.tolist()
    else:
        vec_list = list(vec)
    doc_id_str = str(doc_id)
    template_id_str = str(compilation_template_id).strip() if compilation_template_id else ""

    description = _struct_payload_description(payload)
    content_ltks, content_sm_ltks = _tokenize_for_search(description)

    # Mix the template id into the stable row id so two templates with the
    # same compile_kwd don't collide on identical payloads (e.g. two
    # different list-kind templates that each extract "headline X").
    row_seed_extras = [template_id_str] if template_id_str else []
    row_id = _stable_row_id(content_with_weight, doc_id_str, *row_seed_extras)

    doc = {
        "content_with_weight": content_with_weight,
        "compile_kwd": compile_kwd,
        "knowledge_graph_kwd": kind,
        "doc_id": doc_id_str,
        "source_chunk_ids": list(chunk_ids or []),
        "content_ltks": content_ltks,
        "content_sm_ltks": content_sm_ltks,
        f"q_{len(vec_list)}_vec": vec_list,
        "id": row_id,
    }
    if template_id_str:
        doc["compilation_template_ids"] = [template_id_str]
    if compilation_template_kind:
        doc["compilation_template_kind_kwd"] = str(compilation_template_kind)

    if kind == "relation":
        if src_field:
            src_val = payload.get(src_field)
            if src_val is not None and str(src_val).strip():
                doc["from_entity_kwd"] = str(src_val).strip()
        if target_field:
            tgt_val = payload.get(target_field)
            if tgt_val is not None and str(tgt_val).strip():
                doc["to_entity_kwd"] = str(tgt_val).strip()

    return doc


async def _struct_process_batch(
    packed: list[dict],
    batch_idx: int,
    total: int,
    autotype: str,
    parser_config: dict,
    chat_mdl,
    embd_mdl,
    doc_id: str,
    language: str,
    callback,
    semaphore,
    compilation_template_id: str | None = None,
    compilation_template_kind: str | None = None,
) -> list[dict]:
    """Process one packed batch end-to-end (extract → embed → ES docs).

    ``packed`` is the per-batch shape produced by
    ``_common.build_chunk_batches``: ``[{label, chunk_id, text}, ...]``.
    The ``label`` field is unused here — structure uses ``---`` separators
    instead of per-chunk labels — but ``chunk_id`` is collected so every
    item produced by this batch carries the batch's source chunk ids.

    The semaphore (if any) is taken around the entire batch's LLM +
    embedding work to bound peak concurrency.
    """
    if not packed:
        return []

    batch_ids: list = [e["chunk_id"] for e in packed if e.get("chunk_id")]
    batch_segments: list[str] = [e["text"] for e in packed if isinstance(e.get("text"), str)]
    combined_text = "\n\n---\n\n".join(batch_segments)

    src_field, target_field = _struct_relation_member_fields(parser_config)

    async def _run() -> list[dict]:
        # For hypergraph, entity extraction MUST complete before edge extraction
        # within the same batch, because the edge prompt's {known_nodes}
        # placeholder is filled from this batch's extracted nodes — see
        # _struct_extract_hypergraph. Parallelism across batches is fine; the
        # two stages within one batch are strictly sequential.
        try:
            items, relations = await _struct_extract_hypergraph(combined_text, parser_config, chat_mdl, language)
        except Exception as e:
            logging.exception(f"compile_structure_from_text: extraction failed for batch {batch_idx}: {e}")
            return []

        payloads = items + relations
        kinds = ["entity"] * len(items) + ["relation"] * len(relations)
        if not payloads:
            if callback:
                callback((batch_idx + 1) / total, f"{batch_idx + 1}/{total} batches: 0 items")
            return []

        embed_inputs = [_struct_payload_description(p) for p in payloads]
        try:
            embeddings = await _struct_embed(embd_mdl, embed_inputs)
        except Exception as e:
            logging.exception(f"compile_structure_from_text: embedding failed for batch {batch_idx}: {e}")
            return []

        if len(embeddings) != len(payloads):
            logging.error(f"compile_structure_from_text: embedding count mismatch ({len(embeddings)} vs {len(payloads)}) for batch {batch_idx}")
            return []

        docs = [
            _struct_to_es_doc(
                payload,
                autotype,
                doc_id,
                batch_ids,
                vec,
                kind,
                src_field=src_field,
                target_field=target_field,
                compilation_template_id=compilation_template_id,
                compilation_template_kind=compilation_template_kind,
            )
            for payload, vec, kind in zip(payloads, embeddings, kinds)
        ]

        if callback:
            callback((batch_idx + 1) / total, f"{batch_idx + 1}/{total} batches: {len(payloads)} items")

        return docs

    if semaphore is not None:
        async with semaphore:
            return await _run()
    return await _run()


async def compile_structure_from_text(
    chunks: list[dict],
    parser_config,
    chat_mdl,
    embd_mdl,
    doc_id: str,
    language: str = "en",
    callback=None,
    max_workers: int = 10,
    compilation_template_id: str | None = None,
) -> list[dict]:
    """Extract list/set/hypergraph structures from text chunks and prepare ES docs.

    Each chunk is processed independently — cross-chunk merging of entities and
    relations is deferred to a separate pipeline stage and is intentionally not
    performed here.

    Args:
        chunks: list of dicts; each must expose ``id`` and ``text`` (a
            ``content_with_weight`` fallback is also accepted).
        parser_config: dict already parsed from ``document.parser_config["knowledge_compilation"]`` or
            the raw JSON string from the database.
        chat_mdl: LLMBundle for chat (used via ``gen_json``).
        embd_mdl: LLMBundle for embeddings (used via ``encode``).
        doc_id: source document id, embedded into every ES doc.
        language: language code for resolving multilingual config strings.
        callback: optional progress callback ``(prog: float, msg: str)``.

    Returns:
        List of ES-ready dicts shaped as::

            {
                "content_with_weight": <json>,
                "compile_kwd": "list" | "set" | "hypergraph",
                "doc_id": <doc_id>,
                "source_chunk_ids": [<chunk_id>, ...],
                "q_<dim>_vec": [...],
                "id": <xxhash>,
            }
    """
    if isinstance(parser_config, str):
        try:
            parser_config = json.loads(parser_config)
        except Exception as e:
            logging.exception(f"compile_structure_from_text: invalid parser_config JSON: {e}")
            return []
    if not isinstance(parser_config, dict):
        logging.error("compile_structure_from_text: parser_config must be a dict or JSON string")
        return []

    autotype = _struct_infer_type(parser_config)
    if not _struct_supported_type(parser_config, autotype):
        logging.error(f"compile_structure_from_text: unsupported type '{autotype}'")
        return []

    node_prompt, edge_prompt = _struct_hypergraph_prompts(parser_config, language)
    prompt_overhead = max(num_tokens_from_string(node_prompt), num_tokens_from_string(edge_prompt))

    # ``kind`` for the row stamp follows the template's ``kind`` field if
    # present (e.g. "timeline", "page_index"); we fall back to the
    # inferred autotype ("list" / "set" / "hypergraph") so legacy
    # configs without a kind still get a sensible label on the UI tab.
    template_kind = parser_config.get("kind") if isinstance(parser_config, dict) else None
    if not isinstance(template_kind, str) or not template_kind.strip():
        template_kind = autotype

    packed_batches, _info = _build_chunk_batches(
        chunks,
        chat_mdl,
        prompt_overhead_tokens=prompt_overhead,
    )
    if not packed_batches:
        return []

    async def _process_one(batch: list[dict], bi: int, total: int) -> list[dict]:
        # The engine's semaphore already bounds concurrency.
        return await _struct_process_batch(
            packed=batch,
            batch_idx=bi,
            total=total,
            autotype=autotype,
            parser_config=parser_config,
            chat_mdl=chat_mdl,
            embd_mdl=embd_mdl,
            doc_id=doc_id,
            language=language,
            callback=callback,
            semaphore=None,
            compilation_template_id=compilation_template_id,
            compilation_template_kind=template_kind,
        )

    def _flatten(per_batch: list) -> list[dict]:
        out: list[dict] = []
        for br in per_batch or []:
            if br:
                out.extend(br)
        return out

    return await _run_chunked_pipeline(
        packed_batches,
        process_batch=_process_one,
        aggregate=_flatten,
        max_workers=max_workers,
        callback=callback,
        log_prefix="compile_structure",
    )


# ---------------------------------------------------------------------------
# Structured-knowledge merging: local dedup + ES dedup
# ---------------------------------------------------------------------------
#
# Pipeline (per spec):
#   Phase 1 — Local dedup inside `docs`:
#     - Group by (doc_id, compile_kwd, from_entity_kwd?, to_entity_kwd?).
#     - Within each group, compute pairwise cosine similarity (sklearn) over
#       q_<dim>_vec, and for each pair above ``similarity_threshold`` (0.9 by
#       default) ask the LLM via _struct_merge_pair to decide if they're the
#       same logical item; if yes, collapse in memory (union chunk_ids,
#       regenerate vector + tokens off the merged payload).
#   Phase 2 — ES dedup of the surviving docs:
#     - For each, KNN-search ES with the same filter via MatchDenseExpr; if a
#       top-1 hit comes back above ``similarity_threshold`` and the LLM judges
#       it a duplicate, REPLACE the existing ES doc by its old ``id``
#       (preserving src/target on relations and unioning chunk_ids). Else
#       insert the new doc.
#
# Merge is driven by the user-supplied prompts; a small decision instruction
# is appended so we can branch on the LLM's verdict via gen_json.

MERGE_SYSTEM_PROMPT = """You are an intelligent data merging assistant.
You will merge two JSON objects representing the same entity: Item A (existing) and Item B (incoming).

Merge strategy:
1. Combine information from both items.
2. If fields conflict, use your best judgment to pick the more detailed or recent-looking value.
3. If one item has a null/missing value and the other has data, keep the data.
4. For list fields, combine unique elements from both.
5. Do not invent new information not present in the inputs.
6. Return the result in the exact JSON format of the input items."""

MERGE_USER_PROMPT = """Item A (existing):\n{item_existing}\n\nItem B (incoming):\n{item_incoming}"""

MERGE_DECISION_INSTRUCTION = """First decide whether Item A and Item B refer to the same logical entity (for entities) or the same logical relation (for relations). Use the merge strategy above only if they are the same.

Return ONLY a JSON object with this exact structure (no markdown fences, no commentary):
{
  "duplicated": <true | false>,
  "merged": <merged JSON object using the same keys as the inputs when duplicated=true; otherwise null>
}"""


def _struct_doc_template_id(doc: dict) -> str | None:
    """Pull the (single) compilation_template_id out of an ES row.

    Stored as a list to leave room for future cross-template merges; this
    helper just returns the first non-empty entry, or None.
    """
    raw = doc.get("compilation_template_ids")
    if isinstance(raw, list):
        for v in raw:
            if isinstance(v, str) and v.strip():
                return v.strip()
    if isinstance(raw, str) and raw.strip():
        return raw.strip()
    return None


def _struct_filter_key(doc: dict) -> tuple:
    """Bucket key for dedup candidates. Includes the template id so two
    templates that emit a relation with the same (from, to) endpoints
    don't merge across template boundaries."""
    return (
        doc.get("doc_id"),
        doc.get("compile_kwd"),
        doc.get("from_entity_kwd"),
        doc.get("to_entity_kwd"),
        _struct_doc_template_id(doc),
    )


# Backwards-compat aliases for the shared helpers. New code should call
# the ``_common`` versions directly.
_struct_doc_vec = _find_vec_field


def _struct_union_chunk_ids(*chunk_id_lists) -> list:
    """Order-preserving union (compat shim — prefer ``_common.union_ordered``)."""
    normalized = [[chunk_ids] if isinstance(chunk_ids, str) else chunk_ids for chunk_ids in chunk_id_lists]
    return _union_ordered(*normalized)


async def _struct_merge_pair(existing: dict, incoming: dict, chat_mdl) -> dict | None:
    """LLM-judged merge. Returns merged payload dict if duplicate, else None.

    Operates on the payload (parsed ``content_with_weight``), not the ES doc
    envelope. Caller is responsible for re-embedding and rebuilding the doc.
    """
    try:
        existing_payload = json.loads(existing.get("content_with_weight") or "{}")
        incoming_payload = json.loads(incoming.get("content_with_weight") or "{}")
    except Exception:
        logging.exception("merge: failed to parse content_with_weight")
        return None
    if not isinstance(existing_payload, dict) or not isinstance(incoming_payload, dict):
        return None

    user_prompt = MERGE_USER_PROMPT.format(
        item_existing=json.dumps(existing_payload, ensure_ascii=False),
        item_incoming=json.dumps(incoming_payload, ensure_ascii=False),
    )
    system_prompt = MERGE_SYSTEM_PROMPT + "\n\n" + MERGE_DECISION_INSTRUCTION
    res = await gen_json(system_prompt, user_prompt, chat_mdl, gen_conf={"temperature": 0.0})
    if not isinstance(res, dict):
        return None
    if not res.get("duplicated"):
        return None
    merged = res.get("merged")
    if not isinstance(merged, dict):
        return None
    return merged


def _struct_apply_merge_invariants(existing: dict, merged_payload: dict) -> dict:
    """For relations, force the source/target fields back to the existing payload's
    values — from_entity_kwd / to_entity_kwd must not change across a merge.
    """
    if existing.get("knowledge_graph_kwd") != "relation":
        return merged_payload
    try:
        existing_payload = json.loads(existing.get("content_with_weight") or "{}")
    except Exception:
        return merged_payload
    if not isinstance(existing_payload, dict):
        return merged_payload
    for field in ("source", "src", "from"):
        if field in existing_payload:
            merged_payload[field] = existing_payload[field]
    for field in ("target", "tgt", "to"):
        if field in existing_payload:
            merged_payload[field] = existing_payload[field]
    return merged_payload


def _struct_rebuild_es_doc(
    payload: dict,
    base_doc: dict,
    vec,
    chunk_ids: list,
    preserve_id: bool = True,
) -> dict:
    """Rebuild an ES doc from a merged payload using _struct_to_es_doc, then
    overlay identity fields (id, from_entity_kwd, to_entity_kwd) from base_doc.
    """
    kind = base_doc.get("knowledge_graph_kwd") or "entity"
    src_field = None
    target_field = None
    if kind == "relation":
        try:
            existing_payload = json.loads(base_doc.get("content_with_weight") or "{}")
            if isinstance(existing_payload, dict):
                if "source" in existing_payload and "target" in existing_payload:
                    src_field, target_field = "source", "target"
        except Exception:
            pass

    new_doc = _struct_to_es_doc(
        payload=payload,
        compile_kwd=base_doc.get("compile_kwd"),
        doc_id=base_doc.get("doc_id"),
        chunk_ids=chunk_ids,
        vec=vec,
        kind=kind,
        src_field=src_field,
        target_field=target_field,
    )
    if preserve_id and base_doc.get("id"):
        new_doc["id"] = base_doc["id"]
    # The spec forbids changing from_entity_kwd / to_entity_kwd on a merge.
    for kwd in ("from_entity_kwd", "to_entity_kwd"):
        if kwd in base_doc and base_doc[kwd]:
            new_doc[kwd] = base_doc[kwd]
    return new_doc


async def _struct_reembed_payload(payload: dict, embd_mdl):
    """Re-encode a merged payload's description with embd_mdl and return the vector."""
    text = _struct_payload_description(payload)
    vecs = await _struct_embed(embd_mdl, [text])
    return vecs[0] if vecs else None


async def _struct_local_dedup(
    docs: list[dict],
    chat_mdl,
    embd_mdl,
    similarity_threshold: float,
) -> tuple[list[dict], int]:
    """Single-pass dedup inside ``docs``. Returns (deduped, dropped_count)."""
    from sklearn.metrics.pairwise import cosine_similarity

    groups: dict = {}
    order: list = []
    for doc in docs:
        key = _struct_filter_key(doc)
        if key not in groups:
            groups[key] = []
            order.append(key)
        groups[key].append(doc)

    dropped = 0
    deduped: list[dict] = []

    for key in order:
        kept: list[dict] = []
        for incoming in groups[key]:
            inc_field, inc_vec = _struct_doc_vec(incoming)
            if not inc_vec or not kept:
                kept.append(incoming)
                continue
            kept_with_vecs = []
            for kd in kept:
                _, kv = _struct_doc_vec(kd)
                if kv is not None:
                    kept_with_vecs.append((kd, kv))
            if not kept_with_vecs:
                kept.append(incoming)
                continue
            sims = cosine_similarity([list(inc_vec)], [list(v) for _, v in kept_with_vecs])[0]
            sims_list = sims.tolist() if hasattr(sims, "tolist") else list(sims)
            best_idx = max(range(len(sims_list)), key=lambda i: sims_list[i])
            if sims_list[best_idx] < similarity_threshold:
                kept.append(incoming)
                continue
            existing = kept_with_vecs[best_idx][0]
            merged_payload = await _struct_merge_pair(existing, incoming, chat_mdl)
            if merged_payload is None:
                kept.append(incoming)
                continue
            merged_payload = _struct_apply_merge_invariants(existing, merged_payload)
            merged_chunk_ids = _struct_union_chunk_ids(
                existing.get("source_chunk_ids"),
                incoming.get("source_chunk_ids"),
            )
            new_vec = await _struct_reembed_payload(merged_payload, embd_mdl)
            if new_vec is None:
                # Re-embed failed: keep existing, drop incoming silently.
                dropped += 1
                continue
            rebuilt = _struct_rebuild_es_doc(
                merged_payload,
                existing,
                new_vec,
                merged_chunk_ids,
                preserve_id=True,
            )
            # Replace the kept entry that matched.
            for i, kd in enumerate(kept):
                if kd is existing:
                    kept[i] = rebuilt
                    break
            dropped += 1
        deduped.extend(kept)

    return deduped, dropped


async def _struct_es_dedup_one(
    doc: dict,
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    similarity_threshold: float,
) -> str:
    """Persist a single doc into ES with merge-or-insert semantics.

    Returns one of: 'inserted', 'updated', 'skipped'.
    """
    from common import settings
    from rag.nlp import search as _rag_search
    from common.doc_store.doc_store_base import MatchDenseExpr, OrderByExpr

    index = _rag_search.index_name(tenant_id)

    condition = {
        "compile_kwd": [doc["compile_kwd"]],
        "doc_id": [doc["doc_id"]],
    }
    if doc.get("knowledge_graph_kwd"):
        condition["knowledge_graph_kwd"] = [doc["knowledge_graph_kwd"]]
    if doc.get("from_entity_kwd"):
        condition["from_entity_kwd"] = [doc["from_entity_kwd"]]
    if doc.get("to_entity_kwd"):
        condition["to_entity_kwd"] = [doc["to_entity_kwd"]]
    # KNN dedup must stay within the same template — two templates can
    # produce identical-looking entities (e.g. two "list" kinds extracting
    # the same headline) but they live on independent tabs in the UI.
    incoming_template = _struct_doc_template_id(doc)
    if incoming_template:
        condition["compilation_template_ids"] = [incoming_template]

    vec_field, vec = _struct_doc_vec(doc)
    if not vec_field or vec is None:
        await thread_pool_exec(settings.docStoreConn.insert, [doc], index, kb_id)
        return "inserted"

    match_expr = MatchDenseExpr(
        vector_column_name=vec_field,
        embedding_data=list(vec),
        embedding_data_type="float",
        distance_type="cosine",
        topn=1,
        extra_options={"similarity": similarity_threshold},
    )
    select_fields = [
        "id",
        "content_with_weight",
        "source_chunk_ids",
        "knowledge_graph_kwd",
        "compile_kwd",
        "doc_id",
        "from_entity_kwd",
        "to_entity_kwd",
    ]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            select_fields,
            [],
            condition,
            [match_expr],
            OrderByExpr(),
            0,
            1,
            index,
            [kb_id],
        )
        field_map = settings.docStoreConn.get_fields(res, select_fields)
    except Exception:
        logging.exception("merge_compiled_structures: ES KNN search failed; inserting as new")
        await thread_pool_exec(settings.docStoreConn.insert, [doc], index, kb_id)
        return "inserted"

    if not field_map:
        await thread_pool_exec(settings.docStoreConn.insert, [doc], index, kb_id)
        return "inserted"

    old_id, old_doc = next(iter(field_map.items()))
    old_doc = dict(old_doc)
    old_doc.setdefault("id", old_id)

    merged_payload = await _struct_merge_pair(old_doc, doc, chat_mdl)
    if merged_payload is None:
        await thread_pool_exec(settings.docStoreConn.insert, [doc], index, kb_id)
        return "inserted"

    merged_payload = _struct_apply_merge_invariants(old_doc, merged_payload)
    merged_chunk_ids = _struct_union_chunk_ids(
        old_doc.get("source_chunk_ids"),
        doc.get("source_chunk_ids"),
    )
    new_vec = await _struct_reembed_payload(merged_payload, embd_mdl)
    if new_vec is None:
        return "skipped"

    rebuilt = _struct_rebuild_es_doc(
        merged_payload,
        old_doc,
        new_vec,
        merged_chunk_ids,
        preserve_id=True,
    )
    update_fields = {k: v for k, v in rebuilt.items() if k != "id"}
    try:
        await thread_pool_exec(
            settings.docStoreConn.update,
            {"id": old_id},
            update_fields,
            index,
            kb_id,
        )
        return "updated"
    except Exception:
        logging.exception("merge_compiled_structures: ES update failed for id %s", old_id)
        return "skipped"


def _struct_graph_row_id(
    doc_id: str,
    compile_kwd: str,
    compilation_template_id: str | None = None,
) -> str:
    """Stable id per (doc, compile_kwd, template). Without the template
    suffix, two templates sharing a compile_kwd (e.g. both ``list``)
    would overwrite each other's per-doc graph JSON row."""
    tpl_part = compilation_template_id or ""
    return xxhash.xxh64(
        f"{doc_id}:structure_graph:{compile_kwd}:{tpl_part}".encode(
            "utf-8",
            "surrogatepass",
        ),
    ).hexdigest()


async def _struct_rebuild_graph_json(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    compile_kwd: str,
    compilation_template_id: str | None = None,
) -> dict:
    from common import settings
    from rag.nlp import search as _rag_search
    from common.doc_store.doc_store_base import OrderByExpr

    index = _rag_search.index_name(tenant_id)
    fields = ["content_with_weight", "knowledge_graph_kwd", "source_chunk_ids"]
    condition: dict = {
        "doc_id": [doc_id],
        "compile_kwd": [compile_kwd],
        "knowledge_graph_kwd": ["entity", "relation"],
    }
    if compilation_template_id:
        condition["compilation_template_ids"] = [compilation_template_id]
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        fields,
        [],
        condition,
        [],
        OrderByExpr(),
        0,
        10000,
        index,
        [kb_id],
    )
    rows = settings.docStoreConn.get_fields(res, fields)
    entities: list[dict] = []
    relations: list[dict] = []
    for row in rows.values():
        payload = _struct_load_payload(row)
        if row.get("knowledge_graph_kwd") == "relation":
            relation = _struct_graph_relation(payload)
            if relation:
                relations.append(relation)
        else:
            entity = _struct_graph_entity(payload, row.get("source_chunk_ids"))
            if entity:
                entities.append(entity)

    return {
        "entities": _struct_merge_graph_entities(entities),
        "relations": relations,
    }


async def _struct_upsert_graph_json(
    graph: dict,
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    compile_kwd: str,
    compilation_template_id: str | None = None,
) -> None:
    from common import settings
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    row_id = _struct_graph_row_id(doc_id, compile_kwd, compilation_template_id)
    row = {
        "id": row_id,
        "content_with_weight": json.dumps(graph, ensure_ascii=False),
        "compile_kwd": compile_kwd,
        "knowledge_graph_kwd": "graph",
        "doc_id": doc_id,
        "kb_id": kb_id,
        "available_int": 0,
    }
    if compilation_template_id:
        row["compilation_template_ids"] = [compilation_template_id]
    old = await thread_pool_exec(settings.docStoreConn.get, row_id, index, [kb_id])
    if old:
        await thread_pool_exec(
            settings.docStoreConn.update,
            {"id": row_id},
            {k: v for k, v in row.items() if k != "id"},
            index,
            kb_id,
        )
    else:
        await thread_pool_exec(settings.docStoreConn.insert, [row], index, kb_id)


async def rebuild_structure_graph_json(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    compile_kwd: str,
    compilation_template_id: str | None = None,
) -> dict:
    """Rebuild and persist the compact document-scoped structure graph,
    scoped to one (doc, compile_kwd, template_id) triple."""
    graph = await _struct_rebuild_graph_json(
        tenant_id,
        kb_id,
        doc_id,
        compile_kwd,
        compilation_template_id,
    )
    await _struct_upsert_graph_json(
        graph,
        tenant_id,
        kb_id,
        doc_id,
        compile_kwd,
        compilation_template_id,
    )
    return graph


# ---------------------------------------------------------------------------
# Chain-shape validation for ``list`` / ``timeline`` kinds.
#
# Both kinds model a strict linear chain of entities (one predecessor,
# one successor, no cycles). The per-chunk extractor is happy to emit
# branches / cycles when the source text supports multiple readings, so
# we validate the relation set post-extraction and ask the LLM to pick
# the correct chain out of the offenders. On any failure (timeout,
# exception, malformed LLM output) the validator returns the input
# untouched — correction is best-effort.
# ---------------------------------------------------------------------------

# Kinds whose relations must form a strict linear chain.
CHAIN_KINDS: tuple[str, ...] = ("list", "timeline")

# Max source-chunk text length passed to the LLM in the correction prompt.
_CHAIN_CORRECTION_MAX_CHUNK_CHARS = 8196
_CHAIN_CORRECTION_MAX_CHUNKS = 12


CHAIN_CORRECTION_PROMPT = """You are correcting an extracted {kind}-kind structure.

Constraint: the relations must form a strict linear chain — every entity has
at most one predecessor and at most one successor, and there must be no
cycle. The relations below were flagged by an automated detector as
violating this constraint. Each one carries the issue that was detected.

Bad relations (review and keep only those supported by the source text):
{bad_relations_json}

Source chunks the relations were extracted from:
{source_chunks_text}

Your task: from the bad relations above, pick the subset that should be
kept. Drop the rest. Do not invent new relations. Use only ``from`` and
``to`` slugs that appear verbatim in the bad-relations list. The result
must satisfy the strict-chain constraint.

Return ONLY a JSON object with this exact shape (no markdown fences, no
commentary):
{{
  "keep": [
    {{"from": "<slug>", "to": "<slug>"}},
    ...
  ]
}}
"""


def _chain_extract_edge(doc: dict) -> tuple[str, str] | None:
    """Return ``(from_slug, to_slug)`` for a relation doc, or None."""
    if doc.get("knowledge_graph_kwd") != "relation":
        return None
    src = doc.get("from_entity_kwd")
    tgt = doc.get("to_entity_kwd")
    if isinstance(src, str) and isinstance(tgt, str) and src.strip() and tgt.strip():
        return src.strip(), tgt.strip()
    # Fallback: parse the payload — older relation docs may not have the
    # *_entity_kwd columns set if the upstream extractor was permissive.
    try:
        payload = json.loads(doc.get("content_with_weight") or "{}")
    except Exception:
        return None
    if not isinstance(payload, dict):
        return None
    for src_key, tgt_key in (("source", "target"), ("from", "to"), ("src", "tgt")):
        s = payload.get(src_key)
        t = payload.get(tgt_key)
        if isinstance(s, str) and isinstance(t, str) and s.strip() and t.strip():
            return s.strip(), t.strip()
    return None


def _chain_detect_violations(
    edges: list[tuple[str, str]],
) -> dict[tuple[str, str], list[str]]:
    """Walk the edge list once and return ``{edge: [issue_strings]}`` for
    every edge involved in any of:

    * **self-loop** — ``from == to``.
    * **fan-out** — multiple edges share the same ``from``.
    * **fan-in** — multiple edges share the same ``to``.
    * **cycle** — the edge participates in a directed cycle (size ≥ 2).

    Edges with no issues are simply absent from the result dict.
    """
    issues: dict[tuple[str, str], list[str]] = {}

    def _add(edge: tuple[str, str], reason: str) -> None:
        issues.setdefault(edge, []).append(reason)

    # Self-loops + degree counts.
    out_groups: dict[str, list[tuple[str, str]]] = {}
    in_groups: dict[str, list[tuple[str, str]]] = {}
    for e in edges:
        if e[0] == e[1]:
            _add(e, "self-loop")
        out_groups.setdefault(e[0], []).append(e)
        in_groups.setdefault(e[1], []).append(e)

    for node, group in out_groups.items():
        if len(group) > 1:
            siblings = sorted({g[1] for g in group})
            reason = f"fan-out from '{node}' (also points to {siblings})"
            for e in group:
                _add(e, reason)
    for node, group in in_groups.items():
        if len(group) > 1:
            siblings = sorted({g[0] for g in group})
            reason = f"fan-in to '{node}' (also reached from {siblings})"
            for e in group:
                _add(e, reason)

    # Cycle detection — Tarjan SCC. Any SCC of size ≥ 2 is a cycle; any
    # self-loop already caught above is its own size-1 SCC and is
    # excluded here.
    adj: dict[str, list[str]] = {}
    nodes: set[str] = set()
    for src, tgt in edges:
        nodes.add(src)
        nodes.add(tgt)
        adj.setdefault(src, []).append(tgt)

    index_counter = [0]
    stack: list[str] = []
    on_stack: set[str] = set()
    index: dict[str, int] = {}
    lowlink: dict[str, int] = {}
    sccs: list[set[str]] = []

    def _strongconnect(v: str) -> None:
        index[v] = index_counter[0]
        lowlink[v] = index_counter[0]
        index_counter[0] += 1
        stack.append(v)
        on_stack.add(v)
        for w in adj.get(v, ()):
            if w not in index:
                _strongconnect(w)
                lowlink[v] = min(lowlink[v], lowlink[w])
            elif w in on_stack:
                lowlink[v] = min(lowlink[v], index[w])
        if lowlink[v] == index[v]:
            comp: set[str] = set()
            while True:
                w = stack.pop()
                on_stack.discard(w)
                comp.add(w)
                if w == v:
                    break
            if len(comp) >= 2:
                sccs.append(comp)

    for n in nodes:
        if n not in index:
            try:
                _strongconnect(n)
            except RecursionError:
                # Pathologically deep relation graphs — skip cycle
                # detection rather than crashing the whole flush.
                logging.warning("chain validate: cycle detection hit recursion limit")
                break

    for comp in sccs:
        for src, tgt in edges:
            if src in comp and tgt in comp:
                _add((src, tgt), f"cycle within {sorted(comp)}")

    return issues


def _chain_gather_chunk_text(
    bad_docs: list[dict],
    chunks_by_id: dict[str, str],
) -> list[tuple[str, str]]:
    """Collect (chunk_id, text) pairs for the LLM prompt — deduplicated,
    capped at ``_CHAIN_CORRECTION_MAX_CHUNKS`` chunks, each trimmed to
    ``_CHAIN_CORRECTION_MAX_CHUNK_CHARS`` characters."""
    seen: set[str] = set()
    out: list[tuple[str, str]] = []
    for doc in bad_docs:
        for cid in doc.get("source_chunk_ids") or []:
            if not isinstance(cid, str) or cid in seen:
                continue
            seen.add(cid)
            text = chunks_by_id.get(cid)
            if not isinstance(text, str) or not text.strip():
                continue
            out.append((cid, text[:_CHAIN_CORRECTION_MAX_CHUNK_CHARS]))
            if len(out) >= _CHAIN_CORRECTION_MAX_CHUNKS:
                return out
    return out


async def validate_and_correct_chain(
    docs: list[dict],
    chunks_by_id: dict[str, str],
    chat_mdl,
    kind: str,
    callback=None,
) -> list[dict]:
    """Ensure the chain-shape constraint on ``docs`` (a flush-time mixed
    list of entity and relation docs). On finding a violation we ask the
    LLM to pick the subset of the offending relations that should be
    kept; the dropped offenders are removed from the returned list.

    Best-effort: any exception during detection or LLM call results in
    ``docs`` being returned verbatim, so a misbehaving model can never
    block the merge phase. Callers are still responsible for wrapping
    the call in their own timeout if they want a hard wall.
    """
    if not docs or kind not in CHAIN_KINDS:
        return docs

    try:
        # Bucket: relations keyed by edge for later removal.
        edge_to_docs: dict[tuple[str, str], list[dict]] = {}
        all_edges: list[tuple[str, str]] = []
        for d in docs:
            e = _chain_extract_edge(d)
            if e is None:
                continue
            edge_to_docs.setdefault(e, []).append(d)
            all_edges.append(e)

        violations = _chain_detect_violations(all_edges)
        if not violations:
            return docs

        bad_edges = list(violations.keys())
        bad_docs: list[dict] = []
        for e in bad_edges:
            bad_docs.extend(edge_to_docs.get(e, ()))

        bad_relations_repr = [{"from": e[0], "to": e[1], "issue": "; ".join(reasons)} for e, reasons in violations.items()]
        chunk_pairs = _chain_gather_chunk_text(bad_docs, chunks_by_id)
        source_chunks_text = "\n\n".join(f"[{cid}]\n{text}" for cid, text in chunk_pairs) or "(no source chunks available)"
        prompt = CHAIN_CORRECTION_PROMPT.format(
            kind=kind,
            bad_relations_json=json.dumps(bad_relations_repr, ensure_ascii=False),
            source_chunks_text=source_chunks_text,
        )
        if callable(callback):
            try:
                callback(msg=f"chain validation: {len(bad_edges)} flagged for LLM correction")
            except Exception:
                pass

        res = await gen_json(
            "You correct extracted graph relations to satisfy a strict-chain constraint.",
            prompt,
            chat_mdl,
            gen_conf={"temperature": 0.0},
        )
    except Exception:
        logging.exception("chain validate: detection / LLM call failed; skipping correction")
        return docs

    if not isinstance(res, dict):
        return docs
    keep_raw = res.get("keep")
    if not isinstance(keep_raw, list):
        return docs

    bad_edge_set = set(bad_edges)
    keep_set: set[tuple[str, str]] = set()
    for item in keep_raw:
        if not isinstance(item, dict):
            continue
        s = item.get("from")
        t = item.get("to")
        if not isinstance(s, str) or not isinstance(t, str):
            continue
        edge = (s.strip(), t.strip())
        # Reject anything that wasn't in the bad set — we don't invent
        # new relations and we don't allow the LLM to "rescue" a
        # never-extracted edge.
        if edge in bad_edge_set:
            keep_set.add(edge)

    if keep_set == bad_edge_set:
        # LLM kept everything → no correction applied.
        return docs

    # Drop the bad-edge docs that the LLM didn't keep.
    dropped_doc_ids: set[str] = set()
    for edge in bad_edge_set - keep_set:
        for d in edge_to_docs.get(edge, ()):
            did = d.get("id")
            if isinstance(did, str):
                dropped_doc_ids.add(did)

    if not dropped_doc_ids:
        return docs

    corrected = [d for d in docs if d.get("id") not in dropped_doc_ids]
    if callable(callback):
        try:
            callback(msg=f"chain validation: dropped {len(dropped_doc_ids)} of {len(bad_edges)} flagged relation(s)")
        except Exception:
            pass
    return corrected


async def merge_compiled_structures(
    docs: list[dict],
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    similarity_threshold: float = 0.99,
    compilation_template_id: str | None = None,
    cancel_check: Callable[[], bool] | None = None,
) -> dict:
    """Merge ``docs`` (the output of ``compile_structure_from_text``) before
    inserting them into ES.

    Two phases:
        1. **Local dedup**: bucket by (doc_id, compile_kwd, from_entity_kwd?,
           to_entity_kwd?), pairwise cosine similarity over the q_<dim>_vec
           field via ``sklearn.metrics.pairwise.cosine_similarity``; pairs
           above ``similarity_threshold`` go through ``_struct_merge_pair``
           (LLM-judged). On a duplicate verdict the surviving entry is
           rebuilt from the merged payload (union of ``source_chunk_ids``,
           re-embedded, src/target preserved on relations).
        2. **ES dedup**: for each surviving doc, KNN-search ES with the same
           filter via ``MatchDenseExpr`` (top1, similarity ≥ threshold). On a
           hit + LLM duplicate verdict, the existing ES doc is replaced
           **by its old id** (`settings.docStoreConn.update`). Otherwise the
           doc is inserted as new.

    Args:
        docs: list of ES-ready dicts from ``compile_structure_from_text``.
        chat_mdl: LLMBundle for chat (used to judge duplicate-ness + emit
            merged JSON via ``gen_json``).
        embd_mdl: LLMBundle for embeddings (used to re-embed merged
            descriptions before persistence).
        tenant_id, kb_id: address the doc-store index for the current KB.
        similarity_threshold: minimum cosine similarity for a pair to be
            considered for LLM-judged merge.
        cancel_check: optional callable returning True when the owning parse
            task has been canceled. Checked between ES-dedup iterations so a
            long merge can stop promptly.

    Returns:
        {"inserted": N, "updated": M, "duplicates_dropped": K} summary.
    """
    if not docs:
        return {"inserted": 0, "updated": 0, "duplicates_dropped": 0}

    deduped, dropped = await _struct_local_dedup(
        docs,
        chat_mdl,
        embd_mdl,
        similarity_threshold,
    )

    graph_keys = {
        (
            str(d.get("doc_id")),
            str(d.get("compile_kwd")),
            _struct_doc_template_id(d) or compilation_template_id or "",
        )
        for d in deduped
        if d.get("doc_id") and d.get("compile_kwd") and d.get("knowledge_graph_kwd") in ("entity", "relation")
    }

    def _raise_if_canceled() -> None:
        if callable(cancel_check) and cancel_check():
            raise TaskCanceledException("Task was cancelled during structure ES dedup merge")

    inserted = 0
    updated = 0
    for d in deduped:
        _raise_if_canceled()
        try:
            result = await _struct_es_dedup_one(
                d,
                chat_mdl,
                embd_mdl,
                tenant_id,
                kb_id,
                similarity_threshold,
            )
        except Exception:
            logging.exception("merge_compiled_structures: per-doc dedup failed")
            continue
        if result == "inserted":
            inserted += 1
        elif result == "updated":
            updated += 1

    graphs = 0
    for doc_id, compile_kwd, template_id in graph_keys:
        _raise_if_canceled()
        try:
            await rebuild_structure_graph_json(
                tenant_id,
                kb_id,
                doc_id,
                compile_kwd,
                compilation_template_id=template_id or None,
            )
            graphs += 1
        except Exception:
            logging.exception(
                "merge_compiled_structures: graph rebuild failed for doc=%s compile_kwd=%s template=%s",
                doc_id,
                compile_kwd,
                template_id,
            )

    return {
        "inserted": inserted,
        "updated": updated,
        "duplicates_dropped": dropped,
        "graphs": graphs,
    }


__all__ = [
    "compile_structure_from_text",
    "merge_compiled_structures",
    "rebuild_structure_graph_json",
]
