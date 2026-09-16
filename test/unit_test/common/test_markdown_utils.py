#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
"""`html_to_markdown` keeps the word boundaries the HTML carries.

markdownify drops an inline element that holds nothing but whitespace, and takes the
whitespace with it, so two words end up as one in whatever gets chunked and embedded.
"""

import pytest

from common.markdown_utils import html_to_markdown


@pytest.mark.parametrize(
    "tag",
    ["a", "b", "strong", "em", "i", "u", "s", "del", "code", "sub", "sup"],
)
def test_a_whitespace_only_element_keeps_the_word_boundary(tag):
    attributes = ' href="https://example.com"' if tag == "a" else ""

    assert html_to_markdown(f"<p>First<{tag}{attributes}> </{tag}>Last</p>").strip() == "First Last"


def test_a_space_between_two_styled_runs_survives():
    """A word processor keeps a differently formatted space as a run of its own."""
    assert html_to_markdown("<p><b>First</b><b> </b><b>Last</b></p>").strip() == "**First** **Last**"


def test_a_non_breaking_space_stays_itself():
    assert html_to_markdown("<p>First<b>&#160;</b>Last</p>").strip() == "First Last"


@pytest.mark.parametrize(
    ("html", "expected"),
    [
        ("<p>First <b>bold</b> Last</p>", "First **bold** Last"),
        ("<p>First <em>italic</em> Last</p>", "First *italic* Last"),
        ("<p>First <code>code</code> Last</p>", "First `code` Last"),
        ('<p>First <a href="https://example.com">link</a> Last</p>', "First [link](https://example.com) Last"),
        ("<p>First<b></b>Last</p>", "FirstLast"),
    ],
)
def test_an_element_with_content_converts_as_before(html, expected):
    """The control: only an element with nothing but whitespace in it is passed through."""
    assert html_to_markdown(html).strip() == expected


def test_markdownify_options_are_forwarded():
    assert html_to_markdown("<h1>Title</h1>", heading_style="ATX").strip() == "# Title"


def _table_lines(markdown):
    return [line.strip() for line in markdown.splitlines() if line.strip().startswith("|")]


@pytest.mark.parametrize(
    "html",
    [
        # What mammoth hands over for a Word table: no <thead>, no <th>.
        "<table><tr><td>Product</td><td>Spec</td></tr><tr><td>Cable</td><td>USB-C</td></tr></table>",
        # The same table wrapped in a <tbody>, which is what a browser's DOM has.
        "<table><tbody><tr><td>Product</td><td>Spec</td></tr><tr><td>Cable</td><td>USB-C</td></tr></tbody></table>",
    ],
)
def test_a_table_without_th_uses_its_first_row_as_the_header(html):
    assert _table_lines(html_to_markdown(html)) == ["| Product | Spec |", "| --- | --- |", "| Cable | USB-C |"]


@pytest.mark.parametrize(
    "html",
    [
        "<table><tr><th>Product</th><th>Spec</th></tr><tr><td>Cable</td><td>USB-C</td></tr></table>",
        "<table><thead><tr><th>Product</th><th>Spec</th></tr></thead><tbody><tr><td>Cable</td><td>USB-C</td></tr></tbody></table>",
    ],
)
def test_a_table_that_marks_its_header_is_unchanged(html):
    assert _table_lines(html_to_markdown(html)) == ["| Product | Spec |", "| --- | --- |", "| Cable | USB-C |"]


def test_the_caller_can_still_ask_for_the_empty_header():
    html = "<table><tr><td>Product</td><td>Spec</td></tr><tr><td>Cable</td><td>USB-C</td></tr></table>"

    assert _table_lines(html_to_markdown(html, table_infer_header=False))[0] == "|  |  |"
