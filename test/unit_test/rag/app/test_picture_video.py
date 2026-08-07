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
import io
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import patch

from PIL import Image


def _load_picture_module(tokenized_texts, ocr_text="", vision_error=""):
    """Load the picture parser with lightweight fakes for external services.

    ``ocr_text`` is what the local OCR returns for an image, and ``vision_error``
    makes the vision-model lookup fail with that message.
    """

    class FakeLLMBundle:
        def __init__(self, *args, **kwargs):
            """Accept the same construction arguments as the real LLM bundle."""

            pass

        async def async_chat(self, **kwargs):
            """Return a deterministic video description for the regression test."""

            return "A concise video description."

    llm_service = ModuleType("api.db.services.llm_service")
    llm_service.LLMBundle = FakeLLMBundle

    def resolve_default_model(*_args, **_kwargs):
        if vision_error:
            raise LookupError(vision_error)
        return {}

    tenant_model_service = ModuleType("api.db.joint_services.tenant_model_service")
    tenant_model_service.get_tenant_default_model_by_type = resolve_default_model
    tenant_model_service.get_first_provider_model_name = lambda *args, **kwargs: None
    tenant_model_service.get_composite_model_name_by_id = lambda model_id: model_id
    tenant_model_service.resolve_model_config = lambda *args, **kwargs: {}
    tenant_model_service.ensure_paddleocr_from_env = lambda *args, **kwargs: None

    constants = ModuleType("common.constants")
    constants.LLMType = SimpleNamespace(VISION="vision", OCR="ocr")

    parser_config_utils = ModuleType("common.parser_config_utils")
    parser_config_utils.normalize_layout_recognizer = lambda value: (value, "")

    string_utils = ModuleType("common.string_utils")
    string_utils.clean_markdown_block = lambda value: value

    def fake_ocr(_image):
        """Mimic deepdoc's OCR return shape: (box, (text, confidence)) pairs."""

        return [(None, (ocr_text, 0.99))] if ocr_text else []

    vision = ModuleType("deepdoc.vision")
    vision.OCR = lambda: fake_ocr

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


def _jpeg_bytes():
    """Encode a small blank image the parser can open."""

    buffer = io.BytesIO()
    Image.new("RGB", (64, 64), "white").save(buffer, format="JPEG")
    return buffer.getvalue()


def test_short_ocr_text_is_kept_when_vision_model_is_missing():
    """OCR text under the CV LLM threshold is indexed when no image2text model
    is configured, instead of being dropped and leaving the image with no chunk.

    Guards https://github.com/infiniflow/ragflow/issues/17941.
    """

    tokenized_texts = []
    picture = _load_picture_module(
        tokenized_texts,
        ocr_text="工流宛转绕芳甸",
        vision_error="No default image2text model is set.",
    )

    callback_calls = []
    chunks = picture.chunk(
        "scan.jpg",
        _jpeg_bytes(),
        "tenant",
        "English",
        callback=lambda *args, **kwargs: callback_calls.append((args, kwargs)),
    )

    assert len(chunks) == 1, "OCR text was discarded, leaving the image with no chunk"
    assert chunks[0]["doc_type_kwd"] == "image"
    assert tokenized_texts == ["工流宛转绕芳甸"]


def test_failure_is_reported_when_ocr_finds_no_text():
    """With no OCR text and no vision model there is nothing to index, so the
    task must fail rather than report success with zero chunks."""

    tokenized_texts = []
    picture = _load_picture_module(
        tokenized_texts,
        vision_error="No default image2text model is set.",
    )

    callback_calls = []
    chunks = picture.chunk(
        "blank.jpg",
        _jpeg_bytes(),
        "tenant",
        "English",
        callback=lambda *args, **kwargs: callback_calls.append((args, kwargs)),
    )

    assert chunks == []
    assert not tokenized_texts
    assert [kwargs.get("msg") for _args, kwargs in callback_calls if kwargs.get("prog") == -1] == ["No default image2text model is set."]
