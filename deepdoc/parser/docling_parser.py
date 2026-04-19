#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
from __future__ import annotations

import logging
import re
import base64
import os
from dataclasses import dataclass
from enum import Enum
from io import BytesIO
from os import PathLike
from pathlib import Path
from typing import Any, Callable, Iterable, Optional

import pdfplumber
import requests
from PIL import Image

try:
    from docling.document_converter import DocumentConverter
except Exception:
    DocumentConverter = None  

try:
    from deepdoc.parser.pdf_parser import RAGFlowPdfParser
except Exception:
    class RAGFlowPdfParser:  
        pass

from deepdoc.parser.utils import extract_pdf_outlines



class DoclingContentType(str, Enum):
    IMAGE = "image"
    TABLE = "table"
    TEXT = "text"
    EQUATION = "equation"


@dataclass
class _BBox:
    page_no: int  
    x0: float
    y0: float
    x1: float
    y1: float


def _extract_bbox_from_prov(item, prov_attr: str = "prov") -> Optional[_BBox]:
    prov = getattr(item, prov_attr, None)
    if not prov:
        return None
    
    prov_item = prov[0] if isinstance(prov, list) else prov
    pn = getattr(prov_item, "page_no", None)
    bb = getattr(prov_item, "bbox", None)
    if pn is None or bb is None:
        return None
    
    coords = [getattr(bb, attr) for attr in ("l", "t", "r", "b")]
    if None in coords:
        return None
    
    return _BBox(page_no=int(pn), x0=coords[0], y0=coords[1], x1=coords[2], y1=coords[3])


