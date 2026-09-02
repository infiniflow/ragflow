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

import importlib

import pytest

from rag.app import resume


@pytest.fixture
def unavailable_encoding(monkeypatch):
    """Pin the module into the state a failed encoding build leaves behind."""
    monkeypatch.setattr(resume, "_tiktoken_encoding", None)
    monkeypatch.setattr(resume, "_tiktoken_encoding_ready", True)


@pytest.fixture
def unbuilt_encoding(monkeypatch):
    monkeypatch.setattr(resume, "_tiktoken_encoding", None)
    monkeypatch.setattr(resume, "_tiktoken_encoding_ready", False)


@pytest.mark.p2
def test_import_does_not_build_the_encoding(monkeypatch):
    """Importing the module must not fetch the BPE table.

    Reloading with the build wired to fail is the only way to tell a lazy
    build from an eager one that merely swallows the error.
    """
    calls = []

    def boom(*args, **kwargs):
        calls.append(args)
        raise OSError("unreachable BPE table host")

    monkeypatch.setattr(resume.tiktoken, "encoding_for_model", boom)
    reloaded = importlib.reload(resume)

    assert calls == []
    assert reloaded._tiktoken_encoding_ready is False
    assert reloaded._get_tiktoken_encoding() is None
    assert reloaded._get_tiktoken_encoding() is None
    assert len(calls) == 1


@pytest.mark.p2
def test_failed_build_is_attempted_once(unbuilt_encoding, monkeypatch):
    calls = []

    def boom(*args, **kwargs):
        calls.append(args)
        raise OSError("unreachable BPE table host")

    monkeypatch.setattr(resume.tiktoken, "encoding_for_model", boom)

    assert resume._get_tiktoken_encoding() is None
    assert resume._get_tiktoken_encoding() is None
    assert len(calls) == 1


@pytest.mark.p2
def test_distinct_texts_are_not_duplicates_without_an_encoding(unavailable_encoding):
    """An unavailable encoding must not make every pair look identical.

    _text_shingles yields the empty set for every text, and an empty union
    reports 1.0, which the work-experience and project dedup passes read as a
    duplicate. That would collapse a resume to a single employer.
    """
    first = "Led the payments platform team at Northwind Traders from 2019 to 2022."
    second = "Maintained the internal HR portal at Contoso Ltd during 2015 and 2016."

    assert resume._text_shingles(first) == set()
    assert resume._shingling_jaccard(first, second) == 0.0
    assert resume._shingling_jaccard(first, second) <= 0.5


@pytest.mark.p2
def test_should_remove_random_str_falls_back_to_heuristic(unavailable_encoding):
    import re

    match = re.compile(r"[a-zA-Z0-9\-~_]{40,}").search("a1B2c3D4e5F6g7H8i9J0k1L2m3N4o5P6q7R8s9T0uV")
    assert match is not None
    assert resume._should_remove_random_str(match) is True
