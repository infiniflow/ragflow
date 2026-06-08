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

"""Unit tests for ``rag.nlp.is_english``.

Regression for the single-character regex bug: ``is_english`` is frequently
called with a *list of multi-character tokens* (e.g. ``is_english(txt.split())``
or ``is_english([answer])``). The classification pattern used ``fullmatch``
against a single-character class with no quantifier, so any multi-character
element never matched and English content was misclassified as non-English.
"""

from rag.nlp import is_english


def test_list_of_english_words_is_english():
    assert is_english(["hello", "world", "this", "is", "english"]) is True


def test_single_english_sentence_element_is_english():
    # The common ``is_english([answer])`` call shape.
    assert is_english(["This is a complete English sentence."]) is True


def test_list_of_chinese_words_is_not_english():
    assert is_english(["你好", "世界", "中文"]) is False


def test_english_string_is_english():
    assert is_english("hello world this is english") is True


def test_chinese_string_is_not_english():
    assert is_english("你好世界这是中文") is False


def test_empty_input_is_not_english():
    assert is_english("") is False
    assert is_english([]) is False
    assert is_english(None) is False
