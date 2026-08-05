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

"""Regression tests for the figure-language propagation across the 4
figure-vision wrappers in :mod:`deepdoc.parser.figure_parser` (#17280).

The four wrappers in that module -- ``vision_figure_parser_docx_wrapper``,
``vision_figure_parser_figure_xlsx_wrapper``, ``vision_figure_parser_pdf_wrapper``,
and ``vision_figure_parser_docx_wrapper_naive`` -- previously did not accept
``lang`` as a named parameter, so the dataset language (``lang="Chinese"`` by
default) was captured by the parent ``chunk()`` function in :mod:`rag.app.*`
but never reached the figure-description prompt. ``VisionFigureParser`` fell
back to ``"English"`` for every figure description, regardless of the
dataset's language setting.

This test file covers three layers:

  1. The prompt-rendering functions in :mod:`rag.prompts.generator` accept
     a ``language`` argument and render the prompt's ``{{ language }}``
     variable.
  2. Each of the 4 figure-vision wrappers accepts a ``lang`` keyword and
     forwards it to both ``LLMBundle`` and ``VisionFigureParser``.
  3. ``VisionFigureParser.__init__`` stores ``self.language`` from the
     ``lang`` keyword and the prompt functions in its ``__call__`` worker
     pass that value to the prompt renderer.

The vision model and tenant lookup are mocked at the import-site path used
by the wrappers (the ``common.constants.LLMType`` import is real; the
``get_tenant_default_model_by_type`` and ``LLMBundle`` symbols are
patched at the call-site path to capture the forwarded ``lang``).
"""

from __future__ import annotations

from unittest.mock import MagicMock, patch

# --------------------------------------------------------------------------- #
# 1. Prompt renderers
# --------------------------------------------------------------------------- #


class TestPromptLanguageVariable:
    """The figure-description prompt templates must render ``{{ language }}``
    from the ``language`` argument and use the default ``"English"`` when
    omitted.
    """

    def test_default_prompt_renders_english_when_omitted(self):
        from rag.prompts.generator import vision_llm_figure_describe_prompt

        out = vision_llm_figure_describe_prompt()
        assert "English" in out
        assert "{{ language }}" not in out

    def test_default_prompt_renders_chinese_when_passed(self):
        from rag.prompts.generator import vision_llm_figure_describe_prompt

        out = vision_llm_figure_describe_prompt(language="Chinese")
        assert "Chinese" in out
        assert "{{ language }}" not in out

    def test_context_prompt_renders_language_when_passed(self):
        from rag.prompts.generator import vision_llm_figure_describe_prompt_with_context

        out = vision_llm_figure_describe_prompt_with_context(context_above="header", context_below="footer", language="Japanese")
        assert "Japanese" in out
        assert "header" in out
        assert "footer" in out
        assert "{{ language }}" not in out
        assert "{{ context_above }}" not in out
        assert "{{ context_below }}" not in out

    def test_context_prompt_default_is_english(self):
        from rag.prompts.generator import vision_llm_figure_describe_prompt_with_context

        out = vision_llm_figure_describe_prompt_with_context(context_above="", context_below="")
        assert "English" in out


# --------------------------------------------------------------------------- #
# 2. VisionFigureParser stores self.language
# --------------------------------------------------------------------------- #


class TestVisionFigureParserLanguage:
    """``VisionFigureParser.__init__`` must read ``lang`` from kwargs into
    ``self.language`` so the prompt functions in its ``__call__`` worker
    can render the correct output language.
    """

    def test_lang_kwarg_is_stored_on_self(self):
        from deepdoc.parser.figure_parser import VisionFigureParser

        parser = VisionFigureParser(
            vision_model=MagicMock(),
            figures_data=[],
            lang="Chinese",
        )
        assert parser.language == "Chinese"

    def test_omitted_lang_defaults_to_english(self):
        from deepdoc.parser.figure_parser import VisionFigureParser

        parser = VisionFigureParser(
            vision_model=MagicMock(),
            figures_data=[],
        )
        assert parser.language == "English"

    def test_empty_string_lang_falls_back_to_english(self):
        from deepdoc.parser.figure_parser import VisionFigureParser

        # The wrapper uses ``kwargs.get("lang") or "English"`` so an empty
        # string is treated the same as a missing value.
        parser = VisionFigureParser(
            vision_model=MagicMock(),
            figures_data=[],
            lang="",
        )
        assert parser.language == "English"


