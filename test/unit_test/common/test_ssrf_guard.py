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

"""Tests for the shared SSRF guard in common.ssrf_guard."""

from __future__ import annotations

import socket
from unittest.mock import patch

import pytest

from common import ssrf_guard


def _addr_infos(*ips: str):
    infos = []
    for ip in ips:
        family = socket.AF_INET6 if ":" in ip else socket.AF_INET
        infos.append((family, socket.SOCK_STREAM, 6, "", (ip, 0)))
    return infos


def _resolve_to(*ips: str):
    return patch("common.ssrf_guard.socket.getaddrinfo", return_value=_addr_infos(*ips))


def test_allows_public_ipv4():
    with _resolve_to("93.184.216.34"):
        host, ip = ssrf_guard.assert_url_is_safe("https://example.com/")
    assert host == "example.com"
    assert ip == "93.184.216.34"


@pytest.mark.parametrize(
    "addr",
    [
        "127.0.0.1",
        "10.0.0.5",
        "169.254.169.254",
        "::ffff:127.0.0.1",
        # IPv6 transition addresses embedding a private/link-local IPv4 target.
        "64:ff9b::7f00:1",  # NAT64 well-known prefix -> 127.0.0.1
        "64:ff9b::a9fe:a9fe",  # NAT64 well-known prefix -> 169.254.169.254 (cloud metadata)
        "::7f00:1",  # deprecated IPv4-compatible -> 127.0.0.1
    ],
)
def test_blocks_non_public_targets(addr):
    with _resolve_to(addr), pytest.raises(ValueError, match="non-public address"):
        ssrf_guard.assert_url_is_safe("http://attacker.example/")


def test_nat64_wrapping_public_ipv4_is_allowed():
    # 64:ff9b::808:808 is the NAT64 representation of 8.8.8.8 and stays reachable.
    with _resolve_to("64:ff9b::808:808"):
        host, _ = ssrf_guard.assert_url_is_safe("http://nat64.example/")
    assert host == "nat64.example"


def test_assert_host_is_safe_blocks_nat64_private():
    with _resolve_to("64:ff9b::a9fe:a9fe"), pytest.raises(ValueError, match="non-public address"):
        ssrf_guard.assert_host_is_safe("attacker.example")
