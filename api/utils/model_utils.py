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
from common.constants import ModelTypeBinary

_LEGACY_MODEL_TYPE_ALIASES = {
    "speech2text": "asr",
    "image2text": "vision",
}


def get_model_type_human(model_type: int | str | None) -> list[str]:
    """Convert binary model_type (or integer string / legacy type string) to list of human-readable model type names."""
    if model_type is None:
        return []
    if isinstance(model_type, str):
        try:
            model_type = int(model_type)
        except ValueError:
            canonical = _LEGACY_MODEL_TYPE_ALIASES.get(model_type.lower(), model_type.lower())
            type_value_map = {mt.name.lower(): mt.value for mt in ModelTypeBinary}
            if canonical in type_value_map:
                return [canonical]
            return []
    if not isinstance(model_type, int):
        return []
    return [mt.name.lower() for mt in ModelTypeBinary if model_type & mt.value]


def normalize_model_types(model_type_name_list: list[str] | str) -> list[str]:
    """Return model type names using the canonical API/runtime identifiers."""
    if isinstance(model_type_name_list, str):
        model_type_name_list = [model_type_name_list]
    elif not isinstance(model_type_name_list, list):
        return []

    normalized = []
    for model_type in model_type_name_list:
        canonical = _LEGACY_MODEL_TYPE_ALIASES.get(model_type, model_type)
        if canonical not in normalized:
            normalized.append(canonical)
    return normalized


def calculate_model_type(model_type_name_list: list[str] | str) -> int:
    model_type = 0
    type_value_map = {mt.name.lower(): mt.value for mt in ModelTypeBinary}
    for mt in normalize_model_types(model_type_name_list):
        if mt in type_value_map:
            model_type |= type_value_map[mt]
    return model_type
