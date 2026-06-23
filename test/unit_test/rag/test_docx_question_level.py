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

from rag.nlp import docx_question_level  # noqa: E402


def _para(style_name, text="some text"):
    """Build a minimal python-docx-like paragraph stub."""
    style = types.SimpleNamespace(name=style_name) if style_name is not None else None
    return types.SimpleNamespace(style=style, text=text)


@pytest.mark.p2
@pytest.mark.parametrize(
    "style_name, expected_level",
    [
        ("Heading 1", 1),
        ("Heading 2", 2),
        ("Heading 9", 9),
        ("Heading 10", 10),
    ],
)
def test_numbered_heading_levels(style_name, expected_level):
    level, txt = docx_question_level(_para(style_name, "Title"))
    assert level == expected_level
    assert txt == "Title"


@pytest.mark.p2
@pytest.mark.parametrize(
    "style_name",
    [
        "Heading",  # built-in base heading style, no number -> used to raise ValueError
        "HeadingTitle",  # custom heading style, no separable number
        "Heading Foo",  # non-numeric trailing token
    ],
)
def test_non_numeric_heading_does_not_raise(style_name):
    # Regression for #16163: these previously crashed with
    # "ValueError: invalid literal for int()".
    level, txt = docx_question_level(_para(style_name, "Heading text"))
    assert level == 1
    assert txt == "Heading text"


@pytest.mark.p2
def test_heading_without_space_separator():
    # "Heading1" (no space) also crashed under the old split(' ')[-1] logic.
    level, _ = docx_question_level(_para("Heading1"))
    assert level == 1


@pytest.mark.p2
def test_custom_heading_with_mid_name_digit_falls_back_to_one():
    # "Heading v2 draft" has a digit mid-name, not at the end; must not be
    # misread as level 2. Anchoring to $ ensures only trailing digits count.
    level, _ = docx_question_level(_para("Heading v2 draft"))
    assert level == 1


@pytest.mark.p2
def test_non_heading_paragraph_without_bullets_returns_zero():
    level, txt = docx_question_level(_para("Normal", "body paragraph"))
    assert level == 0
    assert txt == "body paragraph"


@pytest.mark.p2
def test_ideographic_space_is_normalized():
    # U+3000 (ideographic space) is normalized to a regular space and stripped.
    level, txt = docx_question_level(_para("Heading 1", "　Title　"))
    assert level == 1
    assert txt == "Title"
