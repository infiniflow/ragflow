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
from unittest.mock import patch


def _load_picture_module(tokenized_texts):
    """Load the picture parser with lightweight fakes for external services."""

    class FakeLLMBundle:
        def __init__(self, *args, **kwargs):
            """Accept the same construction arguments as the real LLM bundle."""

            pass

        async def async_chat(self, **kwargs):
            """Return a deterministic video description for the regression test."""

            return "A concise video description."

    llm_service = ModuleType("api.db.services.llm_service")
    llm_service.LLMBundle = FakeLLMBundle

    tenant_model_service = ModuleType("api.db.joint_services.tenant_model_service")
    tenant_model_service.get_tenant_default_model_by_type = lambda *args, **kwargs: {}
    tenant_model_service.get_first_provider_model_name = lambda *args, **kwargs: None
    tenant_model_service.resolve_model_config = lambda *args, **kwargs: {}
    tenant_model_service.ensure_paddleocr_from_env = lambda *args, **kwargs: None

    constants = ModuleType("common.constants")
    constants.LLMType = SimpleNamespace(IMAGE2TEXT="image2text", OCR="ocr")

    parser_config_utils = ModuleType("common.parser_config_utils")
    parser_config_utils.normalize_layout_recognizer = lambda value: (value, "")

    string_utils = ModuleType("common.string_utils")
    string_utils.clean_markdown_block = lambda value: value

    vision = ModuleType("deepdoc.vision")
    vision.OCR = lambda: object()

    nlp = ModuleType("rag.nlp")
    nlp.attach_media_context = lambda docs, *_args: docs
    nlp.rag_tokenizer = SimpleNamespace(tokenize=lambda value: value)

    def fake_tokenize(doc, text, *_args, **_kwargs):
        """Capture the exact text passed to tokenization."""

        tokenized_texts.append(text)
        doc["content_with_weight"] = text

    nlp.tokenize = fake_tokenize

    stubs = {
        "api.db.services.llm_service": llm_service,
        "api.db.joint_services.tenant_model_service": tenant_model_service,
        "common.constants": constants,
        "common.parser_config_utils": parser_config_utils,
        "common.string_utils": string_utils,
        "deepdoc.vision": vision,
        "rag.nlp": nlp,
    }

    module_path = Path(__file__).resolve().parents[4] / "rag" / "app" / "picture.py"
    spec = importlib.util.spec_from_file_location("picture_video_under_test", module_path)
    module = importlib.util.module_from_spec(spec)
    with patch.dict(sys.modules, stubs):
        spec.loader.exec_module(module)
    return module


def test_video_description_is_tokenized_once():
    """Ensure one model response produces one tokenized video description."""

    tokenized_texts = []
    picture = _load_picture_module(tokenized_texts)

    callback_calls = []
    chunks = picture.chunk(
        "clip.mp4",
        b"video bytes",
        "tenant",
        "English",
        callback=lambda *args, **kwargs: callback_calls.append((args, kwargs)),
    )

    errors = [kwargs.get("msg") for args, kwargs in callback_calls if kwargs.get("prog") == -1]
    assert not errors, f"chunk() reported an error instead of producing a chunk: {errors}"
    assert len(chunks) == 1
    assert chunks[0]["doc_type_kwd"] == "video"
    assert tokenized_texts == ["A concise video description."]
