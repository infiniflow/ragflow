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

import importlib
import sys
import types

import pytest


def _stub(monkeypatch, name, **attrs):
    mod = types.ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    return mod


@pytest.fixture()
def docx_question_level(monkeypatch):
    _stub(monkeypatch, "common.token_utils", num_tokens_from_string=lambda *a, **k: 0)
    _stub(monkeypatch, "roman_numbers")
    _stub(monkeypatch, "word2number", w2n=types.SimpleNamespace())
    _stub(monkeypatch, "cn2an", cn2an=lambda *a, **k: 0)
    pil = _stub(monkeypatch, "PIL")
    pil.Image = _stub(monkeypatch, "PIL.Image")
    _stub(monkeypatch, "chardet")
    monkeypatch.delitem(sys.modules, "rag.nlp", raising=False)
    return importlib.import_module("rag.nlp").docx_question_level


class _Style:
    def __init__(self, name):
        self.name = name


class _Paragraph:
    def __init__(self, style_name, text="Some title"):
        self.style = _Style(style_name)
        self.text = text


@pytest.mark.p2
@pytest.mark.parametrize(
    "style_name, expected_level",
    [
        ("Heading 1", 1),
        ("Heading 2", 2),
        ("Heading 9", 9),
        ("Heading 10", 10),
        ("Heading1", 1),  # no space
        ("Heading", 1),  # base style, no number -> top level
        ("HeadingTitle", 1),  # custom prefix, no number -> top level
        ("Heading Title", 1),  # custom prefix with space, no number -> top level
    ],
)
def test_docx_question_level_heading_styles(docx_question_level, style_name, expected_level):
    level, text = docx_question_level(_Paragraph(style_name))
    assert level == expected_level
    assert text == "Some title"


@pytest.mark.p2
def test_docx_question_level_no_number_does_not_raise(docx_question_level):
    # Regression for #16163: a "Heading"-prefixed style without a parseable
    # number used to raise ValueError: invalid literal for int().
    for name in ("Heading", "HeadingTitle", "Heading Title"):
        level, _ = docx_question_level(_Paragraph(name))
        assert level == 1


@pytest.mark.p2
def test_docx_question_level_non_heading_default_bull(docx_question_level):
    # Non-heading paragraph with the default bull=-1 returns level 0 (body text).
    level, text = docx_question_level(_Paragraph("Normal", text="just a body line"))
    assert level == 0
    assert text == "just a body line"
