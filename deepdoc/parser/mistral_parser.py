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

import logging
import os
import re
from typing import Optional

from deepdoc.parser.pdf_parser import RAGFlowPdfParser

# RAGFlow internal layout types the rest of the pipeline understands.
_KNOWN_INTERNAL_TYPES = {"text", "image", "table", "equation", "code"}

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
                bbox = [
                    b.get("top_left_x", 0),
                    b.get("top_left_y", 0),
                    b.get("bottom_right_x", 0),
                    b.get("bottom_right_y", 0),
                ]
                blocks.append({
                    "type": b.get("type"),
                    "content": b.get("content"),
                    "bbox": bbox,
                    "table_id": b.get("table_id"),
                })
            out.append({
                "page_num": page.get("index", 0),
                "page_size": page_size,
                "blocks": blocks,
            })
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

    # ------------------------------------------------------------------
    # Sections / tables
    # ------------------------------------------------------------------
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
                    label = caption or f"image {image_seq}"
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
