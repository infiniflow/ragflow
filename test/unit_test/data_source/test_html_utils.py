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
import bs4
import pytest

from common.data_source import html_utils
from common.data_source.config import HtmlBasedConnectorTransformLinksStrategy
from common.data_source.html_utils import format_document_soup


def _fmt(html: str) -> str:
    return format_document_soup(bs4.BeautifulSoup(html, "html.parser"))


@pytest.fixture
def markdown_links(monkeypatch):
    """``format_element_text`` only renders links under the markdown strategy."""
    monkeypatch.setattr(
        html_utils,
        "HTML_BASED_CONNECTOR_TRANSFORM_LINKS_STRATEGY",
        HtmlBasedConnectorTransformLinksStrategy.MARKDOWN,
    )


TABLE = "<table><tr><td>A</td><td>B</td></tr></table>"


def test_paragraph_after_table_keeps_its_newline():
    assert "\nAfter" in _fmt(f"<p>Before</p>{TABLE}<p>After</p>")


def test_list_after_table_keeps_hyphen_markers():
    assert "\n- item1" in _fmt(f"<p>Before</p>{TABLE}<ul><li>item1</li><li>item2</li></ul>")


def test_block_after_table_starts_on_a_new_line():
    # The div must not be folded onto the last table row.
    assert "\nTrailing" in _fmt(f"{TABLE}<div>Trailing</div>")


def test_table_still_separates_rows_and_cells():
    # Control: the table itself must keep working — rows on newlines, cells tab-separated.
    assert _fmt("<table><tr><td>A</td><td>B</td></tr><tr><td>C</td><td>D</td></tr></table>") == "A\tB\n\tC\tD"


def test_content_after_table_matches_the_same_content_without_a_table(markdown_links):
    tail = "<p>After</p><ul><li>item1</li></ul>"
    with_table = _fmt(f"{TABLE}{tail}")
    without_table = _fmt(tail)
    assert with_table.endswith(without_table.lstrip("\n"))


def test_text_after_link_in_same_paragraph_is_not_linkified(markdown_links):
    assert _fmt('<p>see <a href="http://x.com">link</a> after</p>') == "see [link](http://x.com) after"


def test_paragraph_after_link_is_not_linkified(markdown_links):
    out = _fmt('<p><a href="http://x.com">link</a></p><p>next paragraph</p>')
    assert "[next paragraph]" not in out


def test_link_inside_anchor_is_still_linkified(markdown_links):
    # The anchor text itself must keep its markdown link.
    assert _fmt('<p><a href="http://x.com">link</a></p>') == "[link](http://x.com)"


def test_table_cells_do_not_inherit_a_preceding_links_href(markdown_links):
    out = _fmt(f'<p><a href="http://x.com">pre</a></p>{TABLE}')
    assert "[A](http://x.com)" not in out


def test_link_inside_a_table_cell_is_linkified(markdown_links):
    assert _fmt('<table><tr><td><a href="http://x.com">cell</a></td></tr></table>') == "[cell](http://x.com)"


def test_link_is_stripped_under_the_default_strategy():
    # Default strategy is STRIP: no markdown link syntax at all.
    assert _fmt('<p>see <a href="http://x.com">link</a> after</p>') == "see link after"
