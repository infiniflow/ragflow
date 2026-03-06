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

"""Unit tests for PDF garbled text detection and layout garbage filtering.

Tests cover:
- garbled_detection module: is_garbled_char, is_garbled_text,
  has_subset_font_prefix, is_garbled_by_font_encoding
- layout_recognizer.__is_garbage: CID pattern filtering
"""

import re
import sys
import os
import importlib.util

import pytest

# Import the garbled_detection module directly by file path to avoid
# triggering deepdoc/parser/__init__.py which pulls in heavy dependencies
# (pdfplumber, xgboost, etc.) that may not be available in test environments.
_MODULE_PATH = os.path.abspath(
    os.path.join(os.path.dirname(__file__), "..", "..", "..", "deepdoc", "parser", "garbled_detection.py")
)
_spec = importlib.util.spec_from_file_location("garbled_detection", _MODULE_PATH)
_mod = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_mod)

is_garbled_char = _mod.is_garbled_char
is_garbled_text = _mod.is_garbled_text
has_subset_font_prefix = _mod.has_subset_font_prefix
is_garbled_by_font_encoding = _mod.is_garbled_by_font_encoding


# ---------------------------------------------------------------------------
# Tests for is_garbled_char
# ---------------------------------------------------------------------------


class TestIsGarbledChar:
    """Tests for the is_garbled_char function."""

    def test_normal_ascii_chars(self):
        for ch in "Hello World 123 !@#":
            assert is_garbled_char(ch) is False

    def test_normal_chinese_chars(self):
        for ch in "中文测试你好世界":
            assert is_garbled_char(ch) is False

    def test_normal_japanese_chars(self):
        for ch in "日本語テスト":
            assert is_garbled_char(ch) is False

    def test_normal_korean_chars(self):
        for ch in "한국어테스트":
            assert is_garbled_char(ch) is False

    def test_common_whitespace_not_garbled(self):
        assert is_garbled_char('\t') is False
        assert is_garbled_char('\n') is False
        assert is_garbled_char('\r') is False
        assert is_garbled_char(' ') is False

    def test_pua_chars_are_garbled(self):
        assert is_garbled_char('\uE000') is True
        assert is_garbled_char('\uF000') is True
        assert is_garbled_char('\uF8FF') is True

    def test_supplementary_pua_a(self):
        assert is_garbled_char(chr(0xF0000)) is True
        assert is_garbled_char(chr(0xFFFFF)) is True

    def test_supplementary_pua_b(self):
        assert is_garbled_char(chr(0x100000)) is True
        assert is_garbled_char(chr(0x10FFFF)) is True

    def test_replacement_char(self):
        assert is_garbled_char('\uFFFD') is True

    def test_c0_control_chars(self):
        assert is_garbled_char('\x00') is True
        assert is_garbled_char('\x01') is True
        assert is_garbled_char('\x1F') is True

    def test_c1_control_chars(self):
        assert is_garbled_char('\x80') is True
        assert is_garbled_char('\x8F') is True
        assert is_garbled_char('\x9F') is True

    def test_empty_string(self):
        assert is_garbled_char('') is False

    def test_common_punctuation(self):
        for ch in ".,;:!?()[]{}\"'-/\\@#$%^&*+=<>~`|":
            assert is_garbled_char(ch) is False

    def test_unicode_symbols(self):
        for ch in "©®™°±²³µ¶·¹º»¼½¾":
            assert is_garbled_char(ch) is False


# ---------------------------------------------------------------------------
# Tests for is_garbled_text
# ---------------------------------------------------------------------------


class TestIsGarbledText:
    """Tests for the is_garbled_text function."""

    def test_normal_chinese_text(self):
        assert is_garbled_text("这是一段正常的中文文本") is False

    def test_normal_english_text(self):
        assert is_garbled_text("This is normal English text.") is False

    def test_mixed_normal_text(self):
        assert is_garbled_text("Hello 你好 World 世界 123") is False

    def test_empty_text(self):
        assert is_garbled_text("") is False
        assert is_garbled_text("   ") is False

    def test_none_text(self):
        assert is_garbled_text(None) is False

    def test_all_pua_chars(self):
        text = "\uE000\uE001\uE002\uE003\uE004"
        assert is_garbled_text(text) is True

    def test_mostly_garbled(self):
        text = "\uE000\uE001\uE002好"
        assert is_garbled_text(text, threshold=0.5) is True

    def test_few_garbled_below_threshold(self):
        text = "这是正常文本\uE000"
        assert is_garbled_text(text, threshold=0.5) is False

    def test_cid_pattern_detected(self):
        assert is_garbled_text("Hello (cid:123) World") is True
        assert is_garbled_text("(cid : 45)") is True
        assert is_garbled_text("(cid:0)") is True

    def test_cid_like_but_not_matching(self):
        assert is_garbled_text("This is a valid cid reference") is False

    def test_whitespace_only_text(self):
        assert is_garbled_text("   \t\n  ") is False

    def test_custom_threshold(self):
        text = "\uE000正常"
        assert is_garbled_text(text, threshold=0.3) is True
        assert is_garbled_text(text, threshold=0.5) is False

    def test_replacement_chars_in_text(self):
        text = "文档\uFFFD\uFFFD解析"
        assert is_garbled_text(text, threshold=0.5) is False
        assert is_garbled_text(text, threshold=0.3) is True

    def test_real_world_garbled_pattern(self):
        text = "\uE000\uE001\uE002\uE003\uE004\uE005\uE006\uE007"
        assert is_garbled_text(text) is True

    def test_mixed_garbled_and_normal_at_boundary(self):
        text = "AB\uE000\uE001"
        assert is_garbled_text(text, threshold=0.5) is True
        text2 = "ABC\uE000"
        assert is_garbled_text(text2, threshold=0.5) is False


