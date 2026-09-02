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
"""Integration tests for MonkeyOCR composite selectors on the canvas flow parser.

Regression for PR #19044 review blockers: a UI selector
``<model>@<instance>@MonkeyOCR`` must normalize to the MonkeyOCR branch and
bind ``parser_model_name`` — not fall through to the VLM ``else`` path.

Full ``Parser._pdf`` import is environment-sensitive (heavy agent deps), so
this file pins the normalization contract and verifies the flow parser source
delegates to ``normalize_layout_recognizer``.
"""

from pathlib import Path

from common.parser_config_utils import normalize_layout_recognizer

REPO_ROOT = Path(__file__).resolve().parents[5]


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


def test_flow_parser_source_uses_shared_normalizer():
    source = (REPO_ROOT / "rag" / "flow" / "parser" / "parser.py").read_text()
    assert "from common.parser_config_utils import normalize_layout_recognizer" in source
    assert "normalize_layout_recognizer(raw_parse_method)" in source
    assert 'elif lowered.endswith("@mineru")' not in source.split("normalize_layout_recognizer(raw_parse_method)")[1][:800]


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
