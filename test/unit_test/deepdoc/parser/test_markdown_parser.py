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
import types
from pathlib import Path

import pytest

_REPO = Path(__file__).parents[4]


@pytest.fixture
def markdown_element_extractor(monkeypatch):
    try:
        import markdown  # noqa: F401
    except ModuleNotFoundError:
        markdown_stub = types.ModuleType("markdown")
        markdown_stub.markdown = lambda text, extensions=None: text
        monkeypatch.setitem(sys.modules, "markdown", markdown_stub)

    spec = importlib.util.spec_from_file_location(
        "test_markdown_parser_dynamic",
        _REPO / "deepdoc" / "parser" / "markdown_parser.py",
    )
    assert spec and spec.loader
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod.MarkdownElementExtractor


@pytest.mark.p2
class TestMarkdownElementExtractorFences:
    def test_custom_delimiter_preserves_backtick_fence(self, markdown_element_extractor):
        text = "# Title\n```python\nprint('a')\nprint('b')\n```\nAfter"

        sections = markdown_element_extractor(text).extract_elements(delimiter="`\n`", include_meta=True)

        assert [section["content"] for section in sections] == [
            "# Title",
            "```python\nprint('a')\nprint('b')\n```",
            "After",
        ]
        assert sections[1]["start_line"] == 1
        assert sections[1]["end_line"] == 4

    def test_custom_delimiter_still_splits_outside_fences(self, markdown_element_extractor):
        text = "Before\n~~~python\nprint('inside')\n~~~\nAfter"

        sections = markdown_element_extractor(text).extract_elements(delimiter="`\n`")

        assert sections == [
            "Before",
            "~~~python\nprint('inside')\n~~~",
            "After",
        ]

    def test_tilde_fence_is_code_block_without_custom_delimiter(self, markdown_element_extractor):
        text = "# Title\n~~~python\nprint('a')\n~~~\nAfter"

        sections = markdown_element_extractor(text).extract_elements(include_meta=True)

        assert [section["content"] for section in sections] == [
            "# Title",
            "~~~python\nprint('a')\n~~~",
            "After",
        ]
        assert sections[1]["type"] == "code_block"
        assert sections[1]["start_line"] == 1
        assert sections[1]["end_line"] == 3

    def test_longer_outer_fence_preserves_nested_shorter_fence(self, markdown_element_extractor):
        text = "````markdown\n```python\nprint('inner')\n```\n````\nAfter"

        sections = markdown_element_extractor(text).extract_elements(include_meta=True)

        assert [section["content"] for section in sections] == [
            "````markdown\n```python\nprint('inner')\n```\n````",
            "After",
        ]
        assert sections[0]["type"] == "code_block"
        assert sections[0]["start_line"] == 0
        assert sections[0]["end_line"] == 4

    def test_custom_delimiter_preserves_longer_outer_fence(self, markdown_element_extractor):
        text = "Before\n````markdown\n```python\nprint('inner')\n```\n````\nAfter"

        sections = markdown_element_extractor(text).extract_elements(delimiter="`\n`")

        assert sections == [
            "Before",
            "````markdown\n```python\nprint('inner')\n```\n````",
            "After",
        ]


@pytest.mark.p2
class TestMarkdownElementExtractorTables:
    def test_custom_delimiter_preserves_pipe_table(self, markdown_element_extractor):
        text = "# Title\n\n| Name | Value |\n| --- | --- |\n| A | 1 |\n| B | 2 |\n\nAfter"

        sections = markdown_element_extractor(text).extract_elements(delimiter="`\n`", include_meta=True)

        assert [section["content"] for section in sections] == [
            "# Title",
            "| Name | Value |\n| --- | --- |\n| A | 1 |\n| B | 2 |",
            "After",
        ]
        assert sections[1]["start_line"] == 2
        assert sections[1]["end_line"] == 5

    def test_custom_delimiter_preserves_borderless_pipe_table(self, markdown_element_extractor):
        text = "Before\nName | Value\n--- | ---\nA | 1\nB | 2\nAfter"

        sections = markdown_element_extractor(text).extract_elements(delimiter="`\n`")

        assert sections == [
            "Before",
            "Name | Value\n--- | ---\nA | 1\nB | 2",
            "After",
        ]

    def test_custom_delimiter_preserves_html_table(self, markdown_element_extractor):
        text = "Before\n<table>\n<tr><td>A</td></tr>\n<tr><td>B</td></tr>\n</table>\nAfter"

        sections = markdown_element_extractor(text).extract_elements(delimiter="`\n`")

        assert sections == [
            "Before",
            "<table>\n<tr><td>A</td></tr>\n<tr><td>B</td></tr>\n</table>",
            "After",
        ]


@pytest.mark.p2
class TestMarkdownElementExtractorDelimiterHeaders:
    def test_custom_delimiter_merges_consecutive_lone_headers_with_body(self, markdown_element_extractor):
        text = "# Title\n## Intro\nBody paragraph"

        sections = markdown_element_extractor(text).extract_elements(delimiter="`\n`")

        assert sections == ["# Title\n## Intro\nBody paragraph"]

    def test_custom_delimiter_merges_single_lone_header_with_body(self, markdown_element_extractor):
        text = "## Section\nBody paragraph"

        sections = markdown_element_extractor(text).extract_elements(delimiter="`\n`")

        assert sections == ["## Section\nBody paragraph"]
