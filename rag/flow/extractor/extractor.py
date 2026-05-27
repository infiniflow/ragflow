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
import json
import logging
import random
from copy import deepcopy
from typing import Tuple

from common.misc_utils import thread_pool_exec
from common.token_utils import num_tokens_from_string
from rag.nlp import rag_tokenizer
import xxhash

from agent.component.llm import LLMParam, LLM
from rag.flow.base import ProcessBase, ProcessParamBase
from rag.prompts.generator import INPUT_UTILIZATION, gen_json, run_toc_from_text, split_chunks


class ExtractorParam(ProcessParamBase, LLMParam):
    def __init__(self):
        super().__init__()
        self.field_name = ""

    def check(self):
        super().check()
        self.check_empty(self.field_name, "Result Destination")


class Extractor(ProcessBase, LLM):
    component_name = "Extractor"

    async def _build_TOC(self, docs):
        self.callback(0.2,message="Start to generate table of content ...")
        docs = sorted(docs, key=lambda d:(
            d.get("page_num_int", 0)[0] if isinstance(d.get("page_num_int", 0), list) else d.get("page_num_int", 0),
            d.get("top_int", 0)[0] if isinstance(d.get("top_int", 0), list) else d.get("top_int", 0)
        ))
        toc = await run_toc_from_text([d["text"] for d in docs], self.chat_mdl)
        logging.info("------------ T O C -------------\n"+json.dumps(toc, ensure_ascii=False, indent='  '))
        ii = 0
        while ii < len(toc):
            try:
                idx = int(toc[ii]["chunk_id"])
                del toc[ii]["chunk_id"]
                toc[ii]["ids"] = [docs[idx]["id"]]
                if ii == len(toc) -1:
                    break
                for jj in range(idx+1, int(toc[ii+1]["chunk_id"])+1):
                    toc[ii]["ids"].append(docs[jj]["id"])
            except Exception as e:
                logging.exception(e)
            ii += 1

        if toc:
            d = deepcopy(docs[-1])
            d["doc_id"] = self._canvas._doc_id
            d["content_with_weight"] = json.dumps(toc, ensure_ascii=False)
            d["toc_kwd"] = "toc"
            d["available_int"] = 0
            d["page_num_int"] = [100000000]
            d["id"] = xxhash.xxh64((d["content_with_weight"] + str(d["doc_id"])).encode("utf-8", "surrogatepass")).hexdigest()
            return d
        return None

    async def _invoke(self, **kwargs):
        self.set_output("output_format", "chunks")
        self.callback(random.randint(1, 5) / 100.0, "Start to generate.")
        inputs = self.get_input_elements()
        chunks = []
        chunks_key = ""
        args = {}
        for k, v in inputs.items():
            args[k] = v["value"]
            if isinstance(args[k], list):
                chunks = deepcopy(args[k])
                chunks_key = k

        if chunks:
            if self._param.field_name == "toc":
                for ck in chunks:
                    ck["doc_id"] = self._canvas._doc_id
                    ck["id"] = xxhash.xxh64((ck["text"] + str(ck["doc_id"])).encode("utf-8")).hexdigest()
                toc =await self._build_TOC(chunks)
                chunks.append(toc)
                self.set_output("chunks", chunks)
                return

            prog = 0
            for i, ck in enumerate(chunks):
                args[chunks_key] = ck["text"]
                msg, sys_prompt = self._sys_prompt_and_msg([], args)
                msg.insert(0, {"role": "system", "content": sys_prompt})
                ck[self._param.field_name] = await self._generate_async(msg)
                prog += 1./len(chunks)
                if i % (len(chunks)//100+1) == 1:
                    self.callback(prog, f"{i+1} / {len(chunks)}")
            self.set_output("chunks", chunks)
        else:
            msg, sys_prompt = self._sys_prompt_and_msg([], args)
            msg.insert(0, {"role": "system", "content": sys_prompt})
            self.set_output("chunks", [{self._param.field_name: await self._generate_async(msg)}])


import asyncio
import datetime
_STRUCT_TYPES = ("list", "set", "hypergraph")


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
    if isinstance(explicit, str) and explicit.lower() in _STRUCT_TYPES:
        return explicit.lower()
    output = _struct_get(parser_config, "output", default={}) or {}
    if _struct_get(output, "entities") and _struct_get(output, "relations"):
        return "hypergraph"
    return "list"


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


def _struct_hypergraph_prompts(parser_config: dict, language: str="en") -> Tuple[str, str]:
    autotype = _struct_get(parser_config, "compile_type", default="graph")
    guideline = _struct_get(parser_config, "guideline", default={}) or {}
    output = _struct_get(parser_config, "output", default={}) or {}
    options = _struct_get(parser_config, "options", default={}) or {}

    target = _struct_localize(_struct_get(guideline, "target"), language)
    rules_e = _struct_localize(_struct_get(guideline, "rules_for_entities"), language)
    rules_r = _struct_localize(_struct_get(guideline, "rules_for_relations"), language)
    rules_t = _struct_localize(_struct_get(guideline, "rules_for_time"), language)

    observation_time = _struct_get(options, "observation_time") or datetime.date.today().isoformat()
    if rules_t and "{observation_time}" in rules_t:
        rules_t = rules_t.replace("{observation_time}", observation_time)

    entities_cfg = _struct_get(output, "entities", default={}) or {}
    relations_cfg = _struct_get(output, "relations", default={}) or {}
    ent_desc = _struct_localize(_struct_get(entities_cfg, "description"), language)
    rel_desc = _struct_localize(_struct_get(relations_cfg, "description"), language)
    ent_fields_text, ent_skel = _struct_render_fields(_struct_get(entities_cfg, "fields", default=[]) or [], language)
    rel_fields_text, rel_skel = _struct_render_fields(_struct_get(relations_cfg, "fields", default=[]) or [], language)

    node_parts = [f"# Role and Task:\n{target}"] if target else []
    if rules_e:
        node_parts.append(f"## Entity Extraction Rules:\n{rules_e}")
    if ent_desc:
        node_parts.append(f"## Entity Description:\n{ent_desc}")
    node_parts.append(f"## Entity Fields:\n{ent_fields_text}")
    node_parts.append(
        "## Response Format:\n"
        "Reply with a single JSON object of the form: "
        f'{{"items": [{ent_skel}, ...]}}.\n'
        f"Auto-type: \"{autotype}\". "
        + ("Items must be unique. " if autotype == "set" else "")
        + "Return JSON only, no commentary."
    )
    node_prompt = "\n\n".join(node_parts)

    if not relations_cfg:
        return node_prompt, ""

    edge_parts = [f"# Role and Task:\n{target}"] if target else []
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

    edge_prompt = edge_prompt_template.replace("{known_nodes}", known_str)
    edge_res = await gen_json(edge_prompt, user_prompt, chat_mdl, gen_conf={"temperature": 0.1})
    edges = _struct_unwrap_items(edge_res)

    return nodes, edges


async def _struct_embed(payloads: list[str], embd_mdl) -> list:
    if not payloads:
        return []
    embeddings, _ = await thread_pool_exec(embd_mdl.encode, payloads)
    return list(embeddings)


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

    relations_cfg = _struct_get(
        _struct_get(parser_config, "output", default={}) or {},
        "relations",
        default={},
    ) or {}
    field_names = {
        f.get("name")
        for f in (_struct_get(relations_cfg, "fields", default=[]) or [])
        if isinstance(f, dict)
    }
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
) -> dict:
    """Build one ES doc for an extracted entity or relation.

    Args:
        kind: ``"entity"`` or ``"relation"`` — written to ``structure_kwd``.
        src_field / target_field: when ``kind == "relation"`` and these field
            names exist on the payload, the resolved values are written to
            ``src_name_kwd`` / ``target_name_kwd``.
    """
    content_with_weight = json.dumps(payload, ensure_ascii=False)
    if hasattr(vec, "tolist"):
        vec_list = vec.tolist()
    else:
        vec_list = list(vec)
    doc_id_str = str(doc_id)

    description = _struct_payload_description(payload)

    content_ltks = rag_tokenizer.tokenize(description) if description else ""
    content_sm_ltks = rag_tokenizer.fine_grained_tokenize(content_ltks) if content_ltks else ""

    doc = {
        "content_with_weight": content_with_weight,
        "compile_kwd": compile_kwd,
        "structure_kwd": kind,
        "doc_id": doc_id_str,
        "chunk_ids": list(chunk_ids or []),
        "content_ltks": content_ltks,
        "content_sm_ltks": content_sm_ltks,
        f"q_{len(vec_list)}_vec": vec_list,
        "id": xxhash.xxh64(
            (content_with_weight + doc_id_str).encode("utf-8", "surrogatepass")
        ).hexdigest(),
    }

    if kind == "relation":
        if src_field:
            src_val = payload.get(src_field)
            if src_val is not None and str(src_val).strip():
                doc["src_name_kwd"] = str(src_val).strip()
        if target_field:
            tgt_val = payload.get(target_field)
            if tgt_val is not None and str(tgt_val).strip():
                doc["target_name_kwd"] = str(tgt_val).strip()

    return doc


