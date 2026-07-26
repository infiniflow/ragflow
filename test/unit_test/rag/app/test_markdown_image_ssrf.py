#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License on an "AS IS" BASIS, WITHOUT WARRANTIES
#  OR CONDITIONS OF ANY KIND, either express or implied. See the License
#  for the specific language governing permissions and limitations under
#  the License.
#

"""SSRF-guard regression tests for rag.app.naive.Markdown.load_images_from_urls.

Image references are parsed out of the (untrusted) uploaded markdown document
and fetched server-side, so the loader must validate + DNS-pin every hop before
connecting. These tests assert that internal/loopback targets are rejected,
that redirects to internal targets are rejected, and that a legitimate public
image is still fetched.
"""

from __future__ import annotations

import io
import sys
from importlib import import_module, reload
from unittest.mock import MagicMock, patch

import pytest
from PIL import Image

from common import ssrf_guard


@pytest.fixture(scope="module")
def naive_module():
    """Load rag.app.naive with heavy optional dependencies stubbed locally."""
    stub_names = [
        "deepdoc.vision.ocr",
        "deepdoc.parser.figure_parser",
        "deepdoc.parser.docling_parser",
        "deepdoc.parser.tcadp_parser",
        "rag.app.picture",
    ]
    original_modules = {name: sys.modules.get(name) for name in stub_names}

    try:
        for name in stub_names:
            sys.modules[name] = MagicMock()
        module = import_module("rag.app.naive")
        module = reload(module)
        yield module
    finally:
        for name, original in original_modules.items():
            if original is None:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = original


@pytest.fixture(scope="module")
def max_image_redirects(naive_module):
    return naive_module.MAX_IMAGE_REDIRECTS


def _png_bytes() -> bytes:
    buf = io.BytesIO()
    Image.new("RGB", (1, 1), (255, 0, 0)).save(buf, format="PNG")
    return buf.getvalue()


class _Resp:
    """Minimal stand-in for requests.Response."""

    def __init__(self, status_code, headers=None, content=b""):
        self.status_code = status_code
        self.headers = headers or {}
        self.content = content

    def close(self):
        pass


@pytest.fixture
def parser(naive_module):
    return naive_module.Markdown(128)


@pytest.mark.p1
def test_blocks_internal_url_without_fetching(parser):
    """A markdown image pointing at an internal host must never be requested."""
    with (
        patch.object(ssrf_guard, "assert_url_is_safe", side_effect=ValueError("non-public")) as guard,
        patch("requests.get") as get,
    ):
        images, cache = parser.load_images_from_urls(["http://169.254.169.254/latest/meta-data/"])

    guard.assert_called_once()
    get.assert_not_called()  # SSRF guard rejects before any connection is made
    assert images == []
    assert cache["http://169.254.169.254/latest/meta-data/"] is None


@pytest.mark.p1
def test_blocks_redirect_to_internal_target(parser):
    """A public URL that 302-redirects to a loopback target must be rejected."""

    def selective_assert(url, **kwargs):
        if "127.0.0.1" in url or "localhost" in url:
            raise ValueError("redirect resolves to non-public address")
        return ("public.example", "8.8.8.8")

    redirect = _Resp(302, headers={"Location": "http://127.0.0.1/secret"})
    with (
        patch.object(ssrf_guard, "assert_url_is_safe", side_effect=selective_assert),
        patch("requests.get", return_value=redirect) as get,
    ):
        images, _ = parser.load_images_from_urls(["http://public.example/logo.png"])

    # Only the first (public) hop is fetched; the redirect target is blocked
    # by re-validation before a second request is made.
    assert get.call_count == 1
    assert images == []


@pytest.mark.p1
def test_fetches_legitimate_public_image(parser):
    png = _png_bytes()
    ok = _Resp(200, headers={"Content-Type": "image/png"}, content=png)
    with (
        patch.object(ssrf_guard, "assert_url_is_safe", return_value=("public.example", "8.8.8.8")),
        patch("requests.get", return_value=ok) as get,
    ):
        images, cache = parser.load_images_from_urls(["http://public.example/logo.png"])

    get.assert_called_once()
    # allow_redirects must be disabled so redirects are validated per hop.
    assert get.call_args.kwargs.get("allow_redirects") is False
    assert len(images) == 1
    assert isinstance(images[0], Image.Image)


@pytest.mark.p1
def test_redirect_chain_is_bounded(parser, max_image_redirects):
    """An endless redirect loop is abandoned instead of being followed forever."""
    loop = _Resp(302, headers={"Location": "http://public.example/next"})
    with (
        patch.object(ssrf_guard, "assert_url_is_safe", return_value=("public.example", "8.8.8.8")),
        patch("requests.get", return_value=loop) as get,
    ):
        images, _ = parser.load_images_from_urls(["http://public.example/start"])

    assert get.call_count == max_image_redirects + 1
    assert images == []
