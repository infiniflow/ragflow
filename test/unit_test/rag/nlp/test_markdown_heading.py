#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#

import importlib.util
from pathlib import Path

import pytest

_MODULE_PATH = Path(__file__).resolve().parents[4] / "rag/nlp/markdown_heading.py"
_SPEC = importlib.util.spec_from_file_location("markdown_heading", _MODULE_PATH)
assert _SPEC and _SPEC.loader
_markdown_heading = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(_markdown_heading)
group_markdown_sections_by_headings = _markdown_heading.group_markdown_sections_by_headings
is_atx_heading_line = _markdown_heading.is_atx_heading_line
split_text_by_atx_headings = _markdown_heading.split_text_by_atx_headings


SAMPLE_MD = """# Introduction

Intro body.

## Section A

Content for A.

### Subsection

Nested content.

## Section B

Final content.
"""


class TestMarkdownHeading:
    @pytest.mark.p2
    def test_is_atx_heading_line(self):
        assert is_atx_heading_line("# Title")
        assert is_atx_heading_line("###### Deep")
        assert not is_atx_heading_line("Not a heading")
        assert not is_atx_heading_line("#no space")

    @pytest.mark.p2
    def test_split_text_by_atx_headings(self):
        chunks = split_text_by_atx_headings(SAMPLE_MD)
        assert len(chunks) == 4
        assert chunks[0].startswith("# Introduction")
        assert "## Section A" in chunks[1]
        assert "### Subsection" in chunks[2]
        assert chunks[3].startswith("## Section B")

    @pytest.mark.p2
    def test_group_markdown_sections_by_headings(self):
        sections = [
            ("# Introduction", ""),
            ("Intro body.", ""),
            ("## Section A", ""),
            ("Content for A.", ""),
            ("## Section B", ""),
            ("Final content.", ""),
        ]
        chunks = group_markdown_sections_by_headings(sections)
        assert len(chunks) == 3
        assert "Intro body." in chunks[0]
        assert "Content for A." in chunks[1]
        assert "Final content." in chunks[2]

    @pytest.mark.p2
    def test_group_respects_max_depth(self):
        sections = [
            ("### Only H3", ""),
            ("Body under h3.", ""),
            ("## H2 section", ""),
            ("More body.", ""),
        ]
        chunks = group_markdown_sections_by_headings(sections, max_depth=2)
        assert any("### Only H3" in chunk for chunk in chunks)
        assert any("## H2 section" in chunk for chunk in chunks)

    @pytest.mark.p2
    def test_oversized_section_splits_by_paragraph(self):
        long_body = "word " * 500
        sections = [("## Big section", ""), (long_body, "")]
        chunks = group_markdown_sections_by_headings(sections, max_token_num=50)
        assert len(chunks) >= 2
