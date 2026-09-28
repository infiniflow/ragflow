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


_PARAGRAPH = "<p>" + " ".join(["This paragraph describes the installation process in enough detail to count as real content."] * 3) + "</p>"
# Realistic size on purpose: on very short documents trafilatura falls back to its
# plain-text baseline and emits no Markdown structure.
PAGE = f"""<html><head><title>Guide</title></head><body>
<nav><a href="/">Home</a></nav>
<main><h1>Getting started</h1><p>Install the <a href="/docs/cli">CLI</a> first.</p>{_PARAGRAPH}
<h2>Steps</h2><ul><li>step one</li><li>step two</li></ul>{_PARAGRAPH}</main>
<footer>© Example</footer></body></html>"""


def test_web_html_to_markdown_keeps_structure_and_drops_boilerplate():
    parsed = html_utils.web_html_to_markdown(PAGE)

    assert parsed.title == "Guide"
    assert "# Getting started" in parsed.cleaned_text
    assert "## Steps" in parsed.cleaned_text
    assert "[CLI](/docs/cli)" in parsed.cleaned_text
    assert "step one" in parsed.cleaned_text and "step two" in parsed.cleaned_text
    assert "Home" not in parsed.cleaned_text  # <nav> is in WEB_CONNECTOR_IGNORED_ELEMENTS
    assert "© Example" not in parsed.cleaned_text  # <footer> too


def test_web_html_to_markdown_does_not_depend_on_the_flat_text_switch(monkeypatch):
    monkeypatch.setattr(html_utils, "PARSE_WITH_TRAFILATURA", False)
    parsed = html_utils.web_html_to_markdown(PAGE)

    assert "# Getting started" in parsed.cleaned_text


def test_web_html_to_markdown_falls_back_to_flat_text_when_trafilatura_fails(monkeypatch):
    def _boom(html_content, output_format="txt"):
        raise RuntimeError("trafilatura exploded")

    monkeypatch.setattr(html_utils, "parse_html_with_trafilatura", _boom)
    parsed = html_utils.web_html_to_markdown(PAGE)

    assert parsed.title == "Guide"
    assert not parsed.cleaned_text.startswith("Guide")  # title tag dropped before the bs4 fallback
    assert "Getting started" in parsed.cleaned_text
    assert "Home" not in parsed.cleaned_text


def test_web_html_cleanup_and_markdown_share_the_same_cleanup():
    flat = html_utils.web_html_cleanup(PAGE)
    markdown = html_utils.web_html_to_markdown(PAGE)

    assert flat.title == markdown.title == "Guide"
    for boilerplate in ("Home", "© Example"):
        assert boilerplate not in flat.cleaned_text
        assert boilerplate not in markdown.cleaned_text
    assert "Getting started" in flat.cleaned_text
    assert "# Getting started" in markdown.cleaned_text
