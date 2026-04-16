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

import ast
import json
import math


def parse_tag_features(raw, *, allow_json_string=True, allow_python_literal=False):
    if raw is None:
        return {}

    parsed = raw
    if isinstance(raw, str):
        raw = raw.strip()
        if not raw:
            return {}
        parsed = None
        if allow_json_string:
            try:
                parsed = json.loads(raw)
            except Exception:
                parsed = None
        if parsed is None and allow_python_literal:
            try:
                parsed = ast.literal_eval(raw)
            except Exception:
                parsed = None
        if parsed is None:
            return {}
    elif not isinstance(raw, dict):
        return {}

    if not isinstance(parsed, dict):
        return {}

    cleaned = {}
    for key, value in parsed.items():
        if not isinstance(key, str):
            continue
        key = key.strip()
        if not key:
            continue
        if isinstance(value, bool):
            continue
        if isinstance(value, (int, float)) and math.isfinite(float(value)):
            cleaned[key] = float(value)
    return cleaned


def validate_tag_features(raw):
    if raw is None:
        return None

    if not isinstance(raw, dict):
        raise ValueError("must be an object mapping string tags to finite numeric scores")

    cleaned = {}
    for key, value in raw.items():
        if not isinstance(key, str):
            raise ValueError("keys must be strings")
        key = key.strip()
        if not key:
            raise ValueError("keys must be non-empty strings")
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            raise ValueError("values must be finite numbers")
        numeric = float(value)
        if not math.isfinite(numeric):
            raise ValueError("values must be finite numbers")
        cleaned[key] = numeric

    return cleaned
