#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


class _CharEncoder:
    @staticmethod
    def encode(text):
        return list(text)

    @staticmethod
    def decode(tokens):
        return "".join(tokens)


def _load_generator_module(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    json_repair = ModuleType("json_repair")
    json_repair.repair_json = lambda text, **_kwargs: text
    monkeypatch.setitem(sys.modules, "json_repair", json_repair)

    common_pkg = ModuleType("common")
    common_pkg.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common_pkg)

    misc_utils = ModuleType("common.misc_utils")
    misc_utils.hash_str2int = lambda value, _mod=500: 0
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils)

    constants = ModuleType("common.constants")
    constants.TAG_FLD = "tag"
    monkeypatch.setitem(sys.modules, "common.constants", constants)

    token_utils = ModuleType("common.token_utils")
    token_utils.encoder = _CharEncoder()
    token_utils.num_tokens_from_string = lambda text: len(text)
    monkeypatch.setitem(sys.modules, "common.token_utils", token_utils)

    rag_pkg = ModuleType("rag")
    rag_pkg.__path__ = [str(repo_root / "rag")]
    monkeypatch.setitem(sys.modules, "rag", rag_pkg)

    rag_nlp = ModuleType("rag.nlp")
    rag_nlp.rag_tokenizer = SimpleNamespace(tokenize=lambda text: text.split())
    monkeypatch.setitem(sys.modules, "rag.nlp", rag_nlp)

    rag_prompts_pkg = ModuleType("rag.prompts")
    rag_prompts_pkg.__path__ = [str(repo_root / "rag" / "prompts")]
    monkeypatch.setitem(sys.modules, "rag.prompts", rag_prompts_pkg)

    template_mod = ModuleType("rag.prompts.template")
    template_mod.load_prompt = lambda *_args, **_kwargs: ""
    monkeypatch.setitem(sys.modules, "rag.prompts.template", template_mod)

    spec = importlib.util.spec_from_file_location(
        "rag.prompts.generator", repo_root / "rag" / "prompts" / "generator.py"
    )
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p1
def test_message_fit_in_truncates_user_message_by_system_token_budget(monkeypatch):
    generator = _load_generator_module(monkeypatch)
    monkeypatch.setattr(generator, "num_tokens_from_string", lambda text: len(text))
    monkeypatch.setattr(generator, "encoder", _CharEncoder())

    messages = [
        {"role": "system", "content": "1234"},
        {"role": "user", "content": "abcdefghij"},
    ]

    used_tokens, trimmed = generator.message_fit_in(messages, max_length=8)

    assert used_tokens == 8
    assert trimmed[0]["content"] == "1234"
    assert trimmed[-1]["content"] == "abcd"


@pytest.mark.p1
def test_message_fit_in_handles_zero_token_messages(monkeypatch):
    generator = _load_generator_module(monkeypatch)
    monkeypatch.setattr(generator, "num_tokens_from_string", lambda _text: 0)
    monkeypatch.setattr(generator, "encoder", _CharEncoder())

    messages = [
        {"role": "system", "content": ""},
        {"role": "user", "content": ""},
    ]

    used_tokens, trimmed = generator.message_fit_in(messages, max_length=0)

    assert used_tokens == 0
    assert trimmed == messages


@pytest.mark.p1
def test_message_fit_in_clamps_negative_slice_lengths(monkeypatch):
    generator = _load_generator_module(monkeypatch)
    monkeypatch.setattr(generator, "num_tokens_from_string", lambda text: len(text))
    monkeypatch.setattr(generator, "encoder", _CharEncoder())

    messages = [
        {"role": "system", "content": "1234"},
        {"role": "user", "content": "abcdefghij"},
    ]

    used_tokens, trimmed = generator.message_fit_in(messages, max_length=2)

    assert used_tokens == 2
    assert trimmed[0]["content"] == "1234"
    assert trimmed[-1]["content"] == ""
