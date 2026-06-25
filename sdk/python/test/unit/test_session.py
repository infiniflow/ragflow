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

import json

import pytest

from ragflow_sdk.modules.session import Session


class _FakeRag:
    pass


class _FakeResponse:
    def __init__(self, payload=None, *, lines=None):
        self._payload = payload
        self._lines = lines or []

    def json(self):
        if self._payload is None:
            raise ValueError("not json")
        return self._payload

    def iter_lines(self, decode_unicode=True):
        for line in self._lines:
            yield line


def _chat_session():
    session = Session(_FakeRag(), {"chat_id": "chat-1", "id": "session-1"})
    return session


@pytest.mark.p1
def test_ask_raises_api_error_on_nonzero_code():
    session = _chat_session()
    session.post = lambda *args, **kwargs: _FakeResponse(
        {"code": 101, "message": "bad request"}
    )

    with pytest.raises(Exception, match="bad request"):
        list(session.ask("hello", stream=False))


@pytest.mark.p1
def test_ask_raises_on_invalid_json_response():
    session = _chat_session()
    session.post = lambda *args, **kwargs: _FakeResponse(payload=None)

    with pytest.raises(Exception, match="Invalid response"):
        list(session.ask("hello", stream=False))


@pytest.mark.p1
def test_ask_returns_message_on_successful_chat_response():
    session = _chat_session()
    session.post = lambda *args, **kwargs: _FakeResponse(
        {"code": 0, "data": {"answer": "hello", "reference": {}}}
    )

    messages = list(session.ask("hello", stream=False))

    assert len(messages) == 1
    assert messages[0].content == "hello"


@pytest.mark.p1
def test_ask_stream_raises_api_error_on_nonzero_code():
    session = _chat_session()
    session.post = lambda *args, **kwargs: _FakeResponse(
        lines=[json.dumps({"code": 101, "message": "stream error"})]
    )

    with pytest.raises(Exception, match="stream error"):
        list(session.ask("hello", stream=True))