async def _struct_process_batch(
    batch: list,
    batch_idx: int,
    total: int,
    chunk_ids: list,
    autotype: str,
    parser_config: dict,
    chat_mdl,
    embd_mdl,
    doc_id: str,
    language: str,
    callback,
    semaphore,
) -> list[dict]:
    """Process one packed batch end-to-end (extract → embed → ES docs).

    The semaphore (if any) is taken around the entire batch's LLM + embedding
    work to bound peak concurrency.
    """
    if not batch:
        return []

    batch_ids: list = []
    batch_segments: list[str] = []
    for item in batch:
        for idx, text in item.items():
            cid = chunk_ids[idx]
            if cid:
                batch_ids.append(cid)
            batch_segments.append(text)
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
            embeddings = await _struct_embed(embed_inputs, embd_mdl)
        except Exception as e:
            logging.exception(f"compile_structure_from_text: embedding failed for batch {batch_idx}: {e}")
            return []

        if len(embeddings) != len(payloads):
            logging.error(
                f"compile_structure_from_text: embedding count mismatch ({len(embeddings)} vs {len(payloads)}) for batch {batch_idx}"
            )
            return []

        docs = [
            _struct_to_es_doc(
                payload, autotype, doc_id, batch_ids, vec, kind,
                src_field=src_field, target_field=target_field,
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
                "chunk_ids": [<chunk_id>, ...],
                "q_<dim>_vec": [...],
                "id": <xxhash>,
            }
    """
    parser_config = {
        "compile_type": "list",
        "guideline": {
            "target": "你是一位专业的合规分析师，负责从合规文档中准确提取结构化的合规要求。",
            "rules_for_entities": """- '提取的合规要求应完整保留原文的核心含义'
      - '依据法规应标注具体的法规名称和条款'
      - '完成期限应明确标注时间节点'
      - '责任人应尽可能具体到部门或个人'"""
        },
        "output": {
            "entities": {
            "description": "合规要求和监管事项清单",
            "fields":[
              {
                "name": "requirement",
                "type": "str",
                "description":'合规要求描述',
                "required": True
              },
              {
                "name": "regulation",
                "type": "str",
                "description":'依据法规',
                "required": True
              },
              {
                "name": "deadline",
                "type": "str",
                "description":'完成期限',
                "required": False
              },
              {
                "name": "responsible_party",
                "type": "str",
                "description":'责任人或责任部门',
                "required": False
              }
            ]}
        }
    }
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
    if autotype not in _STRUCT_TYPES:
        logging.error(f"compile_structure_from_text: unsupported type '{autotype}'")
        return []

    chunk_ids: list = []
    chunk_texts: list[str] = []
    for chunk in chunks:
        text = chunk.get("text") or chunk.get("content_with_weight") or chunk.get("content") or ""
        if not isinstance(text, str) or not text.strip():
            continue
        chunk_ids.append(chunk.get("id") or chunk.get("chunk_id"))
        chunk_texts.append(text)

    if not chunk_texts:
        return []

    node_prompt, edge_prompt = _struct_hypergraph_prompts(parser_config, language)
    prompt_overhead = max(num_tokens_from_string(node_prompt), num_tokens_from_string(edge_prompt))

    input_budget = int(chat_mdl.max_length * INPUT_UTILIZATION) - prompt_overhead
    if input_budget < 1024:
        input_budget = 1024

    batches = split_chunks(chunk_texts, input_budget)
    total = max(1, len(batches))
    semaphore = asyncio.Semaphore(max_workers) if max_workers and max_workers > 0 else None

    tasks = [
        asyncio.create_task(
            _struct_process_batch(
                batch, bi, total, chunk_ids, autotype,
                parser_config, chat_mdl, embd_mdl, doc_id,
                language, callback, semaphore,
            )
        )
        for bi, batch in enumerate(batches)
        if batch
    ]

    if not tasks:
        return []

    try:
        batch_results = await asyncio.gather(*tasks, return_exceptions=False)
    except Exception:
        for t in tasks:
            t.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        raise

    results: list[dict] = []
    for br in batch_results:
        if br:
            results.extend(br)
    return results


# ---------------------------------------------------------------------------
# Structured-knowledge merging: local dedup + ES dedup
# ---------------------------------------------------------------------------
#
# Pipeline (per spec):
#   Phase 1 — Local dedup inside `docs`:
#     - Group by (doc_id, compile_kwd, src_name_kwd?, target_name_kwd?).
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


def _struct_filter_key(doc: dict) -> tuple:
    """Bucket key for dedup candidates."""
    return (
        doc.get("doc_id"),
        doc.get("compile_kwd"),
        doc.get("src_name_kwd"),
        doc.get("target_name_kwd"),
    )


def _struct_doc_vec(doc: dict):
    """Return (vector_field_name, vector_values) or (None, None)."""
    for k, v in doc.items():
        if isinstance(k, str) and k.startswith("q_") and k.endswith("_vec"):
            return k, v
    return None, None


def _struct_union_chunk_ids(a: list, b: list) -> list:
    """Order-preserving union."""
    seen = []
    for src in (a or [], b or []):
        for cid in src:
            if cid and cid not in seen:
                seen.append(cid)
    return seen


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
    values — src_name_kwd / target_name_kwd must not change across a merge.
    """
    if existing.get("structure_kwd") != "relation":
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
    overlay identity fields (id, src_name_kwd, target_name_kwd) from base_doc.
    """
    kind = base_doc.get("structure_kwd") or "entity"
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
    # The spec forbids changing src_name_kwd / target_name_kwd on a merge.
    for kwd in ("src_name_kwd", "target_name_kwd"):
        if kwd in base_doc and base_doc[kwd]:
            new_doc[kwd] = base_doc[kwd]
    return new_doc


async def _struct_reembed_payload(payload: dict, embd_mdl):
    """Re-encode a merged payload's description with embd_mdl and return the vector."""
    text = _struct_payload_description(payload)
    vecs = await _struct_embed([text], embd_mdl)
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
                existing.get("chunk_ids"), incoming.get("chunk_ids"),
            )
            new_vec = await _struct_reembed_payload(merged_payload, embd_mdl)
            if new_vec is None:
                # Re-embed failed: keep existing, drop incoming silently.
                dropped += 1
                continue
            rebuilt = _struct_rebuild_es_doc(
                merged_payload, existing, new_vec, merged_chunk_ids, preserve_id=True,
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
    if doc.get("structure_kwd"):
        condition["structure_kwd"] = [doc["structure_kwd"]]
    if doc.get("src_name_kwd"):
        condition["src_name_kwd"] = [doc["src_name_kwd"]]
    if doc.get("target_name_kwd"):
        condition["target_name_kwd"] = [doc["target_name_kwd"]]

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
        "id", "content_with_weight", "chunk_ids", "structure_kwd", "compile_kwd",
        "doc_id", "src_name_kwd", "target_name_kwd",
    ]
    try:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            select_fields, [], condition, [match_expr], OrderByExpr(),
            0, 1, index, [kb_id],
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
        old_doc.get("chunk_ids"), doc.get("chunk_ids"),
    )
    new_vec = await _struct_reembed_payload(merged_payload, embd_mdl)
    if new_vec is None:
        return "skipped"

    rebuilt = _struct_rebuild_es_doc(
        merged_payload, old_doc, new_vec, merged_chunk_ids, preserve_id=True,
    )
    update_fields = {k: v for k, v in rebuilt.items() if k != "id"}
    try:
        await thread_pool_exec(
            settings.docStoreConn.update,
            {"id": old_id}, update_fields, index, kb_id,
        )
        return "updated"
    except Exception:
        logging.exception("merge_compiled_structures: ES update failed for id %s", old_id)
        return "skipped"


