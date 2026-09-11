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

import copy
import json
from functools import lru_cache
from json import JSONDecodeError
from pathlib import Path
from typing import Any, Dict

import ruamel.yaml

from core.config.utils.paths import LLM_FACTORY_PATH

yaml = ruamel.yaml.YAML(typ="safe")


def load_yaml(path: Path, allow_missing: bool = False) -> Dict[str, Any]:
    """
    Load a YAML file into a dictionary.

    Args:
        path: Path to the YAML file.
        allow_missing: If True, return empty dict when file doesn't exist.

    Returns:
        Dict[str, Any]: The loaded YAML content.
    """
    if not path.exists():
        if allow_missing:
            return {}
        raise FileNotFoundError(f"YAML config not found: {path}")
    try:
        with path.open("r", encoding="utf-8") as f:
            data = yaml.load(f) or {}
        if not isinstance(data, dict):
            raise ValueError(f"YAML config must be a dict: {path}")
        return data
    except Exception as e:
        raise RuntimeError(f"Failed to load YAML config {path}: {e}")


def merge_dicts(base: Dict[str, Any], override: Dict[str, Any]) -> Dict[str, Any]:
    """
    Merge two dictionaries recursively, values in `override` take precedence.
    """
    result = copy.deepcopy(base)
    for k, v in override.items():
        if (
            k in result
            and isinstance(result[k], dict)
            and isinstance(v, dict)
        ):
            result[k] = merge_dicts(result[k], v)
        else:
            result[k] = v
    return result


@lru_cache()
def get_llm_factories() -> list:
    try:
        with LLM_FACTORY_PATH.open("r", encoding="utf-8") as f:
            data = json.load(f)
            return data.get("factory_llm_infos", [])
    except FileNotFoundError:
        return []
    except (JSONDecodeError, KeyError) as e:
        raise RuntimeError(f"Failed to load LLM factories from {LLM_FACTORY_PATH}: {e}") from e

