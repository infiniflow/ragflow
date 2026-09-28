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
"""Issue #20234 — classify the huggingface_hub errors that surface when
docling's model download fails so the operator gets an actionable hint
instead of a 30-line traceback.

These tests exercise the module-level helper directly. They construct
fake exception classes (since huggingface_hub is an optional dep that
may not be installed locally; the docling_parser module tolerates its
absence and falls back to the class-name branch).
"""

from __future__ import annotations

import importlib.util
import sys
import types
from pathlib import Path

import pytest


ROOT = Path(__file__).resolve().parents[4]


def _load_docling_parser(monkeypatch):
    """Load `deepdoc.parser.docling_parser` with minimal stubs, mirroring the
    strategy used by `test_docling_parser_remote.py`."""
    common_pkg = types.ModuleType("common")
    constants_mod = types.ModuleType("common.constants")
    constants_mod.MAXIMUM_PAGE_NUMBER = 1000

    deepdoc_pkg = types.ModuleType("deepdoc")
    parser_pkg = types.ModuleType("deepdoc.parser")
    parser_pkg.__path__ = []
    utils_mod = types.ModuleType("deepdoc.parser.utils")
    utils_mod.extract_pdf_outlines = lambda _source: []

    pil_pkg = types.ModuleType("PIL")
    image_mod = types.ModuleType("PIL.Image")
    image_mod.Image = object
    pil_pkg.Image = image_mod

    monkeypatch.setitem(sys.modules, "common", common_pkg)
    monkeypatch.setitem(sys.modules, "common.constants", constants_mod)
    monkeypatch.setitem(sys.modules, "deepdoc", deepdoc_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser", parser_pkg)
    monkeypatch.setitem(sys.modules, "deepdoc.parser.utils", utils_mod)
    monkeypatch.setitem(sys.modules, "pdfplumber", types.ModuleType("pdfplumber"))
    monkeypatch.setitem(sys.modules, "PIL", pil_pkg)
    monkeypatch.setitem(sys.modules, "PIL.Image", image_mod)

    spec = importlib.util.spec_from_file_location(
        "_docling_parser_under_test",
        ROOT / "deepdoc" / "parser" / "docling_parser.py",
    )
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, spec.name, module)
    spec.loader.exec_module(module)
    return module


def _make_exception(class_name: str, message: str):
    """Build a synthetic exception with the given class name and message."""
    cls = type(class_name, (Exception,), {})
    return cls(message)


def _hint(monkeypatch, cls_name: str, message: str):
    module = _load_docling_parser(monkeypatch)
    return module._classify_hf_download_error(_make_exception(cls_name, message))


# String-substring classification ---------------------------------------


class TestClassifyBySubstring:
    def test_distinct_resource_message_is_recognised(self, monkeypatch):
        hint = _hint(
            monkeypatch,
            "FileMetadataError",
            "Distant resource does not seem to be on huggingface.co. It is possible that a configuration issue prevents you from downloading resources.",
        )
        assert hint is not None
        assert "huggingface.co" in hint
        assert "HF_HUB_OFFLINE" in hint
        assert "HF_TOKEN" in hint

    def test_repository_not_found_is_recognised(self, monkeypatch):
        hint = _hint(
            monkeypatch,
            "RepositoryNotFoundError",
            "Repository Not Found for url: ds4sd/docling-models.",
        )
        assert hint is not None
        assert "DOCLING" in hint or "docling" in hint

    def test_gated_repo_message_is_recognised(self, monkeypatch):
        hint = _hint(
            monkeypatch,
            "GatedRepoError",
            "Access to this resource is restricted. You must be authenticated to access it.",
        )
        assert hint is not None
        assert "HF_TOKEN" in hint


# Class-name fallback ----------------------------------------------------


class TestClassifyByClassName:
    @pytest.mark.parametrize(
        "cls_name",
        [
            "FileMetadataError",
            "RepositoryNotFoundError",
            "GatedRepoError",
            "RevisionNotFoundError",
            "EntryNotFoundError",
        ],
    )
    def test_known_class_with_neutral_message_is_classified_by_name(self, monkeypatch, cls_name):
        # Class-name fallback: even when the message has no distinctive
        # substring, an exception class whose name matches the known
        # huggingface_hub family still gets a hint so future
        # huggingface_hub additions don't need a string-table entry.
        hint = _hint(monkeypatch, cls_name, "no distinctive message")
        assert hint is not None
        assert "huggingface.co" in hint
        assert "HF_TOKEN" in hint


# Negative cases ---------------------------------------------------------


class TestClassifyNegative:
    def test_unrelated_exception_returns_none(self, monkeypatch):
        assert _hint(monkeypatch, "ValueError", "something else entirely") is None

    def test_empty_message_unknown_class_returns_none(self, monkeypatch):
        assert _hint(monkeypatch, "RandomError", "") is None

    def test_subclass_with_distinct_message_still_classified(self, monkeypatch):
        # A subclass whose message matches the substring still classifies,
        # even if its class name isn't in the fallback table.
        hint = _hint(
            monkeypatch,
            "MyCustomFileMetadataError",
            "Distant resource does not seem to be on huggingface.co.",
        )
        assert hint is not None
        assert "huggingface.co" in hint
