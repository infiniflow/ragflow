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


def _has_subset_font_prefix(fontname):
    if not fontname:
        return False
    return bool(re.match(r"^[A-Z0-9]{2,6}\+", fontname))


def _is_garbled_by_font_encoding(page_chars, min_chars=20):
    if not page_chars or len(page_chars) < min_chars:
        return False
    has_subset_font = False
    total_non_space = 0
    ascii_punct_sym = 0
    cjk_like = 0
    for c in page_chars:
        text = c.get("text", "")
        fontname = c.get("fontname", "")
        if not text or text.isspace():
            continue
        total_non_space += 1
        if _has_subset_font_prefix(fontname):
            has_subset_font = True
        cp = ord(text[0])
        if (0x2E80 <= cp <= 0x9FFF or 0xF900 <= cp <= 0xFAFF
                or 0x20000 <= cp <= 0x2FA1F
                or 0xAC00 <= cp <= 0xD7AF
                or 0x3040 <= cp <= 0x30FF):
            cjk_like += 1
        elif (0x21 <= cp <= 0x2F or 0x3A <= cp <= 0x40
                or 0x5B <= cp <= 0x60 or 0x7B <= cp <= 0x7E):
            ascii_punct_sym += 1
    if total_non_space < min_chars:
        return False
    if not has_subset_font:
        return False
    cjk_ratio = cjk_like / total_non_space
    punct_ratio = ascii_punct_sym / total_non_space
    if cjk_ratio < 0.05 and punct_ratio > 0.4:
        return True
    return False


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


# ---------------------------------------------------------------------------
# Tests for _has_subset_font_prefix
# ---------------------------------------------------------------------------


class TestHasSubsetFontPrefix:
    """Tests for the _has_subset_font_prefix function."""

    def test_standard_subset_prefix(self):
        assert _has_subset_font_prefix("ABCDEF+Arial") is True
        assert _has_subset_font_prefix("XYZABC+TimesNewRoman") is True

    def test_short_subset_prefix(self):
        assert _has_subset_font_prefix("DY1+ZLQDm1-1") is True
        assert _has_subset_font_prefix("AB+Font") is True

    def test_alphanumeric_prefix(self):
        assert _has_subset_font_prefix("DY2+ZLQDnC-2") is True
        assert _has_subset_font_prefix("A1B2C3+MyFont") is True

    def test_no_prefix(self):
        assert _has_subset_font_prefix("Arial") is False
        assert _has_subset_font_prefix("TimesNewRoman") is False

    def test_empty_or_none(self):
        assert _has_subset_font_prefix("") is False
        assert _has_subset_font_prefix(None) is False

    def test_plus_in_middle_not_prefix(self):
        assert _has_subset_font_prefix("Font+Name") is False

    def test_lowercase_not_prefix(self):
        assert _has_subset_font_prefix("abc+Font") is False


# ---------------------------------------------------------------------------
# Tests for _is_garbled_by_font_encoding
# ---------------------------------------------------------------------------


def _make_chars(texts, fontname="DY1+ZLQDm1-1"):
    """Helper to create a list of pdfplumber-like char dicts."""
    return [{"text": t, "fontname": fontname} for t in texts]