# ---------------------------------------------------------------------------
# Tests for has_subset_font_prefix
# ---------------------------------------------------------------------------


class TestHasSubsetFontPrefix:
    """Tests for the has_subset_font_prefix function."""

    def test_standard_subset_prefix(self):
        assert has_subset_font_prefix("ABCDEF+Arial") is True
        assert has_subset_font_prefix("XYZABC+TimesNewRoman") is True

    def test_short_subset_prefix(self):
        assert has_subset_font_prefix("DY1+ZLQDm1-1") is True
        assert has_subset_font_prefix("AB+Font") is True

    def test_alphanumeric_prefix(self):
        assert has_subset_font_prefix("DY2+ZLQDnC-2") is True
        assert has_subset_font_prefix("A1B2C3+MyFont") is True

    def test_no_prefix(self):
        assert has_subset_font_prefix("Arial") is False
        assert has_subset_font_prefix("TimesNewRoman") is False

    def test_empty_or_none(self):
        assert has_subset_font_prefix("") is False
        assert has_subset_font_prefix(None) is False

    def test_plus_in_middle_not_prefix(self):
        assert has_subset_font_prefix("Font+Name") is False

    def test_lowercase_not_prefix(self):
        assert has_subset_font_prefix("abc+Font") is False


# ---------------------------------------------------------------------------
# Tests for is_garbled_by_font_encoding
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
            list('!"#$%&\'(\'&)\'"*$!"#$%&\'\'()*+,$-'),
            fontname="DY1+ZLQDm1-1",
        )
        assert is_garbled_by_font_encoding(chars) is True

    def test_normal_cjk_text_not_garbled(self):
        """Normal Chinese text from subset fonts should not be flagged."""
        chars = _make_chars(
            list("这是一段正常的中文文本用于测试的示例内容没有问题"),
            fontname="ABCDEF+SimSun",
        )
        assert is_garbled_by_font_encoding(chars) is False

    def test_mixed_cjk_and_ascii_not_garbled(self):
        """Mixed CJK and ASCII content should not be flagged."""
        chars = _make_chars(
            list("GB18067-2000居住区大气中酚卫生标准"),
            fontname="DY1+ZLQDm1-1",
        )
        assert is_garbled_by_font_encoding(chars) is False

    def test_non_subset_font_not_flagged(self):
        """ASCII punct from non-subset fonts should not be flagged."""
        chars = _make_chars(
            list('!"#$%&\'()*+,-./!"#$%&\'()*+,-./'),
            fontname="Arial",
        )
        assert is_garbled_by_font_encoding(chars) is False

    def test_too_few_chars_not_flagged(self):
        """Pages with very few chars should not trigger detection."""
        chars = _make_chars(list('!"#$'), fontname="DY1+ZLQDm1-1")
        assert is_garbled_by_font_encoding(chars) is False

    def test_mostly_digits_not_garbled(self):
        """Pages with lots of digits (like data tables) should not be flagged."""
        chars = _make_chars(
            list("1234567890" * 3),
            fontname="DY1+ZLQDm1-1",
        )
        assert is_garbled_by_font_encoding(chars) is False

    def test_english_letters_not_garbled(self):
        """Pages with English letters should not be flagged."""
        chars = _make_chars(
            list("The quick brown fox jumps over the lazy dog"),
            fontname="ABCDEF+Arial",
        )
        assert is_garbled_by_font_encoding(chars) is False

    def test_real_world_gb18067_page1(self):
        """Simulate actual GB.18067-2000.pdf Page 1 character distribution."""
        page_text = '!"#$%&\'(\'&)\'"*$!"#$%&\'\'()*+,$-'
        chars = _make_chars(list(page_text), fontname="DY1+ZLQDm1-1")
        assert is_garbled_by_font_encoding(chars) is True

    def test_real_world_gb18067_page3(self):
        """Simulate actual GB.18067-2000.pdf Page 3 character distribution."""
        page_text = '!"#$%&\'()*+,-.*+/0+123456789:;<'
        chars = _make_chars(list(page_text), fontname="DY1+ZLQDnC-1")
        assert is_garbled_by_font_encoding(chars) is True

    def test_empty_chars(self):
        assert is_garbled_by_font_encoding([]) is False
        assert is_garbled_by_font_encoding(None) is False

    def test_only_spaces(self):
        chars = _make_chars([" "] * 30, fontname="DY1+ZLQDm1-1")
        assert is_garbled_by_font_encoding(chars) is False

    def test_small_min_chars_threshold(self):
        """With reduced min_chars, even small boxes can be detected."""
        chars = _make_chars(list('!"#$%&'), fontname="DY1+ZLQDm1-1")
        assert is_garbled_by_font_encoding(chars, min_chars=5) is True
        assert is_garbled_by_font_encoding(chars, min_chars=20) is False

    def test_boundary_cjk_ratio(self):
        """Just below 5% CJK threshold should still be flagged."""
        # 1 CJK out of 25 chars = 4% CJK, rest are punct from subset font
        chars = _make_chars(list('!"#$%&\'()*+,-./!@#$%^&*'), fontname="DY1+Font")
        chars.append({"text": "中", "fontname": "DY1+Font"})
        assert is_garbled_by_font_encoding(chars, min_chars=5) is True

    def test_boundary_above_cjk_threshold(self):
        """Above 5% CJK ratio should NOT be flagged."""
        # 3 CJK out of 23 chars = ~13% CJK
        chars = _make_chars(list('!"#$%&\'()*+,-./!@#$'), fontname="DY1+Font")
        for ch in "中文字":
            chars.append({"text": ch, "fontname": "DY1+Font"})
        assert is_garbled_by_font_encoding(chars, min_chars=5) is False

    def test_low_subset_ratio_not_flagged(self):
        """When only a few chars come from subset fonts, should not be flagged.

        Addresses reviewer feedback: a single subset font should not cause
        the entire page to be flagged as garbled.
        """
        # 5 chars from subset font, 20 from normal font -> 20% subset ratio < 30%
        chars = _make_chars(list('!"#$%'), fontname="DY1+Font")
        chars.extend(_make_chars(list('!"#$%&\'()*+,-./!@#$%'), fontname="Arial"))
        assert is_garbled_by_font_encoding(chars, min_chars=5) is False

    def test_high_subset_ratio_flagged(self):
        """When most chars come from subset fonts, detection should trigger."""
        # All 30 chars from subset font with punct -> garbled
        chars = _make_chars(
            list('!"#$%&\'()*+,-./!@#$%^&*()[]{}'),
            fontname="BCDGEE+R0015",
        )
        assert is_garbled_by_font_encoding(chars) is True


