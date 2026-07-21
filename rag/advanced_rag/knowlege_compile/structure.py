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
import asyncio
import heapq
import json
import logging
from typing import Awaitable, Callable, Tuple

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

_ES_DEDUP_KNN_CONCURRENCY = 8
_ES_DEDUP_LLM_CONCURRENCY = 16
_ES_DEDUP_LLM_BATCH_SIZE = 16
_ES_DEDUP_EMBED_BATCH_SIZE = 64
_ES_DEDUP_INSERT_BATCH_SIZE = 256


class LLMCallPool:
    """Task-scoped priority scheduler for actual chat-model calls."""

    def __init__(self, max_concurrency: int = 10, max_pending: int | None = None):
        self.max_concurrency = max(1, int(max_concurrency))
        self.max_pending = max(self.max_concurrency, int(max_pending or self.max_concurrency))
        self._active = 0
        self._ticket = 0
        self._waiting: list[tuple[int, int]] = []
        self._condition = asyncio.Condition()

    @property
    def active_count(self) -> int:
        return self._active

    @property
    def pending_count(self) -> int:
        return self._active + len(self._waiting)

    def wrap(self, chat_mdl, *, priority: int, label: str, context: str | None = None):
        return PooledChatModel(self, chat_mdl, priority=priority, label=label, context=context)

    async def call(self, fn, *, priority: int, label: str, context: str | None = None):
        async with self._condition:
            while self.pending_count >= self.max_pending:
                await self._condition.wait()
            ticket = (int(priority), self._ticket)
            self._ticket += 1
            heapq.heappush(self._waiting, ticket)
            try:
                while self._active >= self.max_concurrency or self._waiting[0] != ticket:
                    await self._condition.wait()
            except BaseException:
                if ticket in self._waiting:
                    self._waiting.remove(ticket)
                    heapq.heapify(self._waiting)
                    self._condition.notify_all()
                raise
            heapq.heappop(self._waiting)
            self._active += 1
        try:
            result = await fn()
            return result
        except BaseException:
            raise
        finally:
            async with self._condition:
                self._active -= 1
                self._condition.notify_all()


class PooledChatModel:
    _pool_managed = True

    def __init__(self, pool: LLMCallPool, chat_mdl, *, priority: int, label: str, context: str | None):
        self._pool = pool
        self._chat_mdl = chat_mdl
        self._priority = priority
        self._label = label
        self._context = context

    def __getattr__(self, name):
        return getattr(self._chat_mdl, name)

    async def async_chat(self, system, history, gen_conf=None, **kwargs):
        return await self._pool.call(
            lambda: self._chat_mdl.async_chat(system, history, gen_conf=gen_conf, **kwargs),
            priority=self._priority,
            label=self._label,
            context=self._context,
        )


def _struct_normalize_kind(kind) -> str:
    if not isinstance(kind, str):
        return ""
    return kind.strip().lower().replace("-", "_")


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


def _struct_entity_name(doc_or_payload: dict) -> str:
    value = doc_or_payload.get("name") if isinstance(doc_or_payload, dict) else None
    if value is None and isinstance(doc_or_payload, dict):
        try:
            value = json.loads(doc_or_payload.get("content_with_weight") or "{}").get("name")
        except Exception:
            value = None
    return str(value).strip() if value is not None else ""


def _struct_resolve_entity_alias(name: str, aliases: dict[str, str]) -> str:
    current = str(name).strip()
    seen = set()
    while current in aliases and current not in seen:
        seen.add(current)
        current = aliases[current]
    return current


def _struct_rewrite_relation_payload(payload: dict, aliases: dict[str, str]) -> bool:
    changed = False
    for fields in (("source", "src", "from"), ("target", "tgt", "to")):
        for field in fields:
            if field not in payload or payload[field] is None:
                continue
            old = str(payload[field]).strip()
            new = _struct_resolve_entity_alias(old, aliases)
            if new != old:
                payload[field] = new
                changed = True
    return changed


async def _struct_rewrite_relation_doc(doc: dict, aliases: dict[str, str], embd_mdl) -> dict:
    if doc.get("knowledge_graph_kwd") != "relation" or not aliases:
        return doc
    try:
        payload = json.loads(doc.get("content_with_weight") or "{}")
    except Exception:
        return doc
    if not isinstance(payload, dict) or not _struct_rewrite_relation_payload(payload, aliases):
        return doc
    vecs = await _struct_embed(embd_mdl, [_struct_payload_description(payload)])
    if not vecs:
        return doc
    base = dict(doc)
    base["content_with_weight"] = json.dumps(payload, ensure_ascii=False)
    base["from_entity_kwd"] = _struct_resolve_entity_alias(base.get("from_entity_kwd", ""), aliases)
    base["to_entity_kwd"] = _struct_resolve_entity_alias(base.get("to_entity_kwd", ""), aliases)
    return _struct_rebuild_es_doc(payload, base, vecs[0], doc.get("source_chunk_ids") or [], preserve_id=True)


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
        compilation_template_id=_struct_doc_template_id(base_doc),
        compilation_template_kind=base_doc.get("compilation_template_kind_kwd"),
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


def _struct_es_dedup_condition(doc: dict) -> dict:
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
    template_id = _struct_doc_template_id(doc)
    if template_id:
        condition["compilation_template_ids"] = [template_id]
    return condition