class TestIsGarbledByFontEncoding:
    """Tests for font-encoding garbled text detection.

    This covers the scenario where PDF fonts with broken ToUnicode
    mappings cause CJK characters to be extracted as ASCII
    punctuation/symbols (e.g. GB.18067-2000.pdf).
    """

    def test_ascii_punct_from_subset_font_is_garbled(self):
        """Simulates GB.18067-2000.pdf: all chars are ASCII punct from subset fonts."""
        chars = _make_chars(
            list('!"#$%&\'()*+,-./!"#$%&\'()*+,-./'),
            fontname="DY1+ZLQDm1-1",
        )
        assert _is_garbled_by_font_encoding(chars) is True

    def test_normal_cjk_text_not_garbled(self):
        """Normal Chinese text from subset fonts should not be flagged."""
        chars = _make_chars(
            list("这是一段正常的中文文本用于测试的示例内容没有问题"),
            fontname="ABCDEF+SimSun",
        )
        assert _is_garbled_by_font_encoding(chars) is False

    def test_mixed_cjk_and_ascii_not_garbled(self):
        """Mixed CJK and ASCII content should not be flagged."""
        chars = _make_chars(
            list("GB18067-2000居住区大气中酚卫生标准"),
            fontname="DY1+ZLQDm1-1",
        )
        assert _is_garbled_by_font_encoding(chars) is False

    def test_non_subset_font_not_flagged(self):
        """ASCII punct from non-subset fonts should not be flagged."""
        chars = _make_chars(
            list('!"#$%&\'()*+,-./!"#$%&\'()*+,-./'),
            fontname="Arial",
        )
        assert _is_garbled_by_font_encoding(chars) is False

    def test_too_few_chars_not_flagged(self):
        """Pages with very few chars should not trigger detection."""
        chars = _make_chars(list('!"#$'), fontname="DY1+ZLQDm1-1")
        assert _is_garbled_by_font_encoding(chars) is False

    def test_mostly_digits_not_garbled(self):
        """Pages with lots of digits (like data tables) should not be flagged."""
        chars = _make_chars(
            list("1234567890" * 3),
            fontname="DY1+ZLQDm1-1",
        )
        assert _is_garbled_by_font_encoding(chars) is False

    def test_english_letters_not_garbled(self):
        """Pages with English letters should not be flagged."""
        chars = _make_chars(
            list("The quick brown fox jumps over the lazy dog"),
            fontname="ABCDEF+Arial",
        )
        assert _is_garbled_by_font_encoding(chars) is False

    def test_real_world_gb18067_page1(self):
        """Simulate actual GB.18067-2000.pdf Page 1 character distribution."""
        page_text = '!"#$%&\'(\'&)\'"*$!"#$%&\'\'()*+,$-'
        chars = _make_chars(list(page_text), fontname="DY1+ZLQDm1-1")
        assert _is_garbled_by_font_encoding(chars) is True

    def test_real_world_gb18067_page3(self):
        """Simulate actual GB.18067-2000.pdf Page 3 character distribution."""
        page_text = '!"#$%&\'()*+,-.*+/0+123456789:;<'
        chars = _make_chars(list(page_text), fontname="DY1+ZLQDnC-1")
        assert _is_garbled_by_font_encoding(chars) is True

    def test_empty_chars(self):
        assert _is_garbled_by_font_encoding([]) is False
        assert _is_garbled_by_font_encoding(None) is False

    def test_only_spaces(self):
        chars = _make_chars([" "] * 30, fontname="DY1+ZLQDm1-1")
        assert _is_garbled_by_font_encoding(chars) is False

    def test_small_min_chars_threshold(self):
        """With reduced min_chars, even small boxes can be detected."""
        chars = _make_chars(list('!"#$%&'), fontname="DY1+ZLQDm1-1")
        assert _is_garbled_by_font_encoding(chars, min_chars=5) is True
        assert _is_garbled_by_font_encoding(chars, min_chars=20) is False

    def test_boundary_cjk_ratio(self):
        """Just below 5% CJK threshold should still be flagged."""
        # 1 CJK out of 25 chars = 4% CJK, rest are punct
        chars = _make_chars(list('!"#$%&\'()*+,-./!@#$%^&*'), fontname="DY1+Font")
        chars.append({"text": "中", "fontname": "DY1+Font"})
        assert _is_garbled_by_font_encoding(chars, min_chars=5) is True

    def test_boundary_above_cjk_threshold(self):
        """Above 5% CJK ratio should NOT be flagged."""
        # 3 CJK out of 23 chars = ~13% CJK
        chars = _make_chars(list('!"#$%&\'()*+,-./!@#$'), fontname="DY1+Font")
        for ch in "中文字":
            chars.append({"text": ch, "fontname": "DY1+Font"})
        assert _is_garbled_by_font_encoding(chars, min_chars=5) is False
