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
import ast
import logging
from typing import Any, Callable, Dict

import json_repair

from rag.prompts.generator import gen_meta_filter


def convert_conditions(metadata_condition):
    if metadata_condition is None:
        metadata_condition = {}
    op_mapping = {
        "is": "=",
        "not is": "≠"
    }
    return [
        {
            "op": op_mapping.get(cond["comparison_operator"], cond["comparison_operator"]),
            "key": cond["name"],
            "value": cond["value"]
        }
        for cond in metadata_condition.get("conditions", [])
    ]


def meta_filter(metas: dict, filters: list[dict], logic: str = "and"):
    doc_ids = set([])

    def filter_out(v2docs, operator, value):
        ids = []
        for input, docids in v2docs.items():

            if operator in ["=", "≠", ">", "<", "≥", "≤"]:
                try:
                    if isinstance(input, list):
                        input = input[0]
                    input = ast.literal_eval(input)
                    value = ast.literal_eval(value)
                except Exception:
                    pass
            if isinstance(input, str):
                input = input.lower()
            if isinstance(value, str):
                value = value.lower()

            matched = False
            try:
                if operator == "contains":
                    matched = input in value if not isinstance(input, list) else all(i in value for i in input)
                elif operator == "not contains":
                    matched = input not in value if not isinstance(input, list) else all(i not in value for i in input)
                elif operator == "in":
                    matched = input in value if not isinstance(input, list) else all(i in value for i in input)
                elif operator == "not in":
                    matched = input not in value if not isinstance(input, list) else all(i not in value for i in input)
                elif operator == "start with":
                    matched = str(input).lower().startswith(str(value).lower()) if not isinstance(input, list) else "".join([str(i).lower() for i in input]).startswith(str(value).lower())
                elif operator == "end with":
                    matched = str(input).lower().endswith(str(value).lower()) if not isinstance(input, list) else "".join([str(i).lower() for i in input]).endswith(str(value).lower())
                elif operator == "empty":
                    matched = not input
                elif operator == "not empty":
                    matched = bool(input)
                elif operator == "=":
                    matched = input == value
                elif operator == "≠":
                    matched = input != value
                elif operator == ">":
                    matched = input > value
                elif operator == "<":
                    matched = input < value
                elif operator == "≥":
                    matched = input >= value
                elif operator == "≤":
                    matched = input <= value
            except Exception:
                pass

            if matched:
                ids.extend(docids)
        return ids

    for k, v2docs in metas.items():
        for f in filters:
            if k != f["key"]:
                continue
            ids = filter_out(v2docs, f["op"], f["value"])
            if not doc_ids:
                doc_ids = set(ids)
            else:
                if logic == "and":
                    doc_ids = doc_ids & set(ids)
                else:
                    doc_ids = doc_ids | set(ids)
            if not doc_ids:
                return []
    return list(doc_ids)


async def apply_meta_data_filter(
    meta_data_filter: dict | None,
    metas: dict,
    question: str,
    chat_mdl: Any = None,
    base_doc_ids: list[str] | None = None,
    manual_value_resolver: Callable[[dict], dict] | None = None,
) -> list[str] | None:
    """
    Apply metadata filtering rules and return the filtered doc_ids.

    meta_data_filter supports three modes:
    - auto: generate filter conditions via LLM (gen_meta_filter)
    - semi_auto: generate conditions using selected metadata keys only
    - manual: directly filter based on provided conditions

    Returns:
        list of doc_ids, ["-999"] when manual filters yield no result, or None
        when auto/semi_auto filters return empty.
    """
    doc_ids = list(base_doc_ids) if base_doc_ids else []

    if not meta_data_filter:
        return doc_ids

    method = meta_data_filter.get("method")

    if method == "auto":
        filters: dict = await gen_meta_filter(chat_mdl, metas, question)
        doc_ids.extend(meta_filter(metas, filters["conditions"], filters.get("logic", "and")))
        if not doc_ids:
            return None
    elif method == "semi_auto":
        selected_keys = meta_data_filter.get("semi_auto", [])
        if selected_keys:
            filtered_metas = {key: metas[key] for key in selected_keys if key in metas}
            if filtered_metas:
                filters: dict = await gen_meta_filter(chat_mdl, filtered_metas, question)
                doc_ids.extend(meta_filter(metas, filters["conditions"], filters.get("logic", "and")))
                if not doc_ids:
                    return None
    elif method == "manual":
        filters = meta_data_filter.get("manual", [])
        if manual_value_resolver:
            filters = [manual_value_resolver(flt) for flt in filters]
        doc_ids.extend(meta_filter(metas, filters, meta_data_filter.get("logic", "and")))
        if filters and not doc_ids:
            doc_ids = ["-999"]

    return doc_ids


def dedupe_list(values: list) -> list:
    seen = set()
    deduped = []
    for item in values:
        key = str(item)
        if key in seen:
            continue
        seen.add(key)
        deduped.append(item)
    return deduped


def update_metadata_to(metadata, meta):
    if not meta:
        return metadata
    if isinstance(meta, str):
        try:
            meta = json_repair.loads(meta)
        except Exception:
            logging.error("Meta data format error.")
            return metadata
    if not isinstance(meta, dict):
        return metadata

    for k, v in meta.items():
        if isinstance(v, list):
            v = [vv for vv in v if isinstance(vv, str)]
            if not v:
                continue
            v = dedupe_list(v)
        if not isinstance(v, list) and not isinstance(v, str):
            continue
        if k not in metadata:
            metadata[k] = v
            continue
        if isinstance(metadata[k], list):
            if isinstance(v, list):
                metadata[k].extend(v)
            else:
                metadata[k].append(v)
            metadata[k] = dedupe_list(metadata[k])
        else:
            metadata[k] = v

    return metadata


def metadata_schema(metadata: list|None) -> Dict[str, Any]:
    if not metadata:
        return {}
    properties = {}

    for item in metadata:
        key = item.get("key")
        if not key:
            continue

        prop_schema = {
            "description": item.get("description", "")
        }
        if "enum" in item and item["enum"]:
            prop_schema["enum"] = item["enum"]
            prop_schema["type"] = "string"

        properties[key] = prop_schema

    json_schema = {
        "type": "object",
        "properties": properties,
    }

    json_schema["additionalProperties"] = False
    return json_schema
