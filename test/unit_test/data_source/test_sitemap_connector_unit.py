"""Unit tests for SitemapConnector — no network, no external dependencies."""

import asyncio
import importlib
import sys
import threading
import time
from contextlib import contextmanager
from datetime import UTC, datetime
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


def _fake_response(content: bytes, content_type: str = "text/html; charset=utf-8", headers: dict | None = None, status_code: int = 200):
    resp = MagicMock()
    resp.content = content
    resp.status_code = status_code
    all_headers = {"Content-Type": content_type, **(headers or {})}
    resp.headers.get.side_effect = lambda key, default="": all_headers.get(key, default)
    # The connector streams the body in chunks; emulate requests' iter_content.
    resp.iter_content.side_effect = lambda chunk_size=65536: [content[i : i + chunk_size] for i in range(0, len(content), chunk_size)]
    resp.raise_for_status = MagicMock()
    resp.close = MagicMock()
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
    # The connector uses a dedicated requests.Session (trust_env=False); patch its get().
    _get = _make_get(url_map)
    monkeypatch.setattr(_sitemap_mod.requests.Session, "get", lambda self, url, **kwargs: _get(url, **kwargs))


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
    assert results[0] == ("https://example.com/page-1", datetime(2024, 3, 15, tzinfo=UTC))
    assert results[1] == ("https://example.com/page-2", None)