# --------------------------------------------------------------------------- #
# 3. Each wrapper forwards lang to LLMBundle and VisionFigureParser
# --------------------------------------------------------------------------- #


def _install_vision_mocks(monkeypatch, lang_seen: dict):
    """Patch ``get_tenant_default_model_by_type`` and ``LLMBundle`` at the
    call-site path used by the 4 figure-vision wrappers so we can capture
    the forwarded ``lang`` without making a real Azure / OpenAI call.

    The wrappers import both symbols at module load from
    ``api.db.services.llm_service`` and
    ``api.db.joint_services.tenant_model_service`` -- patching at the
    call-site (``deepdoc.parser.figure_parser``) is the only path that
    takes effect for the module-level bindings.
    """

    def fake_get_tenant_default_model_by_type(tenant_id, model_type):
        return {"model_type": "vision", "model_name": "stub-vision-model", "api_key": "sk-stub"}

    def fake_llmbundle(tenant_id, model_config, lang="Chinese", **kwargs):
        lang_seen["llm_lang"] = lang
        return MagicMock(name="LLMBundle")

    monkeypatch.setattr(
        "deepdoc.parser.figure_parser.get_tenant_default_model_by_type",
        fake_get_tenant_default_model_by_type,
    )
    monkeypatch.setattr(
        "deepdoc.parser.figure_parser.LLMBundle",
        fake_llmbundle,
    )


def _install_vision_parser_recorder(monkeypatch, parser_calls: list):
    """Replace :class:`VisionFigureParser` in the call site so the wrapper's
    real construction is short-circuited and we can record the kwargs
    without invoking the real ``_extract_figures_info`` machinery.

    The real :class:`VisionFigureParser` is patched at the import site
    (``deepdoc.parser.figure_parser``) so the wrappers' module-level
    binding is the one being replaced.
    """

    class FakeParser:
        def __init__(self, *args, **kwargs):
            parser_calls.append(kwargs)
            self.language = kwargs.get("lang", "English")

        def __call__(self, **kwargs):
            return []

    monkeypatch.setattr("deepdoc.parser.figure_parser.VisionFigureParser", FakeParser)


class TestDocxWrapperForwardsLang:
    """``vision_figure_parser_docx_wrapper`` (used by ``rag.app.book`` and
    ``rag.app.manual`` for DOCX parsing) must accept ``lang`` and forward
    it to both ``LLMBundle`` and ``VisionFigureParser``.
    """

    def test_lang_forwarded_to_llmbundle_and_parser(self, monkeypatch):
        from deepdoc.parser.figure_parser import vision_figure_parser_docx_wrapper

        lang_seen: dict = {}
        parser_calls: list = []
        _install_vision_mocks(monkeypatch, lang_seen)
        _install_vision_parser_recorder(monkeypatch, parser_calls)

        # Provide a minimal non-empty section so the wrapper reaches the
        # VisionFigureParser construction path. Use a non-image so
        # vision_figure_parser_figure_data_wrapper yields no figures and
        # we don't have to stub the image handling.
        sections = [("text-only", None)]
        tbls = []
        result = vision_figure_parser_docx_wrapper(
            sections=sections,
            tbls=tbls,
            callback=lambda *_a, **_k: None,
            lang="Chinese",
            tenant_id="stub-tenant",
        )

        assert lang_seen["llm_lang"] == "Chinese"
        assert parser_calls, "VisionFigureParser was not constructed"
        assert parser_calls[0]["lang"] == "Chinese"
        # The wrapper returns tbls unchanged when there are no figures.
        assert result is tbls

    def test_lang_defaults_to_english_when_omitted(self, monkeypatch):
        from deepdoc.parser.figure_parser import vision_figure_parser_docx_wrapper

        lang_seen: dict = {}
        parser_calls: list = []
        _install_vision_mocks(monkeypatch, lang_seen)
        _install_vision_parser_recorder(monkeypatch, parser_calls)

        vision_figure_parser_docx_wrapper(
            sections=[("text-only", None)],
            tbls=[],
            callback=lambda *_a, **_k: None,
            tenant_id="stub-tenant",
        )

        assert lang_seen["llm_lang"] == "English"
        assert parser_calls[0]["lang"] == "English"


