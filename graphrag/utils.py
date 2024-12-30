# Copyright (c) 2024 Microsoft Corporation.
# Licensed under the MIT License
"""
Reference:
 - [graphrag](https://github.com/microsoft/graphrag)
"""

import html
import json
import re
from typing import Any, Callable

import numpy as np
import xxhash

from rag.utils.redis_conn import REDIS_CONN

ErrorHandlerFn = Callable[[BaseException | None, str | None, dict | None], None]


def perform_variable_replacements(
    input: str, history: list[dict] | None = None, variables: dict | None = None
) -> str:
    """Perform variable replacements on the input string and in a chat log."""
    if history is None:
        history = []
    if variables is None:
        variables = {}
    result = input

    def replace_all(input: str) -> str:
        result = input
        for k, v in variables.items():
            result = result.replace(f"{{{k}}}", v)
        return result

    result = replace_all(result)
    for i, entry in enumerate(history):
        if entry.get("role") == "system":
            entry["content"] = replace_all(entry.get("content") or "")

    return result


def clean_str(input: Any) -> str:
    """Clean an input string by removing HTML escapes, control characters, and other unwanted characters."""
    # If we get non-string input, just give it back
    if not isinstance(input, str):
        return input

    result = html.unescape(input.strip())
    # https://stackoverflow.com/questions/4324790/removing-control-characters-from-a-string-in-python
    return re.sub(r"[\"\x00-\x1f\x7f-\x9f]", "", result)


def dict_has_keys_with_types(
    data: dict, expected_fields: list[tuple[str, type]]
) -> bool:
    """Return True if the given dictionary has the given keys with the given types."""
    for field, field_type in expected_fields:
        if field not in data:
            return False

        value = data[field]
        if not isinstance(value, field_type):
            return False
    return True


def get_llm_cache(llmnm, txt, history, genconf):
    hasher = xxhash.xxh64()
    hasher.update(str(llmnm).encode("utf-8"))
    hasher.update(str(txt).encode("utf-8"))
    hasher.update(str(history).encode("utf-8"))
    hasher.update(str(genconf).encode("utf-8"))

    k = hasher.hexdigest()
    bin = REDIS_CONN.get(k)
    if not bin:
        return
    return bin


def set_llm_cache(llmnm, txt, v, history, genconf):
    hasher = xxhash.xxh64()
    hasher.update(str(llmnm).encode("utf-8"))
    hasher.update(str(txt).encode("utf-8"))
    hasher.update(str(history).encode("utf-8"))
    hasher.update(str(genconf).encode("utf-8"))

    k = hasher.hexdigest()
    REDIS_CONN.set(k, v.encode("utf-8"), 24*3600)


def get_embed_cache(llmnm, txt):
    hasher = xxhash.xxh64()
    hasher.update(str(llmnm).encode("utf-8"))
    hasher.update(str(txt).encode("utf-8"))

    k = hasher.hexdigest()
    bin = REDIS_CONN.get(k)
    if not bin:
        return
    return np.array(json.loads(bin))


def set_embed_cache(llmnm, txt, arr):
    hasher = xxhash.xxh64()
    hasher.update(str(llmnm).encode("utf-8"))
    hasher.update(str(txt).encode("utf-8"))

    k = hasher.hexdigest()
    arr = json.dumps(arr.tolist() if isinstance(arr, np.ndarray) else arr)
    REDIS_CONN.set(k, arr.encode("utf-8"), 24*3600)