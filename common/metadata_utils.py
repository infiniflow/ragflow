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
from typing import Any, Callable

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
                    input = float(input)
                    value = float(value)
                except Exception:
                    input = str(input)
                    value = str(value)

            for conds in [
                (operator == "contains", str(value).lower() in str(input).lower()),
                (operator == "not contains", str(value).lower() not in str(input).lower()),
                (operator == "in", str(input).lower() in str(value).lower()),
                (operator == "not in", str(input).lower() not in str(value).lower()),
                (operator == "start with", str(input).lower().startswith(str(value).lower())),
                (operator == "end with", str(input).lower().endswith(str(value).lower())),
                (operator == "empty", not input),
                (operator == "not empty", input),
                (operator == "=", input == value),
                (operator == "≠", input != value),
                (operator == ">", input > value),
                (operator == "<", input < value),
                (operator == "≥", input >= value),
                (operator == "≤", input <= value),
            ]:
                try:
                    if all(conds):
                        ids.extend(docids)
                        break
                except Exception:
                    pass
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