async def _struct_es_knn_candidate(
    doc: dict,
    tenant_id: str,
    kb_id: str,
    similarity_threshold: float,
    index: str,
    select_fields: list[str],
    timing_context: str | None,
    item_index: int,
) -> dict | None:
    """Run one KNN lookup; the caller controls concurrency."""
    from common import settings
    from common.doc_store.doc_store_base import MatchDenseExpr, OrderByExpr

    vec_field, vec = _struct_doc_vec(doc)
    if not vec_field or vec is None:
        return None
    match_expr = MatchDenseExpr(
        vector_column_name=vec_field,
        embedding_data=list(vec),
        embedding_data_type="float",
        distance_type="cosine",
        topn=1,
        extra_options={"similarity": similarity_threshold},
    )
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            select_fields,
            [],
            _struct_es_dedup_condition(doc),
            [match_expr],
            OrderByExpr(),
            0,
            1,
            index,
            [kb_id],
        )
        field_map = settings.docStoreConn.get_fields(res, select_fields)
        if not field_map:
            return None
        old_id, old_doc = next(iter(field_map.items()))
        old_doc = dict(old_doc)
        old_doc.setdefault("id", old_id)
        return old_doc
    except Exception:
        logging.exception("merge_compiled_structures: ES KNN search failed; treating doc as new")
        return None


ES_GROUP_MERGE_PROMPT = """Existing item:
{existing}

Incoming items:
{incoming}

Decide which incoming items refer to the same logical entity or relation as
the existing item. Merge all duplicated incoming items with the existing item.
Incoming items that are not duplicates must remain separate. Do not invent
data and do not merge unrelated incoming items with each other.

Return ONLY JSON with this exact shape:
{{
  "duplicate_indices": [<incoming index>, ...],
  "merged": <merged JSON object when duplicate_indices is non-empty, otherwise null>
}}
"""

ES_GROUP_BATCH_MERGE_PROMPT = """You are judging multiple independent ES deduplication groups.

For every group, compare every incoming item with that group's existing item.
You must make a separate duplicated decision for every incoming item. Only
incoming items marked duplicated=true may contribute to that group's merged
payload. Incoming items marked duplicated=false must remain separate. Do not
merge items from different groups and do not invent data.

Return ONLY JSON with this exact shape:
{{
  "groups": [
    {{
      "group_id": "<group id>",
      "decisions": [
        {{"incoming_index": 0, "duplicated": true}},
        {{"incoming_index": 1, "duplicated": false}}
      ],
      "merged": <merged JSON object when any item is duplicated, otherwise null>
    }}
  ]
}}

Groups:
{groups}
"""

ES_GROUP_DECISION_BATCH_PROMPT = """You are judging multiple independent ES deduplication groups.

For every incoming item, independently decide whether it is a duplicate of
the existing item in the same group. Do not merge anything and do not judge
items from different groups against each other.

Return ONLY JSON with this exact shape:
{{
  "groups": [
    {{
      "group_id": "<group id>",
      "decisions": [
        {{"incoming_index": 0, "duplicated": true}},
        {{"incoming_index": 1, "duplicated": false}}
      ]
    }}
  ]
}}

Groups:
{groups}
"""


async def _struct_judge_es_group_batch(group_specs: list[dict], chat_mdl) -> dict[str, set[int]]:
    """Judge every incoming item independently without generating a merge."""
    prompt_groups = []
    for spec in group_specs:
        try:
            existing_payload = json.loads(spec["old_doc"].get("content_with_weight") or "{}")
            incoming_payloads = [json.loads(d.get("content_with_weight") or "{}") for d in spec["incoming_docs"]]
        except Exception:
            logging.exception("merge: failed to parse ES decision group")
            continue
        if not isinstance(existing_payload, dict) or not all(isinstance(p, dict) for p in incoming_payloads):
            continue
        prompt_groups.append(
            {
                "group_id": spec["request_group_id"],
                "existing": existing_payload,
                "incoming": [{"index": i, "item": payload} for i, payload in enumerate(incoming_payloads)],
            }
        )
    if not prompt_groups:
        return {spec["request_group_id"]: set() for spec in group_specs}

    user_prompt = ES_GROUP_DECISION_BATCH_PROMPT.format(groups=json.dumps(prompt_groups, ensure_ascii=False))
    system_prompt = MERGE_SYSTEM_PROMPT + "\n\n" + ES_GROUP_DECISION_BATCH_PROMPT.split("Groups:", 1)[0]
    res = await gen_json(system_prompt, user_prompt, chat_mdl, gen_conf={"temperature": 0.0})
    raw_groups = res.get("groups") if isinstance(res, dict) else None
    if not isinstance(raw_groups, list):
        return {spec["request_group_id"]: set() for spec in group_specs}

    by_id = {spec["request_group_id"]: spec for spec in group_specs}
    result: dict[str, set[int]] = {}
    for raw in raw_groups:
        if not isinstance(raw, dict) or raw.get("group_id") not in by_id:
            continue
        spec = by_id[raw["group_id"]]
        decisions = raw.get("decisions")
        if not isinstance(decisions, list):
            result[spec["request_group_id"]] = set()
            continue
        result[spec["request_group_id"]] = {
            item["incoming_index"]
            for item in decisions
            if isinstance(item, dict) and item.get("duplicated") is True and isinstance(item.get("incoming_index"), int) and 0 <= item["incoming_index"] < len(spec["incoming_docs"])
        }
    for spec in group_specs:
        result.setdefault(spec["request_group_id"], set())
    return result


