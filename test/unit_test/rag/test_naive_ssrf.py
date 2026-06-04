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
"""
Unit tests for SSRF protection added in PR #15214:
- Markdown._fetch_remote_image_response blocks private/loopback/metadata IPs
- Redirects to private addresses are blocked
- Redirect loops terminate at _MAX_IMAGE_REDIRECTS
- response.close() is called on all code paths (no resource leak)
- cache=None creates a fresh dict; cache={} (falsy-but-valid) is preserved

The real ``Markdown`` class is loaded from ``rag/app/naive.py`` via importlib
with all heavy dependencies (docx, PIL, deepdoc, api.db, …) stubbed out,
exactly as the repo's other unit tests do. Only ``common.ssrf_guard`` is
imported directly since it is stdlib-only and is the module under test.
"""

import importlib.util
import socket
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock, patch

import pytest

# ---------------------------------------------------------------------------
# Re-use the repository's standard stub helper
# ---------------------------------------------------------------------------

def _stub(monkeypatch, name, **attrs):
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


# ---------------------------------------------------------------------------
# Load the real naive.py with heavy deps stubbed
# ---------------------------------------------------------------------------

def _load_naive(monkeypatch):
    """Load rag/app/naive.py with all non-stdlib dependencies stubbed.

    Returns the module so tests can access ``module.Markdown`` and
    ``module._MAX_IMAGE_REDIRECTS``.
    """
    # docx stubs
    _stub(monkeypatch, "docx", Document=object)
    _stub(monkeypatch, "docx.opc.pkgreader", _SerializedRelationships=object, _SerializedRelationship=object)
    _stub(monkeypatch, "docx.table", Table=object)
    _stub(monkeypatch, "docx.text.paragraph", Paragraph=object)
    _stub(monkeypatch, "docx.opc.oxml", parse_xml=lambda *_a, **_k: None)

    # markdown / PIL stubs
    _stub(monkeypatch, "markdown", markdown=lambda *_a, **_k: "")
    pil_image = MagicMock()
    pil_image.open = MagicMock(return_value=MagicMock(convert=MagicMock(return_value=MagicMock())))
    _stub(monkeypatch, "PIL", Image=pil_image)
    _stub(monkeypatch, "PIL.Image", open=pil_image.open)

    # common stubs (except ssrf_guard — we want the real one)
    _stub(monkeypatch, "common.token_utils", num_tokens_from_string=lambda s: len(s or ""))
    _stub(monkeypatch, "common.constants", LLMType=SimpleNamespace(), MAXIMUM_PAGE_NUMBER=100)
    _stub(monkeypatch, "common.float_utils", normalize_overlapped_percent=lambda *_a, **_k: None)
    _stub(monkeypatch, "common.parser_config_utils", normalize_layout_recognizer=lambda *_a, **_k: None)
    _stub(monkeypatch, "common.text_utils", normalize_arabic_presentation_forms=lambda s: s)

    # api.db stubs
    _stub(monkeypatch, "api.db.services.llm_service", LLMBundle=SimpleNamespace())
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        get_tenant_default_model_by_type=lambda *_a, **_k: None,
        get_model_config_from_provider_instance=lambda *_a, **_k: {},
    )

    # rag.utils and rag.nlp stubs
    _stub(
        monkeypatch,
        "rag.utils.file_utils",
        extract_embed_file=lambda *_a, **_k: None,
        extract_links_from_pdf=lambda *_a, **_k: [],
        extract_links_from_docx=lambda *_a, **_k: [],
        extract_html=lambda *_a, **_k: "",
    )
    _stub(
        monkeypatch,
        "rag.nlp",
        concat_img=lambda *_a, **_k: None,
        find_codec=lambda *_a, **_k: "utf-8",
        naive_merge=lambda *_a, **_k: [],
        naive_merge_with_images=lambda *_a, **_k: [],
        naive_merge_docx=lambda *_a, **_k: [],
        rag_tokenizer=SimpleNamespace(),
        tokenize_chunks=lambda *_a, **_k: [],
        doc_tokenize_chunks_with_images=lambda *_a, **_k: [],
        tokenize_table=lambda *_a, **_k: [],
        append_context2table_image4pdf=lambda *_a, **_k: None,
        tokenize_chunks_with_images=lambda *_a, **_k: [],
    )

    # deepdoc stubs
    _stub(
        monkeypatch,
        "deepdoc.parser",
        DocxParser=object,
        EpubParser=object,
        ExcelParser=object,
        HtmlParser=object,
        JsonParser=object,
        MarkdownElementExtractor=object,
        MarkdownParser=object,
        PdfParser=object,
        TxtParser=object,
    )
    _stub(
        monkeypatch,
        "deepdoc.parser.figure_parser",
        VisionFigureParser=object,
        vision_figure_parser_docx_wrapper_naive=lambda *_a, **_k: None,
        vision_figure_parser_pdf_wrapper=lambda *_a, **_k: None,
    )
    _stub(monkeypatch, "deepdoc.parser.pdf_parser", PlainParser=object, VisionParser=object)
    _stub(monkeypatch, "deepdoc.parser.docling_parser", DoclingParser=object)
    _stub(monkeypatch, "deepdoc.parser.tcadp_parser", TCADPParser=object)

    repo_root = Path(__file__).resolve().parents[3]
    module_path = repo_root / "rag" / "app" / "naive.py"
    spec = importlib.util.spec_from_file_location("test_naive_ssrf_naive", module_path)
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_naive_ssrf_naive", module)
    spec.loader.exec_module(module)
    return module