@pytest.mark.p2
def test_iter_sitemap_urls_recurses_into_index(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_requests(
        monkeypatch,
        {
            "https://example.com/sitemap.xml": _fake_response(_SITEMAP_INDEX_XML),
            "https://example.com/sitemap-en.xml": _fake_response(_CHILD_SITEMAP_XML),
        },
    )
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
    lastmod = datetime(2024, 3, 15, tzinfo=UTC)
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
    _patch_requests(
        monkeypatch,
        {
            "https://example.com/doc.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
        },
    )
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
    _patch_requests(
        monkeypatch,
        {
            "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
            "https://example.com/page-1": _fake_response(b"<html>p1</html>"),
            "https://example.com/page-2": _fake_response(b"<html>p2</html>"),
        },
    )
    batches = list(_connector(batch_size=10).load_from_state())
    assert sum(len(b) for b in batches) == 2


@pytest.mark.p2
def test_load_from_state_respects_batch_size(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    _patch_requests(
        monkeypatch,
        {
            "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
            "https://example.com/page-1": _fake_response(b"<html>p1</html>"),
            "https://example.com/page-2": _fake_response(b"<html>p2</html>"),
        },
    )
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
    _patch_requests(
        monkeypatch,
        {
            "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
            "https://example.com/page-1": _fake_response(b"<html>p1</html>"),
        },
    )
    start = datetime(2024, 1, 1, tzinfo=UTC).timestamp()
    end = datetime(2024, 12, 31, tzinfo=UTC).timestamp()
    docs = [doc for batch in _connector().poll_source(start, end) for doc in batch]

    # page-1 has lastmod 2024-03-15 (in range); page-2 has no lastmod (skipped)
    assert [doc.semantic_identifier for doc in docs] == ["example.com/page-1"]


@pytest.mark.p2
def test_poll_source_excludes_url_before_start(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_requests(monkeypatch, {"https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML)})
    # start after 2024-03-15 → page-1 excluded; page-2 has no lastmod → excluded too
    start = datetime(2024, 6, 1, tzinfo=UTC).timestamp()
    end = datetime(2024, 12, 31, tzinfo=UTC).timestamp()
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
    _patch_requests(
        monkeypatch,
        {
            "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
            "https://example.com/page-1": _fake_response(_HTML_WITH_PDF),
            "https://example.com/page-2": _fake_response(b"<html>no pdfs</html>"),
            "https://example.com/docs/report.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
        },
    )
    connector = _connector(follow_pdf_links=True, restrict_pdf_to_domain=True)
    docs = [doc for batch in connector.load_from_state() for doc in batch]

    assert any(doc.extension == ".pdf" for doc in docs)
    assert any(doc.extension == ".md" for doc in docs)


@pytest.mark.p2
def test_follow_pdf_links_sets_parent_url_in_metadata(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    _patch_requests(
        monkeypatch,
        {
            "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
            "https://example.com/page-1": _fake_response(_HTML_WITH_PDF),
            "https://example.com/page-2": _fake_response(b"<html>no pdfs</html>"),
            "https://example.com/docs/report.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
        },
    )
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
    _patch_requests(
        monkeypatch,
        {
            "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
            "https://example.com/page-1": _fake_response(_HTML_WITH_PDF),
            "https://example.com/page-2": _fake_response(b"<html>no pdfs</html>"),
            "https://example.com/docs/report.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
        },
    )
    connector = _connector(follow_pdf_links=True, restrict_pdf_to_domain=True)
    docs = [doc for batch in connector.load_from_state() for doc in batch]
    pdf_identifiers = [doc.semantic_identifier for doc in docs if doc.extension == ".pdf"]

    assert any("report.pdf" in s for s in pdf_identifiers)
    assert not any("external.pdf" in s for s in pdf_identifiers)


@pytest.mark.p2
def test_follow_pdf_links_allows_external_when_unrestricted(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    _patch_requests(
        monkeypatch,
        {
            "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
            "https://example.com/page-1": _fake_response(_HTML_WITH_PDF),
            "https://example.com/page-2": _fake_response(b"<html>no pdfs</html>"),
            "https://example.com/docs/report.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
            "https://other.com/external.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
        },
    )
    connector = _connector(follow_pdf_links=True, restrict_pdf_to_domain=False)
    docs = [doc for batch in connector.load_from_state() for doc in batch]
    pdf_count = sum(1 for doc in docs if doc.extension == ".pdf")
    assert pdf_count == 2


@pytest.mark.p2
def test_pdf_deduplicated_when_linked_from_multiple_pages(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    _patch_requests(
        monkeypatch,
        {
            "https://example.com/sitemap.xml": _fake_response(_SITEMAP_TWO_PAGES),
            "https://example.com/page-1": _fake_response(_HTML_SAME_PDF),
            "https://example.com/page-2": _fake_response(_HTML_SAME_PDF),
            # shared.pdf must be fetched exactly once
            "https://example.com/shared.pdf": _fake_response(_PDF_BYTES, content_type="application/pdf"),
        },
    )
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


@pytest.mark.p2
def test_retrieve_all_slim_docs_includes_discovered_pdfs(monkeypatch):
    """With follow_pdf_links, discovered PDFs must be part of the retained set."""
    _patch_ssrf(monkeypatch)
    _patch_requests(
        monkeypatch,
        {
            "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
            "https://example.com/page-1": _fake_response(_HTML_WITH_PDF),
            "https://example.com/page-2": _fake_response(b"<html>no pdfs</html>"),
            # https://other.com/external.pdf is intentionally NOT in the map
        },
    )
    connector = _connector(follow_pdf_links=True, restrict_pdf_to_domain=True)
    slim_ids = {doc.id for batch in connector.retrieve_all_slim_docs_perm_sync() for doc in batch}

    assert slim_ids == {
        connector._build_document_id("https://example.com/page-1"),
        connector._build_document_id("https://example.com/page-2"),
        connector._build_document_id("https://example.com/docs/report.pdf"),
    }


# ---------------------------------------------------------------------------
# build_connector (used by the connection-test endpoint and the sync worker)
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_build_connector_maps_ui_config():
    connector = SitemapConnector.build_connector(
        {
            "sitemap_url": " https://example.com/sitemap.xml ",
            "url_filter": "^https://example\\.com/docs/",
            "follow_pdf_links": True,
            "restrict_pdf_to_domain": False,
            "user_agent": "",
            "batch_size": "3",
            "credentials": {},
        }
    )

    assert connector.sitemap_url == "https://example.com/sitemap.xml"
    assert connector._url_filter is not None and connector._url_filter.pattern == "^https://example\\.com/docs/"
    assert connector.follow_pdf_links is True
    assert connector.restrict_pdf_to_domain is False
    assert connector.user_agent == "RAGFlow-SitemapConnector/1.0"
    assert connector.batch_size == 3


@pytest.mark.p2
def test_build_connector_defaults():
    connector = SitemapConnector.build_connector({"sitemap_url": "https://example.com/sitemap.xml"})

    assert connector._url_filter is None
    assert connector.follow_pdf_links is False
    assert connector.restrict_pdf_to_domain is True
    assert connector.batch_size >= 1


# ---------------------------------------------------------------------------
# Hardening: regex guard, PDF link detection, DNS pinning, size cap, fetch budget
# ---------------------------------------------------------------------------


@pytest.mark.p2
@pytest.mark.parametrize("pattern", ["^https://(a+)+$", "(x*y)*z", "(?:ab*)+c"])
def test_url_filter_rejects_nested_quantifiers(pattern):
    with pytest.raises(ValueError, match="nested quantifiers"):
        _connector(url_filter=pattern)


@pytest.mark.p2
def test_url_filter_rejects_too_long_pattern():
    with pytest.raises(ValueError, match="too long"):
        _connector(url_filter="a" * (_sitemap_mod._MAX_URL_FILTER_LENGTH + 1))


@pytest.mark.p2
def test_url_filter_accepts_plain_patterns():
    connector = _connector(url_filter=r"^https://example\.com/(docs|blog)/[a-z0-9-]+$")
    assert connector._url_matches("https://example.com/docs/intro")
    assert not connector._url_matches("https://example.com/pricing")


@pytest.mark.p2
def test_extract_pdf_links_accepts_query_strings_and_fragments():
    html = b"""<html><body>
      <a href="/manual.pdf?download=1">Manual</a>
      <a href="/guide.PDF#page=3">Guide</a>
      <a href="/page.html?file=x.pdf">Not a PDF</a>
      <a href="report.pdf">Relative</a>
    </body></html>"""
    links = SitemapConnector._extract_pdf_links(html, "https://example.com/docs/index.html")
    assert links == [
        "https://example.com/manual.pdf?download=1",
        "https://example.com/guide.PDF#page=3",
        "https://example.com/docs/report.pdf",
    ]


@pytest.mark.p2
def test_fetch_raw_pins_dns_on_every_hop(monkeypatch):
    pins: list[tuple[str, str]] = []

    @contextmanager
    def _recording_pin(hostname, ip):
        pins.append((hostname, ip))
        yield

    monkeypatch.setattr(_sitemap_mod, "_pin_dns", _recording_pin)
    monkeypatch.setattr(
        _sitemap_mod,
        "assert_url_is_safe",
        lambda url: ("example.com", "1.2.3.4") if "example.com" in url else ("cdn.example.net", "5.6.7.8"),
    )
    redirect = _fake_response(b"", headers={"Location": "https://cdn.example.net/final"}, status_code=302)
    _patch_requests(
        monkeypatch,
        {
            "https://example.com/start": redirect,
            "https://cdn.example.net/final": _fake_response(b"<html>ok</html>"),
        },
    )

    content, _ = _connector()._fetch_raw("https://example.com/start")

    assert content == b"<html>ok</html>"
    assert pins == [("example.com", "1.2.3.4"), ("cdn.example.net", "5.6.7.8")]


@pytest.mark.p2
def test_fetch_raw_rejects_oversized_content_length(monkeypatch):
    _patch_ssrf(monkeypatch)
    big = _fake_response(b"x", headers={"Content-Length": str(_sitemap_mod._MAX_RESPONSE_BYTES + 1)})
    _patch_requests(monkeypatch, {"https://example.com/big": big})

    with pytest.raises(ValueError, match="maximum allowed size"):
        _connector()._fetch_raw("https://example.com/big")
    big.close.assert_called()


@pytest.mark.p2
def test_fetch_raw_rejects_oversized_streamed_body(monkeypatch):
    _patch_ssrf(monkeypatch)
    monkeypatch.setattr(_sitemap_mod, "_MAX_RESPONSE_BYTES", 10)
    monkeypatch.setattr(_sitemap_mod, "_READ_CHUNK_BYTES", 4)
    _patch_requests(monkeypatch, {"https://example.com/stream": _fake_response(b"0123456789ABCDEF")})

    with pytest.raises(ValueError, match="maximum allowed size"):
        _connector()._fetch_raw("https://example.com/stream")


@pytest.mark.p2
def test_iter_sitemap_urls_visits_each_sitemap_once(monkeypatch):
    """A sitemap index referencing itself and its parent must not be re-fetched."""
    _patch_ssrf(monkeypatch)
    looping_index = f"""<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="{_NS}">
  <sitemap><loc>https://example.com/sitemap.xml</loc></sitemap>
  <sitemap><loc>https://example.com/sitemap-en.xml</loc></sitemap>
  <sitemap><loc>https://example.com/sitemap-en.xml</loc></sitemap>
</sitemapindex>""".encode()
    calls: list[str] = []
    responses = {
        "https://example.com/sitemap.xml": _fake_response(looping_index),
        "https://example.com/sitemap-en.xml": _fake_response(_CHILD_SITEMAP_XML),
    }

    def _get(url, **kwargs):
        calls.append(url)
        return responses[url]

    monkeypatch.setattr(_sitemap_mod.requests.Session, "get", lambda self, url, **kwargs: _get(url, **kwargs))
    urls = [u for u, _ in _connector()._iter_sitemap_urls("https://example.com/sitemap.xml", depth=0)]

    assert urls == ["https://example.com/en/page-1"]
    assert sorted(calls) == ["https://example.com/sitemap-en.xml", "https://example.com/sitemap.xml"]


@pytest.mark.p2
def test_iter_sitemap_urls_stops_at_fetch_budget(monkeypatch):
    _patch_ssrf(monkeypatch)
    monkeypatch.setattr(_sitemap_mod, "_MAX_SITEMAP_FETCHES", 3)
    children = "".join(f"<sitemap><loc>https://example.com/child-{i}.xml</loc></sitemap>" for i in range(10))
    index = f'<?xml version="1.0" encoding="UTF-8"?><sitemapindex xmlns="{_NS}">{children}</sitemapindex>'.encode()
    responses = {"https://example.com/sitemap.xml": _fake_response(index)}
    for i in range(10):
        child = f'<?xml version="1.0" encoding="UTF-8"?><urlset xmlns="{_NS}"><url><loc>https://example.com/page-{i}</loc></url></urlset>'.encode()
        responses[f"https://example.com/child-{i}.xml"] = _fake_response(child)
    calls: list[str] = []

    def _get(url, **kwargs):
        calls.append(url)
        return responses[url]

    monkeypatch.setattr(_sitemap_mod.requests.Session, "get", lambda self, url, **kwargs: _get(url, **kwargs))
    urls = [u for u, _ in _connector()._iter_sitemap_urls("https://example.com/sitemap.xml", depth=0)]

    assert len(calls) == 3  # index + 2 children
    assert urls == ["https://example.com/page-0", "https://example.com/page-1"]


@pytest.mark.p2
def test_session_ignores_environment_proxies_and_netrc():
    connector = _connector()
    assert connector._session.trust_env is False


@pytest.mark.p2
def test_url_filter_evaluation_is_time_bounded(monkeypatch):
    """`(a|aa)+` cannot be rejected syntactically; the timeout must stop the match."""
    monkeypatch.setattr(_sitemap_mod, "_URL_FILTER_TIMEOUT_SECONDS", 0.05)
    connector = _connector(url_filter=r"^https://(a|aa)+$")
    start = time.monotonic()

    assert connector._url_matches("https://" + "a" * 60 + "b") is False
    assert time.monotonic() - start < 2.0
    assert connector._url_matches("https://aaaa") is True


# ---------------------------------------------------------------------------
# iter_in_worker_thread (bridge used by the sync worker)
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_iter_in_worker_thread_yields_all_items_from_another_thread():
    producer_threads: set[str] = set()

    def _source():
        for i in range(5):
            producer_threads.add(threading.current_thread().name)
            yield [i]

    items = list(_sitemap_mod.iter_in_worker_thread(_source(), maxsize=1))

    assert items == [[0], [1], [2], [3], [4]]
    assert producer_threads == {"sitemap-batch-producer"}


@pytest.mark.p2
def test_iter_in_worker_thread_propagates_exceptions():
    def _source():
        yield [1]
        raise RuntimeError("boom")

    it = _sitemap_mod.iter_in_worker_thread(_source())
    assert next(it) == [1]
    with pytest.raises(RuntimeError, match="boom"):
        next(it)


@pytest.mark.p2
def test_iter_in_worker_thread_stops_producer_when_consumer_closes():
    produced: list[int] = []
    closed = threading.Event()

    def _source():
        try:
            for i in range(1000):
                produced.append(i)
                yield [i]
        finally:
            closed.set()

    it = _sitemap_mod.iter_in_worker_thread(_source(), maxsize=1)
    assert next(it) == [0]
    it.close()

    assert closed.wait(timeout=5)
    assert len(produced) < 10


@pytest.mark.p2
def test_iter_in_worker_thread_terminal_writes_do_not_block_after_close():
    """Producer must exit even if the consumer closes while the queue is full."""
    started = threading.Event()

    def _source():
        started.set()
        for i in range(50):
            yield [i]

    it = _sitemap_mod.iter_in_worker_thread(_source(), maxsize=1)
    assert started.wait(timeout=5)
    time.sleep(0.2)  # let the producer fill the queue and block on put()
    it.close()
    it.join(timeout=5)
    assert it.producer_alive is False

    def _failing():
        yield [1]
        raise RuntimeError("late failure")

    it = _sitemap_mod.iter_in_worker_thread(_failing(), maxsize=1)
    time.sleep(0.2)
    it.close()
    it.join(timeout=5)
    assert it.producer_alive is False


@pytest.mark.p2
def test_iter_in_worker_thread_close_cancels_an_in_flight_fetch():
    """Closing the consumer must interrupt the source at its next cancellation check."""
    cancel = threading.Event()
    fetch_started = threading.Event()
    fetch_aborted = threading.Event()

    def _slow_source():
        yield ["first"]
        fetch_started.set()
        # emulate a long fetch that polls the cancellation flag (as _read_capped does)
        for _ in range(200):
            if cancel.is_set():
                fetch_aborted.set()
                return
            time.sleep(0.02)
        yield ["never"]

    it = _sitemap_mod.iter_in_worker_thread(_slow_source(), maxsize=1, on_close=cancel.set)
    assert next(it) == ["first"]
    assert fetch_started.wait(timeout=5)
    it.close()

    assert fetch_aborted.wait(timeout=5)
    it.join(timeout=5)
    assert it.producer_alive is False


@pytest.mark.p2
def test_cancel_stops_traversal_before_next_fetch(monkeypatch):
    _patch_ssrf(monkeypatch)
    _patch_trafilatura(monkeypatch)
    fetched: list[str] = []
    responses = {
        "https://example.com/sitemap.xml": _fake_response(_SITEMAP_XML),
        "https://example.com/page-1": _fake_response(b"<html>1</html>"),
        "https://example.com/page-2": _fake_response(b"<html>2</html>"),
    }
    connector = _connector(batch_size=1)

    def _get(self, url, **kwargs):
        fetched.append(url)
        if url.endswith("/page-1"):
            connector.cancel()  # cancellation arrives while the first page is being fetched
        return responses[url]

    monkeypatch.setattr(_sitemap_mod.requests.Session, "get", _get)
    batches = list(connector.load_from_state())

    assert connector.cancelled is True
    assert "https://example.com/page-2" not in fetched
    assert len(batches) <= 1


@pytest.mark.p2
def test_read_capped_aborts_when_cancelled(monkeypatch):
    connector = _connector()
    chunks = iter([b"aaaa", b"bbbb", b"cccc"])

    def _iter_content(chunk_size=65536):
        for chunk in chunks:
            connector.cancel()
            yield chunk

    resp = _fake_response(b"")
    resp.iter_content.side_effect = _iter_content

    with pytest.raises(ValueError, match="cancelled"):
        connector._read_capped(resp, "https://example.com/big")
    resp.close.assert_called()


@pytest.mark.p2
async def test_validate_connector_in_thread_signals_cancellation_on_timeout():
    """A task timeout during validation must tell the connector to stop."""
    connector = _connector()
    started = threading.Event()
    finished = threading.Event()

    def _blocking_validate():
        started.set()
        while not connector.cancelled:
            time.sleep(0.01)
        finished.set()

    connector.validate_connector_settings = _blocking_validate  # type: ignore[method-assign]

    with pytest.raises(asyncio.TimeoutError):
        await asyncio.wait_for(_sitemap_mod.validate_connector_in_thread(connector), timeout=0.2)

    assert started.is_set()
    assert connector.cancelled is True
    assert finished.wait(timeout=5)


@pytest.mark.p2
async def test_validate_connector_in_thread_propagates_validation_errors():
    connector = _connector()
    connector.validate_connector_settings = lambda: (_ for _ in ()).throw(ValueError("bad sitemap"))  # type: ignore[method-assign]

    with pytest.raises(ValueError, match="bad sitemap"):
        await _sitemap_mod.validate_connector_in_thread(connector)
    assert connector.cancelled is False
