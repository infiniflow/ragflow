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
import json


def normalize_replicate_api_key(key):
    if isinstance(key, dict):
        return key.get("api_key") or json.dumps(key)
    if isinstance(key, str):
        try:
            payload = json.loads(key)
        except (json.JSONDecodeError, TypeError):
            return key
        if isinstance(payload, dict) and payload.get("api_key"):
            return payload["api_key"]
    return key
