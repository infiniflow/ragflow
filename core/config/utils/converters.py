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

def str_to_bool(env_val: str, default: bool = False) -> bool:
    """
    Convert an environment variable string to a boolean.

    True values: "1", "true", "yes" (case-insensitive)
    False values: "0", "false", "no" (case-insensitive)
    If env_val is None or not recognized, return `default`.
    """
    if env_val is None:
        return default
    val = env_val.strip().lower()
    if val in ("1", "true", "yes"):
        return True
    elif val in ("0", "false", "no"):
        return False
    return default
