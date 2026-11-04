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

"""
The Cheaper Inference gateway reads ``reasoning_effort`` and ignores
``thinking`` / ``enable_thinking``, so ``CheaperInferenceChat._clean_conf``
translates the RAGFlow thinking selection into ``reasoning_effort``.
``Base._clean_conf`` drops all three keys, which would silently discard an
explicit selection.
"""

import pytest

from rag.llm.chat_model import CheaperInferenceChat

pytestmark = pytest.mark.p2

BASE_URL = "https://api.cheaperinference.com/v1"


def _chat():
    return CheaperInferenceChat("ci-test", "claude-opus-5", base_url=BASE_URL)


def test_thinking_disabled_turns_reasoning_off():
    cleaned = _chat()._clean_conf({"temperature": 0.2, "thinking": "disabled"})

    assert cleaned["reasoning_effort"] == "none"
    assert cleaned["temperature"] == 0.2
    assert "thinking" not in cleaned


def test_thinking_disabled_in_dict_form_turns_reasoning_off():
    cleaned = _chat()._clean_conf({"thinking": {"type": "disabled"}})

    assert cleaned["reasoning_effort"] == "none"


def test_enable_thinking_false_turns_reasoning_off():
    cleaned = _chat()._clean_conf({"enable_thinking": False})

    assert cleaned["reasoning_effort"] == "none"
    assert "enable_thinking" not in cleaned


def test_thinking_enabled_keeps_the_model_default():
    # The gateway has no "force on" field; a reasoning model reasons by
    # default, so nothing is added.
    cleaned = _chat()._clean_conf({"thinking": "enabled"})

    assert "reasoning_effort" not in cleaned
    assert "thinking" not in cleaned


def test_explicit_reasoning_effort_survives():
    cleaned = _chat()._clean_conf({"reasoning_effort": "low", "thinking": "enabled"})

    assert cleaned["reasoning_effort"] == "low"


def test_thinking_disabled_overrides_an_explicit_effort():
    cleaned = _chat()._clean_conf({"reasoning_effort": "high", "thinking": "disabled"})

    assert cleaned["reasoning_effort"] == "none"


def test_no_selection_adds_nothing():
    cleaned = _chat()._clean_conf({"temperature": 0.7, "max_tokens": 100})

    assert "reasoning_effort" not in cleaned
    assert "max_tokens" not in cleaned