# ---------------------------------------------------------------------------
# Tests for layout_recognizer.__is_garbage
# ---------------------------------------------------------------------------


def _is_garbage(b):
    """Reproduce LayoutRecognizer.__is_garbage for unit testing.

    The original is a closure nested inside LayoutRecognizer.__call__
    (deepdoc/vision/layout_recognizer.py). We replicate it here because
    it cannot be directly imported.
    """
    patt = [r"\(cid\s*:\s*\d+\s*\)"]
    return any([re.search(p, b.get("text", "")) for p in patt])


class TestLayoutRecognizerIsGarbage:
    """Tests for the layout_recognizer __is_garbage function.

    This function filters out text boxes containing CID patterns like
    (cid:123) which indicate unmapped characters in PDF fonts.
    """

    def test_cid_pattern_simple(self):
        assert _is_garbage({"text": "(cid:123)"}) is True

    def test_cid_pattern_with_spaces(self):
        assert _is_garbage({"text": "(cid : 45)"}) is True
        assert _is_garbage({"text": "(cid :  0)"}) is True

    def test_cid_pattern_embedded_in_text(self):
        assert _is_garbage({"text": "Hello (cid:99) World"}) is True

    def test_cid_pattern_multiple(self):
        assert _is_garbage({"text": "(cid:1)(cid:2)(cid:3)"}) is True

    def test_normal_text_not_garbage(self):
        assert _is_garbage({"text": "This is normal text."}) is False

    def test_chinese_text_not_garbage(self):
        assert _is_garbage({"text": "这是正常的中文内容"}) is False

    def test_empty_text_not_garbage(self):
        assert _is_garbage({"text": ""}) is False

    def test_missing_text_key_not_garbage(self):
        assert _is_garbage({}) is False

    def test_parentheses_without_cid_not_garbage(self):
        assert _is_garbage({"text": "(hello:123)"}) is False
        assert _is_garbage({"text": "cid:123"}) is False

    def test_partial_cid_not_garbage(self):
        assert _is_garbage({"text": "(cid:)"}) is False
        assert _is_garbage({"text": "(cid)"}) is False

    def test_cid_with_zero(self):
        assert _is_garbage({"text": "(cid:0)"}) is True

    def test_cid_with_large_number(self):
        assert _is_garbage({"text": "(cid:99999)"}) is True
