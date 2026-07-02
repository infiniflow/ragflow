"""Unit tests for SitemapConnector — no network, no external dependencies."""
import importlib
import sys
from datetime import datetime, timezone
from unittest.mock import MagicMock

import pytest

_sitemap_mod = importlib.import_module("common.data_source.sitemap_connector")
SitemapConnector = _sitemap_mod.SitemapConnector
DocumentSource = importlib.import_module("common.data_source.config").DocumentSource

# ---------------------------------------------------------------------------
# Test data
# ---------------------------------------------------------------------------

_NS = "http://www.sitemaps.org/schemas/sitemap/0.9"

_SITEMAP_XML = f"""<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="{_NS}">
  <url>
    <loc>https://example.com/page-1</loc>
    <lastmod>2024-03-15</lastmod>
  </url>
  <url>
    <loc>https://example.com/page-2</loc>
  </url>
</urlset>""".encode()

_SITEMAP_INDEX_XML = f"""<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="{_NS}">
  <sitemap>
    <loc>https://example.com/sitemap-en.xml</loc>
  </sitemap>
</sitemapindex>""".encode()

_CHILD_SITEMAP_XML = f"""<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="{_NS}">
  <url>
    <loc>https://example.com/en/page-1</loc>
    <lastmod>2024-06-01</lastmod>
  </url>
</urlset>""".encode()

_SITEMAP_TWO_PAGES = f"""<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="{_NS}">
  <url><loc>https://example.com/page-1</loc></url>
  <url><loc>https://example.com/page-2</loc></url>
</urlset>""".encode()

_HTML_WITH_PDF = b"""<html><body>
  <a href="/docs/report.pdf">Report</a>
  <a href="https://other.com/external.pdf">External PDF</a>
  <p>Some content about the report.</p>
</body></html>"""

_HTML_SAME_PDF = b'<html><body><a href="/shared.pdf">Shared</a></html>'

_PDF_BYTES = b"%PDF-1.4 fake pdf content"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _fake_response(content: bytes, content_type: str = "text/html; charset=utf-8"):
    resp = MagicMock()
    resp.content = content
    resp.status_code = 200
    resp.headers.get.side_effect = lambda key, default="": {
        "Content-Type": content_type,
    }.get(key, default)
    resp.raise_for_status = MagicMock()
    return resp


def _make_get(url_map: dict):
    def _get(url, **kwargs):
        if url in url_map:
            return url_map[url]
        raise AssertionError(f"Unexpected URL requested in test: {url!r}")
    return _get


def _patch_ssrf(monkeypatch):
    monkeypatch.setattr(_sitemap_mod, "assert_url_is_safe", lambda url: ("example.com", "1.2.3.4"))


def _patch_requests(monkeypatch, url_map: dict):
    monkeypatch.setattr(_sitemap_mod.requests, "get", _make_get(url_map))


def _patch_trafilatura(monkeypatch, text="# Title\n\nBody text."):
    mock = MagicMock()
    mock.extract.return_value = text
    monkeypatch.setitem(sys.modules, "trafilatura", mock)
    return mock


def _connector(**kwargs):
    return SitemapConnector(sitemap_url="https://example.com/sitemap.xml", **kwargs)


# ---------------------------------------------------------------------------
# validate_connector_settings
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_validate_rejects_non_http_scheme():
    connector = SitemapConnector(sitemap_url="ftp://example.com/sitemap.xml")
    with pytest.raises(ValueError, match="valid http or https URL"):
        connector.validate_connector_settings()


@pytest.mark.p2
def test_validate_rejects_batch_size_below_one(monkeypatch):
    _patch_ssrf(monkeypatch)
    connector = _connector(batch_size=0)
    with pytest.raises(ValueError, match="batch_size"):
        connector.validate_connector_settings()


