# Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""Regression tests for the Book built-in parser coordinate (highlight) bug.

Background: PDF coordinate tags are embedded in section text as
``@@page\tleft\ttop\tright\tbottom##`` by ``Pdf.__call__`` (book.py), matching
``extract_positions``' regex ``@@[0-9-]+\t[0-9.\t]+##``. They are extracted by
``crop(need_position=True)`` inside ``tokenize_chunks``. The naive branch of
``book.chunk`` used to run ``s.split("@")`` which destroyed the double-``@``
tag, so the chunk lost its clickable highlight on the source page.

These tests drive ``rag.app.book.chunk`` with a mocked parser so we can inject
synthetic sections carrying the ``@@`` tag, and a fake ``pdf_parser`` that
records the chunk text handed to ``crop`` and only returns a coordinate image
when the tag survived.

The native ``deepdoc`` package is stubbed (as in the other unit tests under
``test/unit_test``) so the test runs without native libraries; ``rag.app.book``
is imported lazily after the stub is in place.
"""

import importlib
import re
import sys
import types

import pytest

# A realistic coordinate tag in the exact format produced by ``Pdf._line_tag``:
# @@<page>\t<x0>\t<x1>\t<top>\t<bottom>##  (matches extract_positions' regex).
TAG = "@@1\t100.0\t200.0\t300.0\t400.0##"

_COORD_RE = r"@@[0-9-]+\t[0-9.\t]+##"


class FakePdf:
    """Stand-in for deepdoc PdfParser that records crop input and only yields
    a coordinate image when the ``@@`` tag is still present in the chunk."""

    def __init__(self):
        self.seen = []

    def crop(self, ck, need_position=False):
        self.seen.append(ck)
        # Use the SAME regex the real deepdoc crop uses
        # (extract_positions: @@[0-9-]+\t[0-9.\t]+##) so the test fails if the
        # coordinate tag ever drifts out of that exact shape -- not just any
        # "@@...##" string. A fake that accepted arbitrary "@@"/"##" would stay
        # green even when the real crop could no longer extract coordinates.
        if re.search(_COORD_RE, ck):
            # (page, left, right, top, bottom)
            return ("<img>", [[0, 1, 2, 3, 4]])
        return (None, [])

    def remove_tag(self, ck):
        import re

        return re.sub(_COORD_RE, "", ck)


def _parser_factory(sections, fake_pdf):
    def _parser(**kwargs):
        return sections, [], fake_pdf

    return _parser


def _patch_book(monkeypatch, book, sections, fake_pdf):
    monkeypatch.setattr(book, "PARSERS", {"deepdoc": _parser_factory(sections, fake_pdf)})
    # Avoid pulling heavy layout/tenant model resolution into the unit test.
    monkeypatch.setattr(book, "normalize_layout_recognizer", lambda x: ("DeepDOC", None))
    monkeypatch.setattr(book, "get_composite_model_name_by_id", lambda *a, **k: None)


@pytest.fixture(autouse=True)
def _env(monkeypatch):
    """Stub the native ``deepdoc`` package tree, then import book/nlp lazily."""
    saved = {}
    for name in list(sys.modules):
        if name == "deepdoc" or name.startswith("deepdoc."):
            saved[name] = sys.modules.pop(name)

    def mk(name, **attrs):
        m = types.ModuleType(name)
        m.__path__ = []
        for k, v in attrs.items():
            setattr(m, k, v)
        sys.modules[name] = m
        return m

    mk("deepdoc")
    dp = mk(
        "deepdoc.parser",
        PdfParser=object,
        HtmlParser=object,
        DocxParser=object,
        EpubParser=object,
        ExcelParser=object,
        JsonParser=object,
        MarkdownElementExtractor=object,
        MarkdownParser=object,
        TxtParser=object,
        PlainParser=object,
        VisionParser=object,
    )
    dp.utils = mk("deepdoc.parser.utils", get_text=lambda *a, **k: "")
    dp.pdf_parser = mk("deepdoc.parser.pdf_parser", PlainParser=object, VisionParser=object)
    dp.figure_parser = mk(
        "deepdoc.parser.figure_parser",
        VisionFigureParser=object,
        vision_figure_parser_docx_wrapper_naive=lambda *a, **k: [],
        vision_figure_parser_pdf_wrapper=lambda *a, **k: [],
        vision_figure_parser_docx_wrapper=lambda *a, **k: [],
    )
    dp.docling_parser = mk("deepdoc.parser.docling_parser", DoclingParser=object)
    dp.tcadp_parser = mk("deepdoc.parser.tcadp_parser", TCADPParser=object)

    nlp = importlib.import_module("rag.nlp")
    book = importlib.import_module("rag.app.book")
    yield book, nlp

    for name in list(sys.modules):
        if name == "deepdoc" or name.startswith("deepdoc."):
            sys.modules.pop(name, None)
    sys.modules.update(saved)


def _chunk(book, sections, fake_pdf, monkeypatch):
    _patch_book(monkeypatch, book, sections, fake_pdf)
    return book.chunk(
        "dummy.pdf",
        parser_config={"chunk_token_num": 256, "delimiter": "\n。；！？"},
        callback=lambda *a, **k: None,
    )


def test_naive_branch_keeps_coord_tags(monkeypatch, _env):
    """The naive branch (bull < 0) must NOT strip the ``@@`` coordinate tag.

    Prose with no 章/节/编/条 markers always routes to the naive branch; before
    the fix ``s.split("@")`` destroyed the tag and every chunk lost coordinates.
    """
    book, _ = _env
    fake = FakePdf()
    sections = [(f"正文{i}。{TAG}", "") for i in range(20)]
    res = _chunk(book, sections, fake, monkeypatch)

    assert fake.seen, "crop was never called"
    assert any(TAG in ck for ck in fake.seen), "naive branch stripped the @@ coordinate tag from every chunk"
    assert any(d.get("image") for d in res), "no chunk carried a coordinate image (tag was lost before crop)"
    # The tag must be stripped from the displayed text by remove_tag().
    assert all("@@" not in d["content_with_weight"] for d in res), "coordinate tag leaked into displayed chunk text"


def test_naive_branch_does_not_append_layoutno(monkeypatch, _env):
    """The legacy layoutno slot must NOT be appended to the chunk text.

    naive_merge's _reconstruct_text_chunk appends a non-empty ``pos`` (layoutno)
    to the text when it is not already present. The book fix passes "" for that
    slot (matching the hierarchical branch and pre-bug behaviour), so a real,
    non-empty layoutno must never leak into the displayed text.
    """
    book, _ = _env
    fake = FakePdf()
    # "ZZ" is a stand-in non-empty layoutno that never appears in the text.
    sections = [(f"正文{i}。{TAG}", "ZZ") for i in range(20)]
    res = _chunk(book, sections, fake, monkeypatch)

    assert any(d.get("image") for d in res), "no chunk carried a coordinate image"
    assert all("ZZ" not in d["content_with_weight"] for d in res), "layoutno leaked into displayed chunk text"


def test_full_scan_recognizes_sparse_bullet_and_is_deterministic(monkeypatch, _env):
    """A bullet beyond the old random k=100 sample must still be recognized,
    and the bull/structure decision must be identical across runs (no jitter).

    We build 200 sections with the ONLY bullet at the very end. After switching
    to a full scan this is always found; and two runs on the same input must
    produce byte-identical crop inputs.

    The image/coordinate assertion alone cannot distinguish the full-scan path
    from a lucky random sample (every section carries a tag, so the image is
    present in any branch). Spy on ``bullets_category`` instead and assert it
    received ALL sections — that directly proves the deterministic full scan.
    """
    book, _ = _env
    classifications = []
    original_bullets_category = book.bullets_category

    def track_bullets_category(all_sections):
        classifications.append(all_sections)
        return original_bullets_category(all_sections)

    # Spy only. Do NOT fail on random_choices: book.chunk still calls it
    # legitimately for the is_english language probe, unrelated to the
    # bull/structure decision.
    monkeypatch.setattr(book, "bullets_category", track_bullets_category)

    secs = [f"正文{i}。{TAG}" for i in range(199)]
    secs.append("第一章 开篇" + TAG)
    sections = [(s, "") for s in secs]

    fake1 = FakePdf()
    res1 = _chunk(book, sections, fake1, monkeypatch)
    assert any(d.get("image") for d in res1), "sparse bullet beyond the random sample was not recognized"

    # Directly prove the full-scan path: bull detection saw every section.
    assert len(classifications) == 1, "bull detection ran more than once"
    assert len(classifications[0]) == len(sections), "bull detection did not scan ALL sections"

    # Determinism: re-running on the same input yields identical crop inputs.
    fake2 = FakePdf()
    _chunk(book, sections, fake2, monkeypatch)
    assert fake1.seen == fake2.seen, "bull/structure decision is non-deterministic across runs"
