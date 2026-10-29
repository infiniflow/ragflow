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

"""Close HTTP responses when callers stop consuming session streams."""

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Thread

import pytest
from ragflow_sdk import RAGFlow
from ragflow_sdk.modules.session import Session

pytestmark = pytest.mark.p2


@pytest.fixture
def streaming_api():
    """Serve deterministic SSE bodies and retain the real Requests responses."""
    state = {"responses": []}

    class CapturingRAGFlow(RAGFlow):
        def post(self, path, json=None, stream=False, files=None):
            response = super().post(path, json=json, stream=stream, files=files)
            state["responses"].append(response)
            return response

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, *args):
            """Suppress access logs for the local fixture."""

        def do_POST(self):
            """Return protocol messages followed by scenario-specific SSE data."""
            payload = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            is_chat = self.path == "/api/v1/chats/chat-1/completions"
            message = {"data": {"answer": "hello", "id": "message-1"}} if is_chat else {"event": "message", "message_id": "message-1", "data": {"content": "hello"}}
            scenario = payload.get("question") or payload.get("query")

            if scenario == "malformed":
                message = {"data": {}} if is_chat else {"event": "message", "message_id": "message-1"}
            events = [message]
            if scenario == "terminated" and is_chat:
                events.append({"data": True})

            body = "".join(f"data:{json.dumps(event)}\n\n" for event in events)
            if scenario == "terminated" and not is_chat:
                body += "data:[DONE]\n\n"
            body = body.encode()
            if scenario in {"early-close", "terminated", "malformed"}:
                body += b":" + b"x" * 2048 + b"\n\n"

            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    server.daemon_threads = True
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    rag = CapturingRAGFlow("test-key", f"http://127.0.0.1:{server.server_port}")
    try:
        yield rag, state
    finally:
        for response in state["responses"]:
            response.close()
        server.shutdown()
        server.server_close()
        thread.join()


@pytest.mark.parametrize("session_type", ["chat", "agent"])
def test_closing_stream_generator_closes_response(streaming_api, session_type):
    """Close an unread response when the caller cancels stream consumption."""
    rag, state = streaming_api
    session = Session(rag, {"id": "session-1", f"{session_type}_id": f"{session_type}-1"})
    stream = session.ask("early-close", stream=True)

    assert next(stream).content == "hello"
    response = state["responses"][0]
    assert not response.raw.closed

    stream.close()

    assert response.raw.closed


@pytest.mark.parametrize("session_type", ["chat", "agent"])
def test_stream_terminator_closes_response_before_eof(streaming_api, session_type):
    """Close unread bytes after the Chat sentinel or Agent DONE marker."""
    rag, state = streaming_api
    session = Session(rag, {"id": "session-1", f"{session_type}_id": f"{session_type}-1"})

    messages = list(session.ask("terminated", stream=True))

    assert [message.content for message in messages] == ["hello"]
    assert state["responses"][0].raw.closed


@pytest.mark.parametrize("session_type", ["chat", "agent"])
def test_stream_parsing_error_closes_response(streaming_api, session_type):
    """Release the response if a protocol message cannot be structured."""
    rag, state = streaming_api
    session = Session(rag, {"id": "session-1", f"{session_type}_id": f"{session_type}-1"})

    with pytest.raises(KeyError):
        list(session.ask("malformed", stream=True))

    assert state["responses"][0].raw.closed


@pytest.mark.parametrize("session_type", ["chat", "agent"])
def test_stream_eof_preserves_message_and_closes_response(streaming_api, session_type):
    """Keep the message output unchanged when the response ends normally."""
    rag, state = streaming_api
    session = Session(rag, {"id": "session-1", f"{session_type}_id": f"{session_type}-1"})

    messages = list(session.ask("eof", stream=True))

    assert [message.content for message in messages] == ["hello"]
    assert state["responses"][0].raw.closed
