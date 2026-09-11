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
from pathlib import Path

from common.parser_config_utils import normalize_layout_recognizer

REPO_ROOT = Path(__file__).resolve().parents[3]


def test_normalize_layout_recognizer_monkeyocr_suffix():
    assert normalize_layout_recognizer("my-llm@my-instance@my-provider@monkeyocr") == (
        "MonkeyOCR",
        "my-llm@my-instance@my-provider@monkeyocr",
    )


def test_normalize_layout_recognizer_monkeyocr_ui_composite():
    """Canvas/UI selectors are ``<model>@<instance>@MonkeyOCR`` (PR #19044)."""
    selector = "monkeyocr-local@default@MonkeyOCR"
    assert normalize_layout_recognizer(selector) == ("MonkeyOCR", selector)


def test_flow_parser_uses_shared_layout_normalizer():
    source = (REPO_ROOT / "rag" / "flow" / "parser" / "parser.py").read_text()
    assert "from common.parser_config_utils import normalize_layout_recognizer" in source
    assert "normalize_layout_recognizer(raw_parse_method)" in source


def test_builtin_parsers_forward_monkeyocr_llm_name():
    files = (
        "rag/app/book.py",
        "rag/app/laws.py",
        "rag/app/manual.py",
        "rag/app/one.py",
        "rag/app/presentation.py",
        "rag/app/paper.py",
        "rag/app/naive.py",
    )
    for rel in files:
        text = (REPO_ROOT / rel).read_text()
        assert "monkeyocr_llm_name=parser_model_name" in text, rel