class TestXlsxWrapperForwardsLang:
    """``vision_figure_parser_figure_xlsx_wrapper`` (used by ``rag.app.table``
    for Excel image extraction) must accept ``lang`` and forward it to
    both ``LLMBundle`` and ``VisionFigureParser``.
    """

    def test_lang_forwarded_to_llmbundle_and_parser(self, monkeypatch):
        from deepdoc.parser.figure_parser import vision_figure_parser_figure_xlsx_wrapper

        lang_seen: dict = {}
        parser_calls: list = []
        _install_vision_mocks(monkeypatch, lang_seen)
        _install_vision_parser_recorder(monkeypatch, parser_calls)

        # ``images`` here is a list of dicts with the shape produced by
        # ``Excel._extract_images_from_worksheet``. The wrapper calls
        # ``ensure_pil_image`` on ``img["image"]`` -- patch that to a
        # no-op so we don't need a real image.
        with patch("deepdoc.parser.figure_parser.ensure_pil_image", return_value=MagicMock(name="PIL.Image")):
            vision_figure_parser_figure_xlsx_wrapper(
                images=[{"image": b"fake-bytes", "image_description": "stub"}],
                callback=lambda *_a, **_k: None,
                lang="Japanese",
                tenant_id="stub-tenant",
            )

        assert lang_seen["llm_lang"] == "Japanese"
        assert parser_calls[0]["lang"] == "Japanese"


class TestPdfWrapperForwardsLang:
    """``vision_figure_parser_pdf_wrapper`` (used by ``rag.app.naive`` for
    ``by_deepdoc``, ``rag.app.manual`` and ``rag.app.paper`` for PDF
    parsing) must accept ``lang`` and forward it to both ``LLMBundle``
    and ``VisionFigureParser``.
    """

    def test_lang_forwarded_to_llmbundle_and_parser(self, monkeypatch):
        from deepdoc.parser.figure_parser import vision_figure_parser_pdf_wrapper

        lang_seen: dict = {}
        parser_calls: list = []
        _install_vision_mocks(monkeypatch, lang_seen)
        _install_vision_parser_recorder(monkeypatch, parser_calls)

        # ``tbls`` items have the shape produced by the deepdoc PDF parser.
        # The wrapper filters by ``is_image_like(item[0][0])`` -- patch
        # that to True so we reach the VisionFigureParser construction.
        fake_image = MagicMock(name="PIL.Image")
        tbls = [((fake_image, ["stub-desc"]), [(0, 0, 0, 0, 0)])]
        with patch("deepdoc.parser.figure_parser.is_image_like", return_value=True):
            vision_figure_parser_pdf_wrapper(
                tbls=tbls,
                callback=lambda *_a, **_k: None,
                lang="English",
                tenant_id="stub-tenant",
            )

        assert lang_seen["llm_lang"] == "English"
        assert parser_calls[0]["lang"] == "English"


