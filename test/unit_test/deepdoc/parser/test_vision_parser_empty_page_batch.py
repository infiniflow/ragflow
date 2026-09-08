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
"""Regression tests for ``VisionParser.__call__`` empty-page-batch handling
in ``deepdoc/parser/pdf_parser.py``.

Issue: infiniflow/ragflow#17173.

When the user sets the PDF parser to a vision model (e.g. "qwen" series),
``by_plaintext`` dispatches to ``VisionParser`` via the layout_recognizer
fallback. Pre-fix, two failure modes were both silent:

1. ``VisionParser.__images__`` fails (pdfplumber exception path), so
   ``self.page_images`` is ``None`` and the loop iterates 0 times. The
   user sees "No chunk built from …" with no hint that rasterization
   failed.
2. Rasterization succeeds but the vision model returns empty text for
   every page in the batch. The user again sees "No chunk built
   from …" with no hint that the vision model is misconfigured or
   returning empty.

The fix surfaces both cases through the task callback so the user sees
a clear, actionable message in the task log instead of the generic
"No chunk built from …" line.
"""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock


REPO_ROOT = Path(__file__).resolve().parents[4]


def _load_pdf_parser(monkeypatch):
    _stub_module(monkeypatch, "pdfplumber")
    _stub_module(monkeypatch, "pypdf", PdfReader=object)
    _stub_module(monkeypatch, "huggingface_hub", snapshot_download=lambda **_kwargs: "")
    _stub_module(monkeypatch, "xgboost", Booster=_AcceptAllArgs)
    _stub_module(monkeypatch, "sklearn")
    _stub_module(monkeypatch, "sklearn.cluster", KMeans=object)
    _stub_module(monkeypatch, "sklearn.metrics", silhouette_score=lambda *_a, **_k: 0)

    common_mod = _stub_module(monkeypatch, "common")
    common_mod.__path__ = [str(REPO_ROOT / "common")]
    _stub_module(monkeypatch, "common.constants", MAXIMUM_PAGE_NUMBER=1024)
    _stub_module(monkeypatch, "common.file_utils", get_project_base_directory=lambda: str(REPO_ROOT))
    _stub_module(monkeypatch, "common.settings", PARALLEL_DEVICES=1)
    _stub_module(monkeypatch, "common.misc_utils", thread_pool_exec=lambda fn, *args, **kwargs: fn(*args, **kwargs))

    deepdoc_mod = _stub_module(monkeypatch, "deepdoc")
    deepdoc_mod.__path__ = [str(REPO_ROOT / "deepdoc")]
    _stub_module(monkeypatch, "deepdoc.parser")
    _stub_module(monkeypatch, "deepdoc.parser.utils", extract_pdf_outlines=lambda *_a, **_k: [])
    _stub_module(
        monkeypatch,
        "deepdoc.vision",
        OCR=_AcceptAllArgs,
        AscendLayoutRecognizer=_AcceptAllArgs,
        LayoutRecognizer=_AcceptAllArgs,
        Recognizer=_FakeRecognizer,
        TableStructureRecognizer=_FakeTSR,
    )

    rag_mod = _stub_module(monkeypatch, "rag")
    rag_mod.__path__ = [str(REPO_ROOT / "rag")]
    _stub_module(monkeypatch, "rag.nlp", rag_tokenizer=SimpleNamespace(tokenize=lambda text: text))
    prompts_mod = _stub_module(monkeypatch, "rag.prompts")
    prompts_mod.__path__ = [str(REPO_ROOT / "rag" / "prompts")]
    _stub_module(monkeypatch, "rag.prompts.generator", vision_llm_describe_prompt=lambda **_kwargs: "describe prompt")
    # Pre-populate rag.app.picture so the lazy `from rag.app.picture import
    # vision_llm_chunk` in VisionParser.__call__ resolves to a mock without
    # pulling in rag.app.picture's own dependency chain (PipelineTaskType,
    # settings, etc.). Each test sets monkeypatch-style state on this module
    # via `setattr(sys.modules["rag.app.picture"], "vision_llm_chunk", fn)`.
    _stub_module(monkeypatch, "rag.app")
    picture_mod = _stub_module(monkeypatch, "rag.app.picture")
    picture_mod.vision_llm_chunk = lambda **_kwargs: ""
    rag_mod.app = ModuleType("rag.app")
    rag_mod.app.picture = picture_mod

    module_name = "test_vision_parser_unit_module"
    module_path = REPO_ROOT / "deepdoc" / "parser" / "pdf_parser.py"
    spec = importlib.util.spec_from_file_location(module_name, module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _stub_module(monkeypatch, name, **attrs):
    module = ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    monkeypatch.setitem(sys.modules, name, module)
    return module


class _FakeRecognizer:
    @staticmethod
    def sort_Y_firstly(arr, _threshold):
        return sorted(arr, key=lambda item: (item["top"], item["x0"]))

    @staticmethod
    def layouts_cleanup(_boxes, layouts, _far=2, _thr=0.7):
        return layouts


class _FakeTSR:
    @staticmethod
    def is_caption(_bx):
        return False


class _AcceptAllArgs:
    """Stub that accepts any args/kwargs for instantiation, attribute
    access, and call. Used for the heavy classes (LayoutRecognizer,
    AscendLayoutRecognizer, OCR, xgboost.Booster) that the parser
    instantiates with constructor args and then exercises via method
    calls during init. The empty-batch control flow under test never
    touches these methods, so the stub just needs to not raise.
    """

    def __init__(self, *_args, **_kwargs):
        pass

    def __getattr__(self, _name):
        return _AcceptAllArgs()

    def __call__(self, *_args, **_kwargs):
        return _AcceptAllArgs()


def _make_vision_parser(pdf_parser_module, vision_model=None):
    return pdf_parser_module.VisionParser(vision_model=vision_model or MagicMock())


def test_vision_parser_warns_when_images_rasterization_fails(monkeypatch):
    """Regression for #17173 (silence on rasterization failure).

    When ``__images__`` fails (pdfplumber exception path),
    ``self.page_images`` is None. Pre-fix, the loop iterates 0 times
    and returns an empty list silently. The fix surfaces the failure
    through the callback with progress=-1 + a clear message naming
    rasterization, so the user sees a useful hint in the task log.
    """
    module = _load_pdf_parser(monkeypatch)
    parser = _make_vision_parser(module)
    parser.page_images = None  # simulate the __images__ failure path
    parser.total_page = 12

    callback_messages = []

    def cb(prog, msg):
        callback_messages.append((prog, msg))

    docs, tbls = parser("test.pdf", from_page=0, to_page=12, callback=cb)

    assert docs == []
    assert tbls == []
    assert any("could not rasterize" in msg for _, msg in callback_messages), f"expected a clear rasterization-failure callback, got {callback_messages!r}"
    assert any(prog == -1 for prog, _ in callback_messages), f"expected progress=-1 to flag the failure, got {callback_messages!r}"


def test_vision_parser_warns_when_vision_model_returns_empty(monkeypatch):
    """Regression for #17173 (silence on empty vision model output).

    The first page batch in the issue produced 12 chunks; subsequent
    batches returned 0 chunks. The fix surfaces the empty-batch case
    through the callback with a clear message naming the configured
    layout_recognizer model, so the user can distinguish "no images on
    these pages" from "vision model returned nothing".
    """
    module = _load_pdf_parser(monkeypatch)
    parser = _make_vision_parser(module)
    # Skip __images__ (the real one would need pdfplumber); inject
    # 3 mock page images directly. The loop only needs `img_binary`
    # to be truthy and the page image to have a .size tuple.
    parser.__images__ = lambda **_kwargs: None
    mock_image = MagicMock()
    mock_image.size = (612, 792)
    parser.page_images = [mock_image, mock_image, mock_image]
    parser.total_page = 12

    # Force picture_vision_llm_chunk to return empty for every page.
    sys.modules["rag.app.picture"].vision_llm_chunk = lambda **_kwargs: ""

    callback_messages = []

    def cb(prog, msg):
        callback_messages.append((prog, msg))

    docs, tbls = parser("test.pdf", from_page=0, to_page=3, callback=cb)

    assert docs == []
    assert tbls == []
    assert any("vision model returned no text" in msg for _, msg in callback_messages), f"expected a clear empty-output callback, got {callback_messages!r}"


def test_vision_parser_does_not_warn_when_some_pages_succeed(monkeypatch):
    """Regression guard: the empty-batch warning must NOT fire when at
    least one page in the batch produced text. Only fire when ALL pages
    returned empty.
    """
    module = _load_pdf_parser(monkeypatch)
    parser = _make_vision_parser(module)
    parser.__images__ = lambda **_kwargs: None
    mock_image = MagicMock()
    mock_image.size = (612, 792)
    parser.page_images = [mock_image, mock_image, mock_image]
    parser.total_page = 12

    # First page returns text; subsequent pages return empty.
    responses = iter(["page 1 text", "", ""])

    def fake_vision_llm_chunk(**_kwargs):
        return next(responses)

    sys.modules["rag.app.picture"].vision_llm_chunk = fake_vision_llm_chunk

    callback_messages = []

    def cb(prog, msg):
        callback_messages.append((prog, msg))

    docs, _ = parser("test.pdf", from_page=0, to_page=3, callback=cb)

    assert len(docs) == 1
    assert not any("vision model returned no text" in msg for _, msg in callback_messages), f"empty-batch warning fired even though page 1 succeeded: {callback_messages!r}"
