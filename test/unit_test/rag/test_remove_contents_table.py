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
"""Regression tests for remove_contents_table heading detection.

`re.IGNORECASE` was passed to the inner `re.sub` (whitespace strip) instead of
the outer `re.match`, so the table-of-contents heading match ran
case-sensitively. Real-world capitalized headings ("Contents", "Acknowledge",
"Table of Contents") were never detected, so the TOC block was left in parsed
book/law documents. The `"table of contents"` alternative was additionally dead
because the `re.sub` strips the spaces the pattern expected.
"""

import sys
import types

import pytest


def _stub(name, **attrs):
    mod = types.ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    sys.modules.setdefault(name, mod)
    return mod


# Stub heavy module-level imports so rag.nlp can be imported in isolation.
_stub("common.token_utils", num_tokens_from_string=lambda *a, **k: 0)
_stub("roman_numbers")
_stub("word2number", w2n=types.SimpleNamespace())
_stub("cn2an", cn2an=lambda *a, **k: 0)
_pil = _stub("PIL")
_pil.Image = _stub("PIL.Image")
_stub("chardet")

from rag.nlp import remove_contents_table


@pytest.mark.p2
@pytest.mark.parametrize(
    "heading",
    ["Contents", "CONTENTS", "Contents ", "Acknowledge", "ACKNOWLEDGE", "Table of Contents", "TABLE OF CONTENTS"],
)
def test_capitalized_toc_heading_is_removed(heading):
    sections = [heading]
    remove_contents_table(sections, eng=True)
    assert sections == [], f"{heading!r} should be detected as a TOC heading and removed"


@pytest.mark.p2
@pytest.mark.parametrize("heading", ["contents", "目录", "目次", "致谢"])
def test_existing_lowercase_and_chinese_headings_still_removed(heading):
    sections = [heading]
    remove_contents_table(sections, eng=False)
    assert sections == []


@pytest.mark.p2
@pytest.mark.parametrize("text", ["Introduction", "Chapter One", "The Contents of the Box", "Some heading"])
def test_non_toc_heading_is_not_removed(text):
    sections = [text]
    remove_contents_table(sections, eng=True)
    assert sections == [text], f"{text!r} is not a TOC heading and must be kept"
