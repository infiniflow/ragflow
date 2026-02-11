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

from ragflow_sdk.modules.session import Session


class _DummyRag:
    pass


class _StubResponse:
    def __init__(self, payload=None, lines=None):
        self._payload = payload
        self._lines = lines or []

    def json(self):
        return self._payload

    def iter_lines(self, decode_unicode=True):
        for line in self._lines:
            yield line


@pytest.mark.p2
class TestSessionParsing:
    def test_session_ask_non_stream_chat_structures_answer(self, monkeypatch):
        session = Session(_DummyRag(), {"id": "s1", "chat_id": "c1"})
        captured = {}

        def _post(path, json=None, stream=False, **_kwargs):
            captured["path"] = path
            captured["stream"] = stream
            return _StubResponse(
                payload={"data": {"answer": "hi", "reference": {"chunks": ["c1"]}}}
            )

        monkeypatch.setattr(session, "post", _post)

        messages = list(session.ask(question="q", stream=False))
        assert captured["path"].endswith("/chats/c1/completions")
        assert captured["stream"] is False
        assert len(messages) == 1
        assert messages[0].content == "hi"
        assert messages[0].reference == ["c1"]

    def test_session_stream_parsing_chat_terminates_cleanly(self, monkeypatch):
        session = Session(_DummyRag(), {"id": "s1", "chat_id": "c1"})

        lines = [
            'data: {"data": {"answer": "hi", "reference": {"chunks": ["c1"]}}}',
            'data: {"data": true}',
        ]

        def _post(path, json=None, stream=False, **_kwargs):
            return _StubResponse(lines=lines)

        monkeypatch.setattr(session, "post", _post)

        messages = list(session.ask(question="q", stream=True))
        assert messages
        assert messages[0].content == "hi"
