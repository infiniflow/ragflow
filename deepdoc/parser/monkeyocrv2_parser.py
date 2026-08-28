"""Client for the MonkeyOCRv2-Parsing ZIP API."""
import io
import json
import logging
import zipfile
from pathlib import Path

import requests


LOGGER = logging.getLogger(__name__)


class MonkeyOCRv2Parser:
    def __init__(self, server_url: str, timeout: int = 1800):
        self.server_url = server_url.rstrip("/")
        self.timeout = timeout

    def check_installation(self):
        """Return whether the configured MonkeyOCRv2 service is reachable."""
        try:
            return requests.get(f"{self.server_url}/health", timeout=10).ok
        except requests.RequestException:
            return False

    def parse_pdf(self, filepath, binary=None, callback=None, page_from=0, page_to=99999, **_kwargs):
        """Upload one document and convert the service's ZIP response."""
        payload = binary if binary is not None else Path(filepath).read_bytes()
        LOGGER.info("MonkeyOCRv2 parse request: file=%s pages=%s:%s", Path(filepath).name, page_from, page_to)
        response = requests.post(
            f"{self.server_url}/parse",
            files={"files": (Path(filepath).name, payload, "application/pdf")},
            data={"start_page_id": page_from, "end_page_id": page_to},
            timeout=self.timeout,
        )
        response.raise_for_status()
        if "zip" not in response.headers.get("content-type", "").lower():
            raise RuntimeError("MonkeyOCRv2 /parse did not return a ZIP archive")
        try:
            result = self._convert_zip(response.content, page_from=page_from)
        except (zipfile.BadZipFile, OSError, ValueError, TypeError, KeyError, IndexError) as exc:
            LOGGER.exception("MonkeyOCRv2 ZIP conversion failed")
            raise RuntimeError("Invalid MonkeyOCRv2 parse response") from exc
        LOGGER.info("MonkeyOCRv2 parse completed: sections=%d tables=%d", len(result[0]), len(result[1]))
        return result

    def _convert_zip(self, archive_bytes, page_from=0):
        """Convert native JSON layout records to RAGFlow sections and tables."""
        sections, tables = [], []
        with zipfile.ZipFile(io.BytesIO(archive_bytes)) as archive:
            names = archive.namelist()
            roots = {
                name.split("/", 1)[0]
                for name in names
                if "/" in name
                and (
                    name.endswith(".md")
                    or "/jsons/" in name and name.endswith(".json")
                    or name.endswith("/all_results.json")
                )
            }
            for root in roots:
                candidates = [n for n in names if n.startswith(root + "/jsons/") and n.endswith(".json")]
                records = []
                for name in candidates:
                    try:
                        records.append(json.loads(archive.read(name)))
                    except (ValueError, UnicodeDecodeError):
                        pass
                if not records:
                    summary = f"{root}/all_results.json"
                    if summary in names:
                        try:
                            records = json.loads(archive.read(summary))
                            if isinstance(records, dict):
                                records = [records]
                        except (ValueError, UnicodeDecodeError):
                            records = []
                for document in records if isinstance(records, list) else [records]:
                    if not isinstance(document, dict):
                        continue
                    layouts = document.get("layouts", [])
                    if not isinstance(layouts, list):
                        continue
                    for layout in layouts:
                        if not isinstance(layout, dict):
                            continue
                        raw_text = layout.get("content")
                        if not isinstance(raw_text, str):
                            continue
                        text = raw_text.strip()
                        if not text:
                            continue
                        try:
                            page = max(0, int(layout.get("page_num", 1)) - 1 + page_from)
                        except (TypeError, ValueError):
                            continue
                        bbox = layout.get("bbox", [0, 0, 0, 0])
                        if not isinstance(bbox, (list, tuple)) or len(bbox) != 4:
                            continue
                        tag = f"@@{page + 1}\t{bbox[0]}\t{bbox[2]}\t{bbox[1]}\t{bbox[3]}##"
                        if str(layout.get("label", "")).lower() == "table":
                            tables.append(((None, text), [(page, bbox[0], bbox[2], bbox[1], bbox[3])]))
                        else:
                            sections.append((text, tag))
        return sections, tables
