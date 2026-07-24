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
from unittest.mock import Mock

import pytest


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


def _load_figure_parser(monkeypatch):
    repo_root = Path(__file__).resolve().parents[4]

    for package_name in (
        "api",
        "api.db",
        "api.db.services",
        "api.db.joint_services",
        "common",
        "rag",
        "rag.app",
        "rag.prompts",
        "rag.utils",
    ):
        _package(monkeypatch, package_name)

    class FakeImage:
        def close(self):
            pass

    image_module = _module(monkeypatch, "PIL.Image", Image=FakeImage)
    pil_module = _package(monkeypatch, "PIL")
    pil_module.Image = image_module

    _module(
        monkeypatch,
        "common.constants",
        LLMType=SimpleNamespace(VISION="vision"),
    )
    _module(
        monkeypatch,
        "api.db.services.llm_service",
        LLMBundle=Mock(),
    )
    _module(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=Mock(),
    )

    def timeout(*_args, **_kwargs):
        return lambda function: function

    _module(monkeypatch, "common.connection_utils", timeout=timeout)
    _module(
        monkeypatch,
        "rag.app.picture",
        vision_llm_chunk=Mock(return_value="description"),
    )
    _module(
        monkeypatch,
        "rag.prompts.generator",
        vision_llm_figure_describe_prompt=Mock(return_value="prompt"),
        vision_llm_figure_describe_prompt_with_context=Mock(return_value="prompt"),
    )
    _module(
        monkeypatch,
        "rag.nlp",
        append_context2table_image4pdf=Mock(return_value=[]),
    )
    _module(
        monkeypatch,
        "rag.utils.lazy_image",
        ensure_pil_image=lambda image: image,
        open_image_for_processing=lambda image, **_kwargs: (image, False),
        is_image_like=lambda _image: True,
    )

    module_path = repo_root / "deepdoc" / "parser" / "figure_parser.py"
    spec = importlib.util.spec_from_file_location(
        "test_figure_parser_module",
        module_path,
    )
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)
    return module, FakeImage


@pytest.mark.p1
@pytest.mark.parametrize(
    ("context_above", "context_below", "prompt_name", "expected_arguments"),
    [
        (
            "",
            "",
            "vision_llm_figure_describe_prompt",
            {"language": "Chinese"},
        ),
        (
            "Above ",
            "Below",
            "vision_llm_figure_describe_prompt_with_context",
            {
                "context_above": "Above Caption",
                "context_below": "Below",
                "language": "Chinese",
            },
        ),
    ],
)
def test_docx_wrapper_passes_dataset_language_to_vision_model_and_prompt(
    monkeypatch,
    context_above,
    context_below,
    prompt_name,
    expected_arguments,
):
    module, FakeImage = _load_figure_parser(monkeypatch)
    model_config = {"llm_name": "vision-model"}
    vision_model = object()

    module.get_tenant_default_model_by_type = Mock(return_value=model_config)
    module.LLMBundle = Mock(return_value=vision_model)
    module.picture_vision_llm_chunk = Mock(return_value="description")

    default_prompt = Mock(return_value="prompt")
    contextual_prompt = Mock(return_value="prompt")
    module.vision_llm_figure_describe_prompt = default_prompt
    module.vision_llm_figure_describe_prompt_with_context = contextual_prompt

    chunks = [
        {
            "image": FakeImage(),
            "text": "Caption",
            "context_above": context_above,
            "context_below": context_below,
        }
    ]

    module.vision_figure_parser_docx_wrapper_naive(
        chunks=chunks,
        idx_lst=[0],
        callback=lambda *_args, **_kwargs: None,
        tenant_id="tenant-id",
        lang="Chinese",
    )

    module.LLMBundle.assert_called_once_with(
        "tenant-id",
        model_config,
        lang="Chinese",
    )

    selected_prompt = getattr(module, prompt_name)
    selected_prompt.assert_called_once_with(**expected_arguments)
    assert chunks[0]["text"].endswith("description")


@pytest.mark.p1
def test_vision_figure_parser_passes_dataset_language_to_prompt(monkeypatch):
    module, FakeImage = _load_figure_parser(monkeypatch)
    prompt = Mock(return_value="prompt")
    module.vision_llm_figure_describe_prompt = prompt
    module.picture_vision_llm_chunk = Mock(return_value="description")

    parser = module.VisionFigureParser(
        vision_model=object(),
        figures_data=[(FakeImage(), ["caption"])],
        lang="Chinese",
    )

    parser(callback=lambda *_args, **_kwargs: None)

    prompt.assert_called_once_with(language="Chinese")


@pytest.mark.p1
@pytest.mark.parametrize(
    "wrapper_name",
    [
        "vision_figure_parser_docx_wrapper",
        "vision_figure_parser_figure_xlsx_wrapper",
        "vision_figure_parser_pdf_wrapper",
    ],
)
def test_figure_wrappers_pass_dataset_language_to_model_and_parser(
    monkeypatch,
    wrapper_name,
):
    module, FakeImage = _load_figure_parser(monkeypatch)
    model_config = {"llm_name": "vision-model"}
    vision_model = object()
    parser_instance = Mock(return_value=[])

    module.get_tenant_default_model_by_type = Mock(return_value=model_config)
    module.LLMBundle = Mock(return_value=vision_model)
    module.VisionFigureParser = Mock(return_value=parser_instance)

    if wrapper_name == "vision_figure_parser_docx_wrapper":
        arguments = {
            "sections": [("caption", FakeImage())],
            "tbls": [],
        }
    elif wrapper_name == "vision_figure_parser_figure_xlsx_wrapper":
        arguments = {
            "images": [
                {
                    "image": FakeImage(),
                    "image_description": "caption",
                }
            ],
        }
    else:
        arguments = {
            "tbls": [
                (
                    (FakeImage(), ["caption"]),
                    [(0, 0, 0, 0, 0)],
                )
            ],
            "sections": [],
        }

    getattr(module, wrapper_name)(
        **arguments,
        callback=lambda *_args, **_kwargs: None,
        tenant_id="tenant-id",
        lang="Chinese",
    )

    module.LLMBundle.assert_called_once_with(
        "tenant-id",
        model_config,
        lang="Chinese",
    )
    assert module.VisionFigureParser.call_args.kwargs["lang"] == "Chinese"
    parser_instance.assert_called_once()
