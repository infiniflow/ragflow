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

"""Per-PDF layout parser selection for mixed text/scanned corpora (issue #19886)."""

from __future__ import annotations

import logging
import re
from io import BytesIO
from typing import Any, Callable

from common.constants import MAXIMUM_PAGE_NUMBER

LAYOUT_RECOGNIZE_AUTO = "Auto"
DEFAULT_AUTO_TEXT_LAYOUT = "DeepDOC"
DEFAULT_AUTO_SCANNED_LAYOUT = "DeepDOC"
DEFAULT_AUTO_MIN_CHARS_PER_PAGE = 100

_PARSER_CONFIG_TEXT_KEY = "layout_recognize_auto_text"
_PARSER_CONFIG_SCANNED_KEY = "layout_recognize_auto_scanned"
_PARSER_CONFIG_MIN_CHARS_KEY = "layout_recognize_auto_min_chars_per_page"


def is_auto_layout_recognize(value: Any) -> bool:
    return isinstance(value, str) and value.strip().lower() == "auto"


def _coerce_page_range(from_page: int, to_page: int) -> tuple[int, int]:
    start = max(0, int(from_page or 0))
    end = max(start, int(to_page if to_page is not None else MAXIMUM_PAGE_NUMBER))
    return start, min(end, MAXIMUM_PAGE_NUMBER)


def average_non_whitespace_chars_per_page(
    filename: str,
    binary: bytes | None = None,
    *,
    from_page: int = 0,
    to_page: int = MAXIMUM_PAGE_NUMBER,
) -> float:
    """Return mean non-whitespace character count per page in the slice."""
    import pdfplumber

    start, end = _coerce_page_range(from_page, to_page)
    try:
        src = filename if binary is None else BytesIO(binary)
        with pdfplumber.open(src) as pdf:
            pages = pdf.pages[start:end]
            if not pages:
                return 0.0
            counts = []
            for page in pages:
                text = page.extract_text() or ""
                counts.append(sum(1 for ch in text if not ch.isspace()))
            return sum(counts) / len(counts)
    except Exception:
        logging.exception("[pdf_auto_layout] failed to probe PDF text layer for %r", filename)
        return 0.0


def pdf_has_usable_text_layer(
    filename: str,
    binary: bytes | None = None,
    *,
    from_page: int = 0,
    to_page: int = MAXIMUM_PAGE_NUMBER,
    min_chars_per_page: int = DEFAULT_AUTO_MIN_CHARS_PER_PAGE,
) -> bool:
    threshold = max(1, int(min_chars_per_page or DEFAULT_AUTO_MIN_CHARS_PER_PAGE))
    return average_non_whitespace_chars_per_page(
        filename,
        binary,
        from_page=from_page,
        to_page=to_page,
    ) >= threshold


def _auto_sub_parsers(parser_config: dict | None) -> tuple[str, str, int]:
    cfg = parser_config if isinstance(parser_config, dict) else {}
    text = cfg.get(_PARSER_CONFIG_TEXT_KEY) or DEFAULT_AUTO_TEXT_LAYOUT
    scanned = cfg.get(_PARSER_CONFIG_SCANNED_KEY) or DEFAULT_AUTO_SCANNED_LAYOUT
    try:
        min_chars = int(cfg.get(_PARSER_CONFIG_MIN_CHARS_KEY, DEFAULT_AUTO_MIN_CHARS_PER_PAGE))
    except (TypeError, ValueError):
        min_chars = DEFAULT_AUTO_MIN_CHARS_PER_PAGE
    return str(text), str(scanned), max(1, min_chars)


def resolve_auto_layout_recognize(
    parser_config: dict | None,
    *,
    filename: str,
    binary: bytes | None,
    from_page: int = 0,
    to_page: int = MAXIMUM_PAGE_NUMBER,
    callback: Callable[[float, str], Any] | None = None,
) -> str:
    """Pick text or scanned layout_recognize for one PDF."""
    text_parser, scanned_parser, min_chars = _auto_sub_parsers(parser_config)
    has_text = pdf_has_usable_text_layer(
        filename,
        binary,
        from_page=from_page,
        to_page=to_page,
        min_chars_per_page=min_chars,
    )
    chosen = text_parser if has_text else scanned_parser
    avg = average_non_whitespace_chars_per_page(
        filename,
        binary,
        from_page=from_page,
        to_page=to_page,
    )
    msg = (
        f"Auto PDF parser: avg {avg:.0f} chars/page (threshold {min_chars}) → "
        f"{'text' if has_text else 'scanned'} parser ({chosen})."
    )
    logging.info("[pdf_auto_layout] %s", msg)
    if callback:
        callback(0.08, msg)
    return chosen


