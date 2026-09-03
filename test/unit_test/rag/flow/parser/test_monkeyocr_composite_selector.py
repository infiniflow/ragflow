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
"""Composite-selector normalization tests for MonkeyOCR on the canvas flow parser."""

from common.parser_config_utils import normalize_layout_recognizer


def test_flow_parser_normalizes_ui_monkeyocr_composite():
    selector = "ocr-model@default@MonkeyOCR"
    parse_method, parser_model_name = _normalize_like_flow_parser(selector)
    assert parse_method == "MonkeyOCR"
    assert parser_model_name == selector


def test_flow_parser_normalizes_four_segment_monkeyocr_composite():
    selector = "my-llm@my-instance@my-provider@monkeyocr"
    parse_method, parser_model_name = _normalize_like_flow_parser(selector)
    assert parse_method == "MonkeyOCR"
    assert parser_model_name == selector


def test_remove_toc_pdf_filter_keeps_items_without_page_number():
    items = [{"text": "orphan", "layout_type": "text"}, {"text": "body", "layout_type": "text", "page_number": 5}]
    toc_start_page = 1
    content_start_page = 5

    filtered = [
        item
        for item in items
        if item.get("page_number") is None or not (toc_start_page <= item["page_number"] < content_start_page)
    ]

    assert filtered == items


def _normalize_like_flow_parser(raw_parse_method: str) -> tuple[str, str | None]:
    """Mirror the normalization block in ``Parser._pdf`` after the review fix."""
    parser_model_name = None
    parse_method = raw_parse_method or ""
    if isinstance(raw_parse_method, str):
        normalized, model_name = normalize_layout_recognizer(raw_parse_method)
        if model_name:
            parser_model_name = model_name
            parse_method = normalized
    return parse_method, parser_model_name
