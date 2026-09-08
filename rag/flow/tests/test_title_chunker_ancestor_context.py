import asyncio
import types

"""Regression test for issue #18293.

HierarchyTitleChunker with method=hierarchy splits the text stream into
separate tree-building runs when a structured image/table chunk
(``doc_type_kwd != "text"``) is encountered.  Before the fix, the flush
that precedes the media chunk cleared *all* heading context, so body
text that still belongs to the same section but appears *after* the
media chunk lost its ancestor heading path.

This test drives the real HierarchyTitleChunker chain with stubbed
dependencies (same pattern as test_title_chunker_position_int.py) and
verifies that post-media body text retains the ancestor heading prefix.
"""

# Re-use the stub-loading context manager from the sibling test module.
from test_title_chunker_position_int import _load_title_chunker_with_stubs


def _build_chunker(common_module, hierarchy_module, items, *, hierarchy=1, levels=None, include_heading_content=False):
    async def _restore_previews(*_a, **_k):
        return None

    common_module.restore_pdf_text_previews = _restore_previews

    from_upstream = types.SimpleNamespace(
        output_format="chunks",
        json_result=None,
        markdown_result=None,
        text_result=None,
        html_result=None,
        chunks=items,
        file=None,
        name="ancestor-test",
    )

    param = common_module.TitleChunkerParam()
    param.method = "hierarchy"
    param.hierarchy = hierarchy
    param.levels = levels or [[r"^# "]]
    param.include_heading_content = include_heading_content
    param.root_chunk_as_heading = False

    process = common_module.ProcessBase(None, "title_chunker", param)
    process._canvas = types.SimpleNamespace(_doc_id="doc", _tenant_id="tenant")
    process._outputs = {}

    return hierarchy_module.HierarchyTitleChunker(process, from_upstream), process


def _run(chunker):
    asyncio.run(chunker.invoke())


def test_post_media_text_keeps_ancestor_heading():
    """One-level hierarchy: body text after an image must carry the H1 prefix."""
    with _load_title_chunker_with_stubs() as (common_module, hierarchy_module):
        items = [
            {"text": "# Chapter 1", "doc_type_kwd": "text"},
            {"text": "body before figure", "doc_type_kwd": "text"},
            {"text": "figure caption", "doc_type_kwd": "image", "img_id": "fig1"},
            {"text": "body after figure", "doc_type_kwd": "text"},
        ]
        chunker, process = _build_chunker(
            common_module,
            hierarchy_module,
            items,
            hierarchy=1,
            include_heading_content=True,
        )
        _run(chunker)
        chunks = process._outputs.get("chunks", [])
        assert chunks, "no chunks emitted"

        after = next(
            (ck for ck in chunks if "body after figure" in ck.get("text", "")),
            None,
        )
        assert after is not None, "no chunk contains 'body after figure'"
        assert "Chapter 1" in after["text"], f"post-media chunk lost ancestor heading, got: {after['text']!r}"


def test_post_media_text_keeps_nested_ancestor_heading():
    """Two-level hierarchy: body text after an image still under ## 1.1
    must carry the full path "Chapter 1 › 1.1 Background"."""
    with _load_title_chunker_with_stubs() as (common_module, hierarchy_module):
        items = [
            {"text": "# Chapter 1", "doc_type_kwd": "text"},
            {"text": "## 1.1 Background", "doc_type_kwd": "text"},
            {"text": "text before figure", "doc_type_kwd": "text"},
            {"text": "figure caption", "doc_type_kwd": "image", "img_id": "fig1"},
            {"text": "text after figure", "doc_type_kwd": "text"},
            {"text": "## 1.2 Goals", "doc_type_kwd": "text"},
            {"text": "text goals", "doc_type_kwd": "text"},
        ]
        chunker, process = _build_chunker(
            common_module,
            hierarchy_module,
            items,
            hierarchy=2,
            levels=[[r"^# ", r"^## "]],
            include_heading_content=True,
        )
        _run(chunker)
        chunks = process._outputs.get("chunks", [])
        assert chunks, "no chunks emitted"

        after = next(
            (ck for ck in chunks if "text after figure" in ck.get("text", "")),
            None,
        )
        assert after is not None, "no chunk contains 'text after figure'"
        assert "Chapter 1" in after["text"], f"post-media chunk lost H1 ancestor, got: {after['text']!r}"
        assert "1.1 Background" in after["text"], f"post-media chunk lost H2 ancestor, got: {after['text']!r}"

        goals = next(
            (ck for ck in chunks if "text goals" in ck.get("text", "")),
            None,
        )
        assert goals is not None, "no chunk contains 'text goals'"
        assert "Chapter 1" in goals["text"], f"sibling heading chunk lost H1 ancestor, got: {goals['text']!r}"

        for ck in chunks:
            text = ck.get("text", "")
            if "figure caption" in text:
                assert "Chapter 1" not in text, f"image chunk should not carry heading text, got: {text!r}"


def test_no_duplicate_heading_only_chunks():
    """Retained headings that get a new sibling must NOT produce a
    heading-only duplicate chunk for the old section."""
    with _load_title_chunker_with_stubs() as (common_module, hierarchy_module):
        items = [
            {"text": "# H1", "doc_type_kwd": "text"},
            {"text": "body one", "doc_type_kwd": "text"},
            {"text": "img", "doc_type_kwd": "image", "img_id": "a"},
            {"text": "# H2", "doc_type_kwd": "text"},
            {"text": "body two", "doc_type_kwd": "text"},
        ]
        chunker, process = _build_chunker(
            common_module,
            hierarchy_module,
            items,
            hierarchy=1,
            include_heading_content=True,
        )
        _run(chunker)
        chunks = process._outputs.get("chunks", [])
        assert chunks, "no chunks emitted"

        h1_only = [ck for ck in chunks if ck.get("text", "").strip() == "H1"]
        assert not h1_only, f"unexpected heading-only duplicate for H1: {[ck['text'] for ck in h1_only]}"
