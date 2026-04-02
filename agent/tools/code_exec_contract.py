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

from __future__ import annotations

import json
from collections.abc import Mapping
from typing import Any


SYSTEM_OUTPUT_KEYS = frozenset(
    {
        "content",
        "actual_type",
        "_ERROR",
        "_ARTIFACTS",
        "_ATTACHMENT_CONTENT",
        "raw_result",
        "_created_time",
        "_elapsed_time",
    }
)


class ContractError(ValueError):
    pass


def _validate_business_output_name(name: str) -> None:
    if not name or not name.strip():
        raise ContractError("CodeExec business output name must not be empty")
    if name in SYSTEM_OUTPUT_KEYS:
        raise ContractError(f"CodeExec reserved output name is not allowed: {name}")
    if "." in name:
        raise ContractError(f"CodeExec business output name must not contain '.': {name}")


def select_business_output(outputs: Mapping[str, Any]) -> tuple[str, Any]:
    business_outputs = [(name, meta) for name, meta in outputs.items() if name not in SYSTEM_OUTPUT_KEYS]
    if len(business_outputs) != 1:
        raise ContractError(
            f"CodeExec contract must contain exactly one business output, got {len(business_outputs)}"
        )
    _validate_business_output_name(business_outputs[0][0])
    return business_outputs[0]


def normalize_output_value(value: Any) -> Any:
    if isinstance(value, (tuple, list)):
        return [normalize_output_value(item) for item in value]
    if isinstance(value, dict):
        return {key: normalize_output_value(item) for key, item in value.items()}
    return value


def infer_actual_type(value: Any) -> str:
    value = normalize_output_value(value)
    if value is None:
        return "Null"
    if isinstance(value, bool):
        return "Boolean"
    if _is_number(value):
        return "Number"
    if isinstance(value, str):
        return "String"
    if isinstance(value, dict):
        return "Object"
    if isinstance(value, list):
        if not value:
            return "Array<Any>"
        inferred = {infer_actual_type(item) for item in value}
        if len(inferred) == 1:
            return f"Array<{inferred.pop()}>"
        return "Array<Any>"
    return "Any"


def render_canonical_content(value: Any) -> str:
    value = normalize_output_value(value)
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    if isinstance(value, (dict, list)):
        return json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True)
    return str(value)


def _is_number(value: Any) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool)


def _validate_top_level_value_domain(value: Any) -> None:
    allowed = value is None or isinstance(value, (bool, str, dict, list)) or _is_number(value)
    if not allowed:
        raise ContractError(
            f"CodeExec unsupported top-level result type: {type(value).__name__}. "
            "Allowed top-level values are String, Number, Boolean, Object, Array, or Null."
        )


def _normalize_expected_type(expected_type: str) -> str:
    etype = expected_type.strip()
    low = etype.lower()
    simple_types = {
        "string": "String",
        "number": "Number",
        "boolean": "Boolean",
        "object": "Object",
        "null": "Null",
        "any": "Any",
    }
    if low in simple_types:
        return simple_types[low]
    if low.startswith("array<") and low.endswith(">"):
        inner = etype[etype.find("<") + 1 : -1].strip()
        if not inner:
            raise ContractError(f"Unsupported expected type: {expected_type}")
        return f"Array<{_normalize_expected_type(inner)}>"
    return etype


def _validate_expected_type(expected_type: str, value: Any, path: str = "") -> None:
    etype = _normalize_expected_type(expected_type)
    if not etype or etype.lower() == "any":
        return

    value = normalize_output_value(value)

    if etype.startswith("Array<") and etype.endswith(">"):
        inner_type = etype[6:-1].strip()
        if not isinstance(value, list):
            raise ContractError(
                f"CodeExec contract mismatch at {path or 'value'}: expected type {etype}, got {infer_actual_type(value)}"
            )
        for index, item in enumerate(value):
            child_path = f"{path}[{index}]" if path else f"[{index}]"
            _validate_expected_type(inner_type, item, child_path)
        return

    actual_type = infer_actual_type(value)
    if etype == "String":
        valid = isinstance(value, str)
    elif etype == "Number":
        valid = _is_number(value)
    elif etype == "Boolean":
        valid = isinstance(value, bool)
    elif etype == "Object":
        valid = isinstance(value, dict)
    elif etype == "Null":
        valid = value is None
    else:
        raise ContractError(f"Unsupported expected type: {expected_type}")

    if not valid:
        raise ContractError(
            f"CodeExec contract mismatch at {path or 'value'}: expected type {etype}, got {actual_type}"
        )


def build_code_exec_contract(outputs: Mapping[str, Any], raw_result: Any) -> dict[str, Any]:
    business_name, business_meta = select_business_output(outputs)
    expected_type = ""
    if isinstance(business_meta, Mapping):
        expected_type = str(business_meta.get("type") or "")

    normalized_value = normalize_output_value(raw_result)
    _validate_top_level_value_domain(normalized_value)
    _validate_expected_type(expected_type, normalized_value)

    return {
        "business_output": business_name,
        "value": normalized_value,
        "actual_type": infer_actual_type(normalized_value),
        "content": render_canonical_content(normalized_value),
    }
