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

"""Regression tests for IMAP connector address parsing.

Covers:
- #14963: ``_parse_addrs`` must respect RFC 5322 quoted display names that
  contain commas (e.g. ``"Schlüter, Sabine" <s@x>``) and return one address,
  not two.
- #14964: ``_parse_singular_addr`` must not raise on multi-address headers;
  it should warn and return the first address so a single odd email does not
  crash the entire sync.
"""

import logging

import pytest

from common.data_source.imap_connector import _parse_addrs, _parse_singular_addr

pytestmark = pytest.mark.p2


class TestParseAddrs:
    def test_empty_header_returns_empty_list(self):
        assert _parse_addrs("") == []

    def test_simple_address(self):
        assert _parse_addrs("user@example.com") == [("", "user@example.com")]

    def test_address_with_display_name(self):
        assert _parse_addrs("Alice <alice@example.com>") == [
            ("Alice", "alice@example.com")
        ]

    def test_quoted_display_name_with_comma_returns_single_address(self):
        # #14963: the bug was that ``split(",")`` produced two bogus tuples.
        # ``getaddresses`` must keep the quoted "Last, First" as one unit.
        result = _parse_addrs('"Schlüter, Sabine" <sabine.schlueter@ihklw.de>')
        assert result == [("Schlüter, Sabine", "sabine.schlueter@ihklw.de")]

    def test_multiple_addresses_comma_separated(self):
        result = _parse_addrs("a@example.com, b@example.com")
        assert result == [("", "a@example.com"), ("", "b@example.com")]

    def test_multiple_addresses_with_quoted_comma_in_name(self):
        result = _parse_addrs(
            '"Wilkens, Michael" <m@example.com>, "Müller, Hans" <h@example.com>'
        )
        assert result == [
            ("Wilkens, Michael", "m@example.com"),
            ("Müller, Hans", "h@example.com"),
        ]


class TestParseSingularAddr:
    def test_empty_header_returns_unknown(self):
        assert _parse_singular_addr("") == ("Unknown", "unknown@example.com")

    def test_single_address(self):
        assert _parse_singular_addr("Alice <alice@example.com>") == (
            "Alice",
            "alice@example.com",
        )

    def test_quoted_comma_display_name_does_not_raise(self):
        # #14963 cascade: before the fix, ``_parse_addrs`` returned two bogus
        # tuples and ``_parse_singular_addr`` then raised RuntimeError.
        assert _parse_singular_addr(
            '"Schlüter, Sabine" <sabine.schlueter@ihklw.de>'
        ) == ("Schlüter, Sabine", "sabine.schlueter@ihklw.de")

    def test_multi_address_header_warns_and_returns_first(self, caplog):
        # #14964: a legitimately multi-address From header must not crash sync.
        header = '"User A" <a@example.com>, "User B" <b@example.com>'
        with caplog.at_level(logging.WARNING):
            result = _parse_singular_addr(header)
        assert result == ("User A", "a@example.com")
        assert any(
            "Multiple addresses" in rec.message for rec in caplog.records
        ), f"expected warning about multiple addresses, got: {caplog.records}"

    def test_multi_address_header_does_not_raise(self):
        # Explicit guard: no RuntimeError should propagate.
        try:
            _parse_singular_addr("a@example.com, b@example.com")
        except RuntimeError as e:  # pragma: no cover - guard only
            pytest.fail(f"_parse_singular_addr unexpectedly raised: {e}")
