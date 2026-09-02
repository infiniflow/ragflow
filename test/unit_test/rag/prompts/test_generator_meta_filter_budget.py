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
"""``gen_meta_filter`` must keep its prompt inside the model's context.

The metadata block it renders is the dataset's entire value space, so its size
is set by the data: one high-cardinality key can push the system prompt past
max_length on its own. Without a budget that is a failed model request rather
than a degraded filter.
"""

import importlib.util
import json
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

    spec = importlib.util.spec_from_file_location("rag.prompts.generator", repo_root / "rag" / "prompts" / "generator.py")
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "rag.prompts.generator", module)
    spec.loader.exec_module(module)
    return module


_TEMPLATE = "Available metadata keys: {{ metadata_keys }}\nUser query: {{ user_question }}\nToday: {{ current_date }}"


def _chat_model(max_length: int):
    seen = {}

    async def async_chat(system, history, gen_conf=None):
        seen["system"] = system
        seen["history"] = history
        return '{"logic": "and", "conditions": [{"key": "project", "value": "p1", "op": "="}]}'

    return SimpleNamespace(max_length=max_length, async_chat=async_chat), seen


def _prepare(monkeypatch, max_length):
    generator = _load_generator_module(monkeypatch)
    # Char-per-token proxy, as the sibling test in this directory does.
    monkeypatch.setattr(generator, "num_tokens_from_string", lambda text: len(text))
    monkeypatch.setattr(generator, "encoder", _CharEncoder())
    monkeypatch.setattr(generator, "META_FILTER", _TEMPLATE)
    # The shared loader stubs json_repair with repair_json only; gen_meta_filter
    # parses with json_repair.loads.
    generator.json_repair.loads = json.loads
    chat_mdl, seen = _chat_model(max_length)
    return generator, chat_mdl, seen


@pytest.mark.p1
async def test_oversized_value_space_yields_no_conditions_and_no_model_call(monkeypatch):
    """The conditions become a hard document scope, so a filter chosen from a
    value list that lost entries would exclude matching documents -- and the
    model cannot report that it only saw part of the metadata. No conditions
    leaves the search unscoped, which is recoverable."""
    generator, chat_mdl, seen = _prepare(monkeypatch, max_length=200)
    # One key with far more values than the budget can hold.
    meta_data = {"ref": {f"ref-{index:05d}": [f"doc-{index}"] for index in range(500)}}

    result = await generator.gen_meta_filter(chat_mdl, meta_data, "which project?")

    assert result == {"logic": "and", "conditions": []}
    assert seen == {}, "the model must not be called for a value space that does not fit"


@pytest.mark.p1
async def test_value_space_within_budget_is_sent_untouched(monkeypatch):
    generator, chat_mdl, seen = _prepare(monkeypatch, max_length=100000)
    meta_data = {"project": {"p1": ["doc-1"], "p2": ["doc-2"]}}

    await generator.gen_meta_filter(chat_mdl, meta_data, "which project?")

    assert '"p1"' in seen["system"] and '"p2"' in seen["system"]
    assert seen["history"][-1]["content"] == "Generate filters:"
