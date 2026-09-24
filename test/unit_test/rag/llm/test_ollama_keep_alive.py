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
"""``OLLAMA_KEEP_ALIVE`` accepts Ollama's duration strings, not just integers."""

import pytest

from rag.llm.cv_model import OllamaCV
from rag.llm.embedding_model import OllamaEmbed
from rag.llm.ollama_utils import resolve_ollama_keep_alive


@pytest.mark.parametrize(
    ("env_value", "expected"),
    [
        (None, -1),
        ("", -1),
        ("-1", -1),
        ("300", 300),
        (" 0 ", 0),
        ("5m", "5m"),
        ("24h", "24h"),
    ],
)
def test_env_value(monkeypatch, env_value, expected):
    if env_value is None:
        monkeypatch.delenv("OLLAMA_KEEP_ALIVE", raising=False)
    else:
        monkeypatch.setenv("OLLAMA_KEEP_ALIVE", env_value)
    assert resolve_ollama_keep_alive({}) == expected


def test_explicit_kwarg_wins_and_env_is_not_parsed(monkeypatch):
    monkeypatch.setenv("OLLAMA_KEEP_ALIVE", "not-a-duration")
    assert resolve_ollama_keep_alive({"ollama_keep_alive": "10m"}) == "10m"


@pytest.mark.parametrize("model_cls", [OllamaEmbed, OllamaCV])
def test_models_accept_duration_env(monkeypatch, model_cls):
    monkeypatch.setenv("OLLAMA_KEEP_ALIVE", "24h")
    model = model_cls("x", "bge-m3", base_url="http://localhost:11434")
    assert model.keep_alive == "24h"
