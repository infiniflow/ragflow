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

import xml.etree.ElementTree as ET

import pytest

# PubMed depends on biopython (`Bio`); skip cleanly where it isn't installed.
pytest.importorskip("Bio")

from agent.tools.pubmed import PubMed  # noqa: E402


SAMPLE_ARTICLE = """
<PubmedArticle>
  <MedlineCitation>
    <PMID>12345678</PMID>
    <Article>
      <ArticleTitle>Deep learning for retrieval augmented generation</ArticleTitle>
      <Abstract><AbstractText>A short abstract.</AbstractText></Abstract>
      <Journal>
        <Title>Nature Machine Intelligence</Title>
        <JournalIssue><Volume>10</Volume><Issue>2</Issue></JournalIssue>
      </Journal>
      <Pagination><MedlinePgn>101-110</MedlinePgn></Pagination>
      <AuthorList>
        <Author><LastName>Khan</LastName><ForeName>Furqan</ForeName></Author>
        <Author><LastName>Smith</LastName><ForeName>Jane</ForeName></Author>
      </AuthorList>
    </Article>
  </MedlineCitation>
  <PubmedData>
    <ArticleIdList>
      <ArticleId IdType="doi">10.1000/example.doi</ArticleId>
    </ArticleIdList>
  </PubmedData>
</PubmedArticle>
"""


def _format(article_xml: str) -> str:
    # _format_pubmed_content only reads its `child` argument, so we can bypass
    # the canvas-bound __init__ and exercise the pure parsing logic directly.
    pm = PubMed.__new__(PubMed)
    return pm._format_pubmed_content(ET.fromstring(article_xml))


def test_authors_are_parsed_per_author():
    """Regression: authors used to collapse to 'Unknown Authors' because the
    safe_find closure searched from the article root instead of each <Author>."""
    out = _format(SAMPLE_ARTICLE)
    assert "Authors: Furqan Khan, Jane Smith" in out
    assert "Unknown Authors" not in out


def test_other_fields_still_parse():
    out = _format(SAMPLE_ARTICLE)
    assert "Title: Deep learning for retrieval augmented generation" in out
    assert "Journal: Nature Machine Intelligence" in out
    assert "DOI: 10.1000/example.doi" in out


NO_AUTHORS_ARTICLE = """
<PubmedArticle>
  <MedlineCitation>
    <PMID>87654321</PMID>
    <Article>
      <ArticleTitle>An article without an author list</ArticleTitle>
      <Abstract><AbstractText>No authors here.</AbstractText></Abstract>
    </Article>
  </MedlineCitation>
</PubmedArticle>
"""


def test_missing_authors_falls_back():
    out = _format(NO_AUTHORS_ARTICLE)
    assert "Authors: Unknown Authors" in out
