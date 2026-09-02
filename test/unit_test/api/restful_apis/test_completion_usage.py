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
"""Unit tests for the chat-completion token-usage block (issue #8035)."""

from unittest.mock import patch

from api.apps.restful_apis.chat_api import completion_usage

_words = lambda s: len((s or "").split())  # noqa: E731 — deterministic stand-in for the tokenizer


def test_usage_sums_prompt_and_completion():
    with patch("api.apps.restful_apis.chat_api.num_tokens_from_string", side_effect=_words):
        u = completion_usage("how are you", "i am fine")
    assert u == {"prompt_tokens": 3, "completion_tokens": 3, "total_tokens": 6}


def test_usage_handles_empty_and_none():
    with patch("api.apps.restful_apis.chat_api.num_tokens_from_string", side_effect=_words):
        assert completion_usage("", "") == {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}
        assert completion_usage(None, None) == {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0}


def test_usage_total_is_sum():
    with patch("api.apps.restful_apis.chat_api.num_tokens_from_string", side_effect=_words):
        u = completion_usage("a b c d", "e")
    assert u["total_tokens"] == u["prompt_tokens"] + u["completion_tokens"] == 5
