#!/usr/bin/env python3
"""Standalone MonkeyOCR HTTP integration check (run outside pytest collection)."""

from __future__ import annotations

import json
import sys
import threading
import zipfile
from http.server import BaseHTTPRequestHandler, HTTPServer
from io import BytesIO
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(REPO_ROOT))


class _Handler(BaseHTTPRequestHandler):
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


def main() -> int:
    server = HTTPServer(("127.0.0.1", 0), _Handler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    base_url = f"http://127.0.0.1:{server.server_address[1]}"

    import requests

    probe = requests.head(f"{base_url}/openapi.json", timeout=5)
    if probe.status_code not in (200, 301, 302, 307, 308):
        print(f"FAIL: mock server probe status={probe.status_code}")
        return 1

    from io import BytesIO

    from pypdf import PdfWriter

    from deepdoc.parser.monkeyocr_parser import MonkeyOCRParser
    from deepdoc.parser.pdf_parser import MAXIMUM_PAGE_NUMBER
    from rag.llm.ocr_model import MonkeyOCROcrModel

    tmp = Path("/tmp/monkeyocr_e2e")
    tmp.mkdir(parents=True, exist_ok=True)
    pdf_path = tmp / "doc.pdf"
    writer = PdfWriter()
    writer.add_blank_page(width=72, height=72)
    buf = BytesIO()
    writer.write(buf)
    pdf_path.write_bytes(buf.getvalue())

    parser = MonkeyOCRParser(monkeyocr_api=base_url)
    ok, reason = parser.check_installation()
    if not ok:
        print(f"FAIL: check_installation: {reason}")
        return 1

    sections, _tables = parser.parse_pdf(
        filepath=str(pdf_path),
        binary=pdf_path.read_bytes(),
        output_dir=str(tmp / "parser-out"),
        delete_output=False,
        parse_method="raw",
        page_to=MAXIMUM_PAGE_NUMBER,
    )
    texts = [s[0] for s in sections if s]
    if not any(_Handler.BODY in t for t in texts):
        print(f"FAIL: parser sections missing body: {texts!r}")
        return 1

    model = MonkeyOCROcrModel(
        {"monkeyocr_apiserver": base_url, "monkeyocr_output_dir": str(tmp / "model-out"), "monkeyocr_delete_output": 0},
        "monkeyocr-test",
    )
    sections2, _tables2 = model.parse_pdf(
        filepath=str(pdf_path),
        binary=pdf_path.read_bytes(),
        parse_method="raw",
        page_to=MAXIMUM_PAGE_NUMBER,
    )
    texts2 = [s[0] for s in sections2 if s]
    if not any(_Handler.BODY in t for t in texts2):
        print(f"FAIL: ocr model sections missing body: {texts2!r}")
        return 1

    server.shutdown()
    print("OK: MonkeyOCR HTTP integration (parser + OCR model)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
