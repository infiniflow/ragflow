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
"""Regression tests for Session.ask SSE decoding (#20194).

The HTTP/HTML SSE spec mandates UTF-8, but `requests` defaults to ISO-8859-1
when the server's Content-Type header omits a charset. Without forcing UTF-8
on the response, ``iter_lines(decode_unicode=True)`` corrupts non-ASCII
payloads even though the JSON parse itself succeeds.
"""

import io
import json

import pytest
import requests
from ragflow_sdk.modules.session import Session

UTF8_ANSWER = "你好, café! \U0001f680"
CHAT_EVENT = {"code": 0, "data": {"answer": UTF8_ANSWER, "id": "message-1"}}
AGENT_MESSAGE_EVENT = {"event": "message", "message_id": "message-1", "data": {"content": UTF8_ANSWER}}
AGENT_MESSAGE_END_EVENT = {"event": "message_end", "message_id": "message-1", "data": {"content": ""}}


def _sse_response(events, content_type="text/event-stream"):
    """Build a ``requests.Response`` whose ``iter_lines(decode_unicode=True)``
    yields SSE-formatted lines from UTF-8 bytes.

    Mirrors what the real backend sends: Content-Type without a charset, raw
    bytes containing UTF-8 JSON.
    """
    body = "".join("data:" + json.dumps(ev, ensure_ascii=False) + "\n\n" for ev in events).encode("utf-8")
    resp = requests.Response()
    resp.status_code = 200
    resp.headers["Content-Type"] = content_type
    resp.raw = io.BytesIO(body)
    return resp


def _make_session(session_type):
    """Construct a Session whose ``_ask_*`` returns a stubbed response."""
    s = Session.__new__(Session)
    s.rag = None
    s.id = "session-1"
    if session_type == "chat":
        s.chat_id = "chat-1"
    else:
        s.agent_id = "agent-1"
    s._Session__session_type = session_type
    return s


@pytest.mark.p1
def test_chat_stream_decodes_utf8_without_charset_header():
    """SSE without ``charset=utf-8`` must still yield correct UTF-8 answer text (#20194)."""
    s = _make_session("chat")
    s._ask_chat = lambda *a, **kw: _sse_response([CHAT_EVENT])

    messages = list(s.ask("q", stream=True))

    assert len(messages) == 1
    assert messages[0].content == UTF8_ANSWER


@pytest.mark.p1
def test_agent_stream_decodes_utf8_without_charset_header():
    """The agent ``message`` envelope must decode UTF-8 when the charset is omitted."""
    s = _make_session("agent")
    s._ask_agent = lambda *a, **kw: _sse_response(
        [
            AGENT_MESSAGE_EVENT,
            AGENT_MESSAGE_END_EVENT,
        ]
    )

    messages = list(s.ask("q", stream=True))

    assert any(m.content == UTF8_ANSWER for m in messages)


@pytest.mark.p1
def test_chat_stream_ascii_still_works_without_charset_header():
    """ASCII payloads continue to work when the charset is omitted."""
    ascii_answer = "hello world"
    s = _make_session("chat")
    s._ask_chat = lambda *a, **kw: _sse_response([{"code": 0, "data": {"answer": ascii_answer, "id": "x"}}])

    messages = list(s.ask("q", stream=True))

    assert messages[0].content == ascii_answer


@pytest.mark.p1
def test_chat_stream_still_respects_explicit_utf8_charset_header():
    """When the server already declares ``charset=utf-8``, behavior is unchanged."""
    s = _make_session("chat")
    s._ask_chat = lambda *a, **kw: _sse_response([CHAT_EVENT], content_type="text/event-stream; charset=utf-8")

    messages = list(s.ask("q", stream=True))

    assert messages[0].content == UTF8_ANSWER


@pytest.mark.p1
def test_chat_stream_forces_utf8_when_charset_claims_utf16():
    """A charset that merely contains "utf" (e.g. ``utf-16``) must not defeat the override.

    The SSE stream is UTF-8 by spec, so trusting a ``utf-16`` declaration would decode
    the UTF-8 wire bytes as UTF-16 and corrupt every answer.
    """
    s = _make_session("chat")

    def stub(*args, **kwargs):
        resp = _sse_response([CHAT_EVENT], content_type="text/event-stream; charset=utf-16")
        resp.encoding = "utf-16"  # what requests' Session.send would set from that header
        return resp

    s._ask_chat = stub

    messages = list(s.ask("q", stream=True))

    assert messages[0].content == UTF8_ANSWER


@pytest.mark.p1
def test_non_stream_ask_leaves_response_encoding_untouched():
    """The SSE override is stream-only: non-streamed answers must keep requests' own detection.

    ``res.json()`` handles UTF-16/UTF-32 bodies through BOM sniffing when no charset is
    declared; forcing UTF-8 on that path would break those parses.
    """
    body = json.dumps({"code": 0, "data": {"answer": "plain answer", "id": "x"}}).encode("utf-8")
    resp = requests.Response()
    resp.status_code = 200
    resp.headers["Content-Type"] = "application/json"
    resp.raw = io.BytesIO(body)

    s = _make_session("chat")
    s._ask_chat = lambda *a, **kw: resp

    messages = list(s.ask("q", stream=False))

    assert messages[0].content == "plain answer"
    assert resp.encoding is None
