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

"""Lightweight garbled-text detection utilities for PDF parsing.

This module contains detection functions for identifying garbled text
extracted from PDFs with broken font encodings or unmapped CID characters.
It intentionally has no heavy dependencies (only ``re`` and ``unicodedata``)
so that it can be imported from both the main parser and unit tests without
pulling in pdfplumber, xgboost, etc.
"""

import re
import unicodedata

# CID pattern regex for unmapped font characters from pdfminer
CID_PATTERN = re.compile(r"\(cid\s*:\s*\d+\s*\)")


def is_garbled_char(ch):
    """Check if a single character is garbled (unmappable from PDF font encoding).

    A character is considered garbled if it falls into Unicode Private Use Areas
    or certain replacement/control character ranges that typically indicate
    pdfminer failed to map a CID to a valid Unicode codepoint.

    Args:
        ch: A single character string.

    Returns:
        True if the character appears to be garbled.
    """
    if not ch:
        return False
    cp = ord(ch)
    # Unicode Private Use Areas (PUA) — commonly used when ToUnicode
    # mapping is missing
    if 0xE000 <= cp <= 0xF8FF:
        return True
    # Supplementary Private Use Area-A
    if 0xF0000 <= cp <= 0xFFFFF:
        return True
    # Supplementary Private Use Area-B
    if 0x100000 <= cp <= 0x10FFFF:
        return True
    # Unicode replacement character
    if cp == 0xFFFD:
        return True
    # C0/C1 control characters (except common whitespace)
    if cp < 0x20 and ch not in ('\t', '\n', '\r'):
        return True
    if 0x80 <= cp <= 0x9F:
        return True
    # Check for Unicode category "unassigned" (Cn) or "surrogate" (Cs)
    cat = unicodedata.category(ch)
    if cat in ("Cn", "Cs"):
        return True
    return False


def is_garbled_text(text, threshold=0.5):
    """Check if a text string contains too many garbled characters.

    Examines each character in the text and determines if the overall
    proportion of garbled characters exceeds the given threshold.
    Also detects pdfminer's CID placeholder patterns like '(cid:123)'.

    Args:
        text: The text string to check.
        threshold: The ratio of garbled characters above which the text
            is considered garbled. Defaults to 0.5.

    Returns:
        True if the text is considered garbled.
    """
    if not text or not text.strip():
        return False
    # Check for CID patterns from pdfminer
    if CID_PATTERN.search(text):
        return True
    garbled_count = 0
    total = 0
    for ch in text:
        if ch.isspace():
            continue
        total += 1
        if is_garbled_char(ch):
            garbled_count += 1
    if total == 0:
        return False
    return garbled_count / total >= threshold


def has_subset_font_prefix(fontname):
    """Check if a font name has a subset prefix (e.g. 'DY1+ZLQDm1-1').

    PDF subset fonts use a 6-letter uppercase tag followed by '+' before
    the actual font name. Some tools use shorter tags (e.g. 'DY1+').

    Args:
        fontname: The font name string to check.

    Returns:
        True if the font name has a valid subset prefix.
    """
    if not fontname:
        return False
    return bool(re.match(r"^[A-Z0-9]{2,6}\+", fontname))


def is_garbled_by_font_encoding(page_chars, min_chars=20):
    """Detect garbled text caused by broken font encoding mappings.

    Some PDFs (especially older Chinese standards) embed custom fonts that
    map CJK glyphs to ASCII codepoints. The extracted text appears as
    random ASCII punctuation/symbols instead of actual CJK characters.

    Detection strategy: if a significant proportion of characters come from
    subset-embedded fonts and the page produces overwhelmingly ASCII
    (punctuation, digits, symbols) with virtually no CJK/Hangul/Kana
    characters, the page is likely garbled due to broken font encoding.

    Args:
        page_chars: List of pdfplumber character dicts with 'text' and
            'fontname' keys.
        min_chars: Minimum number of non-space chars required to trigger
            detection. Pages with fewer chars are not flagged.

    Returns:
        True if the page appears to have font-encoding garbled text.
    """
    if not page_chars or len(page_chars) < min_chars:
        return False

    subset_font_count = 0
    total_non_space = 0
    ascii_punct_sym = 0
    cjk_like = 0

    for c in page_chars:
        text = c.get("text", "")
        fontname = c.get("fontname", "")
        if not text or text.isspace():
            continue
        total_non_space += 1

        if has_subset_font_prefix(fontname):
            subset_font_count += 1

        cp = ord(text[0])
        # Count CJK Unified Ideographs + Extensions, Hangul, Kana
        if (0x2E80 <= cp <= 0x9FFF or 0xF900 <= cp <= 0xFAFF
                or 0x20000 <= cp <= 0x2FA1F
                or 0xAC00 <= cp <= 0xD7AF
                or 0x3040 <= cp <= 0x30FF):
            cjk_like += 1
        # Count ASCII punctuation and symbols (0x21-0x2F, 0x3A-0x40,
        # 0x5B-0x60, 0x7B-0x7E)
        elif (0x21 <= cp <= 0x2F or 0x3A <= cp <= 0x40
                or 0x5B <= cp <= 0x60 or 0x7B <= cp <= 0x7E):
            ascii_punct_sym += 1

    if total_non_space < min_chars:
        return False

    # Require that a significant proportion of characters come from
    # subset fonts (>= 30%), not just a single one.
    subset_ratio = subset_font_count / total_non_space
    if subset_ratio < 0.3:
        return False

    # If there are essentially no CJK characters and the majority of
    # characters are ASCII punctuation/symbols, this is likely garbled
    # text from broken font encoding.
    cjk_ratio = cjk_like / total_non_space
    punct_ratio = ascii_punct_sym / total_non_space
    if cjk_ratio < 0.05 and punct_ratio > 0.4:
        return True

    return False