async def _struct_merge_es_group_batch(group_specs: list[dict], chat_mdl) -> dict[str, tuple[list[dict], dict | None]]:
    """Judge multiple old_id groups in one LLM request."""
    prompt_groups = []
    for spec in group_specs:
        old_doc = spec["old_doc"]
        incoming_docs = spec["incoming_docs"]
        try:
            existing_payload = json.loads(old_doc.get("content_with_weight") or "{}")
            incoming_payloads = [json.loads(d.get("content_with_weight") or "{}") for d in incoming_docs]
        except Exception:
            logging.exception("merge: failed to parse grouped content_with_weight")
            continue
        if not isinstance(existing_payload, dict) or not all(isinstance(p, dict) for p in incoming_payloads):
            continue
        prompt_groups.append(
            {
                "group_id": spec["old_id"],
                "existing": existing_payload,
                "incoming": [{"index": i, "item": payload} for i, payload in enumerate(incoming_payloads)],
            }
        )
    if not prompt_groups:
        return {spec["old_id"]: (list(spec["incoming_docs"]), None) for spec in group_specs}

    user_prompt = ES_GROUP_BATCH_MERGE_PROMPT.format(groups=json.dumps(prompt_groups, ensure_ascii=False))
    system_prompt = MERGE_SYSTEM_PROMPT + "\n\n" + ES_GROUP_BATCH_MERGE_PROMPT.split("Groups:", 1)[0]
    res = await gen_json(system_prompt, user_prompt, chat_mdl, gen_conf={"temperature": 0.0})
    raw_groups = res.get("groups") if isinstance(res, dict) else None
    if not isinstance(raw_groups, list):
        return {spec["old_id"]: (list(spec["incoming_docs"]), None) for spec in group_specs}

    result = {}
    by_id = {spec["old_id"]: spec for spec in group_specs}
    for raw in raw_groups:
        if not isinstance(raw, dict) or raw.get("group_id") not in by_id:
            continue
        spec = by_id[raw["group_id"]]
        decisions = raw.get("decisions")
        merged = raw.get("merged")
        if not isinstance(decisions, list):
            result[spec["old_id"]] = (list(spec["incoming_docs"]), None)
            continue
        duplicate_indices = {item.get("incoming_index") for item in decisions if isinstance(item, dict) and item.get("duplicated") is True and isinstance(item.get("incoming_index"), int)}
        duplicate_indices = {i for i in duplicate_indices if 0 <= i < len(spec["incoming_docs"])}
        if not duplicate_indices or not isinstance(merged, dict):
            result[spec["old_id"]] = (list(spec["incoming_docs"]), None)
            continue
        separate = [d for i, d in enumerate(spec["incoming_docs"]) if i not in duplicate_indices]
        result[spec["old_id"]] = (separate, merged)

    for spec in group_specs:
        result.setdefault(spec["old_id"], (list(spec["incoming_docs"]), None))
    return result


async def _struct_merge_es_group(old_doc: dict, incoming_docs: list[dict], chat_mdl) -> tuple[list[dict], dict | None]:
    """Judge one ES candidate group with one LLM request.

    Returns ``(non_duplicate_docs, merged_payload)``. The existing ES row is
    updated only when ``merged_payload`` is a dict.
    """
    if len(incoming_docs) == 1:
        merged = await _struct_merge_pair(old_doc, incoming_docs[0], chat_mdl)
        return ([] if merged is not None else list(incoming_docs), merged)

    try:
        existing_payload = json.loads(old_doc.get("content_with_weight") or "{}")
        incoming_payloads = [json.loads(d.get("content_with_weight") or "{}") for d in incoming_docs]
    except Exception:
        logging.exception("merge: failed to parse grouped content_with_weight")
        return list(incoming_docs), None
    if not isinstance(existing_payload, dict) or not all(isinstance(p, dict) for p in incoming_payloads):
        return list(incoming_docs), None

    system_prompt = MERGE_SYSTEM_PROMPT + "\n\n" + ES_GROUP_MERGE_PROMPT
    user_prompt = ES_GROUP_MERGE_PROMPT.format(
        existing=json.dumps(existing_payload, ensure_ascii=False),
        incoming=json.dumps(
            [{"index": i, "item": payload} for i, payload in enumerate(incoming_payloads)],
            ensure_ascii=False,
        ),
    )
    res = await gen_json(system_prompt, user_prompt, chat_mdl, gen_conf={"temperature": 0.0})
    if not isinstance(res, dict):
        return list(incoming_docs), None
    indices = res.get("duplicate_indices")
    merged = res.get("merged")
    if not isinstance(indices, list) or not isinstance(merged, dict):
        return list(incoming_docs), None
    duplicate_indices = {i for i in indices if isinstance(i, int) and 0 <= i < len(incoming_docs)}
    if not duplicate_indices:
        return list(incoming_docs), None
    separate = [d for i, d in enumerate(incoming_docs) if i not in duplicate_indices]
    return separate, merged


