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
"""Mistral OCR PDF parser.

Calls Mistral's ``POST /v1/ocr`` endpoint (a dedicated document-OCR API, not
chat-completions), then normalizes the per-block response into the same shape
SoMark produces so the proven section-building contract is reused verbatim.
"""

import base64
import logging
import re
from io import BytesIO
from os import PathLike
from pathlib import Path
from typing import Optional

import numpy as np
import pdfplumber
import requests
from PIL import Image

from deepdoc.parser.pdf_parser import MAXIMUM_PAGE_NUMBER, RAGFlowPdfParser
from deepdoc.parser.utils import extract_pdf_outlines

# Mistral block "type" -> RAGFlow internal layout type.
# header/footer are resolved at runtime via keep_header_footer.
MISTRAL_TYPE_TO_RAGFLOW = {
    "text": "text",
    "title": "text",
    "list": "text",
    "table": "table",
    "image": "image",
    "equation": "equation",
    "code": "code",
}

# Always discarded regardless of flags.
ALWAYS_DISCARDED: set[str] = set()


class MistralParser(RAGFlowPdfParser):
    """Parse a PDF through Mistral OCR into (sections, tables)."""

    def __init__(
        self,
        base_url: str,
        api_key: str = "",
        *,
        model: str = "mistral-ocr-latest",
        table_format: str = "html",
        keep_header_footer: bool = False,
        timeout: int = 600,
        inline_max_bytes: int = 20 * 1024 * 1024,
    ):
        self.base_url = (base_url or "https://api.mistral.ai/v1").strip().rstrip("/")
        self.api_key = api_key
        self.model = model
        self.table_format = table_format.strip().lower() if table_format else "html"
        self.keep_header_footer = bool(keep_header_footer)
        self.timeout = int(timeout)
        self.inline_max_bytes = int(inline_max_bytes)
        self.outlines: list = []
        self.logger = logging.getLogger(self.__class__.__name__)
        # RAGFlowPdfParser.__init__ sets this default; MistralParser skips
        # that heavy __init__, so crop() needs it set before __images__ runs.
        self.page_from = 0
        # Optional tenant vision model (LLMBundle) for figure description;
        # set from parse_pdf's vision_model kwarg. None -> no enrichment.
        self.vision_model = None

    # ------------------------------------------------------------------
    # Page image rendering
    # ------------------------------------------------------------------
    def __images__(self, fnm, zoomin: int = 1, page_from: int = 0, page_to: int = MAXIMUM_PAGE_NUMBER, callback=None):
        self.page_from = page_from
        self.page_to = page_to
        try:
            ctx = pdfplumber.open(fnm) if isinstance(fnm, (str, PathLike)) else pdfplumber.open(BytesIO(fnm))
            with ctx as pdf:
                self.pdf = pdf
                self.page_images = [p.to_image(resolution=72 * zoomin, antialias=True).original for _, p in enumerate(self.pdf.pages[page_from:page_to])]
        except Exception as exc:
            self.page_images = None
            self.total_page = 0
            self.logger.exception(exc)

    # ------------------------------------------------------------------
    # Block classification
    # ------------------------------------------------------------------
    def _resolve_internal_type(self, block_type: str) -> Optional[str]:
        """Return the RAGFlow internal layout type, or None to discard.

        header/footer obey keep_header_footer; unknown types fall back to text
        to avoid silent loss.
        """
        if block_type in ALWAYS_DISCARDED:
            return None
        if block_type in ("header", "footer"):
            return "text" if self.keep_header_footer else None
        return MISTRAL_TYPE_TO_RAGFLOW.get(block_type, "text")

    @staticmethod
    def _block_text(block: dict, internal_type: str) -> str:
        """Textual payload for a block. image blocks contribute no text."""
        if internal_type == "image":
            return ""
        return (block.get("content") or "").strip()

    # ------------------------------------------------------------------
    # Response -> normalized pages (SoMark-compatible block shape)
    # ------------------------------------------------------------------
    def _normalize_pages(self, response: dict) -> list[dict]:
        """Map a /v1/ocr response into per-page dicts whose blocks carry a
        [x0, top, x1, bott] bbox and a {w,h} page_size, so downstream section
        building is identical to SoMark's."""
        out: list[dict] = []
        for page in (response or {}).get("pages") or []:
            dims = page.get("dimensions") or {}
            page_size = {"w": dims.get("width") or 1, "h": dims.get("height") or 1}
            blocks = []
            for b in page.get("blocks") or []:
                coord_keys = ("top_left_x", "top_left_y", "bottom_right_x", "bottom_right_y")
                if any(k in b for k in coord_keys):
                    bbox = [
                        b.get("top_left_x", 0),
                        b.get("top_left_y", 0),
                        b.get("bottom_right_x", 0),
                        b.get("bottom_right_y", 0),
                    ]
                else:
                    bbox = None
                blocks.append(
                    {
                        "type": b.get("type"),
                        "content": b.get("content"),
                        "bbox": bbox,
                        "table_id": b.get("table_id"),
                    }
                )
            out.append(
                {
                    "page_num": page.get("index", 0),
                    "page_size": page_size,
                    "blocks": blocks,
                }
            )
        return out

    # ------------------------------------------------------------------
    # Position tagging (compatible with RAGFlow extract_positions/crop)
    # ------------------------------------------------------------------
    def _line_tag(self, bx: dict) -> str:
        """Build a ``@@page\tx0\tx1\ttop\tbott##`` tag (page 1-based, order
        x0,x1,top,bott), rescaling bbox from Mistral's dimensions space to the
        locally rendered page pixels."""
        page_idx = bx.get("page_idx", 0)
        pn = [page_idx + 1]
        bbox = bx.get("bbox") or [0, 0, 0, 0]
        if len(bbox) != 4:
            bbox = [0, 0, 0, 0]
        x0, top, x1, bott = bbox
        page_size = bx.get("page_size") or {}
        src_w = page_size.get("w") or 1
        src_h = page_size.get("h") or 1

        if x0 > x1:
            x0, x1 = x1, x0
        if top > bott:
            top, bott = bott, top

        if getattr(self, "page_images", None) and len(self.page_images) > page_idx:
            page_width, page_height = self.page_images[page_idx].size
            x0 = (x0 / src_w) * page_width
            x1 = (x1 / src_w) * page_width
            top = (top / src_h) * page_height
            bott = (bott / src_h) * page_height

        return "@@{}\t{:.1f}\t{:.1f}\t{:.1f}\t{:.1f}##".format("-".join(str(x) for x in pn), x0, x1, top, bott)

    @staticmethod
    def extract_positions(txt: str):
        poss = []
        for tag in re.findall(r"@@[0-9-]+\t[0-9.\t]+##", txt):
            pn, left, right, top, bottom = tag.strip("#").strip("@").split("\t")
            left, right, top, bottom = float(left), float(right), float(top), float(bottom)
            poss.append(([int(p) - 1 for p in pn.split("-")], left, right, top, bottom))
        return poss

    def crop(self, text, ZM=1, need_position=False):
        """Image crop based on tags."""
        imgs = []
        poss = self.extract_positions(text)
        if not poss:
            return (None, None) if need_position else None
        if not getattr(self, "page_images", None):
            self.logger.warning("[Mistral OCR] crop called without page images; skip.")
            return (None, None) if need_position else None

        page_count = len(self.page_images)
        filtered = []
        for pns, left, right, top, bottom in poss:
            valid_pns = [p for p in pns if 0 <= p < page_count]
            if valid_pns:
                filtered.append((valid_pns, left, right, top, bottom))
        if not filtered:
            return (None, None) if need_position else None
        poss = filtered

        max_width = max(np.max([right - left for (_, left, right, _, _) in poss]), 6)
        GAP = 6
        first = poss[0]
        poss.insert(
            0,
            (
                [first[0][0]],
                first[1],
                first[2],
                max(0, first[3] - 120),
                max(first[3] - GAP, 0),
            ),
        )
        last = poss[-1]
        last_pn = last[0][-1]
        if not (0 <= last_pn < page_count):
            return (None, None) if need_position else None
        last_h = self.page_images[last_pn].size[1]
        poss.append(
            (
                [last_pn],
                last[1],
                last[2],
                min(last_h, last[4] + GAP),
                min(last_h, last[4] + 120),
            )
        )

        positions = []
        for ii, (pns, left, right, top, bottom) in enumerate(poss):
            right = left + max_width
            if bottom <= top:
                bottom = top + 2
            for pn in pns[1:]:
                if 0 <= pn - 1 < page_count:
                    bottom += self.page_images[pn - 1].size[1]
            if not (0 <= pns[0] < page_count):
                continue
            base_img = self.page_images[pns[0]]
            x0, y0, x1, y1 = (
                int(left),
                int(top),
                int(right),
                int(min(bottom, base_img.size[1])),
            )
            if x0 > x1:
                x0, x1 = x1, x0
            if y0 > y1:
                y0, y1 = y1, y0
            if x1 <= x0 or y1 <= y0:
                continue
            imgs.append(base_img.crop((x0, y0, x1, y1)))
            if 0 < ii < len(poss) - 1:
                positions.append((pns[0] + self.page_from, x0, x1, y0, y1))
            bottom -= base_img.size[1]
            for pn in pns[1:]:
                if not (0 <= pn < page_count):
                    continue
                page = self.page_images[pn]
                x0, y0, x1, y1 = (
                    int(left),
                    0,
                    int(right),
                    int(min(bottom, page.size[1])),
                )
                if x0 > x1:
                    x0, x1 = x1, x0
                if y0 > y1:
                    y0, y1 = y1, y0
                if x1 <= x0 or y1 <= y0:
                    bottom -= page.size[1]
                    continue
                imgs.append(page.crop((x0, y0, x1, y1)))
                if 0 < ii < len(poss) - 1:
                    positions.append((pn + self.page_from, x0, x1, y0, y1))
                bottom -= page.size[1]

        if not imgs:
            return (None, None) if need_position else None

        height = sum(img.size[1] + GAP for img in imgs)
        width = int(np.max([i.size[0] for i in imgs]))
        pic = Image.new("RGB", (width, int(height)), (245, 245, 245))
        offset = 0
        for ii, img in enumerate(imgs):
            if ii == 0 or ii + 1 == len(imgs):
                img = img.convert("RGBA")
                overlay = Image.new("RGBA", img.size, (0, 0, 0, 0))
                overlay.putalpha(128)
                img = Image.alpha_composite(img, overlay).convert("RGB")
            pic.paste(img, (0, int(offset)))
            offset += img.size[1] + GAP

        return (pic, positions) if need_position else pic

    # ------------------------------------------------------------------
    # Sections / tables
    # ------------------------------------------------------------------
    def _describe_image(self, line_tag: str) -> str:
        """Best-effort VLM caption of the figure at ``line_tag`` using the tenant
        vision model (``self.vision_model``). Crops the figure from the locally
        rendered page and describes it. Returns "" on any failure so figure
        enrichment never breaks parsing (mirrors MinerU's image enhancement)."""
        try:
            img = self.crop("figure" + line_tag, ZM=1)
            if img is None:
                return ""
            from rag.app.picture import vision_llm_chunk
            from rag.prompts.generator import vision_llm_figure_describe_prompt

            # vision_llm_chunk expects a PIL Image (it calls img.size / img.save),
            # not raw bytes — pass the crop directly.
            desc = vision_llm_chunk(binary=img, vision_model=self.vision_model, prompt=vision_llm_figure_describe_prompt())
            return (desc or "").strip()
        except Exception:
            self.logger.info("[Mistral OCR] figure description skipped", exc_info=True)
            return ""

    def _transfer_to_sections(self, pages: list[dict], parse_method: Optional[str] = None) -> list[tuple]:
        """manual/pipeline (rag/flow DAG) want typed 3-tuples
        (text, layout_type, line_tag); every other caller (naive.py) wants
        2-tuples (text, line_tag) that naive_merge consumes directly."""
        typed = parse_method in {"manual", "pipeline"}
        sections: list[tuple] = []
        image_seq = 0
        for page in pages or []:
            page_idx = page.get("page_num", 0)
            page_size = page.get("page_size") or {}
            for block in page.get("blocks") or []:
                internal = self._resolve_internal_type(block.get("type"))
                if internal is None:
                    continue
                bbox = block.get("bbox")
                tag_input = {"page_idx": page_idx, "bbox": bbox, "page_size": page_size}
                if internal == "image":
                    if not bbox or len(bbox) != 4:
                        continue  # no geometry -> nothing to crop
                    line_tag = self._line_tag(tag_input)
                    image_seq += 1
                    caption = (block.get("content") or "").strip()
                    description = self._describe_image(line_tag) if self.vision_model is not None else ""
                    label = description or caption or f"image {image_seq}"
                    if typed:
                        sections.append((label, internal, line_tag))
                    else:
                        # chunk id is hash(content + doc_id): empty image text would
                        # collide across figures. Keep a unique caption; append the
                        # tag so crop() recovers the figure, remove_tag() strips it.
                        sections.append((f"{label}{line_tag}", ""))
                    continue
                text = self._block_text(block, internal)
                if not text:
                    continue
                line_tag = self._line_tag(tag_input)
                if typed:
                    sections.append((text, internal, line_tag))
                else:
                    sections.append((text, line_tag))
        return sections

    def _transfer_to_tables(self, pages: list[dict]) -> list:
        # Tables are inlined as HTML in section text; no separate extraction.
        return []

    # ------------------------------------------------------------------
    # HTTP client
    # ------------------------------------------------------------------
    def _headers(self) -> dict:
        return {"Authorization": f"Bearer {self.api_key}"}

    def check_installation(self) -> tuple[bool, str]:
        if not self.api_key:
            return False, "Mistral API key is not configured."
        return True, ""

    def _raise_for_status(self, resp, action: str) -> None:
        if resp.status_code != 200:
            raise RuntimeError(f"Mistral {action} failed: {resp.status_code} {resp.text[:300]}")

    def _ocr_payload(self, document: dict, pages: Optional[list[int]]) -> dict:
        payload = {
            "model": self.model,
            "document": document,
            "include_blocks": True,
            "table_format": self.table_format,
            "include_image_base64": False,
        }
        if pages:
            payload["pages"] = pages
        return payload

    def _call_ocr(self, pdf_bytes: bytes, filename: str, pages: Optional[list[int]], callback=None) -> dict:
        ok, reason = self.check_installation()
        if not ok:
            raise RuntimeError(reason)

        if len(pdf_bytes) <= self.inline_max_bytes:
            b64 = base64.b64encode(pdf_bytes).decode()
            document = {"type": "document_url", "document_url": f"data:application/pdf;base64,{b64}"}
            return self._post_ocr(self._ocr_payload(document, pages))

        # Large file: upload -> signed url -> ocr -> delete.
        file_id = None
        try:
            r = requests.post(f"{self.base_url}/files", headers=self._headers(), files={"file": (filename, pdf_bytes, "application/pdf")}, data={"purpose": "ocr"}, timeout=self.timeout)
            self._raise_for_status(r, "/files upload")
            file_id = r.json().get("id")
            r = requests.get(f"{self.base_url}/files/{file_id}/url", headers=self._headers(), params={"expiry": 24}, timeout=self.timeout)
            self._raise_for_status(r, "signed-url fetch")
            signed = r.json().get("url")
            document = {"type": "document_url", "document_url": signed}
            return self._post_ocr(self._ocr_payload(document, pages))
        finally:
            if file_id:
                try:
                    resp = requests.delete(f"{self.base_url}/files/{file_id}", headers=self._headers(), timeout=self.timeout)
                    if not 200 <= resp.status_code < 300:
                        self.logger.warning("failed to delete uploaded file %s: %s %s", file_id, resp.status_code, resp.text[:200])
                except Exception:
                    self.logger.warning("failed to delete uploaded file %s", file_id)

    def _post_ocr(self, payload: dict) -> dict:
        r = requests.post(f"{self.base_url}/ocr", headers=self._headers(), json=payload, timeout=self.timeout)
        self._raise_for_status(r, "OCR")
        return r.json()

    # ------------------------------------------------------------------
    # Public entry point
    # ------------------------------------------------------------------
    def parse_pdf(self, filepath: str | PathLike[str], binary=None, callback=None, parse_method: str = "raw", from_page: int = 0, to_page: int = MAXIMUM_PAGE_NUMBER, **kwargs) -> tuple[list, list]:
        # Optional tenant vision model for figure description (best-effort).
        self.vision_model = kwargs.pop("vision_model", None)

        # Load bytes.
        if binary is not None:
            pdf_bytes = binary.getvalue() if hasattr(binary, "getvalue") else bytes(binary)
        else:
            pdf_bytes = Path(filepath).read_bytes()

        self.outlines = extract_pdf_outlines(pdf_bytes)

        # Render the WHOLE document locally: _line_tag/crop index page_images in
        # absolute page order, so a sliced render would crop the wrong page.
        self.__images__(pdf_bytes, zoomin=1)

        # pages is a selector: include only when the caller restricted the range.
        pages: Optional[list[int]] = None
        if from_page > 0 or to_page < MAXIMUM_PAGE_NUMBER:
            # Bound the selector by the real page count, never by the sentinel
            # to_page (MAXIMUM_PAGE_NUMBER) — otherwise a failed render would
            # build a ~100k-element list. Fall back to the PDF page count.
            rendered = len(self.page_images) if getattr(self, "page_images", None) else 0
            total = rendered or self.total_page_number(Path(filepath).name, pdf_bytes)
            end = min(to_page, total) if total else from_page
            pages = list(range(from_page, end))
            if not pages:
                return [], []

        if callback:
            callback(0.15, "[Mistral OCR] submitting document")

        response = self._call_ocr(pdf_bytes, Path(filepath).name, pages, callback=callback)
        norm = self._normalize_pages(response)

        if callback:
            n_blocks = sum(len(p.get("blocks") or []) for p in norm)
            callback(0.75, f"[Mistral OCR] parsed {n_blocks} blocks across {len(norm)} pages")

        sections = self._transfer_to_sections(norm, parse_method)
        tables = self._transfer_to_tables(norm)
        return sections, tables
