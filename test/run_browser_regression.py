"""Run mocked browser journeys against the built SPA without a live backend."""

import argparse
from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import os
from pathlib import Path
import subprocess
import sys
from threading import Thread
from urllib.parse import urlsplit


ROOT = Path(__file__).resolve().parents[1]
SUITES = [
    "test/playwright/e2e/test_business_documents_access_ui.py",
    "test/playwright/e2e/test_navigation_visibility_admin.py",
    "test/playwright/e2e/test_auth_boundaries_ui.py",
    "test/playwright/e2e/test_upload_boundaries_ui.py",
]


class SPAServer(ThreadingHTTPServer):
    request_queue_size = 128
    daemon_threads = True


class SPAHandler(SimpleHTTPRequestHandler):
    def do_GET(self):
        path = urlsplit(self.path).path
        if path.startswith(("/api/", "/v1/")):
            self.send_error(503, "Browser regression requires mocked API responses")
            return
        if not Path(self.translate_path(path)).is_file() and not Path(path).suffix:
            self.path = "/index.html"
        super().do_GET()

    def log_message(self, *_args):
        pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--browser", choices=("chromium", "firefox", "webkit"), default="chromium")
    parser.add_argument("pytest_args", nargs="*")
    args = parser.parse_args()
    dist = ROOT / "web" / "dist"
    if not (dist / "index.html").is_file():
        parser.error("Build the frontend first: cd web && pnpm build")
    server = SPAServer(("127.0.0.1", 0), partial(SPAHandler, directory=str(dist)))
    thread = Thread(target=server.serve_forever, daemon=True)
    thread.start()
    env = {**os.environ, "RAGFLOW_BASE_URL": f"http://127.0.0.1:{server.server_port}", "PW_BROWSER": args.browser}
    try:
        return subprocess.call([sys.executable, "-m", "pytest", *SUITES, *args.pytest_args], cwd=ROOT, env=env)
    finally:
        server.shutdown()
        server.server_close()
        thread.join()


if __name__ == "__main__":
    raise SystemExit(main())