async def _struct_es_dedup_batch(
    docs: list[dict],
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    similarity_threshold: float,
    timing_context: str | None = None,
    cancel_check: Callable[[], bool] | None = None,
) -> tuple[int, int]:
    """Batch ES dedup: concurrent KNN, parallel decisions, then grouped merges."""
    from common import settings
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)

    def _raise_if_canceled() -> None:
        if callable(cancel_check) and cancel_check():
            raise TaskCanceledException("Task was cancelled during ES dedup")

    select_fields = [
        "id",
        "content_with_weight",
        "source_chunk_ids",
        "knowledge_graph_kwd",
        "compile_kwd",
        "doc_id",
        "from_entity_kwd",
        "to_entity_kwd",
        "compilation_template_ids",
        "compilation_template_kind_kwd",
    ]

    # One semaphore shared by all tasks; constructing it per task would not
    # limit concurrency.
    knn_semaphore = asyncio.Semaphore(_ES_DEDUP_KNN_CONCURRENCY)

    async def run_knn_shared(item_index: int, doc: dict):
        _raise_if_canceled()
        async with knn_semaphore:
            return doc, await _struct_es_knn_candidate(
                doc,
                tenant_id,
                kb_id,
                similarity_threshold,
                index,
                select_fields,
                timing_context,
                item_index,
            )

    _raise_if_canceled()
    knn_results = await asyncio.gather(*(run_knn_shared(i, d) for i, d in enumerate(docs)))
    _raise_if_canceled()
    groups: dict[str, tuple[dict, list[dict]]] = {}
    inserts: list[dict] = []
    for doc, old_doc in knn_results:
        if old_doc is None:
            inserts.append(doc)
            continue
        old_id = str(old_doc["id"])
        if old_id not in groups:
            groups[old_id] = (old_doc, [])
        groups[old_id][1].append(doc)

    # Stage 1 is deliberately read-only: every decision for an old_id uses
    # the same KNN snapshot.  This lets sub-batches of one large group run in
    # parallel without one completed request changing the input of another.
    states = {
        old_id: {
            "old_doc": old_doc,
            "incoming_docs": incoming,
            "separate": [],
            "duplicate_docs": [],
            "merged": None,
            "chunk_ids": list(old_doc.get("source_chunk_ids") or []),
            "entity_aliases": {},
        }
        for old_id, (old_doc, incoming) in groups.items()
    }
    entity_aliases: dict[str, str] = {}
    llm_semaphore = asyncio.Semaphore(_ES_DEDUP_LLM_CONCURRENCY)

    decision_specs = []
    for old_id, state in states.items():
        incoming_docs = state["incoming_docs"]
        for part, start in enumerate(range(0, len(incoming_docs), _ES_DEDUP_LLM_BATCH_SIZE)):
            decision_specs.append(
                {
                    "old_id": old_id,
                    "request_group_id": f"{old_id}:part-{part}",
                    "old_doc": state["old_doc"],
                    "incoming_docs": incoming_docs[start : start + _ES_DEDUP_LLM_BATCH_SIZE],
                }
            )

    decision_batches = []
    current_batch = []
    current_size = 0
    for spec in decision_specs:
        size = len(spec["incoming_docs"])
        if current_batch and current_size + size > _ES_DEDUP_LLM_BATCH_SIZE:
            decision_batches.append(current_batch)
            current_batch = []
            current_size = 0
        current_batch.append(spec)
        current_size += size
    if current_batch:
        decision_batches.append(current_batch)

    async def run_decision_batch(batch_no: int, batch_specs: list[dict]):
        _raise_if_canceled()
        async with llm_semaphore:
            try:
                result = await _struct_judge_es_group_batch(batch_specs, chat_mdl)
            except Exception:
                logging.exception("merge_compiled_structures: ES decision batch failed")
                result = {spec["request_group_id"]: set() for spec in batch_specs}
            return result

    decision_results = await asyncio.gather(
        *(run_decision_batch(i, batch) for i, batch in enumerate(decision_batches)),
    )
    _raise_if_canceled()
    for batch, result in zip(decision_batches, decision_results):
        for spec in batch:
            state = states[spec["old_id"]]
            duplicate_indices = result.get(spec["request_group_id"], set())
            for incoming_index, doc in enumerate(spec["incoming_docs"]):
                if incoming_index in duplicate_indices:
                    state["duplicate_docs"].append(doc)
                else:
                    state["separate"].append(doc)

    async def merge_one_group(old_id: str, state: dict):
        _raise_if_canceled()
        duplicate_docs = state["duplicate_docs"]
        if not duplicate_docs:
            return
        old_doc = state["old_doc"]
        current_doc = dict(old_doc)
        current_chunk_ids = list(state["chunk_ids"])
        merged_payload = None
        # A normal group gets exactly one merge request.  Only pathological
        # groups are folded in <= batch-sized sequential pieces.
        for start in range(0, len(duplicate_docs), _ES_DEDUP_LLM_BATCH_SIZE):
            _raise_if_canceled()
            candidate_docs = duplicate_docs[start : start + _ES_DEDUP_LLM_BATCH_SIZE]
            separate, candidate_merged = await _struct_merge_es_group(current_doc, candidate_docs, chat_mdl)
            state["separate"].extend(separate)
            if candidate_merged is None:
                continue
            candidate_merged = _struct_apply_merge_invariants(current_doc, candidate_merged)
            if old_doc.get("knowledge_graph_kwd") == "entity":
                old_name = _struct_entity_name(current_doc)
                canonical_name = _struct_entity_name(candidate_merged) or old_name
                for candidate in candidate_docs:
                    candidate_name = _struct_entity_name(candidate)
                    if candidate_name and candidate_name != canonical_name:
                        state["entity_aliases"][candidate_name] = canonical_name
                if old_name and old_name != canonical_name:
                    state["entity_aliases"][old_name] = canonical_name
            separate_ids = {id(doc) for doc in separate}
            current_chunk_ids = _struct_union_chunk_ids(
                current_chunk_ids,
                *(d.get("source_chunk_ids") for d in candidate_docs if id(d) not in separate_ids),
            )
            current_doc["content_with_weight"] = json.dumps(candidate_merged, ensure_ascii=False)
            current_doc["source_chunk_ids"] = current_chunk_ids
            merged_payload = candidate_merged
        if merged_payload is not None:
            state["merged"] = merged_payload
            state["chunk_ids"] = current_chunk_ids

    merge_jobs = [merge_one_group(old_id, state) for old_id, state in states.items() if state["duplicate_docs"]]
    await asyncio.gather(*merge_jobs)
    _raise_if_canceled()

    existing_relation_updates = 0

    merged_jobs = []
    for old_id, state in states.items():
        separate_docs = state["separate"]
        inserts.extend(separate_docs)
        if state["merged"] is None:
            continue
        merged_jobs.append(
            {
                "old_id": old_id,
                "old_doc": state["old_doc"],
                "payload": state["merged"],
                "chunk_ids": state["chunk_ids"],
                "entity_aliases": dict(state.get("entity_aliases") or {}),
            }
        )

    # Encode all merged groups in batches, independent of the LLM grouping.
    for start in range(0, len(merged_jobs), _ES_DEDUP_EMBED_BATCH_SIZE):
        batch = merged_jobs[start : start + _ES_DEDUP_EMBED_BATCH_SIZE]
        texts = [_struct_payload_description(job["payload"]) for job in batch]
        try:
            vectors = await _struct_embed(embd_mdl, texts)
        except Exception:
            logging.exception("merge_compiled_structures: grouped embedding failed for %d docs", len(batch))
            vectors = []
        for job, vec in zip(batch, vectors):
            job["rebuilt"] = _struct_rebuild_es_doc(
                job["payload"],
                job["old_doc"],
                vec,
                job["chunk_ids"],
                preserve_id=True,
            )

    updated_jobs = [job for job in merged_jobs if job.get("rebuilt")]
    writes = inserts + [job["rebuilt"] for job in updated_jobs]
    inserted = 0
    updated = 0
    successful_entity_aliases: dict[str, str] = {}
    for start in range(0, len(writes), _ES_DEDUP_INSERT_BATCH_SIZE):
        _raise_if_canceled()
        batch = writes[start : start + _ES_DEDUP_INSERT_BATCH_SIZE]
        if not batch:
            continue
        try:
            await thread_pool_exec(settings.docStoreConn.insert, batch, index, kb_id)
            updated_in_batch = sum(1 for doc in batch if any(doc is job.get("rebuilt") for job in updated_jobs))
            updated += updated_in_batch
            inserted += len(batch) - updated_in_batch
            for job in updated_jobs:
                if job.get("rebuilt") not in batch:
                    continue
                if job["old_doc"].get("knowledge_graph_kwd") == "entity":
                    successful_entity_aliases.update(job.get("entity_aliases") or {})
        except Exception:
            logging.exception("merge_compiled_structures: bulk insert failed for %d docs", len(batch))

    # Only publish aliases after the canonical entity writes have completed.
    entity_aliases.update(successful_entity_aliases)
    if entity_aliases:
        relation_fields = [
            "id",
            "content_with_weight",
            "source_chunk_ids",
            "knowledge_graph_kwd",
            "compile_kwd",
            "doc_id",
            "from_entity_kwd",
            "to_entity_kwd",
            "compilation_template_ids",
            "compilation_template_kind_kwd",
        ]
        from common.doc_store.doc_store_base import OrderByExpr

        scopes = {
            (
                state["old_doc"].get("doc_id"),
                state["old_doc"].get("compile_kwd"),
                _struct_doc_template_id(state["old_doc"]),
            )
            for state in states.values()
            if state["old_doc"].get("knowledge_graph_kwd") == "entity"
        }
        for doc_id, compile_kwd, template_id in scopes:
            condition = {
                "doc_id": [doc_id],
                "compile_kwd": [compile_kwd],
                "knowledge_graph_kwd": ["relation"],
            }
            if template_id:
                condition["compilation_template_ids"] = [template_id]
            try:
                res = await thread_pool_exec(
                    settings.docStoreConn.search,
                    relation_fields,
                    [],
                    condition,
                    [],
                    OrderByExpr(),
                    0,
                    10000,
                    index,
                    [kb_id],
                )
                rows = settings.docStoreConn.get_fields(res, relation_fields)
            except Exception:
                logging.exception("merge_compiled_structures: relation reference search failed")
                continue
            rewrite_batch = []
            for row_id, row in rows.items():
                payload = _struct_load_payload(row)
                if not isinstance(payload, dict) or not _struct_rewrite_relation_payload(payload, entity_aliases):
                    continue
                base = dict(row)
                base["id"] = row_id
                base["content_with_weight"] = json.dumps(payload, ensure_ascii=False)
                base["from_entity_kwd"] = _struct_resolve_entity_alias(base.get("from_entity_kwd", ""), entity_aliases)
                base["to_entity_kwd"] = _struct_resolve_entity_alias(base.get("to_entity_kwd", ""), entity_aliases)
                rewrite_batch.append((base, payload))
            for start in range(0, len(rewrite_batch), _ES_DEDUP_EMBED_BATCH_SIZE):
                batch = rewrite_batch[start : start + _ES_DEDUP_EMBED_BATCH_SIZE]
                vectors = await _struct_embed(embd_mdl, [_struct_payload_description(payload) for _, payload in batch])
                rewritten = [_struct_rebuild_es_doc(payload, base, vector, base.get("source_chunk_ids") or [], preserve_id=True) for (base, payload), vector in zip(batch, vectors)]
                if rewritten:
                    await thread_pool_exec(settings.docStoreConn.insert, rewritten, index, kb_id)
                    existing_relation_updates += len(rewritten)
        rewritten_inserts = [await _struct_rewrite_relation_doc(doc, entity_aliases, embd_mdl) if doc.get("knowledge_graph_kwd") == "relation" else doc for doc in inserts]
        if rewritten_inserts != inserts:
            await thread_pool_exec(settings.docStoreConn.insert, rewritten_inserts, index, kb_id)
        for job in merged_jobs:
            if job["old_doc"].get("knowledge_graph_kwd") != "relation":
                continue
            if not _struct_rewrite_relation_payload(job["payload"], entity_aliases):
                continue
            vector = await _struct_reembed_payload(job["payload"], embd_mdl)
            if vector is not None:
                rewritten = _struct_rebuild_es_doc(job["payload"], job["old_doc"], vector, job["chunk_ids"], preserve_id=True)
                await thread_pool_exec(settings.docStoreConn.insert, [rewritten], index, kb_id)
    return inserted, updated + existing_relation_updates


