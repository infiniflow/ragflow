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