async def merge_compiled_structures(
    docs: list[dict],
    chat_mdl,
    embd_mdl,
    tenant_id: str,
    kb_id: str,
    similarity_threshold: float = 0.9,
) -> dict:
    """Merge ``docs`` (the output of ``compile_structure_from_text``) before
    inserting them into ES.

    Two phases:
        1. **Local dedup**: bucket by (doc_id, compile_kwd, src_name_kwd?,
           target_name_kwd?), pairwise cosine similarity over the q_<dim>_vec
           field via ``sklearn.metrics.pairwise.cosine_similarity``; pairs
           above ``similarity_threshold`` go through ``_struct_merge_pair``
           (LLM-judged). On a duplicate verdict the surviving entry is
           rebuilt from the merged payload (union of ``chunk_ids``,
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

    Returns:
        {"inserted": N, "updated": M, "duplicates_dropped": K} summary.
    """
    if not docs:
        return {"inserted": 0, "updated": 0, "duplicates_dropped": 0}

    deduped, dropped = await _struct_local_dedup(
        docs, chat_mdl, embd_mdl, similarity_threshold,
    )

    inserted = 0
    updated = 0
    for d in deduped:
        try:
            result = await _struct_es_dedup_one(
                d, chat_mdl, embd_mdl, tenant_id, kb_id, similarity_threshold,
            )
        except Exception:
            logging.exception("merge_compiled_structures: per-doc dedup failed")
            continue
        if result == "inserted":
            inserted += 1
        elif result == "updated":
            updated += 1

    return {
        "inserted": inserted,
        "updated": updated,
        "duplicates_dropped": dropped,
    }