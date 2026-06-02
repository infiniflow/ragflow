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

import re

from common.token_utils import num_tokens_from_string

ATX_HEADING_RE = re.compile(r"^(#{1,6})\s+\S")

DEFAULT_MARKDOWN_HEADING_DEPTH = 6


def is_atx_heading_line(line: str, max_depth: int = DEFAULT_MARKDOWN_HEADING_DEPTH) -> bool:
    match = ATX_HEADING_RE.match(line.strip())
    if not match:
        return False
    return len(match.group(1)) <= max(1, int(max_depth))


def split_text_by_atx_headings(text: str, max_depth: int = DEFAULT_MARKDOWN_HEADING_DEPTH) -> list[str]:
    """Split markdown into chunks that start at ATX headings (# .. ######)."""
    if not text or not text.strip():
        return []

    lines = text.splitlines()
    chunks: list[str] = []
    current: list[str] = []

    for line in lines:
        if is_atx_heading_line(line, max_depth):
            if current:
                chunk = "\n".join(current).strip()
                if chunk:
                    chunks.append(chunk)
            current = [line]
        else:
            current.append(line)

    if current:
        chunk = "\n".join(current).strip()
        if chunk:
            chunks.append(chunk)

    return chunks


def group_markdown_sections_by_headings(
    sections,
    max_token_num: int = 0,
    max_depth: int = DEFAULT_MARKDOWN_HEADING_DEPTH,
) -> list[str]:
    """
    Merge MarkdownElementExtractor sections so each chunk begins at an ATX heading
    and includes following body blocks until the next heading.
    """
    chunks: list[str] = []
    current: list[str] = []

    def flush():
        nonlocal current
        if not current:
            return
        chunk = "\n".join(current).strip()
        current = []
        if not chunk:
            return
        if max_token_num > 0 and num_tokens_from_string(chunk) > max_token_num:
            chunks.extend(_split_oversized_chunk(chunk, max_token_num))
        else:
            chunks.append(chunk)

    for section in sections:
        text = section[0] if isinstance(section, tuple) else section
        if not isinstance(text, str) or not text.strip():
            continue

        first_line = text.strip().splitlines()[0]
        if is_atx_heading_line(first_line, max_depth) and current:
            flush()
        current.append(text.strip())

    flush()
    return chunks


def _split_oversized_chunk(text: str, max_token_num: int) -> list[str]:
    """Fallback: split an oversized heading section by paragraphs, then by word windows."""
    parts = [part.strip() for part in re.split(r"\n\s*\n", text) if part.strip()]
    if not parts:
        return [text]

    merged: list[str] = []
    current = ""
    current_tokens = 0
    for part in parts:
        part_tokens = num_tokens_from_string(part)
        if part_tokens > max_token_num:
            if current:
                merged.append(current)
                current = ""
                current_tokens = 0
            merged.extend(_split_text_by_token_window(part, max_token_num))
            continue
        if current and current_tokens + part_tokens > max_token_num:
            merged.append(current)
            current = part
            current_tokens = part_tokens
        else:
            current = f"{current}\n\n{part}".strip() if current else part
            current_tokens = num_tokens_from_string(current)
    if current:
        if num_tokens_from_string(current) > max_token_num:
            merged.extend(_split_text_by_token_window(current, max_token_num))
        else:
            merged.append(current)
    return merged or [text]


def _split_text_by_token_window(text: str, max_token_num: int) -> list[str]:
    words = text.split()
    if not words:
        return [text]

    chunks: list[str] = []
    current_words: list[str] = []
    current_tokens = 0
    for word in words:
        word_tokens = num_tokens_from_string(word)
        if current_words and current_tokens + word_tokens > max_token_num:
            chunks.append(" ".join(current_words))
            current_words = [word]
            current_tokens = word_tokens
        else:
            current_words.append(word)
            current_tokens += word_tokens
    if current_words:
        chunks.append(" ".join(current_words))
    return chunks or [text]