def resolve_pdf_layout_recognize_raw(
    layout_recognize_raw: str,
    parser_config: dict | None,
    *,
    filename: str,
    binary: bytes | None,
    from_page: int = 0,
    to_page: int = MAXIMUM_PAGE_NUMBER,
    callback: Callable[[float, str], Any] | None = None,
    resolve_tenant_model_id: Callable[[str], str] | None = None,
) -> str:
    """If layout is Auto, probe the PDF and return the effective parser name."""
    if is_auto_layout_recognize(layout_recognize_raw):
        cfg = dict(parser_config) if isinstance(parser_config, dict) else {}
        if resolve_tenant_model_id:
            for key in (_PARSER_CONFIG_TEXT_KEY, _PARSER_CONFIG_SCANNED_KEY):
                val = cfg.get(key)
                if isinstance(val, str) and val.strip():
                    cfg[key] = resolve_tenant_model_id(val)
        return resolve_auto_layout_recognize(
            cfg,
            filename=filename,
            binary=binary,
            from_page=from_page,
            to_page=to_page,
            callback=callback,
        )
    if resolve_tenant_model_id and isinstance(layout_recognize_raw, str):
        return resolve_tenant_model_id(layout_recognize_raw)
    return layout_recognize_raw


def resolve_chunk_pdf_layout_recognize(
    parser_config: dict | None,
    layout_recognize_raw: str,
    *,
    filename: str,
    binary: bytes | None,
    from_page: int = 0,
    to_page: int = MAXIMUM_PAGE_NUMBER,
    callback: Callable[[float, str], Any] | None = None,
    tenant_id: str | None = None,
    model_id_resolver: Callable[[str], str] | None = None,
) -> str:
    """Resolve dataset chunk-method layout_recognize, including Auto per PDF."""

    def _resolve(raw: str) -> str:
        if not tenant_id or not model_id_resolver:
            return raw
        try:
            return model_id_resolver(raw)
        except LookupError:
            return raw

    resolver = _resolve if tenant_id and model_id_resolver else None
    return resolve_pdf_layout_recognize_raw(
        layout_recognize_raw,
        parser_config,
        filename=filename,
        binary=binary,
        from_page=from_page,
        to_page=to_page,
        callback=callback,
        resolve_tenant_model_id=resolver,
    )


def resolve_pipeline_auto_parse_method(
    conf: dict,
    blob: bytes,
    *,
    callback: Callable[[float, str], Any] | None = None,
) -> str:
    """Resolve pipeline PDF ``parse_method`` when set to auto."""
    text_method = conf.get(_PARSER_CONFIG_TEXT_KEY) or DEFAULT_AUTO_TEXT_LAYOUT
    scanned_method = conf.get(_PARSER_CONFIG_SCANNED_KEY) or DEFAULT_AUTO_SCANNED_LAYOUT
    try:
        min_chars = int(conf.get(_PARSER_CONFIG_MIN_CHARS_KEY, DEFAULT_AUTO_MIN_CHARS_PER_PAGE))
    except (TypeError, ValueError):
        min_chars = DEFAULT_AUTO_MIN_CHARS_PER_PAGE

    parser_config = {
        _PARSER_CONFIG_TEXT_KEY: text_method,
        _PARSER_CONFIG_SCANNED_KEY: scanned_method,
        _PARSER_CONFIG_MIN_CHARS_KEY: min_chars,
    }
    chosen = resolve_auto_layout_recognize(
        parser_config,
        filename="document.pdf",
        binary=blob,
        callback=callback,
    )
    return _layout_to_pipeline_parse_method(chosen)


def _layout_to_pipeline_parse_method(layout_value: str) -> str:
    """Map dataset-style layout_recognize values to pipeline parse_method tokens."""
    if not isinstance(layout_value, str):
        return str(layout_value)
    lowered = layout_value.strip().lower()
    if lowered.endswith("@mineru") or lowered == "mineru":
        return layout_value if "@" in layout_value else "MinerU"
    if lowered.endswith("@paddleocr") or lowered == "paddleocr":
        return layout_value if "@" in layout_value else "PaddleOCR"
    if lowered.endswith("@somark") or lowered == "somark":
        return layout_value if "@" in layout_value else "SoMark"
    if lowered.endswith("@mistral ocr") or lowered == "mistral ocr":
        return layout_value if "@" in layout_value else "Mistral OCR"
    if lowered.endswith("@opendataloader") or lowered == "opendataloader":
        return layout_value if "@" in layout_value else "OpenDataLoader"
    if lowered.endswith("@monkeyocrv2") or lowered == "monkeyocrv2":
        return layout_value if "@" in layout_value else "MonkeyOCRv2"

    aliases = {
        "deepdoc": "deepdoc",
        "plain text": "plain_text",
        "plaintext": "plain_text",
        "docling": "docling",
        "tcadp parser": "tcadp parser",
        "opendataloader": "opendataloader",
    }
    if lowered in aliases:
        return aliases[lowered]
    # Vision / composite model names pass through unchanged.
    if re.search(r"@.+@", layout_value):
        return layout_value
    return layout_value
