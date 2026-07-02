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
"""Unit tests for the LLM request-context user-forwarding precedence rules.

Covers the behaviour flagged in review: (1) session_id is preferred over
user_id, (2) an explicit caller-supplied ``user`` overrides the context value,
and (3) a caller-supplied empty ``user`` suppresses forwarding entirely.
"""
import types

import pytest

from common.llm_request_context import (
    current_llm_user,
    llm_request_context,
    reset_llm_request_context,
    set_llm_request_context,
)


@pytest.fixture(autouse=True)
def _isolate_context():
    """Ensure every test starts and ends with no active request context."""
    token = llm_request_context.set(None)
    yield
    try:
        llm_request_context.reset(token)
    except ValueError:
        llm_request_context.set(None)


@pytest.mark.p2
class TestCurrentLlmUser:
    def test_no_active_context_returns_none(self):
        assert current_llm_user() is None

    def test_session_id_preferred_over_user_id(self):
        token = set_llm_request_context(session_id="sess-1", user_id="user-1")
        try:
            assert current_llm_user() == "sess-1"
        finally:
            reset_llm_request_context(token)

    def test_falls_back_to_user_id_without_session(self):
        token = set_llm_request_context(session_id=None, user_id="user-1")
        try:
            assert current_llm_user() == "user-1"
        finally:
            reset_llm_request_context(token)

    def test_empty_identifiers_return_none(self):
        token = set_llm_request_context(session_id=None, user_id=None)
        try:
            assert current_llm_user() is None
        finally:
            reset_llm_request_context(token)

    def test_identifier_is_truncated_to_128_chars(self):
        token = set_llm_request_context(session_id="s" * 200)
        try:
            assert current_llm_user() == "s" * 128
        finally:
            reset_llm_request_context(token)


@pytest.mark.p2
class TestSetResetLifecycle:
    def test_reset_restores_previous_context(self):
        outer = set_llm_request_context(session_id="outer")
        try:
            inner = set_llm_request_context(session_id="inner")
            assert current_llm_user() == "inner"
            reset_llm_request_context(inner)
            assert current_llm_user() == "outer"
        finally:
            reset_llm_request_context(outer)

    def test_reset_with_stale_token_does_not_raise(self):
        token = set_llm_request_context(session_id="x")
        reset_llm_request_context(token)
        # Resetting again with the now-stale token must fall back, not raise
        # (mirrors an async generator closed from a different context).
        reset_llm_request_context(token)
        assert current_llm_user() is None


@pytest.mark.p2
class TestCompletionArgsUserPrecedence:
    """Exercise the provider chokepoint: LiteLLMBase._construct_completion_args."""

    def _construct(self, **kwargs):
        chat_model = pytest.importorskip("rag.llm.chat_model")
        # provider="" keeps every provider-specific branch inert, so only the
        # generic completion_args + user-forwarding path is exercised.
        fake = types.SimpleNamespace(model_name="m", api_key="k", max_retries=0, provider="")
        return chat_model.LiteLLMBase._construct_completion_args(fake, [], False, False, **kwargs)

    def test_context_user_applied_when_caller_omits_it(self):
        token = set_llm_request_context(session_id="sess-9", user_id="user-9")
        try:
            args = self._construct()
            assert args["user"] == "sess-9"
        finally:
            reset_llm_request_context(token)

    def test_caller_user_overrides_context(self):
        token = set_llm_request_context(session_id="sess-9")
        try:
            args = self._construct(user="explicit-caller")
            assert args["user"] == "explicit-caller"
        finally:
            reset_llm_request_context(token)

    def test_caller_empty_user_suppresses_forwarding(self):
        token = set_llm_request_context(session_id="sess-9")
        try:
            args = self._construct(user="")
            # Key presence (not truthiness) is honoured: the empty string wins.
            assert args["user"] == ""
        finally:
            reset_llm_request_context(token)

    def test_no_context_leaves_user_unset(self):
        args = self._construct()
        assert "user" not in args