class TestNaiveDocxWrapperForwardsLang:
    """``vision_figure_parser_docx_wrapper_naive`` (used by
    ``rag.app.naive`` and ``rag.app.one`` for the naive DOCX flow) must
    accept ``lang`` and forward it to ``LLMBundle`` and to the prompt
    functions in its per-chunk worker.
    """

    def test_lang_forwarded_to_llmbundle(self, monkeypatch):
        from deepdoc.parser.figure_parser import vision_figure_parser_docx_wrapper_naive

        lang_seen: dict = {}
        captured: dict = {}
        _install_vision_mocks(monkeypatch, lang_seen)

        # Patch the prompt functions so we can record the ``language``
        # kwarg forwarded by the worker. We also patch
        # ``picture_vision_llm_chunk`` (the actual VLM call) to a no-op
        # so we don't need a real vision model.
        def fake_default_prompt(*args, **kwargs):
            captured["default_language"] = kwargs.get("language")
            return "stub-prompt"

        def fake_context_prompt(*args, **kwargs):
            captured["context_language"] = kwargs.get("language")
            return "stub-prompt"

        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.vision_llm_figure_describe_prompt",
            fake_default_prompt,
        )
        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.vision_llm_figure_describe_prompt_with_context",
            fake_context_prompt,
        )
        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.picture_vision_llm_chunk",
            lambda *a, **k: "stub-description",
        )

        # ``chunks`` here is a list of dicts; only the image entries
        # (those with ``image`` set) are processed by the worker. The
        # worker calls ``open_image_for_processing`` -- patch it to
        # return a real ``Image.Image`` mock so the worker proceeds.
        from PIL import Image

        fake_img = MagicMock(spec=Image.Image)
        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.open_image_for_processing",
            lambda *_a, **_k: (fake_img, False),
        )

        chunks = [
            {"text": "caption", "image": b"fake-bytes", "context_above": "", "context_below": ""},
        ]

        vision_figure_parser_docx_wrapper_naive(
            chunks=chunks,
            idx_lst=[0],
            callback=lambda *_a, **_k: None,
            lang="Chinese",
            tenant_id="stub-tenant",
        )

        assert lang_seen["llm_lang"] == "Chinese"
        assert captured["default_language"] == "Chinese"

    def test_lang_forwarded_to_context_prompt_when_context_present(self, monkeypatch):
        from deepdoc.parser.figure_parser import vision_figure_parser_docx_wrapper_naive

        lang_seen: dict = {}
        captured: dict = {}
        _install_vision_mocks(monkeypatch, lang_seen)

        def fake_context_prompt(*args, **kwargs):
            captured["context_language"] = kwargs.get("language")
            return "stub-prompt"

        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.vision_llm_figure_describe_prompt",
            lambda *a, **k: "stub-prompt",
        )
        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.vision_llm_figure_describe_prompt_with_context",
            fake_context_prompt,
        )
        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.picture_vision_llm_chunk",
            lambda *a, **k: "stub-description",
        )

        from PIL import Image

        fake_img = MagicMock(spec=Image.Image)
        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.open_image_for_processing",
            lambda *_a, **_k: (fake_img, False),
        )

        chunks = [
            {
                "text": "caption",
                "image": b"fake-bytes",
                "context_above": "before",
                "context_below": "after",
            },
        ]

        vision_figure_parser_docx_wrapper_naive(
            chunks=chunks,
            idx_lst=[0],
            callback=lambda *_a, **_k: None,
            lang="Japanese",
            tenant_id="stub-tenant",
        )

        assert lang_seen["llm_lang"] == "Japanese"
        assert captured["context_language"] == "Japanese"


# --------------------------------------------------------------------------- #
# 4. End-to-end: prompt receives the correct language
# --------------------------------------------------------------------------- #


class TestPromptReceivesParserLanguage:
    """End-to-end: ``VisionFigureParser.__call__`` must pass
    ``self.language`` to both prompt functions.
    """

    def test_call_uses_self_language(self, monkeypatch):
        from deepdoc.parser.figure_parser import VisionFigureParser
        from PIL import Image

        captured: dict = {}

        def fake_default_prompt(*args, **kwargs):
            captured["default_language"] = kwargs.get("language")
            return "stub-prompt"

        def fake_context_prompt(*args, **kwargs):
            captured["context_language"] = kwargs.get("language")
            return "stub-prompt"

        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.vision_llm_figure_describe_prompt",
            fake_default_prompt,
        )
        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.vision_llm_figure_describe_prompt_with_context",
            fake_context_prompt,
        )
        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.picture_vision_llm_chunk",
            lambda *a, **k: "stub-description",
        )

        fake_img = MagicMock(spec=Image.Image)
        fake_img.close = MagicMock()
        parser = VisionFigureParser(
            vision_model=MagicMock(),
            figures_data=[(fake_img, ["stub"])],
            lang="Chinese",
        )
        parser(callback=lambda *_a, **_k: None)

        assert captured["default_language"] == "Chinese"

    def test_call_uses_self_language_for_context_prompt(self, monkeypatch):
        from deepdoc.parser.figure_parser import VisionFigureParser
        from PIL import Image

        captured: dict = {}

        def fake_default_prompt(*args, **kwargs):
            captured["default_language"] = kwargs.get("language")
            return "stub-prompt"

        def fake_context_prompt(*args, **kwargs):
            captured["context_language"] = kwargs.get("language")
            return "stub-prompt"

        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.vision_llm_figure_describe_prompt",
            fake_default_prompt,
        )
        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.vision_llm_figure_describe_prompt_with_context",
            fake_context_prompt,
        )
        monkeypatch.setattr(
            "deepdoc.parser.figure_parser.picture_vision_llm_chunk",
            lambda *a, **k: "stub-description",
        )

        fake_img = MagicMock(spec=Image.Image)
        fake_img.close = MagicMock()
        parser = VisionFigureParser(
            vision_model=MagicMock(),
            figures_data=[(fake_img, ["stub"])],
            figure_contexts=[("above", "below")],
            context_size=1,
            lang="Japanese",
        )
        parser(callback=lambda *_a, **_k: None)

        assert captured["context_language"] == "Japanese"
