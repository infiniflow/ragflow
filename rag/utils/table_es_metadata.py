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

"""Table manual-mode ES field resolution and document metadata aggregation (lightweight; used by task_executor)."""

import logging

from common import settings
from common.metadata_utils import dedupe_list


def _knowledgebase_service_cls():
    """Lazy import for KnowledgebaseService (used by aggregate; mockable in unit tests)."""
    from api.db.services.knowledgebase_service import KnowledgebaseService

    return KnowledgebaseService


def merge_table_parser_config_from_kb(task: dict) -> dict:
    """Merge dataset-level table parser keys into document parser_config (see build_chunks)."""
    pc = task.get("parser_config") or {}
    if task.get("parser_id", "").lower() != "table" or not task.get("kb_parser_config"):
        return pc
    out = dict(pc)
    kb_pc = task["kb_parser_config"]
    for _k in ("table_column_mode", "table_column_roles", "table_column_names"):
        if _k in kb_pc:
            out[_k] = kb_pc[_k]
    return out


def table_parser_strip_doc_metadata_keys(eff_parser_config: dict) -> frozenset[str]:
    """
    Table manual mode stores per-column values under document metadata keys equal to the
    CSV column name. On reparse, strip these keys from existing metadata before merging
    a fresh aggregate so columns switched to indexing-only (or removed) do not persist.
    """
    names = eff_parser_config.get("table_column_names")
    if names:
        return frozenset(str(n).strip() for n in names if n is not None and str(n).strip())
    roles = eff_parser_config.get("table_column_roles") or {}
    return frozenset(str(k).strip() for k in roles if k is not None and str(k).strip())


def _field_map_typed_key_for_column(field_map: dict, col: str) -> str | None:
    """Map CSV column name to ES typed field key (field_map: typed_key -> display name)."""
    if not field_map or not col:
        return None
    col_s = str(col).strip()
    col_norm = col_s.replace("_", " ").strip().lower()
    for tk, disp in field_map.items():
        disp_s = str(disp).strip()
        if disp_s.lower() == col_norm or disp_s.lower() == col_s.lower():
            return tk
    return None


def _probe_es_typed_key_for_column(col: str, sample_chunk: dict) -> str | None:
    """
    When field_map is missing/stale, try to infer the ES field key present on a chunk.
    Table chunks use normalized/pinyin keys of the form <normalized_base><suffix>, where suffix is
    one of: _raw, _tks, _dt, _long, _flt, _kwd (see rag/app/table.py).
    """
    if not col or not isinstance(sample_chunk, dict):
        return None
    base_raw = str(col).strip()
    if not base_raw:
        return None
    base_norm = base_raw.replace("_", " ").strip().lower().replace(" ", "")
    suffixes = ("_tks", "_raw", "_dt", "_long", "_flt", "_kwd")
    for key in sample_chunk.keys():
        key_s = str(key)
        if not key_s:
            continue
        key_norm = key_s.strip().lower()
        if key_norm == base_raw.lower() or key_norm.replace("_", "").replace(" ", "") == base_norm:
            return key_s
    for key in sample_chunk.keys():
        key_s = str(key)
        if not key_s:
            continue
        key_lower = key_s.lower()
        for sfx in suffixes:
            if key_lower.endswith(sfx):
                core = key_lower[: -len(sfx)]
                core_norm = core.replace("_", "").replace(" ", "")
                if core_norm == base_norm:
                    return key_s
    return None


def _resolve_es_chunk_field_key(
    col: str, field_map: dict, sample_chunk: dict | None
) -> tuple[str | None, str]:
    """Prefer field_map when key exists on chunk; else probe by suffix (matches table.py naming)."""
    tk_fm = _field_map_typed_key_for_column(field_map, col) if field_map else None
    if sample_chunk:
        if tk_fm and tk_fm in sample_chunk:
            return tk_fm, "field_map"
        probed = _probe_es_typed_key_for_column(col, sample_chunk)
        if probed:
            return probed, "probe" if not tk_fm else "probe_field_map_mismatch"
        if tk_fm:
            return tk_fm, "field_map_absent_on_chunk"
    if tk_fm:
        return tk_fm, "field_map"
    return None, "none"


def _value_to_meta_string(val) -> str | None:
    """Normalize chunk field values for DocMetadataService (strings / list of strings only)."""
    if val is None:
        return None
    if isinstance(val, bool):
        return str(val).lower()
    if isinstance(val, (int, float)):
        return str(val)
    if isinstance(val, str):
        s = val.strip()
        return s if s else None
    return str(val)


def _es_raw_field_key_from_typed(tk: str | None) -> str | None:
    """ES text columns use *_tks (tokenized); raw display value is stored as {same_base}_raw (see rag/app/table.py)."""
    if not tk or not tk.endswith("_tks"):
        return None
    return tk[: -len("_tks")] + "_raw"


def _es_field_value_to_doc_metadata(val, *, from_tks_fallback: bool) -> str | None:
    """Prefer raw strings; for legacy *_tks tokenized fields, normalize list/str to a single display string."""
    if val is None:
        return None
    if from_tks_fallback and isinstance(val, list):
        parts = [str(x).strip() for x in val if x is not None and str(x).strip()]
        if not parts:
            return None
        return " ".join(parts)
    return _value_to_meta_string(val)


