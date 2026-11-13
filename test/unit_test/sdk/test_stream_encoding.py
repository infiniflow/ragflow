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

"""Decode SDK completion streams as UTF-8 with or without a charset header."""

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Thread

import pytest
from ragflow_sdk import RAGFlow
from ragflow_sdk.modules.session import Session

pytestmark = pytest.mark.p2


@pytest.fixture
def completion_api():
    state = {"content_type": "text/event-stream", "answer": ""}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            """Suppress access logs for the local fixture."""

        def do_POST(self):
            self.rfile.read(int(self.headers["Content-Length"]))
            reference = {"chunks": [{"id": "chunk-1", "content": state["answer"]}]}
            if self.path == "/api/v1/chats/chat-1/completions":
                events = [
                    {"code": 0, "data": {"answer": state["answer"], "id": "message-1", "reference": reference}},
                    {"code": 0, "data": True},
                ]
            elif self.path == "/api/v1/agents/chat/completions":
                events = [
                    {"event": "message", "message_id": "message-1", "data": {"content": state["answer"]}},
                    {"event": "message_end", "message_id": "message-1", "data": {"reference": reference}},
                ]
            else:
                self.send_error(404)
                return
            body = ("".join("data:" + json.dumps(event, ensure_ascii=False) + "\n\n" for event in events) + "data:[DONE]\n\n").encode("utf-8")
            self.send_response(200)
            self.send_header("Content-Type", state["content_type"])
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
@pytest.mark.parametrize("content_type", ["text/event-stream", "text/event-stream; charset=utf-8"])
@pytest.mark.parametrize("answer", ["ASCII answer", "\u4f60\u597d, caf\u00e9! \U0001f680"], ids=["ascii", "unicode"])
def test_ask_decodes_stream_as_utf8(completion_api, session_type, content_type, answer):
    rag, state = completion_api
    state.update(content_type=content_type, answer=answer)
    session = Session(rag, {"id": "session-1", f"{session_type}_id": f"{session_type}-1"})

    messages = list(session.ask("question", stream=True))

    assert [message.content for message in messages] == ([answer, ""] if session_type == "agent" else [answer])
    assert [message.id for message in messages] == ["message-1"] * len(messages)
    assert messages[-1].reference == [{"id": "chunk-1", "content": answer}]