async def _struct_local_dedup(
    docs: list[dict],
    chat_mdl,
    embd_mdl,
    similarity_threshold: float,
    timing_context: str | None = None,
    rewrite_relations: bool = True,
    return_aliases: bool = False,
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
    entity_aliases: dict[str, str] = {}

    for group_index, key in enumerate(order):
        kept: list[dict] = []
        for incoming_index, incoming in enumerate(groups[key]):
            _, inc_vec = _struct_doc_vec(incoming)
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
            best_score = float(sims_list[best_idx])
            existing = kept_with_vecs[best_idx][0]
            if best_score < similarity_threshold:
                kept.append(incoming)
                continue
            merged_payload = await _struct_merge_pair(existing, incoming, chat_mdl)
            if merged_payload is None:
                kept.append(incoming)
                continue
            if existing.get("knowledge_graph_kwd") == "entity":
                old_name = _struct_entity_name(existing)
                incoming_name = _struct_entity_name(incoming)
                canonical_name = _struct_entity_name(merged_payload) or old_name
                for alias in (old_name, incoming_name):
                    if alias and alias != canonical_name:
                        entity_aliases[alias] = canonical_name
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

    if rewrite_relations and entity_aliases:
        entity_docs = [d for d in deduped if d.get("knowledge_graph_kwd") != "relation"]
        relation_docs = [d for d in deduped if d.get("knowledge_graph_kwd") == "relation"]
        rewritten_relations = [await _struct_rewrite_relation_doc(d, entity_aliases, embd_mdl) for d in relation_docs]
        relation_deduped, relation_dropped = await _struct_local_dedup(
            rewritten_relations,
            chat_mdl,
            embd_mdl,
            similarity_threshold,
            timing_context=timing_context,
            rewrite_relations=False,
        )
        deduped = entity_docs + relation_deduped
        dropped += relation_dropped

    if return_aliases:
        return deduped, dropped, entity_aliases
    return deduped, dropped


_LOCAL_DEDUP_GROUP_CONCURRENCY = 8


def _struct_entity_candidate_groups(docs: list[dict], similarity_threshold: float) -> list[list[dict]]:
    """Partition entity candidates into independent cosine-connected groups."""
    from sklearn.metrics.pairwise import cosine_similarity

    buckets: dict[tuple, list[dict]] = {}
    order: list[tuple] = []
    for doc in docs:
        key = _struct_filter_key(doc)
        if key not in buckets:
            buckets[key] = []
            order.append(key)
        buckets[key].append(doc)

    result: list[list[dict]] = []
    for key in order:
        bucket = buckets[key]
        vectors = [_struct_doc_vec(doc)[1] for doc in bucket]
        valid = [i for i, vector in enumerate(vectors) if vector]
        parent = list(range(len(bucket)))

        def find(index: int) -> int:
            while parent[index] != index:
                parent[index] = parent[parent[index]]
                index = parent[index]
            return index

        def union(left: int, right: int) -> None:
            left_root, right_root = find(left), find(right)
            if left_root != right_root:
                parent[right_root] = left_root

        if len(valid) > 1:
            matrix = cosine_similarity([list(vectors[i]) for i in valid])
            for left_offset, left in enumerate(valid):
                for right_offset in range(left_offset + 1, len(valid)):
                    right = valid[right_offset]
                    if float(matrix[left_offset, right_offset]) >= similarity_threshold:
                        union(left, right)

        components: dict[int, list[dict]] = {}
        component_order: list[int] = []
        for index, doc in enumerate(bucket):
            root = find(index) if index in valid else index
            if root not in components:
                components[root] = []
                component_order.append(root)
            components[root].append(doc)
        result.extend(components[root] for root in component_order)
    return result


async def _struct_local_dedup_parallel(
    docs: list[dict],
    chat_mdl,
    embd_mdl,
    similarity_threshold: float,
    timing_context: str | None = None,
) -> tuple[list[dict], int]:
    """Deduplicate entities and relations in dependency order with group concurrency."""
    if not docs:
        return [], 0

    entity_docs = [doc for doc in docs if doc.get("knowledge_graph_kwd") != "relation"]
    relation_docs = [doc for doc in docs if doc.get("knowledge_graph_kwd") == "relation"]
    entity_groups = _struct_entity_candidate_groups(entity_docs, similarity_threshold)
    group_semaphore = asyncio.Semaphore(_LOCAL_DEDUP_GROUP_CONCURRENCY)

    async def dedup_group(group: list[dict]):
        async with group_semaphore:
            return await _struct_local_dedup(
                group,
                chat_mdl,
                embd_mdl,
                similarity_threshold,
                timing_context=timing_context,
                rewrite_relations=False,
                return_aliases=True,
            )

    entity_results = await asyncio.gather(*(dedup_group(group) for group in entity_groups))
    deduped_entities: list[dict] = []
    entity_aliases: dict[str, str] = {}
    dropped = 0
    for entity_result, group in zip(entity_results, entity_groups):
        group_docs, group_dropped, group_aliases = entity_result
        deduped_entities.extend(group_docs)
        dropped += group_dropped
        entity_aliases.update(group_aliases)

    rewritten_relations = await asyncio.gather(*(_struct_rewrite_relation_doc(doc, entity_aliases, embd_mdl) for doc in relation_docs))
    relation_buckets: dict[tuple, list[dict]] = {}
    relation_order: list[tuple] = []
    for doc in rewritten_relations:
        key = _struct_filter_key(doc)
        if key not in relation_buckets:
            relation_buckets[key] = []
            relation_order.append(key)
        relation_buckets[key].append(doc)

    relation_results = await asyncio.gather(*(dedup_group(relation_buckets[key]) for key in relation_order))
    deduped_relations: list[dict] = []
    for relation_result in relation_results:
        group_docs, group_dropped, _ = relation_result
        deduped_relations.extend(group_docs)
        dropped += group_dropped
    return deduped_entities + deduped_relations, dropped


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


async def cleanup_timeline_isolated_entities(
    tenant_id: str,
    kb_id: str,
    doc_id: str,
    compilation_template_id: str | None = None,
) -> int:
    """Remove timeline entity rows that are not used by any relation.

    This runs after all structure flushes for the document have completed;
    otherwise an entity can look isolated in one flush and be referenced by a
    relation from a later flush. The cleanup is intentionally limited to the
    ``timeline`` compile kind.
    """
    from common import settings
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search as _rag_search

    index = _rag_search.index_name(tenant_id)
    fields = [
        "content_with_weight",
        "knowledge_graph_kwd",
        "from_entity_kwd",
        "to_entity_kwd",
    ]
    condition: dict = {
        "doc_id": [doc_id],
        "compile_kwd": ["timeline"],
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
    rows = settings.docStoreConn.get_fields(res, fields) or {}
    connected_names: set[str] = set()
    for row in rows.values():
        if row.get("knowledge_graph_kwd") != "relation":
            continue
        edge = _chain_extract_edge(row)
        if edge is not None:
            connected_names.update(name.casefold() for name in edge if name)

    orphan_ids = [
        row_id
        for row_id, row in rows.items()
        if row.get("knowledge_graph_kwd") == "entity"
        and _struct_entity_name(row).casefold() not in connected_names
    ]
    if orphan_ids:
        await thread_pool_exec(
            settings.docStoreConn.delete,
            {"id": orphan_ids},
            index,
            kb_id,
        )
        logging.info(
            "structure graph: removed %d isolated timeline entity row(s) for doc=%s template=%s",
            len(orphan_ids),
            doc_id,
            compilation_template_id or "legacy",
        )

    # Refresh the compact graph after source-row cleanup. This also handles
    # the no-relation case, where every timeline entity is isolated.
    await rebuild_structure_graph_json(
        tenant_id,
        kb_id,
        doc_id,
        "timeline",
        compilation_template_id,
    )
    return len(orphan_ids)


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
_CHAIN_CORRECTION_MAX_RELATIONS = 16
_CHAIN_CORRECTION_CONCURRENCY = 10


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

        if callable(callback):
            try:
                callback(msg=f"chain validation: {len(bad_edges)} flagged for LLM correction")
            except Exception:
                pass

    except Exception:
        logging.exception("chain validate: detection failed; skipping correction")
        return docs

    bad_edge_set = set(bad_edges)
    keep_set: set[tuple[str, str]] = set()
    correction_batches = [bad_edges[i : i + _CHAIN_CORRECTION_MAX_RELATIONS] for i in range(0, len(bad_edges), _CHAIN_CORRECTION_MAX_RELATIONS)]
    correction_semaphore = asyncio.Semaphore(_CHAIN_CORRECTION_CONCURRENCY)

    async def correct_batch(batch_no: int, batch_edges: list[tuple[str, str]]) -> set[tuple[str, str]]:
        # Fail open for a failed or malformed batch: retain its relations.
        batch_keep = set(batch_edges)
        batch_docs = [doc for edge in batch_edges for doc in edge_to_docs.get(edge, ())]
        batch_relations = [{"from": e[0], "to": e[1], "issue": "; ".join(violations.get(e, ("cross-batch conflict",)))} for e in batch_edges]
        chunk_pairs = _chain_gather_chunk_text(batch_docs, chunks_by_id)
        source_chunks_text = "\n\n".join(f"[{cid}]\n{text}" for cid, text in chunk_pairs) or "(no source chunks available)"
        prompt = CHAIN_CORRECTION_PROMPT.format(
            kind=kind,
            bad_relations_json=json.dumps(batch_relations, ensure_ascii=False),
            source_chunks_text=source_chunks_text,
        )
        try:
            async with correction_semaphore:
                res = await gen_json(
                    "You correct extracted graph relations to satisfy a strict-chain constraint.",
                    prompt,
                    chat_mdl,
                    gen_conf={"temperature": 0.0},
                )
            keep_raw = res.get("keep") if isinstance(res, dict) else None
            if isinstance(keep_raw, list):
                batch_keep = set()
                batch_edge_set = set(batch_edges)
                for item in keep_raw:
                    if not isinstance(item, dict):
                        continue
                    s, t = item.get("from"), item.get("to")
                    edge = (s.strip(), t.strip()) if isinstance(s, str) and isinstance(t, str) else None
                    if edge in batch_edge_set:
                        batch_keep.add(edge)
        except Exception:
            logging.exception("chain validate: correction batch %d failed; retaining its relations", batch_no)
        return batch_keep

    batch_keeps = await asyncio.gather(*(correct_batch(i, batch) for i, batch in enumerate(correction_batches)))
    for batch_keep in batch_keeps:
        keep_set.update(batch_keep)

    # Independent corrections can be valid inside each request but conflict
    # after their results are combined. Re-check the combined keep set and
    # give the model one final decision over the remaining conflicts.
    combined_violations = _chain_detect_violations(list(keep_set))
    if combined_violations:
        conflict_edges = list(combined_violations)
        final_keep = await correct_batch(-1, conflict_edges)
        keep_set.difference_update(conflict_edges)
        keep_set.update(final_keep)

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
    timing_context: str | None = None,
    chunks_by_id: dict[str, str] | None = None,
    chain_kind: str = "",
    chain_callback=None,
    chain_timeout_seconds: float = 120.0,
    es_waiter: Callable[[], Awaitable[None]] | None = None,
    es_releaser: Callable[[], Awaitable[None]] | None = None,
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

    if callable(cancel_check) and cancel_check():
        raise TaskCanceledException("Task was cancelled before local dedup")
    deduped, dropped = await _struct_local_dedup_parallel(
        docs,
        chat_mdl,
        embd_mdl,
        similarity_threshold,
        timing_context=timing_context,
    )

    if callable(cancel_check) and cancel_check():
        raise TaskCanceledException("Task was cancelled after local dedup")
    if chain_kind in CHAIN_KINDS:
        try:
            deduped = await asyncio.wait_for(
                validate_and_correct_chain(
                    deduped,
                    chunks_by_id or {},
                    chat_mdl,
                    chain_kind,
                    callback=chain_callback,
                ),
                timeout=chain_timeout_seconds,
            )
        except asyncio.TimeoutError:
            logging.warning("chain validate: timed out after %ss; using local-deduped docs", chain_timeout_seconds)
        except Exception:
            logging.exception("chain validate: unexpected failure; using local-deduped docs")

    if callable(cancel_check) and cancel_check():
        raise TaskCanceledException("Task was cancelled after chain validation")
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

    if es_waiter is not None:
        await es_waiter()
    _raise_if_canceled()
    try:
        inserted, updated = await _struct_es_dedup_batch(
            deduped,
            chat_mdl,
            embd_mdl,
            tenant_id,
            kb_id,
            similarity_threshold,
            timing_context=timing_context,
            cancel_check=cancel_check,
        )
    except Exception:
        logging.exception("merge_compiled_structures: batched ES dedup failed")
        inserted = updated = 0
    if es_releaser is not None:
        await es_releaser()

    graphs = 0
    for graph_index, (doc_id, compile_kwd, template_id) in enumerate(graph_keys):
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

    info = {
        "inserted": inserted,
        "updated": updated,
        "duplicates_dropped": dropped,
        "graphs": graphs,
    }
    return info


__all__ = [
    "compile_structure_from_text",
    "merge_compiled_structures",
    "cleanup_timeline_isolated_entities",
    "rebuild_structure_graph_json",
]
