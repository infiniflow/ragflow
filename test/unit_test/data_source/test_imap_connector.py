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

import logging
import sys
import types

from pydantic import BaseModel

sys.modules.setdefault("anthropic", types.SimpleNamespace(BaseModel=BaseModel))
from common.data_source.imap_connector import _parse_singular_addr


def test_parse_singular_addr_returns_unknown_when_no_addresses_parse():
    assert _parse_singular_addr("") == ("Unknown", "unknown@example.com")


def test_parse_singular_addr_returns_first_address_and_warns_for_multiple(caplog):
    raw_header = "Alice <alice@example.com>, Bob <bob@example.com>"

    with caplog.at_level(logging.WARNING):
        parsed_addr = _parse_singular_addr(raw_header)

    assert parsed_addr == ("Alice", "alice@example.com")
    assert "Expected a singular address, but instead got multiple" in caplog.text
