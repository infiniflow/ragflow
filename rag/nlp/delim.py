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

"""Canonical parser for the ``parser_config.delimiter`` field.

Background
----------
The single string field ``parser_config.delimiter`` is consumed by several
parser implementations depending only on the file extension. Before this
module existed, six implementations diverged on:

  * whether bare (non-backtick) characters are honored
  * dedupe behavior
  * sort order
  * CRLF / CR normalization
  * whether ``re.I`` is applied
  * the ``re.escape`` round-trip dance in ``txt_parser``

This module owns the canonical parsing rule. All six implementations now
call :func:`parse_delimiter_field` and :func:`compile_delimiter_pattern`.

Parsing rule
------------
A "delimiter field" is a string with the following grammar::

    delimiter_field := token*
    token           := backtick_wrapped | bare_char
    backtick_wrapped := "`" bare_char+ "`"
    bare_char        := any single Unicode character except "`"

Semantics:

  1. Any character(s) between matching backticks is one multi-character
     delimiter.
  2. Any character outside backticks is its own single-character
     delimiter.
  3. The two are combined, deduplicated, and sorted longest-first so
     ``##`` matches before ``#``.
  4. ``\\r\\n`` and standalone ``\\r`` are normalized to ``\\n`` at the
     top of :func:`parse_delimiter_field` so Windows-line-ending
     documents produce identical splits to Unix-line-ending ones.
  5. No ``re.I`` is used. Delimiter matching is case-sensitive.

Returns
-------
:func:`parse_delimiter_field` returns a ``list[str]`` of raw delimiter
strings (sorted longest-first, deduplicated, CRLF-normalized).
:func:`compile_delimiter_pattern` takes that list and returns a regex
alternation pattern with ``re.escape`` applied, ready for
``re.split(r"(%s)" % pattern, ...)``.

Frontend parity
---------------
The web UI preview in ``web/src/utils/delimiter-preview.ts``
(``parseDelimitersForDisplay``) follows the same parsing rule
(normalization, dedupe, longest-first order) and applies whitespace
glyph substitution only for display.
"""

from __future__ import annotations

import logging
import re

# Match a backtick-wrapped token. Case-sensitive on purpose (see #17384).
_BACKTICK_RE = re.compile(r"`([^`]+)`")


def normalize_text_newlines(text: str) -> str:
    """Normalize CRLF and standalone CR to LF in source text."""
    if not text:
        return text
    return text.replace("\r\n", "\n").replace("\r", "\n")


def has_wrapped_delimiter(s: str) -> bool:
    """True when the delimiter field contains at least one backtick-wrapped token.

    Used to decide the historical "custom delimiter" mode that bypasses
    ``chunk_token_num``. Separate from whether any delimiter is present
    after parsing (bare single-character delimiters still split).
    """
    if not s:
        return False
    return _BACKTICK_RE.search(s) is not None


def parse_delimiter_field(s: str) -> list[str]:
    """Parse the delimiter field into a list of delimiter strings.

    Returns an empty list for an empty field. Whitespace characters are
    treated as valid single-character delimiters.

    The output is sorted longest-first and deduplicated while
    preserving the first-occurrence order for equal-length items (the
    sort is stable). CRLF / CR line endings inside the field are
    normalized to LF so a user typing ``"\\r\\n"`` and a user typing
    ``"\\n"`` get the same effective delimiter (a single newline).
    """
    if not s:
        return []

    # CRLF normalization: \r\n → \n, then standalone \r → \n. We do this
    # before parsing so the parser never sees a \r in either bare-char
    # position or backtick-wrapped content.
    normalized = normalize_text_newlines(s)

    # Insertion-ordered dedupe so equal-length items keep their first-
    # occurrence order, which is then preserved by the stable sort below.
    delimiters: list[str] = []
    seen: set[str] = set()
    cursor = 0
    for match in _BACKTICK_RE.finditer(normalized):
        start, end = match.span()
        # Bare characters before this backtick-wrapped token.
        for ch in normalized[cursor:start]:
            if ch not in seen:
                seen.add(ch)
                delimiters.append(ch)
        # The backtick-wrapped token (verbatim, except CRLF normalization).
        token = match.group(1)
        if token and token not in seen:
            seen.add(token)
            delimiters.append(token)
        cursor = end
    # Bare characters after the last token (or the whole string if no
    # backticks were present).
    for ch in normalized[cursor:]:
        if ch not in seen:
            seen.add(ch)
            delimiters.append(ch)

    # Stable sort by length, longest-first.
    result = sorted(delimiters, key=len, reverse=True)
    logging.debug(
        "parse_delimiter_field: parsed %d delimiters with lengths %s",
        len(result),
        [len(delimiter) for delimiter in result],
    )
    return result


def compile_delimiter_pattern(delimiters: list[str]) -> str:
    """Build an alternation regex pattern from a list of delimiter strings.

    Each delimiter is ``re.escape``'d so that whitespace and regex
    metacharacters are matched literally. The returned pattern is empty
    when ``delimiters`` is empty.

    The pattern is intended for use with
    ``re.split(r"(%s)" % pattern, text)`` (capture group so delimiters
    appear in the split output) or with
    ``re.compile(pattern).finditer(text)``.
    """
    if not delimiters:
        return ""
    return "|".join(re.escape(d) for d in delimiters if d)
