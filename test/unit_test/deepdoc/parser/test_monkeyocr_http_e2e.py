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
import logging
import threading
import zipfile
from http.server import BaseHTTPRequestHandler, HTTPServer
from io import BytesIO

import pytest
from pypdf import PdfWriter

from common.constants import MAXIMUM_PAGE_NUMBER
from deepdoc.parser.monkeyocr_parser import MonkeyOCRParser

log = logging.getLogger(__name__)


class _MockMonkeyOCRHandler(BaseHTTPRequestHandler):
    BODY = "MonkeyOCR HTTP integration body"

    def log_message(self, format, *args):
        return

    def do_HEAD(self):
        if self.path == "/openapi.json":
            self.send_response(200)
            self.end_headers()
            return
        self.send_response(404)
        self.end_headers()

    def do_GET(self):
        if self.path == "/openapi.json":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b'{"openapi":"3.0.0"}')
            return
        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        if self.path != "/file_parse":
            self.send_response(404)
            self.end_headers()
            return
        length = int(self.headers.get("Content-Length", "0"))
        if length:
            self.rfile.read(length)
        buf = BytesIO()
        with zipfile.ZipFile(buf, "w") as zf:
            zf.writestr(
                "doc/doc_content_list.json",
                json.dumps(
                    [
                        {
                            "type": "text",
                            "text": self.BODY,
                            "bbox": [100, 100, 200, 120],
                            "page_idx": 0,
                        }
                    ]
                ),
            )
        body = buf.getvalue()
        self.send_response(200)
        self.send_header("Content-Type", "application/zip")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


@pytest.fixture
def monkeyocr_http_server():
    server = HTTPServer(("127.0.0.1", 0), _MockMonkeyOCRHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    base_url = f"http://127.0.0.1:{server.server_address[1]}"
    log.info("Started mock MonkeyOCR server at %s", base_url)
    try:
        yield base_url
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


@pytest.mark.p1
def test_monkeyocr_http_parser_and_model(monkeyocr_http_server, tmp_path):
    import requests

    probe = requests.head(f"{monkeyocr_http_server}/openapi.json", timeout=5)
    assert probe.status_code in (200, 301, 302, 307, 308)

    pdf_path = tmp_path / "doc.pdf"
    writer = PdfWriter()
    writer.add_blank_page(width=72, height=72)
    pdf_buf = BytesIO()
    writer.write(pdf_buf)
    pdf_path.write_bytes(pdf_buf.getvalue())

    parser = MonkeyOCRParser(monkeyocr_api=monkeyocr_http_server)
    ok, reason = parser.check_installation()
    assert ok, reason

    sections, _tables = parser.parse_pdf(
        filepath=str(pdf_path),
        binary=pdf_path.read_bytes(),
        output_dir=str(tmp_path / "parser-out"),
        delete_output=False,
        parse_method="raw",
        page_to=MAXIMUM_PAGE_NUMBER,
    )
    texts = [s[0] for s in sections if s]
    assert any(_MockMonkeyOCRHandler.BODY in t for t in texts)
