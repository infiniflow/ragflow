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
import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import Mock

import pytest

REPO_ROOT = Path(__file__).resolve().parents[5]


def _package(monkeypatch, name):
    package = ModuleType(name)
    package.__path__ = []
    monkeypatch.setitem(sys.modules, name, package)
    return package


def _module(monkeypatch, name, **attributes):
    module = ModuleType(name)
    for key, value in attributes.items():
        setattr(module, key, value)
    monkeypatch.setitem(sys.modules, name, module)
    return module


def _load_flow_utils(monkeypatch):
    for package_name in (
        "api",
        "api.db",
        "api.db.services",
        "api.db.joint_services",
        "common",
        "deepdoc",
        "deepdoc.parser",
        "rag",
    ):
        _package(monkeypatch, package_name)

    _module(monkeypatch, "api.db.services.llm_service", LLMBundle=Mock())
    _module(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=Mock(),
        resolve_model_config=Mock(),
    )
    _module(monkeypatch, "common.constants", LLMType=SimpleNamespace(VISION="vision"))
    _module(monkeypatch, "deepdoc.parser.figure_parser", VisionFigureParser=Mock())
    _module(
        monkeypatch,
        "rag.nlp",
        is_english=Mock(return_value=False),
        random_choices=Mock(return_value=[]),
        remove_contents_table=Mock(),
    )

    module_path = REPO_ROOT / "rag/flow/parser/utils.py"
    spec = importlib.util.spec_from_file_location("test_flow_parser_utils_module", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)
    return module


@pytest.mark.p1
@pytest.mark.parametrize(
    ("language", "expected_language"),
    [
        ("Japanese", "Japanese"),
        ("", "English"),
    ],
)
def test_media_enhancement_forwards_language_to_model_and_parser(monkeypatch, language, expected_language):
    utils = _load_flow_utils(monkeypatch)
    model_config = {"llm_name": "vision-model"}
    vision_model = object()
    llm_bundle = Mock(return_value=vision_model)
    parser_instance = Mock(return_value=[((None, "description"), None)])
    parser_factory = Mock(return_value=parser_instance)

    monkeypatch.setattr(utils, "resolve_model_config", Mock(return_value=model_config))
    monkeypatch.setattr(utils, "LLMBundle", llm_bundle)
    monkeypatch.setattr(utils, "VisionFigureParser", parser_factory)

    sections = [{"text": "caption", "image": object(), "doc_type_kwd": "image"}]
    result = utils.enhance_media_sections_with_vision(
        sections,
        "tenant-id",
        {"llm_id": "vision-model"},
        lang=language,
    )

    llm_bundle.assert_called_once_with("tenant-id", model_config, lang=expected_language)
    assert parser_factory.call_args.kwargs["vision_model"] is vision_model
    assert parser_factory.call_args.kwargs["lang"] == expected_language
    assert result[0]["text"] == "caption\ndescription"


@pytest.mark.p1
def test_all_flow_media_enhancement_callers_forward_language():
    tree = ast.parse((REPO_ROOT / "rag/flow/parser/parser.py").read_text())
    calls = [node for node in ast.walk(tree) if isinstance(node, ast.Call) and isinstance(node.func, ast.Name) and node.func.id == "enhance_media_sections_with_vision"]

    assert len(calls) == 3
    for call in calls:
        assert any(keyword.arg == "lang" for keyword in call.keywords), f"parser.py:{call.lineno} does not forward lang"