# ---------------------------------------------------------------------------
# Helpers shared across tests
# ---------------------------------------------------------------------------

def _make_response(status_code=200, content_type="image/png", body=b"", location=None):
    r = MagicMock()
    r.status_code = status_code
    headers = {"Content-Type": content_type}
    if location:
        headers["Location"] = location
    r.headers = headers
    r.content = body
    r.close = MagicMock()
    return r


def _public_dns(host, port, *a, **kw):
    """Return a single public IP for any hostname."""
    return [(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("93.184.216.34", port or 0))]


# ---------------------------------------------------------------------------
# Tests for Markdown._fetch_remote_image_response
# ---------------------------------------------------------------------------

@pytest.mark.p1
class TestFetchRemoteImageResponse:

    @pytest.mark.p1
    def test_blocks_localhost(self, monkeypatch):
        md = _load_naive(monkeypatch).Markdown.__new__(_load_naive(monkeypatch).Markdown)
        assert md._fetch_remote_image_response("http://localhost/image.png") is None

    @pytest.mark.p1
    def test_blocks_loopback_ip(self, monkeypatch):
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)
        assert md._fetch_remote_image_response("http://127.0.0.1/image.png") is None

    @pytest.mark.p1
    def test_blocks_private_10_range(self, monkeypatch):
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)
        assert md._fetch_remote_image_response("http://10.0.0.1/image.png") is None

    @pytest.mark.p1
    def test_blocks_private_192_168_range(self, monkeypatch):
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)
        assert md._fetch_remote_image_response("http://192.168.1.1/image.png") is None

    @pytest.mark.p1
    def test_blocks_private_172_16_range(self, monkeypatch):
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)
        assert md._fetch_remote_image_response("http://172.16.0.1/image.png") is None

    @pytest.mark.p1
    def test_blocks_metadata_169_254(self, monkeypatch):
        """AWS/GCP instance-metadata endpoint must be blocked."""
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)
        assert md._fetch_remote_image_response("http://169.254.169.254/latest/meta-data/") is None

    @pytest.mark.p1
    def test_blocks_file_scheme(self, monkeypatch):
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)
        assert md._fetch_remote_image_response("file:///etc/passwd") is None

    @pytest.mark.p1
    def test_redirect_to_private_is_blocked(self, monkeypatch):
        """A 302 from a public host to a private IP must be refused."""
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)

        redirect_resp = _make_response(status_code=302, location="http://192.168.1.1/secret")

        with patch("common.ssrf_guard.socket.getaddrinfo") as mock_dns, \
             patch("requests.get", return_value=redirect_resp):
            def dns_side(host, port, *a, **kw):
                # example.com → public; IP literals resolve to themselves
                if host == "example.com":
                    return [(socket.AF_INET, socket.SOCK_STREAM, 6, "", ("93.184.216.34", port or 0))]
                return [(socket.AF_INET, socket.SOCK_STREAM, 6, "", (host, port or 0))]
            mock_dns.side_effect = dns_side

            result = md._fetch_remote_image_response("http://example.com/redirect")

        assert result is None
        assert redirect_resp.close.call_count >= 1

    @pytest.mark.p1
    def test_redirect_loop_terminates(self, monkeypatch):
        """More than _MAX_IMAGE_REDIRECTS hops must return None."""
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)
        max_redirects = module._MAX_IMAGE_REDIRECTS

        loop_resp = _make_response(status_code=302, location="http://example.com/loop")

        with patch("common.ssrf_guard.socket.getaddrinfo", side_effect=_public_dns), \
             patch("requests.get", return_value=loop_resp):
            result = md._fetch_remote_image_response("http://example.com/loop")

        assert result is None
        # close() is called once per redirect hop (before re-validating the target)
        assert loop_resp.close.call_count == max_redirects + 1

    @pytest.mark.p1
    def test_successful_public_fetch_returns_response(self, monkeypatch):
        """A valid public URL must return the response object."""
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)

        ok_resp = _make_response(status_code=200, content_type="image/png", body=b"PNG")

        with patch("common.ssrf_guard.socket.getaddrinfo", side_effect=_public_dns), \
             patch("requests.get", return_value=ok_resp):
            result = md._fetch_remote_image_response("http://example.com/image.png")

        assert result is not None
        assert result.status_code == 200


