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

"""Unit tests for PDF garbled text detection functions.

These tests directly exercise the garbled-character and garbled-text
detection logic that lives in RAGFlowPdfParser without importing the
full parser (which pulls in many heavy, optional dependencies).
"""

import re
import unicodedata

import pytest

# ---------------------------------------------------------------------------
# Reproduce the detection functions here so the tests can run in any
# environment without needing pdfplumber, xgboost, etc.
# The canonical implementations live in deepdoc/parser/pdf_parser.py.
# ---------------------------------------------------------------------------

_CID_PATTERN = re.compile(r"\(cid\s*:\s*\d+\s*\)")


def _is_garbled_char(ch):
    if not ch:
        return False
    cp = ord(ch)
    if 0xE000 <= cp <= 0xF8FF:
        return True
    if 0xF0000 <= cp <= 0xFFFFF:
        return True
    if 0x100000 <= cp <= 0x10FFFF:
        return True
    if cp == 0xFFFD:
        return True
    if cp < 0x20 and ch not in ('\t', '\n', '\r'):
        return True
    if 0x80 <= cp <= 0x9F:
        return True
    cat = unicodedata.category(ch)
    if cat in ("Cn", "Cs"):
        return True
    return False


def _is_garbled_text(text, threshold=0.5):
    if not text or not text.strip():
        return False
    if _CID_PATTERN.search(text):
        return True
    garbled_count = 0
    total = 0
    for ch in text:
        if ch.isspace():
            continue
        total += 1
        if _is_garbled_char(ch):
            garbled_count += 1
    if total == 0:
        return False
    return garbled_count / total >= threshold


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


class TestIsGarbledChar:
    """Tests for the _is_garbled_char function."""

    def test_normal_ascii_chars(self):
        for ch in "Hello World 123 !@#":
            assert _is_garbled_char(ch) is False

    def test_normal_chinese_chars(self):
        for ch in "中文测试你好世界":
            assert _is_garbled_char(ch) is False

    def test_normal_japanese_chars(self):
        for ch in "日本語テスト":
            assert _is_garbled_char(ch) is False

    def test_normal_korean_chars(self):
        for ch in "한국어테스트":
            assert _is_garbled_char(ch) is False

    def test_common_whitespace_not_garbled(self):
        assert _is_garbled_char('\t') is False
        assert _is_garbled_char('\n') is False
        assert _is_garbled_char('\r') is False
        assert _is_garbled_char(' ') is False

    def test_pua_chars_are_garbled(self):
        assert _is_garbled_char('\uE000') is True
        assert _is_garbled_char('\uF000') is True
        assert _is_garbled_char('\uF8FF') is True

    def test_supplementary_pua_a(self):
        assert _is_garbled_char(chr(0xF0000)) is True
        assert _is_garbled_char(chr(0xFFFFF)) is True

    def test_supplementary_pua_b(self):
        assert _is_garbled_char(chr(0x100000)) is True
        assert _is_garbled_char(chr(0x10FFFF)) is True

    def test_replacement_char(self):
        assert _is_garbled_char('\uFFFD') is True

    def test_c0_control_chars(self):
        assert _is_garbled_char('\x00') is True
        assert _is_garbled_char('\x01') is True
        assert _is_garbled_char('\x1F') is True

    def test_c1_control_chars(self):
        assert _is_garbled_char('\x80') is True
        assert _is_garbled_char('\x8F') is True
        assert _is_garbled_char('\x9F') is True

    def test_empty_string(self):
        assert _is_garbled_char('') is False

    def test_common_punctuation(self):
        for ch in ".,;:!?()[]{}\"'-/\\@#$%^&*+=<>~`|":
            assert _is_garbled_char(ch) is False

    def test_unicode_symbols(self):
        for ch in "©®™°±²³µ¶·¹º»¼½¾":
            assert _is_garbled_char(ch) is False


class TestIsGarbledText:
    """Tests for the _is_garbled_text function."""

    def test_normal_chinese_text(self):
        assert _is_garbled_text("这是一段正常的中文文本") is False

    def test_normal_english_text(self):
        assert _is_garbled_text("This is normal English text.") is False

    def test_mixed_normal_text(self):
        assert _is_garbled_text("Hello 你好 World 世界 123") is False

    def test_empty_text(self):
        assert _is_garbled_text("") is False
        assert _is_garbled_text("   ") is False

    def test_none_text(self):
        assert _is_garbled_text(None) is False

    def test_all_pua_chars(self):
        text = "\uE000\uE001\uE002\uE003\uE004"
        assert _is_garbled_text(text) is True

    def test_mostly_garbled(self):
        text = "\uE000\uE001\uE002好"
        assert _is_garbled_text(text, threshold=0.5) is True

    def test_few_garbled_below_threshold(self):
        text = "这是正常文本\uE000"
        assert _is_garbled_text(text, threshold=0.5) is False

    def test_cid_pattern_detected(self):
        assert _is_garbled_text("Hello (cid:123) World") is True
        assert _is_garbled_text("(cid : 45)") is True
        assert _is_garbled_text("(cid:0)") is True

    def test_cid_like_but_not_matching(self):
        assert _is_garbled_text("This is a valid cid reference") is False

    def test_whitespace_only_text(self):
        assert _is_garbled_text("   \t\n  ") is False

    def test_custom_threshold(self):
        text = "\uE000正常"
        assert _is_garbled_text(text, threshold=0.3) is True
        assert _is_garbled_text(text, threshold=0.5) is False

    def test_replacement_chars_in_text(self):
        text = "文档\uFFFD\uFFFD解析"
        assert _is_garbled_text(text, threshold=0.5) is False
        assert _is_garbled_text(text, threshold=0.3) is True

    def test_real_world_garbled_pattern(self):
        text = "\uE000\uE001\uE002\uE003\uE004\uE005\uE006\uE007"
        assert _is_garbled_text(text) is True

    def test_mixed_garbled_and_normal_at_boundary(self):
        text = "AB\uE000\uE001"
        assert _is_garbled_text(text, threshold=0.5) is True
        text2 = "ABC\uE000"
        assert _is_garbled_text(text2, threshold=0.5) is False
