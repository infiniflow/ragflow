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
"""SSRF-hardening regression tests for ``rag.utils.file_utils.extract_html``.

The URL passed to ``extract_html`` is attacker-controlled — it comes from
hyperlinks embedded in uploaded DOCX/PDF documents and is fetched server-side
when ``analyze_hyperlink`` is enabled. These tests pin the guard behaviour:
unsafe targets (loopback, link-local/metadata, disallowed schemes) are rejected
before any request, and redirects are re-validated per hop.
"""
import contextlib
from unittest.mock import patch

import pytest
import requests

import common.ssrf_guard as ssrf_guard
from rag.utils import file_utils


class _FakeResp:
    def __init__(self, status_code, location=None, content=b"<html>ok</html>", url="http://public.test/"):
        self.status_code = status_code
        self.headers = {}
        if location is not None:
            self.headers["Location"] = location
        self.headers["Content-Type"] = "text/html"
        self.content = content
        self.url = url

    def raise_for_status(self):
        if self.status_code >= 400:
            raise requests.exceptions.HTTPError(f"status {self.status_code}")


class _FakeSession:
    def __init__(self, responses):
        self._responses = list(responses)
        self.calls = []

    def get(self, url, timeout=None, allow_redirects=True):
        self.calls.append((url, allow_redirects))
        return self._responses.pop(0)


@contextlib.contextmanager
def _noop_pin(_hostname, _ip):
    yield


def _patch_session(session):
    return patch.object(file_utils, "_get_session", return_value=session)


class TestExtractHtmlSSRF:
    def test_loopback_url_blocked(self):
        body, meta = file_utils.extract_html("http://127.0.0.1/admin")
        assert body is None
        assert "unsafe" in meta["error"].lower()

    def test_metadata_ip_blocked(self):
        body, meta = file_utils.extract_html("http://169.254.169.254/latest/meta-data/")
        assert body is None
        assert "unsafe" in meta["error"].lower()

    def test_disallowed_scheme_blocked(self):
        body, meta = file_utils.extract_html("file:///etc/passwd")
        assert body is None
        assert "unsafe" in meta["error"].lower()

    def test_safe_url_is_fetched(self):
        session = _FakeSession([_FakeResp(200, content=b"<html>hi</html>", url="http://public.test/")])
        with (
            patch.object(ssrf_guard, "assert_url_is_safe", return_value=("public.test", "8.8.8.8")),
            patch.object(ssrf_guard, "pin_dns", _noop_pin),
            _patch_session(session),
        ):
            body, meta = file_utils.extract_html("http://public.test/page")
        assert body == b"<html>hi</html>"
        assert meta["status_code"] == "200"
        # The request must be made without auto-following redirects.
        assert session.calls == [("http://public.test/page", False)]

    def test_redirect_to_loopback_blocked(self):
        from urllib.parse import urlparse

        real_assert = ssrf_guard.assert_url_is_safe

        def selective_assert(url, **kwargs):
            host = urlparse(url).hostname or ""
            if host in ("127.0.0.1", "localhost", "169.254.169.254"):
                return real_assert(url, **kwargs)  # real guard rejects these
            return ("public.test", "8.8.8.8")

        session = _FakeSession([_FakeResp(302, location="http://127.0.0.1/next")])
        with (
            patch.object(ssrf_guard, "assert_url_is_safe", side_effect=selective_assert),
            patch.object(ssrf_guard, "pin_dns", _noop_pin),
            _patch_session(session),
        ):
            body, meta = file_utils.extract_html("http://public.test/start")
        assert body is None
        assert "redirect" in meta["error"].lower()

    def test_too_many_redirects_blocked(self):
        # A session that always 302s back to an (allowed) host.
        loop = [_FakeResp(302, location=f"http://public.test/{i}") for i in range(10)]
        session = _FakeSession(loop)
        with (
            patch.object(ssrf_guard, "assert_url_is_safe", return_value=("public.test", "8.8.8.8")),
            patch.object(ssrf_guard, "pin_dns", _noop_pin),
            patch.object(file_utils, "_MAX_HTML_REDIRECTS", 2),
            _patch_session(session),
        ):
            body, meta = file_utils.extract_html("http://public.test/start")
        assert body is None
        assert "redirect" in meta["error"].lower()


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