def aggregate_table_manual_doc_metadata(chunks: list, task: dict) -> dict:
    """
    Collect unique values per metadata/both column across chunks for document-level metadata.
    Used when table_column_mode == manual (parallel to LLM gen_metadata, no schema required).
    """
    logging.debug(
        f"[TABLE_META_DEBUG] aggregate_table_manual_doc_metadata called with {len(chunks)} chunks"
    )
    eff = merge_table_parser_config_from_kb(task)
    if eff.get("table_column_mode") != "manual":
        logging.debug(
            f"[TABLE_META_DEBUG] skip aggregate: table_column_mode={eff.get('table_column_mode')!r}"
        )
        return {}
    roles = eff.get("table_column_roles") or {}
    table_column_names = eff.get("table_column_names") or []
    if table_column_names:
        meta_cols = [
            col
            for col in table_column_names
            if roles.get(col, "both") in ("metadata", "both")
        ]
    else:
        meta_cols = [c for c, r in roles.items() if r in ("metadata", "both")]
    if not meta_cols:
        logging.debug(
            "[TABLE_META_DEBUG] skip aggregate: no metadata/both columns "
            f"(table_column_names_present={bool(table_column_names)})"
        )
        return {}
    fm = (task.get("kb_parser_config") or {}).get("field_map") or {}
    kb_id = task.get("kb_id")
    if not fm and kb_id:
        try:
            KBS = _knowledgebase_service_cls()
            ok, kb = KBS.get_by_id(kb_id)
            if ok and kb:
                fresh_pc = kb.parser_config or {}
                reloaded = fresh_pc.get("field_map") or {}
                if reloaded:
                    fm = reloaded
                    logging.debug(
                        f"[TABLE_META_DEBUG] reloaded field_map from DB: {len(fm)} entries"
                    )
                else:
                    logging.debug(
                        "[TABLE_META_DEBUG] KB reload: parser_config has no field_map yet; "
                        "will use ES key probe on chunk dicts if applicable"
                    )
        except Exception as e:
            logging.debug(
                "[TABLE_META_DEBUG] failed to reload field_map from DB: %s",
                e,
                exc_info=True,
            )
    if not fm and not (settings.DOC_ENGINE_INFINITY or settings.DOC_ENGINE_OCEANBASE):
        logging.debug(
            "[TABLE_META_DEBUG] field_map empty on task snapshot — will use ES key probe on chunk dicts; "
            f"kb_parser_config keys={list((task.get('kb_parser_config') or {}).keys())}"
        )
    logging.debug(
        f"[TABLE_META_DEBUG] meta_cols={meta_cols}, field_map entries={len(fm)}, "
        f"infinity={settings.DOC_ENGINE_INFINITY}, oceanbase={settings.DOC_ENGINE_OCEANBASE}"
    )
    sample_ck = next((c for c in chunks if isinstance(c, dict)), None)
    if sample_ck:
        sk = [
            k
            for k in sample_ck.keys()
            if not (str(k).startswith("q_") and str(k).endswith("_vec"))
        ][:50]
        logging.debug(f"[TABLE_META_DEBUG] first chunk non-vector keys (sample): {sk}")

    es_col_keys: dict[str, tuple[str | None, str]] = {}
    if not (settings.DOC_ENGINE_INFINITY or settings.DOC_ENGINE_OCEANBASE):
        for col in meta_cols:
            tk, src = _resolve_es_chunk_field_key(col, fm, sample_ck)
            es_col_keys[col] = (tk, src)
            logging.debug(
                f"[TABLE_META_DEBUG] column '{col}' -> ES key {tk!r} (source={src})"
            )

    acc: dict[str, list] = {c: [] for c in meta_cols}

    for i, ck in enumerate(chunks):
        if not isinstance(ck, dict):
            continue
        if settings.DOC_ENGINE_INFINITY or settings.DOC_ENGINE_OCEANBASE:
            cd = ck.get("chunk_data")
            if not isinstance(cd, dict):
                continue
            for col in meta_cols:
                if col not in cd:
                    continue
                s = _value_to_meta_string(cd[col])
                if s is not None:
                    acc[col].append(s)
        else:
            for col in meta_cols:
                tk, _src = es_col_keys.get(col, (None, "none"))
                if not tk:
                    if i == 0:
                        logging.debug(
                            f"[TABLE_META_DEBUG] no resolved ES key for column '{col}'"
                        )
                    continue
                raw_k = _es_raw_field_key_from_typed(tk)
                val = None
                from_tks = False
                if raw_k and raw_k in ck:
                    val = ck[raw_k]
                elif tk in ck:
                    val = ck[tk]
                    from_tks = tk.endswith("_tks")
                else:
                    if i == 0:
                        logging.debug(
                            f"[TABLE_META_DEBUG] chunk missing ES field {tk!r}"
                            f"{' and ' + raw_k + ' (raw)' if raw_k else ''} for column '{col}'"
                        )
                    continue
                s = _es_field_value_to_doc_metadata(val, from_tks_fallback=from_tks)
                if s is not None:
                    acc[col].append(s)

    for col, vals in acc.items():
        logging.debug(
            "[TABLE_META_DEBUG] Column '%s' values found (count=%d)",
            col,
            len(vals),
        )

    out = {}
    for col, vals in acc.items():
        if vals:
            out[col] = dedupe_list(vals)
    logging.debug(
        f"[TABLE_META_DEBUG] aggregated metadata dict keys={list(out.keys())}, "
        f"sizes={[len(v) for v in out.values()]}"
    )
    return out
