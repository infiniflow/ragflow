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

import pytest

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
def test_is_english_multi_word_phrase():
    # Regression: splitting a string into characters made short spaced phrases fail.
    assert is_english("I am good") is True


@pytest.mark.p2
def test_is_english_chinese_list_is_false():
    assert is_english(["这是中文段落。", "另一个中文段落。"]) is False
