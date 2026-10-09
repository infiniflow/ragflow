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
"""Shared helpers for the native Ollama client used by the embedding and CV models."""

import os


def resolve_ollama_keep_alive(kwargs: dict) -> int | str:
    """Return the ``keep_alive`` value to send to Ollama.

    An explicit ``ollama_keep_alive`` kwarg wins; otherwise ``OLLAMA_KEEP_ALIVE`` is read,
    defaulting to ``-1`` (keep loaded). Ollama accepts either a number of seconds or a
    duration string such as ``"5m"`` / ``"24h"`` (the format its own docs use for this
    variable), so integers are converted and anything else is passed through unchanged.
    """
    if "ollama_keep_alive" in kwargs:
        return kwargs["ollama_keep_alive"]
    value = os.environ.get("OLLAMA_KEEP_ALIVE", "").strip()
    if not value:
        return -1
    try:
        return int(value)
    except ValueError:
        return value
