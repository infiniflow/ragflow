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

"""Split streamed SSE frames only on newline characters, not on Unicode line separators."""

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Thread

import pytest
from ragflow_sdk import RAGFlow
from ragflow_sdk.modules.session import Session

pytestmark = pytest.mark.p2


@pytest.fixture
def completion_api():
    """Stream Chat and Agent events whose text carries raw U+2028/U+2029/U+0085, as ensure_ascii=False emits them."""
    state = {"text": None}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            """Suppress access logs for the local fixture."""

        def do_POST(self):
            """Return one SSE stream per protocol with the separator inside the JSON payloads."""
            self.rfile.read(int(self.headers["Content-Length"]))
            text = state["text"]
            reference = {"chunks": [{"id": "chunk-1", "content": text}]}
            if self.path == "/api/v1/chats/chat-1/completions":
                events = [{"code": 0, "data": {"answer": text, "reference": reference, "session_id": "session-1"}}, {"code": 0, "data": True}]
            elif self.path == "/api/v1/agents/chat/completions":
                event = {"event": "message", "session_id": "session-1", "data": {"content": text}}
                events = [event, {**event, "event": "message_end", "data": {"content": text, "reference": reference}}]
            else:
                self.send_error(404)
                return
            body = ("".join("data:" + json.dumps(event, ensure_ascii=False) + "\n\n" for event in events) + "data:[DONE]\n\n").encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream; charset=utf-8")
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
@pytest.mark.parametrize("separator", [" ", " ", "\u0085"])
def test_ask_stream_keeps_frames_with_unicode_line_separators(completion_api, session_type, separator):
    """A separator inside message text must not split the frame and drop the message or its reference."""
    rag, state = completion_api
    state["text"] = f"first{separator}second"
    session = Session(rag, {"id": "session-1", f"{session_type}_id": f"{session_type}-1"})
    messages = list(session.ask("question", stream=True))

    expected = [state["text"], ""] if session_type == "agent" else [state["text"]]
    assert [message.content for message in messages] == expected
    assert messages[-1].reference == [{"id": "chunk-1", "content": state["text"]}]