class DoclingParser(RAGFlowPdfParser):
    def __init__(self, docling_server_url: str = "", request_timeout: int = 600):
        self.logger = logging.getLogger(self.__class__.__name__)
        self.page_images: list[Image.Image] = []
        self.page_from = 0
        self.page_to = 10_000
        self.outlines = []
        self.docling_server_url = (docling_server_url or "").rstrip("/")
        self.request_timeout = request_timeout

    def _effective_server_url(self, docling_server_url: Optional[str] = None) -> str:
        return (docling_server_url or self.docling_server_url or "").rstrip("/") or (
            os.environ.get("DOCLING_SERVER_URL", "").rstrip("/")
        )

    @staticmethod
    def _is_http_endpoint_valid(url: str, timeout: int = 5) -> bool:
        try:
            response = requests.head(url, timeout=timeout, allow_redirects=True)
            return response.status_code in [200, 301, 302, 307, 308]
        except Exception:
            try:
                response = requests.get(url, timeout=timeout, allow_redirects=True)
                return response.status_code in [200, 301, 302, 307, 308]
            except Exception:
                return False

    def check_installation(self, docling_server_url: Optional[str] = None) -> bool:
        server_url = self._effective_server_url(docling_server_url)
        if server_url:
            for path in ("/openapi.json", "/docs", "/v1/convert/source"):
                if self._is_http_endpoint_valid(f"{server_url}{path}", timeout=5):
                    return True
            self.logger.warning(f"[Docling] external server not reachable: {server_url}")
            return False

        if DocumentConverter is None:
            self.logger.warning("[Docling] 'docling' is not importable, please: pip install docling")
            return False
        try:
            _ = DocumentConverter()
            return True
        except Exception as e:
            self.logger.error(f"[Docling] init DocumentConverter failed: {e}")
            return False

    def __images__(self, fnm, zoomin: int = 1, page_from=0, page_to=600, callback=None):
        self.page_from = page_from
        self.page_to = page_to
        bytes_io = None
        try:
            if not isinstance(fnm, (str, PathLike)):
                bytes_io = BytesIO(fnm)

            opener = pdfplumber.open(fnm) if isinstance(fnm, (str, PathLike)) else pdfplumber.open(bytes_io)
            with opener as pdf:
                pages = pdf.pages[page_from:page_to]
                self.page_images = [p.to_image(resolution=72 * zoomin, antialias=True).original for p in pages]
        except Exception as e:
            self.page_images = []
            self.logger.exception(e)
        finally:
            if bytes_io:
                bytes_io.close()

    def _make_line_tag(self,bbox: _BBox) -> str:
        if bbox is None:
            return ""
        x0,x1, top, bott = bbox.x0, bbox.x1, bbox.y0, bbox.y1
        if hasattr(self, "page_images") and self.page_images and len(self.page_images) >= bbox.page_no:
            _, page_height = self.page_images[bbox.page_no-1].size
            top, bott = page_height-top ,page_height-bott
        return "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##".format(
            bbox.page_no, x0,x1, top, bott
        )

    @staticmethod
    def extract_positions(txt: str) -> list[tuple[list[int], float, float, float, float]]:
        poss = []
        for tag in re.findall(r"@@[0-9-]+\t[0-9.\t]+##", txt):
            pn, left, right, top, bottom = tag.strip("#").strip("@").split("\t")
            left, right, top, bottom = float(left), float(right), float(top), float(bottom)
            poss.append(([int(p) - 1 for p in pn.split("-")], left, right, top, bottom))
        return poss

    def crop(self, text: str, ZM: int = 1, need_position: bool = False):
        imgs = []
        poss = self.extract_positions(text)
        if not poss:
            return (None, None) if need_position else None

        GAP = 6
        pos = poss[0]
        poss.insert(0, ([pos[0][0]], pos[1], pos[2], max(0, pos[3] - 120), max(pos[3] - GAP, 0)))
        pos = poss[-1]
        poss.append(([pos[0][-1]], pos[1], pos[2], min(self.page_images[pos[0][-1]].size[1], pos[4] + GAP), min(self.page_images[pos[0][-1]].size[1], pos[4] + 120)))
        positions = []
        for ii, (pns, left, right, top, bottom) in enumerate(poss):
            if bottom <= top:
                bottom = top + 4
            img0 = self.page_images[pns[0]]
            x0, y0, x1, y1 = int(left), int(top), int(right), int(min(bottom, img0.size[1]))
            
            crop0 = img0.crop((x0, y0, x1, y1))
            imgs.append(crop0)
            if 0 < ii < len(poss)-1:
                positions.append((pns[0] + self.page_from, x0, x1, y0, y1))
            remain_bottom = bottom - img0.size[1]
            for pn in pns[1:]:
                if remain_bottom <= 0:
                    break
                page = self.page_images[pn]
                x0, y0, x1, y1 = int(left), 0, int(right), int(min(remain_bottom, page.size[1]))
                cimgp = page.crop((x0, y0, x1, y1))
                imgs.append(cimgp)
                if 0 < ii < len(poss) - 1:
                    positions.append((pn + self.page_from, x0, x1, y0, y1))
                remain_bottom -= page.size[1]

        if not imgs:
            return (None, None) if need_position else None

        height = sum(i.size[1] + GAP for i in imgs)
        width = max(i.size[0] for i in imgs)
        pic = Image.new("RGB", (width, int(height)), (245, 245, 245))
        h = 0
        for ii, img in enumerate(imgs):
            if ii == 0 or ii + 1 == len(imgs):
                img = img.convert("RGBA")
                overlay = Image.new("RGBA", img.size, (0, 0, 0, 0))
                overlay.putalpha(128)
                img = Image.alpha_composite(img, overlay).convert("RGB")
            pic.paste(img, (0, int(h)))
            h += img.size[1] + GAP

        return (pic, positions) if need_position else pic

    def _iter_doc_items(self, doc) -> Iterable[tuple[str, Any, Optional[_BBox]]]:
        for t in getattr(doc, "texts", []):
            parent = getattr(t, "parent", "")
            ref = getattr(parent, "cref", "")
            label = getattr(t, "label", "")
            if (label in ("section_header", "text") and ref in ("#/body",)) or label in ("list_item",):
                text = getattr(t, "text", "") or ""
                bbox = _extract_bbox_from_prov(t)
                yield (DoclingContentType.TEXT.value, text, bbox)

        for item in getattr(doc, "texts", []):
            if getattr(item, "label", "") in ("FORMULA",):
                text = getattr(item, "text", "") or ""
                bbox = _extract_bbox_from_prov(item)
                yield (DoclingContentType.EQUATION.value, text, bbox)

    def _transfer_to_sections(self, doc, parse_method: str) -> list[tuple[str, ...]]:
        sections: list[tuple[str, ...]] = []
        for typ, payload, bbox in self._iter_doc_items(doc):
            if typ == DoclingContentType.TEXT.value:
                section = payload.strip()
                if not section:
                    continue
            elif typ == DoclingContentType.EQUATION.value:
                section = payload.strip()
            else:
                continue
            
            tag = self._make_line_tag(bbox) if isinstance(bbox,_BBox) else ""
            if parse_method in {"manual", "pipeline"}:
                sections.append((section, typ, tag))
            elif parse_method == "paper":
                sections.append((section + tag, typ))
            else:
                sections.append((section, tag))
        return sections

    def cropout_docling_table(self, page_no: int, bbox: tuple[float, float, float, float], zoomin: int = 1):
        if not getattr(self, "page_images", None):
            return None, ""

        idx = (page_no - 1) - getattr(self, "page_from", 0)
        if idx < 0 or idx >= len(self.page_images):
            return None, ""

        page_img = self.page_images[idx]
        W, H = page_img.size
        left, top, right, bott = bbox

        x0 = float(left)
        y0 = float(H-top)
        x1 = float(right)
        y1 = float(H-bott)

        x0, y0 = max(0.0, min(x0, W - 1)), max(0.0, min(y0, H - 1))
        x1, y1 = max(x0 + 1.0, min(x1, W)), max(y0 + 1.0, min(y1, H))

        try:
            crop = page_img.crop((int(x0), int(y0), int(x1), int(y1))).convert("RGB")
        except Exception:
            return None, ""

        pos = (page_no-1 if page_no>0 else 0, x0, x1, y0, y1)
        return crop, [pos]

    def _transfer_to_tables(self, doc):
        tables = []
        for tab in getattr(doc, "tables", []):
            img = None
            positions = ""
            bbox = _extract_bbox_from_prov(tab)
            if bbox:
                img, positions = self.cropout_docling_table(bbox.page_no, (bbox.x0, bbox.y0, bbox.x1, bbox.y1))
            html = ""
            try:
                html = tab.export_to_html(doc=doc)
            except Exception:
                pass
            tables.append(((img, html), positions if positions else ""))
        for pic in getattr(doc, "pictures", []):
            img = None
            positions = ""
            bbox = _extract_bbox_from_prov(pic)
            if bbox:
                img, positions = self.cropout_docling_table(bbox.page_no, (bbox.x0, bbox.y0, bbox.x1, bbox.y1))
            captions = ""
            try:
                captions = pic.caption_text(doc=doc)
            except Exception:
                pass
            tables.append(((img, [captions]), positions if positions else ""))
        return tables

    @staticmethod
    def _sections_from_remote_text(text: str, parse_method: str) -> list[tuple[str, ...]]:
        txt = (text or "").strip()
        if not txt:
            return []
        if parse_method in {"manual", "pipeline"}:
            return [(txt, DoclingContentType.TEXT.value, "")]
        if parse_method == "paper":
            return [(txt, DoclingContentType.TEXT.value)]
        return [(txt, "")]

    @staticmethod
    def _extract_remote_document_entries(payload: Any) -> list[dict[str, Any]]:
        if not isinstance(payload, dict):
            return []
        if isinstance(payload.get("document"), dict):
            return [payload["document"]]
        if isinstance(payload.get("documents"), list):
            return [d for d in payload["documents"] if isinstance(d, dict)]
        if isinstance(payload.get("results"), list):
            docs = []
            for it in payload["results"]:
                if isinstance(it, dict):
                    if isinstance(it.get("document"), dict):
                        docs.append(it["document"])
                    elif isinstance(it.get("result"), dict):
                        docs.append(it["result"])
                    else:
                        docs.append(it)
            return docs
        return []

    def _parse_pdf_remote(
        self,
        filepath: str | PathLike[str],
        binary: BytesIO | bytes | None = None,
        callback: Optional[Callable] = None,
        *,
        parse_method: str = "raw",
        docling_server_url: Optional[str] = None,
        request_timeout: Optional[int] = None,
    ):
        server_url = self._effective_server_url(docling_server_url)
        if not server_url:
            raise RuntimeError("[Docling] DOCLING_SERVER_URL is not configured.")

        timeout = request_timeout or self.request_timeout
        if binary is not None:
            if isinstance(binary, (bytes, bytearray)):
                pdf_bytes = bytes(binary)
            else:
                pdf_bytes = bytes(binary.getbuffer())
        else:
            src_path = Path(filepath)
            if not src_path.exists():
                raise FileNotFoundError(f"PDF not found: {src_path}")
            with open(src_path, "rb") as f:
                pdf_bytes = f.read()

        if callback:
            callback(0.2, f"[Docling] Requesting external server: {server_url}")

        filename = Path(filepath).name or "input.pdf"
        b64 = base64.b64encode(pdf_bytes).decode("ascii")
        
        # Standard payloads
        v1_payload = {
            "options": {"from_formats": ["pdf"], "to_formats": ["json", "md", "text"]},
            "sources": [{"kind": "file", "filename": filename, "base64_string": b64}],
        }
        v1alpha_payload = {
            "options": {"from_formats": ["pdf"], "to_formats": ["json", "md", "text"]},
            "file_sources": [{"filename": filename, "base64_string": b64}],
        }
        
        # --- NEW: Chunking Payload ---
        # Docling-serve's native chunking endpoint parameters
        chunking_payload = {
            "sources": [{"kind": "file", "filename": filename, "base64_string": b64}],
            "options": {
                "max_tokens": 512, # A safe default to prevent embedding overflows
                "overlap_tokens": 50
            }
        }

        errors = []
        response_json = None
        is_chunked_response = False
        
        # We prioritize the new chunking endpoints first!
        for endpoint, payload, chunk_flag in (
            ("/v1/chunk/source", chunking_payload, True),          # New stable chunking
            ("/v1alpha/chunk/source", chunking_payload, True),     # New alpha chunking
            ("/v1/convert/source", v1_payload, False),             # Fallback to standard
            ("/v1alpha/convert/source", v1alpha_payload, False),   # Fallback to alpha standard
        ):
            try:
                resp = requests.post(
                    f"{server_url}{endpoint}",
                    json=payload,
                    timeout=timeout,
                )
                if resp.status_code < 300:
                    response_json = resp.json()
                    is_chunked_response = chunk_flag
                    break
                errors.append(f"{endpoint}: HTTP {resp.status_code} {resp.text[:300]}")
            except Exception as exc:
                errors.append(f"{endpoint}: {exc}")

        if response_json is None:
            raise RuntimeError("[Docling] remote convert failed: " + " | ".join(errors))

        sections: list[tuple[str, ...]] = []
        tables = []
        
        # --- NEW: Handle Native Chunked Response ---
        if is_chunked_response:
            # The chunking endpoint returns an array of chunk items
            chunks = response_json if isinstance(response_json, list) else response_json.get("results", [])
            for chunk_data in chunks:
                if not isinstance(chunk_data, dict):
                    continue
                # Depending on the exact docling-serve spec, the text might be nested
                chunk_text = chunk_data.get("text", "")
                if not chunk_text and isinstance(chunk_data.get("chunk"), dict):
                    chunk_text = chunk_data["chunk"].get("text", "")
                
                if chunk_text.strip():
                    # Feed the pre-sliced chunks directly into RAGFlow's expected format
                    sections.extend(self._sections_from_remote_text(chunk_text, parse_method=parse_method))
                    
            if callback:
                callback(0.95, f"[Docling] Native chunks received: {len(sections)}")
            return sections, tables

        # --- FALLBACK: Standard RAGFlow parsing for older docling servers ---
        docs = self._extract_remote_document_entries(response_json)
        if not docs:
            raise RuntimeError("[Docling] remote response does not contain parsed documents.")

        for doc in docs:
            md = doc.get("md_content")
            txt = doc.get("text_content")
            if isinstance(md, str) and md.strip():
                sections.extend(self._sections_from_remote_text(md, parse_method=parse_method))
            elif isinstance(txt, str) and txt.strip():
                sections.extend(self._sections_from_remote_text(txt, parse_method=parse_method))

            json_content = doc.get("json_content")
            if isinstance(json_content, dict):
                md_fallback = json_content.get("md_content")
                if isinstance(md_fallback, str) and md_fallback.strip() and not sections:
                    sections.extend(self._sections_from_remote_text(md_fallback, parse_method=parse_method))

        if callback:
            callback(0.95, f"[Docling] Remote sections: {len(sections)}")
        return sections, tables

    def parse_pdf(
        self,
        filepath: str | PathLike[str],
        binary: BytesIO | bytes | None = None,
        callback: Optional[Callable] = None,
        *,
        output_dir: Optional[str] = None, 
        lang: Optional[str] = None,        
        method: str = "auto",             
        delete_output: bool = True,
        parse_method: str = "raw",
        docling_server_url: Optional[str] = None,
        request_timeout: Optional[int] = None,
    ):
        self.outlines = extract_pdf_outlines(binary if binary is not None else filepath)

        if not self.check_installation(docling_server_url=docling_server_url):
            raise RuntimeError("Docling not available, please install `docling`")

        server_url = self._effective_server_url(docling_server_url)
        if server_url:
            return self._parse_pdf_remote(
                filepath=filepath,
                binary=binary,
                callback=callback,
                parse_method=parse_method,
                docling_server_url=server_url,
                request_timeout=request_timeout,
            )

        if binary is not None:
            tmpdir = Path(output_dir) if output_dir else Path.cwd() / ".docling_tmp"
            tmpdir.mkdir(parents=True, exist_ok=True)
            name = Path(filepath).name or "input.pdf"
            tmp_pdf = tmpdir / name
            with open(tmp_pdf, "wb") as f:
                if isinstance(binary, (bytes, bytearray)):
                    f.write(binary)
                else:
                    f.write(binary.getbuffer())
            src_path = tmp_pdf
        else:
            src_path = Path(filepath)
            if not src_path.exists():
                raise FileNotFoundError(f"PDF not found: {src_path}")

        if callback:
            callback(0.1, f"[Docling] Converting: {src_path}")

        try:
            self.__images__(str(src_path), zoomin=1)
        except Exception as e:
            self.logger.warning(f"[Docling] render pages failed: {e}")

        conv = DocumentConverter()  
        conv_res = conv.convert(str(src_path))
        doc = conv_res.document
        if callback:
            callback(0.7, f"[Docling] Parsed doc: {getattr(doc, 'num_pages', 'n/a')} pages")

        sections = self._transfer_to_sections(doc, parse_method=parse_method)
        tables = self._transfer_to_tables(doc)

        if callback:
            callback(0.95, f"[Docling] Sections: {len(sections)}, Tables: {len(tables)}")

        if binary is not None and delete_output:
            try:
                Path(src_path).unlink(missing_ok=True)
            except Exception:
                pass

        if callback:
            callback(1.0, "[Docling] Done.")
        return sections, tables


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)
    parser = DoclingParser()
    print("Docling available:", parser.check_installation())
    sections, tables = parser.parse_pdf(filepath="test_docling/toc.pdf", binary=None)
    print(len(sections), len(tables))
