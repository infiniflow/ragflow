#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#

"""Regression coverage for #20194: ``Session.ask(stream=True)`` must decode
SSE bytes as UTF-8 even when the response Content-Type omits a charset.
"""

from __future__ import annotations

import json
from types import SimpleNamespace

import pytest


_UNICODE_ANSWER = "你好, café! 🚀"


def _build_response(body: bytes, content_type: str):
    """Build a minimal ``requests.Response``-shaped object for stubbing."""

    response = SimpleNamespace()
    response.status_code = 200
    response.headers = {"Content-Type": content_type}
    response.content = body
    response.encoding = None

    def _iter_lines(chunk_size=1, decode_unicode=False, delimiter=None):
        # Mirror requests.Response.iter_lines: when decode_unicode is True the
        # bytes are decoded using ``response.encoding`` (falling back to
        # ISO-8859-1). This is exactly the failure path the fix addresses.
        start = 0
        while start < len(body):
            end = body.find(b"\n", start)
            if end == -1:
                end = len(body)
            line = body[start:end]
            if line.endswith(b"\r"):
                line = line[:-1]
            if decode_unicode:
                yield line.decode(response.encoding or "iso-8859-1")
            else:
                yield bytes(line)
            start = end + 1

    response.iter_lines = _iter_lines
    response.json = lambda: json.loads(body.decode("utf-8"))
    return response


class _PostStub:
    """Programmable ``requests.post`` replacement bound to a single response.

    Each test sets ``stub.respond_with(body, content_type)`` before calling
    ``Session.ask``; the stub records the outbound request and returns a
    ``requests.Response``-shaped object containing the configured body.
    """

    def __init__(self):
        self.captured: dict = {}
        self._next: tuple[bytes, str] | None = None

    def respond_with(self, body: bytes, content_type: str):
        self._next = (body, content_type)

    def __call__(self, url, json=None, headers=None, stream=False, files=None):
        self.captured["url"] = url
        self.captured["json"] = json
        self.captured["stream"] = stream
        body, content_type = self._next
        return _build_response(body, content_type)


@pytest.fixture
def stub_post(monkeypatch):
    stub = _PostStub()
    monkeypatch.setattr("requests.post", stub)
    return stub


def _chat_sse_body(answer: str) -> bytes:
    payload = {"code": 0, "data": {"answer": answer, "id": "message-1"}}
    return ("data:" + json.dumps(payload, ensure_ascii=False) + "\n\n").encode("utf-8")


def _agent_sse_body(answer: str) -> bytes:
    chunk_event = {
        "event": "message",
        "message_id": "message-1",
        "data": {"content": answer, "reference": {}},
    }
    end_event = {
        "event": "message_end",
        "message_id": "message-1",
        "data": {"content": "", "reference": {}},
    }
    return ("data:" + json.dumps(chunk_event, ensure_ascii=False) + "\n\n" + "data:" + json.dumps(end_event, ensure_ascii=False) + "\n\n").encode("utf-8")


def _build_chat_session(stub):
    from ragflow_sdk.modules.session import Session

    rag = SimpleNamespace(post=stub)
    return Session(rag, {"chat_id": "chat-1", "id": "session-1"})


def _build_agent_session(stub):
    from ragflow_sdk.modules.session import Session

    rag = SimpleNamespace(post=stub)
    return Session(rag, {"agent_id": "agent-1", "id": "session-1"})


def test_chat_stream_decodes_utf8_when_content_type_omits_charset(stub_post):
    """Primary regression: no-charset header + UTF-8 SSE → correctly decoded text."""
    stub_post.respond_with(_chat_sse_body(_UNICODE_ANSWER), "text/event-stream")
    session = _build_chat_session(stub_post)

    messages = list(session.ask("hello", stream=True))

    assert len(messages) == 1
    assert messages[0].content == _UNICODE_ANSWER
    assert stub_post.captured["json"]["session_id"] == "session-1"
    assert stub_post.captured["stream"] is True


def test_chat_stream_decodes_utf8_when_charset_is_present(stub_post):
    """Regression: explicit charset header still works."""
    stub_post.respond_with(_chat_sse_body(_UNICODE_ANSWER), "text/event-stream; charset=utf-8")
    session = _build_chat_session(stub_post)

    messages = list(session.ask("hello", stream=True))

    assert len(messages) == 1
    assert messages[0].content == _UNICODE_ANSWER


def test_chat_stream_ascii_payload_unchanged(stub_post):
    """Regression: ASCII payload is unaffected by the encoding pin."""
    stub_post.respond_with(_chat_sse_body("plain ascii answer"), "text/event-stream")
    session = _build_chat_session(stub_post)

    messages = list(session.ask("hello", stream=True))

    assert len(messages) == 1
    assert messages[0].content == "plain ascii answer"


def test_agent_stream_decodes_utf8_when_content_type_omits_charset(stub_post):
    """Same defect also affects Agent completion envelopes."""
    stub_post.respond_with(_agent_sse_body(_UNICODE_ANSWER), "text/event-stream")
    session = _build_agent_session(stub_post)

    messages = list(session.ask("hello", stream=True))

    contents = [m.content for m in messages if m.content]
    assert contents == [_UNICODE_ANSWER]
    assert stub_post.captured["json"]["agent_id"] == "agent-1"


def test_non_stream_path_unaffected_by_encoding_pin(stub_post):
    """Non-stream completion path uses res.json() and must keep working."""
    payload = {"code": 0, "data": {"answer": _UNICODE_ANSWER, "id": "m-1"}}
    stub_post.respond_with(json.dumps(payload).encode("utf-8"), "application/json; charset=utf-8")
    session = _build_chat_session(stub_post)

    messages = list(session.ask("hello", stream=False))

    assert len(messages) == 1
    assert messages[0].content == _UNICODE_ANSWER
    assert stub_post.captured["stream"] is False