# ---------------------------------------------------------------------------
# Tests for Markdown.load_images_from_urls
# ---------------------------------------------------------------------------

@pytest.mark.p1
class TestLoadImagesFromUrls:

    @pytest.mark.p1
    def test_none_cache_creates_fresh_dict_each_call(self, monkeypatch):
        """cache=None must not share state across calls (mutable default bug)."""
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)
        _, cache1 = md.load_images_from_urls([], cache=None)
        _, cache2 = md.load_images_from_urls([], cache=None)
        assert cache1 is not cache2

    @pytest.mark.p1
    def test_empty_cache_dict_is_preserved(self, monkeypatch):
        """cache={} (falsy but valid) must not be replaced by a new dict."""
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)
        my_cache = {}
        _, returned = md.load_images_from_urls([], cache=my_cache)
        assert returned is my_cache

    @pytest.mark.p1
    def test_private_url_stored_as_none_in_cache(self, monkeypatch):
        """Private URLs must appear in cache as None so they are not re-fetched."""
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)
        url = "http://127.0.0.1/image.png"
        images, cache = md.load_images_from_urls([url])
        assert images == []
        assert url in cache
        assert cache[url] is None

    @pytest.mark.p1
    def test_response_closed_on_non_image_content_type(self, monkeypatch):
        """response.close() must be called even when Content-Type is not image/*."""
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)

        resp = _make_response(status_code=200, content_type="text/html", body=b"<html/>")

        with patch("common.ssrf_guard.socket.getaddrinfo", side_effect=_public_dns), \
             patch("requests.get", return_value=resp):
            images, _ = md.load_images_from_urls(["http://example.com/page.html"])

        resp.close.assert_called_once()
        assert images == []

    @pytest.mark.p1
    def test_cache_hit_skips_fetch(self, monkeypatch):
        """A URL already in cache must not trigger a network fetch."""
        module = _load_naive(monkeypatch)
        md = module.Markdown.__new__(module.Markdown)

        sentinel = MagicMock()
        cache = {"http://example.com/img.png": sentinel}

        with patch("requests.get") as mock_get:
            images, _ = md.load_images_from_urls(["http://example.com/img.png"], cache=cache)

        mock_get.assert_not_called()
        assert sentinel in images


# ---------------------------------------------------------------------------
# Direct tests of assert_url_is_safe (stdlib-only; no stub needed)
# ---------------------------------------------------------------------------

@pytest.mark.p1
class TestSSRFGuardDirect:

    @pytest.mark.p1
    @pytest.mark.parametrize("url", [
        "file:///etc/passwd",
        "ftp://example.com/file",
        "http://localhost/",
        "http://127.0.0.1/",
        "http://10.0.0.1/",
        "http://192.168.0.1/",
        "http://172.16.1.1/",
        "http://169.254.169.254/",
        "http:///no-host",
    ])
    def test_blocked_url(self, url):
        from common.ssrf_guard import assert_url_is_safe
        with pytest.raises(ValueError):
            assert_url_is_safe(url)

    @pytest.mark.p1
    def test_public_ip_allowed(self):
        from common.ssrf_guard import assert_url_is_safe
        with patch("common.ssrf_guard.socket.getaddrinfo", side_effect=_public_dns):
            hostname, ip = assert_url_is_safe("http://example.com/image.png")
        assert hostname == "example.com"
        assert ip == "93.184.216.34"

    @pytest.mark.p1
    def test_ipv4_mapped_ipv6_loopback_blocked(self):
        """::ffff:127.0.0.1 must be normalized to its IPv4 form and blocked."""
        from common.ssrf_guard import assert_url_is_safe

        def dns_ipv4_mapped(host, port, *a, **kw):
            return [(socket.AF_INET6, socket.SOCK_STREAM, 6, "", ("::ffff:127.0.0.1", port or 0, 0, 0))]

        with patch("common.ssrf_guard.socket.getaddrinfo", side_effect=dns_ipv4_mapped):
            with pytest.raises(ValueError):
                assert_url_is_safe("http://example.com/image.png")
