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

from rag.nlp import is_english


@pytest.mark.p2
def test_is_english_string_path_unchanged():
    assert is_english("This is English") is True


@pytest.mark.p2
def test_is_english_list_of_english_sentences():
    assert is_english(["The quick brown fox jumps.", "Hello world today.", "Good morning sir."]) is True


@pytest.mark.p2
def test_is_english_single_english_answer_in_list():
    assert is_english(["This is a normal English answer."]) is True


@pytest.mark.p2
def test_is_english_chinese_list_is_false():
    assert is_english(["这是中文段落。", "另一个中文段落。"]) is False
