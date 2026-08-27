"""Client for the MonkeyOCRv2-Parsing ZIP API."""
import io
import json
import zipfile
from pathlib import Path

import requests


class MonkeyOCRv2Parser:
    def __init__(self, server_url: str, timeout: int = 1800):
        self.server_url = server_url.rstrip("/")
        self.timeout = timeout

    def check_installation(self):
        try:
            return requests.get(f"{self.server_url}/health", timeout=10).ok
        except requests.RequestException:
            return False

    def parse_pdf(self, filepath, binary=None, callback=None, page_from=0, page_to=99999, **_kwargs):
        payload = binary if binary is not None else Path(filepath).read_bytes()
        response = requests.post(f"{self.server_url}/parse", files={"files": (Path(filepath).name, payload, "application/pdf")}, data={"start_page_id": page_from, "end_page_id": page_to}, timeout=self.timeout)
        response.raise_for_status()
        if "zip" not in response.headers.get("content-type", "").lower():
            raise RuntimeError("MonkeyOCRv2 /parse did not return a ZIP archive")
        return self._convert_zip(response.content)

    def _convert_zip(self, archive_bytes):
        sections, tables = [], []
        with zipfile.ZipFile(io.BytesIO(archive_bytes)) as archive:
            roots = {n.split("/", 1)[0] for n in archive.namelist() if n.endswith(".md")}
            for root in roots:
                candidates = [n for n in archive.namelist() if n.startswith(root + "/jsons/") and n.endswith(".json")]
                records = []
                for name in candidates:
                    try:
                        records.append(json.loads(archive.read(name)))
                    except (ValueError, UnicodeDecodeError):
                        pass
                if not records:
                    summary = f"{root}/all_results.json"
                    if summary in archive.namelist():
                        try:
                            records = json.loads(archive.read(summary))
                            if isinstance(records, dict):
                                records = [records]
                        except (ValueError, UnicodeDecodeError):
                            records = []
                for document in records:
                    for layout in document.get("layouts", []):
                        text = (layout.get("content") or "").strip()
                        if not text:
                            continue
                        page = max(0, int(layout.get("page_num", 1)) - 1)
                        bbox = layout.get("bbox", [0, 0, 0, 0])
                        tag = f"@@{page + 1}\t{bbox[0]}\t{bbox[2]}\t{bbox[1]}\t{bbox[3]}##"
                        if str(layout.get("label", "")).lower() == "table":
                            tables.append(((None, text), [(page, *bbox)]))
                        else:
                            sections.append((text, tag))
        return sections, tables
