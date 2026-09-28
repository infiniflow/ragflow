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

from __future__ import annotations

import pytest

from rag.app import audio


@pytest.mark.p2
def test_missing_extension_reports_error():
    """A file without an extension is reported via the failure callback."""
    messages: list[tuple] = []

    def callback(prog=None, msg=""):
        messages.append((prog, msg))

    res = audio.chunk("no_extension_file", b"data", tenant_id="t", lang="english", callback=callback)

    assert res == []
    assert messages and messages[-1][0] == -1
    assert "No extension detected" in messages[-1][1]


@pytest.mark.p2
def test_unsupported_extension_reports_error():
    messages: list[tuple] = []

    def callback(prog=None, msg=""):
        messages.append((prog, msg))

    res = audio.chunk("song.xyz", b"data", tenant_id="t", lang="english", callback=callback)

    assert res == []
    assert messages and messages[-1][0] == -1
    assert "not supported yet" in messages[-1][1]
