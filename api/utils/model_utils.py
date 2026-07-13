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
from typing import List

from common.constants import ModelTypeBinary


def get_model_type_human(model_type: int) -> List[str]:
    return [mt.name.lower() for mt in ModelTypeBinary if model_type & mt.value]


_LEGACY_MODEL_TYPE_ALIASES = {
    "speech2text": "asr",
    "image2text": "vision",
}


def calculate_model_type(model_type_name_list: List[str]|str) -> int:
    model_type = 0
    if isinstance(model_type_name_list, str):
        model_type_name_list = [model_type_name_list]
    type_value_map = {mt.name.lower(): mt.value for mt in ModelTypeBinary}
    for mt in model_type_name_list:
        normalized_mt = _LEGACY_MODEL_TYPE_ALIASES.get(mt, mt)
        if normalized_mt in type_value_map:
            model_type |= type_value_map[normalized_mt]
    return model_type
