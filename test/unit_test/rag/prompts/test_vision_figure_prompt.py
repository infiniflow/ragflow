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
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


def _load_generator(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    json_repair = ModuleType("json_repair")
    json_repair.repair_json = lambda text, **_kwargs: text
    monkeypatch.setitem(sys.modules, "json_repair", json_repair)

    common = ModuleType("common")
    common.__path__ = [str(repo_root / "common")]
    monkeypatch.setitem(sys.modules, "common", common)

    misc_utils = ModuleType("common.misc_utils")
    misc_utils.hash_str2int = lambda value, _mod=500: 0
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils)

    constants = ModuleType("common.constants")
    constants.TAG_FLD = "tag"
    monkeypatch.setitem(sys.modules, "common.constants", constants)

    token_utils = ModuleType("common.token_utils")
    token_utils.encoder = SimpleNamespace()
    token_utils.num_tokens_from_string = len
    monkeypatch.setitem(sys.modules, "common.token_utils", token_utils)

    rag = ModuleType("rag")
    rag.__path__ = [str(repo_root / "rag")]
    monkeypatch.setitem(sys.modules, "rag", rag)

    rag_nlp = ModuleType("rag.nlp")
    rag_nlp.rag_tokenizer = SimpleNamespace()
    monkeypatch.setitem(sys.modules, "rag.nlp", rag_nlp)

    prompts = ModuleType("rag.prompts")
    prompts.__path__ = [str(repo_root / "rag" / "prompts")]
    monkeypatch.setitem(sys.modules, "rag.prompts", prompts)

    template = ModuleType("rag.prompts.template")
    template.load_prompt = lambda name: (repo_root / "rag" / "prompts" / f"{name}.md").read_text(encoding="utf-8").strip()
    monkeypatch.setitem(sys.modules, "rag.prompts.template", template)

    module_path = repo_root / "rag" / "prompts" / "generator.py"
    spec = importlib.util.spec_from_file_location(
        "test_vision_figure_prompt_generator",
        module_path,
    )
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p1
@pytest.mark.parametrize(
    ("function_name", "arguments", "expected_language"),
    [
        (
            "vision_llm_figure_describe_prompt",
            {},
            "English",
        ),
        (
            "vision_llm_figure_describe_prompt",
            {"language": "Chinese"},
            "Chinese",
        ),
        (
            "vision_llm_figure_describe_prompt_with_context",
            {"context_above": "Above", "context_below": "Below"},
            "English",
        ),
        (
            "vision_llm_figure_describe_prompt_with_context",
            {
                "context_above": "Above",
                "context_below": "Below",
                "language": "Chinese",
            },
            "Chinese",
        ),
    ],
)
def test_figure_prompt_renders_output_language(
    monkeypatch,
    function_name,
    arguments,
    expected_language,
):
    generator = _load_generator(monkeypatch)

    prompt = getattr(generator, function_name)(**arguments)

    assert f"Write all descriptions and field values in {expected_language}." in prompt
    assert "Preserve all visible text verbatim in its original language" in prompt
    assert "{{ language }}" not in prompt
