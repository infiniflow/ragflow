"""Client for the MonkeyOCRv2 ZIP API."""

import io
import json
import re
import zipfile
from pathlib import Path

import requests
from PIL import Image


class MonkeyOCRv2Parser:
    _MAX_RESPONSE_BYTES = 512 * 1024 * 1024
    _MAX_ZIP_MEMBERS = 10000
    _MAX_UNCOMPRESSED_BYTES = 2 * 1024 * 1024 * 1024

    def __init__(self, server_url: str, timeout: int = 1800):
        self.server_url = server_url.rstrip("/")
        self.timeout = timeout

    def check_installation(self):
        """Return whether the configured MonkeyOCRv2 service is reachable."""
        try:
            return requests.get(f"{self.server_url}/health", timeout=10).ok
        except requests.RequestException:
            return False

    def crop(self, _text, ZM=1, need_position=False):
        """Compatibility hook used by RAGFlow's PDF chunk tokenizer.

        MonkeyOCRv2 returns already-rendered layout content and image
        artifacts; it does not retain a local PDF canvas for post-hoc crops.
        Returning no crop is preferable to failing chunk generation.
        """
        del ZM
        positions = self.extract_positions(_text)
        return (None, positions) if need_position else None

    @staticmethod
    def remove_tag(text):
        """Return text unchanged; native layout tags are already normalized."""
        return re.sub(r"@@[\t0-9.-]+?##", "", text)

    @staticmethod
    def extract_positions(text):
        positions = []
        for tag in re.findall(r"@@[0-9-]+\t[0-9.\t]+##", text):
            page, left, right, top, bottom = tag.strip("#").strip("@").split("\t")
            positions.append((int(page) - 1, float(left), float(right), float(top), float(bottom)))
        return positions

    def parse_pdf(self, filepath, binary=None, callback=None, page_from=0, page_to=99999, **_kwargs):
        """Upload one document and convert the service's ZIP response."""
        payload = binary if binary is not None else Path(filepath).read_bytes()
        response = requests.post(
            f"{self.server_url}/parse", files={"files": (Path(filepath).name, payload, "application/pdf")}, data={"start_page_id": page_from, "end_page_id": page_to}, timeout=self.timeout, stream=True
        )
        try:
            response.raise_for_status()
            chunks = []
            size = 0
            if hasattr(response, "iter_content"):
                for chunk in response.iter_content(1024 * 1024):
                    if chunk:
                        size += len(chunk)
                        if size > self._MAX_RESPONSE_BYTES:
                            raise RuntimeError(f"MonkeyOCRv2 response exceeds {self._MAX_RESPONSE_BYTES} bytes")
                        chunks.append(chunk)
                archive_bytes = b"".join(chunks)
            else:
                archive_bytes = response.content
                if len(archive_bytes) > self._MAX_RESPONSE_BYTES:
                    raise RuntimeError(f"MonkeyOCRv2 response exceeds {self._MAX_RESPONSE_BYTES} bytes")
        finally:
            if hasattr(response, "close"):
                response.close()
        if "zip" not in response.headers.get("content-type", "").lower() and not zipfile.is_zipfile(io.BytesIO(archive_bytes)):
            raise RuntimeError("MonkeyOCRv2 /parse did not return a ZIP archive")
        try:
            result = self._convert_zip(archive_bytes)
        except (zipfile.BadZipFile, OSError, ValueError, TypeError, KeyError, IndexError) as exc:
            raise RuntimeError("Invalid MonkeyOCRv2 parse response") from exc
        return result

    def _convert_zip(self, archive_bytes):
        """Convert native JSON layout records to RAGFlow sections and tables."""
        sections, tables = [], []
        with zipfile.ZipFile(io.BytesIO(archive_bytes)) as archive:
            names = archive.namelist()
            infos = archive.infolist()
            if len(infos) > self._MAX_ZIP_MEMBERS or sum(info.file_size for info in infos) > self._MAX_UNCOMPRESSED_BYTES:
                raise ValueError("MonkeyOCRv2 ZIP exceeds safety limits")
            image_data = {}
            for info in infos:
                if info.filename.lower().endswith((".png", ".jpg", ".jpeg", ".webp")):
                    image_data[Path(info.filename).name] = archive.read(info)
            roots = {
                name.split("/", 1)[0]
                for name in names
                if "/" in name
                and (name.endswith(".md") or name.endswith("/all_results.json") or name == f"{name.split('/', 1)[0]}/{name.split('/', 1)[0]}.json" or (name.endswith(".json") and "/jsons/" in name))
            }
            for root in roots:
                canonical = f"{root}/{root}.json"
                candidates = [canonical] if canonical in names else []
                if not candidates:
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
                            page = max(0, int(layout.get("page_num", 1)) - 1)
                        except (TypeError, ValueError):
                            continue
                        bbox = layout.get("bbox", [0, 0, 0, 0])
                        if not isinstance(bbox, (list, tuple)) or len(bbox) != 4:
                            continue
                        tag = f"@@{page + 1}\t{bbox[0]}\t{bbox[2]}\t{bbox[1]}\t{bbox[3]}##"
                        label = str(layout.get("label", "")).lower()
                        if label in {"picture", "figure", "image"}:
                            match = re.search(r"!\[[^]]*\]\(([^)]+)\)", text)
                            if match:
                                image = image_data.get(Path(match.group(1).strip().strip("\"'")).name)
                                if image:
                                    try:
                                        media = Image.open(io.BytesIO(image)).copy()
                                        tables.append(((media, [""]), [(page, bbox[0], bbox[2], bbox[1], bbox[3])]))
                                    except OSError:
                                        pass
                            continue
                        if label == "table":
                            tables.append(((None, text), [(page, bbox[0], bbox[2], bbox[1], bbox[3])]))
                        else:
                            sections.append((text, tag))
        return sections, tables