@pytest.mark.p2
def test_validate_rejects_sitemap_with_no_matching_urls(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_requests(monkeypatch, {"https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML)})
    connector = _connector(url_filter=r"/blog/.*")
    with pytest.raises(ValueError, match="no URLs matching"):
        connector.validate_connector_settings()


@pytest.mark.p2
def test_validate_accepts_valid_sitemap(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_requests(monkeypatch, {"https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML)})
    _connector().validate_connector_settings()  # should not raise


# ---------------------------------------------------------------------------
# _iter_sitemap_urls
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_iter_sitemap_urls_parses_standard_urlset(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_requests(monkeypatch, {"https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML)})
    results = list(_connector()._iter_sitemap_urls("https://example.com/sitemap.xml", depth=0))

    assert len(results) == 2
    assert results[0] == ("https://example.com/page-1", datetime(2024, 3, 15, tzinfo=timezone.utc))
    assert results[1] == ("https://example.com/page-2", None)


@pytest.mark.p2
def test_iter_sitemap_urls_recurses_into_index(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_requests(monkeypatch, {
        "https://example.com/sitemap.xml": _fake_response(_SITEMAP_INDEX_XML),
        "https://example.com/sitemap-en.xml": _fake_response(_CHILD_SITEMAP_XML),
    })
    results = list(_connector()._iter_sitemap_urls("https://example.com/sitemap.xml", depth=0))

    assert len(results) == 1
    assert results[0][0] == "https://example.com/en/page-1"


# ---------------------------------------------------------------------------
# _fetch_and_build_document
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_fetch_builds_markdown_doc_for_html(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch, text="# Title\n\nHello world.")
    _patch_requests(monkeypatch, {"https://example.com/page-1": _fake_response(b"<html>page</html>")})
    lastmod = datetime(2024, 3, 15, tzinfo=timezone.utc)
    doc = _connector()._fetch_and_build_document("https://example.com/page-1", lastmod)

    assert doc is not None
    assert doc.source == DocumentSource.SITEMAP
    assert doc.extension == ".md"
    assert doc.semantic_identifier == "example.com/page-1"
    assert b"Hello world" in doc.blob
    assert doc.doc_updated_at == lastmod
    assert "parent_url" not in doc.metadata


@pytest.mark.p2
def test_fetch_builds_pdf_doc_for_pdf_content_type(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_requests(monkeypatch, {
        "https://example.com/doc.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
    })
    doc = _connector()._fetch_and_build_document("https://example.com/doc.pdf", None)

    assert doc is not None
    assert doc.extension == ".pdf"
    assert doc.blob == _PDF_BYTES
    assert doc.semantic_identifier == "example.com/doc.pdf"


@pytest.mark.p2
def test_fetch_skips_empty_html_content(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch, text="")
    _patch_requests(monkeypatch, {"https://example.com/page": _fake_response(b"<html></html>")})
    doc = _connector()._fetch_and_build_document("https://example.com/page", None)
    assert doc is None


# ---------------------------------------------------------------------------
# load_from_state
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_load_from_state_yields_all_documents(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    _patch_requests(monkeypatch, {
        "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
        "https://example.com/page-1": _fake_response(b"<html>p1</html>"),
        "https://example.com/page-2": _fake_response(b"<html>p2</html>"),
    })
    batches = list(_connector(batch_size=10).load_from_state())
    assert sum(len(b) for b in batches) == 2


@pytest.mark.p2
def test_load_from_state_respects_batch_size(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    _patch_requests(monkeypatch, {
        "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
        "https://example.com/page-1": _fake_response(b"<html>p1</html>"),
        "https://example.com/page-2": _fake_response(b"<html>p2</html>"),
    })
    batches = list(_connector(batch_size=1).load_from_state())
    assert len(batches) == 2
    assert all(len(b) == 1 for b in batches)


# ---------------------------------------------------------------------------
# poll_source
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_poll_source_includes_url_within_range(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    _patch_requests(monkeypatch, {
        "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
        "https://example.com/page-1": _fake_response(b"<html>p1</html>"),
    })
    start = datetime(2024, 1, 1, tzinfo=timezone.utc).timestamp()
    end = datetime(2024, 12, 31, tzinfo=timezone.utc).timestamp()
    docs = [doc for batch in _connector().poll_source(start, end) for doc in batch]

    # page-1 has lastmod 2024-03-15 (in range); page-2 has no lastmod (skipped)
    assert [doc.semantic_identifier for doc in docs] == ["example.com/page-1"]


@pytest.mark.p2
def test_poll_source_excludes_url_before_start(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_requests(monkeypatch, {"https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML)})
    # start after 2024-03-15 → page-1 excluded; page-2 has no lastmod → excluded too
    start = datetime(2024, 6, 1, tzinfo=timezone.utc).timestamp()
    end = datetime(2024, 12, 31, tzinfo=timezone.utc).timestamp()
    batches = list(_connector().poll_source(start, end))
    assert batches == []


# ---------------------------------------------------------------------------
# _extract_pdf_links
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_extract_pdf_links_returns_absolute_urls():
    links = SitemapConnector._extract_pdf_links(_HTML_WITH_PDF, "https://example.com/page")
    assert "https://example.com/docs/report.pdf" in links
    assert "https://other.com/external.pdf" in links


@pytest.mark.p2
def test_extract_pdf_links_empty_when_no_pdf_hrefs():
    links = SitemapConnector._extract_pdf_links(b"<html><a href='/page'>link</a></html>", "https://example.com/")
    assert links == []


# ---------------------------------------------------------------------------
# follow_pdf_links
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_follow_pdf_links_yields_pdf_documents(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    _patch_requests(monkeypatch, {
        "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
        "https://example.com/page-1": _fake_response(_HTML_WITH_PDF),
        "https://example.com/page-2": _fake_response(b"<html>no pdfs</html>"),
        "https://example.com/docs/report.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
    })
    connector = _connector(follow_pdf_links=True, restrict_pdf_to_domain=True)
    docs = [doc for batch in connector.load_from_state() for doc in batch]

    assert any(doc.extension == ".pdf" for doc in docs)
    assert any(doc.extension == ".md" for doc in docs)


@pytest.mark.p2
def test_follow_pdf_links_sets_parent_url_in_metadata(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    _patch_requests(monkeypatch, {
        "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
        "https://example.com/page-1": _fake_response(_HTML_WITH_PDF),
        "https://example.com/page-2": _fake_response(b"<html>no pdfs</html>"),
        "https://example.com/docs/report.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
    })
    connector = _connector(follow_pdf_links=True, restrict_pdf_to_domain=True)
    docs = [doc for batch in connector.load_from_state() for doc in batch]
    pdf_docs = [doc for doc in docs if doc.extension == ".pdf"]

    assert len(pdf_docs) == 1
    assert pdf_docs[0].metadata["parent_url"] == "https://example.com/page-1"


@pytest.mark.p2
def test_follow_pdf_links_restricts_to_domain(monkeypatch):
    """External PDF (other.com) must not be fetched when restrict_pdf_to_domain=True."""
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    # https://other.com/external.pdf is intentionally NOT in the map
    # _make_get raises AssertionError if it is called
    _patch_requests(monkeypatch, {
        "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
        "https://example.com/page-1": _fake_response(_HTML_WITH_PDF),
        "https://example.com/page-2": _fake_response(b"<html>no pdfs</html>"),
        "https://example.com/docs/report.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
    })
    connector = _connector(follow_pdf_links=True, restrict_pdf_to_domain=True)
    docs = [doc for batch in connector.load_from_state() for doc in batch]
    pdf_identifiers = [doc.semantic_identifier for doc in docs if doc.extension == ".pdf"]

    assert any("report.pdf" in s for s in pdf_identifiers)
    assert not any("external.pdf" in s for s in pdf_identifiers)


@pytest.mark.p2
def test_follow_pdf_links_allows_external_when_unrestricted(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    _patch_requests(monkeypatch, {
        "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
        "https://example.com/page-1": _fake_response(_HTML_WITH_PDF),
        "https://example.com/page-2": _fake_response(b"<html>no pdfs</html>"),
        "https://example.com/docs/report.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
        "https://other.com/external.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
    })
    connector = _connector(follow_pdf_links=True, restrict_pdf_to_domain=False)
    docs = [doc for batch in connector.load_from_state() for doc in batch]
    pdf_count = sum(1 for doc in docs if doc.extension == ".pdf")
    assert pdf_count == 2


@pytest.mark.p2
def test_pdf_deduplicated_when_linked_from_multiple_pages(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    _patch_requests(monkeypatch, {
        "https://example.com/sitemap.xml": _fake_response(_SITEMAP_TWO_PAGES),
        "https://example.com/page-1": _fake_response(_HTML_SAME_PDF),
        "https://example.com/page-2": _fake_response(_HTML_SAME_PDF),
        # shared.pdf must be fetched exactly once
        "https://example.com/shared.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
    })
    connector = _connector(follow_pdf_links=True, restrict_pdf_to_domain=True)
    docs = [doc for batch in connector.load_from_state() for doc in batch]
    pdf_docs = [doc for doc in docs if doc.extension == ".pdf"]
    assert len(pdf_docs) == 1


# ---------------------------------------------------------------------------
# retrieve_all_slim_docs_perm_sync
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_retrieve_all_slim_docs_yields_one_per_url(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_requests(monkeypatch, {"https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML)})
    slim_docs = [doc for batch in _connector().retrieve_all_slim_docs_perm_sync() for doc in batch]

    assert len(slim_docs) == 2
    assert all(doc.id.startswith("sitemap:") for doc in slim_docs)
