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

"""Preserve server message identifiers across both SDK completion protocols."""

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Thread

import pytest
from ragflow_sdk import RAGFlow
from ragflow_sdk.modules.session import Session

pytestmark = pytest.mark.p2


@pytest.fixture
def completion_api():
    """Serve the Chat and Agent response envelopes through real HTTP and SSE."""
    state = {"message_id": None, "requests": []}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            """Suppress access logs for the local fixture."""

        def do_POST(self):
            """Return protocol-specific IDs distinct from session and task IDs."""
            payload = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            state["requests"].append((self.path, payload))
            reference = {"chunks": [{"id": "chunk-1"}]}
            if self.path == "/api/v1/chats/chat-1/completions":
                answer = {"answer": "answer text", "reference": reference, "session_id": "session-1"}
                if state["message_id"] is not None:
                    answer["id"] = state["message_id"]
                envelope = {"code": 0, "data": answer}
                events = [envelope, {"code": 0, "data": True}]
            elif self.path == "/api/v1/agents/chat/completions":
                event = {"event": "message", "session_id": "session-1", "task_id": "task-1", "data": {"content": "answer text"}}
                if state["message_id"] is not None:
                    event["message_id"] = state["message_id"]
                end = {**event, "event": "message_end", "data": {"content": "answer text", "reference": reference}}
                envelope = {"code": 0, "data": end}
                events = [event, end]
            else:
                self.send_error(404)
                return
            if payload["stream"]:
                body = ("".join("data:" + json.dumps(event) + "\n\n" for event in events) + "data:[DONE]\n\n").encode()
                content_type = "text/event-stream; charset=utf-8"
            else:
                body = json.dumps(envelope).encode()
                content_type = "application/json"
            self.send_response(200)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        yield RAGFlow("test-key", f"http://127.0.0.1:{server.server_port}"), state
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


@pytest.mark.parametrize("session_type", ["chat", "agent"])
@pytest.mark.parametrize("stream", [False, True])
@pytest.mark.parametrize("message_id", ["server-message-1", "", None])
def test_ask_preserves_message_id(completion_api, session_type, stream, message_id):
    """Keep the response ID, including reference-only Agent events and defaults."""
    rag, state = completion_api
    state["message_id"] = message_id
    session = Session(rag, {"id": "session-1", f"{session_type}_id": f"{session_type}-1"})
    messages = list(session.ask("question", stream=stream))

    assert [message.id for message in messages] == [message_id] * len(messages)
    assert [message.to_json()["id"] for message in messages] == [message_id] * len(messages)
    assert [message.content for message in messages] == (["answer text", ""] if stream and session_type == "agent" else ["answer text"])
    assert messages[-1].reference == [{"id": "chunk-1"}]
    assert all(message.role == "assistant" for message in messages)
    expected_path = "/api/v1/chats/chat-1/completions" if session_type == "chat" else "/api/v1/agents/chat/completions"
    assert len(state["requests"]) == 1
    path, payload = state["requests"][0]
    assert path == expected_path
    assert payload["session_id"] == "session-1"
    assert payload["stream"] is stream
