"""Exercise the real CLI and process workers against a loopback HTTP fixture."""

import json
import os
import subprocess
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import pytest


@pytest.fixture
def benchmark_server():
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def send_body(self, body, status=200, content_type="application/json"):
            self.send_response(status)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            self.send_body(b'{"code":0,"data":{"id":"chat","llm_id":"fixture-model"}}')

        def do_POST(self):
            self.rfile.read(int(self.headers.get("Content-Length", "0")))
            with self.server.lock:
                index = self.server.requests
                self.server.requests += 1
            if index == 4:
                # Close without response headers: requests raises ConnectionError.
                self.close_connection = True
                return
            if self.path.endswith("/retrieval"):
                bodies = [b'{"code":0,"data":{"chunks":[]}}', b'{"code":102}', b'{"code":0}', b"[]"]
                self.send_body(bodies[index], status=503 if index == 2 else 200)
            else:
                content = b'data: {"choices":[{"delta":{"content":"answer"}}]}\n\n'
                bodies = [content + b"data: [DONE]\n\n", content, content + b"data: [DONE]\n\n", content + b'data: {"error":{"message":"failed"}}\n\n']
                self.send_body(bodies[index], status=503 if index == 2 else 200, content_type="text/event-stream; charset=utf-8")

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    server.requests = 0
    server.lock = threading.Lock()
    worker = threading.Thread(target=server.serve_forever, daemon=True)
    worker.start()
    try:
        yield server
    finally:
        server.shutdown()
        worker.join()
        server.server_close()


@pytest.mark.parametrize("command", ["chat", "retrieval"])
@pytest.mark.parametrize("concurrency", [1, 2])
def test_cli_finishes_mixed_workload_and_reports_only_successful_latency(benchmark_server, command, concurrency):
    repo = Path(__file__).resolve().parents[3]
    args = [
        sys.executable,
        "-m",
        "benchmark",
        command,
        "--base-url",
        f"http://127.0.0.1:{benchmark_server.server_port}",
        "--api-key",
        "local-fixture-token",
        "--iterations",
        "5",
        "--concurrency",
        str(concurrency),
        "--json",
        "--print-response",
    ]
    if command == "chat":
        args += ["--chat-id", "chat", "--model", "fixture-model", "--message", "hello"]
    else:
        args += ["--dataset-id", "dataset", "--question", "hello"]
    env = {**os.environ, "PYTHONPATH": str(repo / "test"), "NO_PROXY": "127.0.0.1", "no_proxy": "127.0.0.1"}
    result = subprocess.run(args, cwd=repo, env=env, capture_output=True, text=True, timeout=30, check=False)
    assert result.returncode == 1, result.stderr
    report = json.loads(result.stdout)
    assert benchmark_server.requests == 5
    assert report["success"] == 1
    assert report["failure"] == 4
    assert report["failure_rate"] == 0.8
    assert report["success_qps"] == pytest.approx(report["qps"] / 5)
    assert len(report["errors"]) == 4
    assert len(report["responses"]) == 5
    latency = "total_latency" if command == "chat" else "latency"
    assert report[latency]["count"] == 1
    if command == "chat":
        assert report["first_token_latency"]["count"] == 1
        assert any("[error]" in response and "answer" in response for response in report["responses"])
    assert "Traceback" not in result.stderr
