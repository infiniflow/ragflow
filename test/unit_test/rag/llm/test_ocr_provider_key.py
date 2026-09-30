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
import json
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[4]


def _parse_ocr_provider_key(key: str | dict | None) -> dict:
    """Load the helper without importing MinerU/MonkeyOCR parser stacks."""
    source = (REPO_ROOT / "rag" / "llm" / "ocr_model.py").read_text()
    start = source.index("def _parse_ocr_provider_key")
    end = source.index("\nclass Base:")
    ns: dict = {"json": json}
    exec(source[start:end], ns)
    return ns["_parse_ocr_provider_key"](key)


def test_parse_ocr_provider_key_preserves_dict():
    payload = {"monkeyocr_apiserver": "http://adapter:9000"}
    assert _parse_ocr_provider_key(payload) == payload


def test_parse_ocr_provider_key_unwraps_nested_api_key():
    inner = {"MINERU_APISERVER": "http://mineru:8000"}
    assert _parse_ocr_provider_key({"api_key": inner}) == inner


def test_parse_ocr_provider_key_parses_json_string():
    payload = {"MONKEYOCR_APISERVER": "http://adapter:9000"}
    assert _parse_ocr_provider_key(json.dumps(payload)) == payload


def test_parse_ocr_provider_key_invalid_string_is_empty():
    assert _parse_ocr_provider_key("not-json") == {}
    assert _parse_ocr_provider_key("") == {}
    assert _parse_ocr_provider_key(None) == {}
