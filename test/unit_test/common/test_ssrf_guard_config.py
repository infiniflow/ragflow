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
"""Tests for ssrf_guard.assert_config_urls_are_safe (#22869).

Connector configs mix URLs with bare host/port pairs, ports, paths and free
text. The scanner must validate every absolute URL (scheme AND netloc) against
the SSRF guard while leaving non-URL values alone.
"""

import socket
from unittest.mock import patch

import pytest

from common import ssrf_guard


def _resolve_to(*ips):
    addrs = list(ips)

    def fake_getaddrinfo(host, port, *args, **kwargs):
        ip = addrs.pop(0) if len(addrs) > 1 else addrs[0]
        return [(socket.AF_INET, socket.SOCK_STREAM, 6, "", (ip, 0))]

    return patch("common.ssrf_guard.socket.getaddrinfo", side_effect=fake_getaddrinfo)


def test_public_url_in_flat_config_allowed():
    with _resolve_to("93.184.216.34"):
        ssrf_guard.assert_config_urls_are_safe({"seafile_url": "https://example.com/"})


def test_private_url_in_flat_config_rejected():
    with _resolve_to("127.0.0.1"), pytest.raises(ValueError, match="non-public"):
        ssrf_guard.assert_config_urls_are_safe({"seafile_url": "http://localhost/"})


def test_private_url_nested_in_dict_and_list_rejected():
    config = {"outer": {"items": [{"jira_url": "https://metadata.internal/"}]}}
    with _resolve_to("169.254.169.254"), pytest.raises(ValueError, match="non-public"):
        ssrf_guard.assert_config_urls_are_safe(config)


def test_disallowed_scheme_rejected_when_it_looks_like_http():
    # An explicit allowlist narrower than the default rejects other URL schemes.
    with _resolve_to("93.184.216.34"), pytest.raises(ValueError, match="scheme"):
        ssrf_guard.assert_config_urls_are_safe({"feed": "http://example.com/"}, allowed_schemes=frozenset({"https"}))


def test_non_url_values_ignored():
    config = {
        "host": "localhost",
        "port": 8080,
        "path": "/var/data",
        "note": "plain text, not a url",
        "nested": {"list": ["10.0.0.1", "user@example.com", ""]},
    }
    with patch("common.ssrf_guard.socket.getaddrinfo", side_effect=AssertionError("resolver called on a non-URL value")):
        ssrf_guard.assert_config_urls_are_safe(config)


def test_mixed_config_stops_at_first_unsafe_url():
    config = {"ok": "https://example.com/", "evil": "http://192.168.0.1/"}
    with _resolve_to("93.184.216.34", "192.168.0.1"), pytest.raises(ValueError):
        ssrf_guard.assert_config_urls_are_safe(config)
