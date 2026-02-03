# -*- coding: utf-8 -*-
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

from markdown import markdown


class RAGFlowMarkdownParser:
    def __init__(self, chunk_token_num=128):
        self.chunk_token_num = int(chunk_token_num)

    def extract_table_segments(self, text: str, render_md_table: bool = False) -> list[tuple[str, str]]:
        """
        Return ordered segments in original appearance order.

        Output:
            [
              ("text",  "..."),
              ("table", "..."),   # markdown table or html table (optionally rendered)
              ("text",  "..."),
              ...
            ]

        Notes:
        - This keeps order (interleaving), unlike (remainder, tables).
        - It performs light HTML tag normalization: <table ...> -> <table>, etc.
        """

        text = text

        # 1) Normalize table-related HTML tags: <table ...> -> <table>
        TAGS = ["table", "td", "tr", "th", "tbody", "thead", "div"]
        table_with_attributes_pattern = re.compile(
            rf"<(?:{'|'.join(TAGS)})[^>]*>", re.IGNORECASE
        )

        def replace_tag(m: re.Match) -> str:
            tag_name = re.match(r"<(\w+)", m.group()).group(1)
            return f"<{tag_name}>"

        text = re.sub(table_with_attributes_pattern, replace_tag, text)

        matches: list[tuple[str, int, int]] = []

        # 2) Markdown table patterns (only try if '|' exists for performance)
        if "|" in text:
            border_table_pattern = re.compile(
                r"""
                (?:\n|^)
                (?:\|.*?\|.*?\|.*?\n)
                (?:\|(?:\s*[:-]+[-| :]*\s*)\|.*?\n)
                (?:\|.*?\|.*?\|.*?\n)+
                """,
                re.VERBOSE,
            )

            no_border_table_pattern = re.compile(
                r"""
                (?:\n|^)
                (?:\S.*?\|.*?\n)
                (?:(?:\s*[:-]+[-| :]*\s*).*?\n)
                (?:\S.*?\|.*?\n)+
                """,
                re.VERBOSE,
            )

            for pat in (border_table_pattern, no_border_table_pattern):
                for m in pat.finditer(text):
                    matches.append(("md", m.start(), m.end()))

        # 3) HTML table pattern (only try if '<table>' exists after normalization)
        if "<table>" in text.lower():
            html_table_pattern = re.compile(
                r"""
                (?:\n|^)
                \s*
                (?:
                    # case1: <html><body><table>...</table></body></html>
                    (?:<html>\s*<body>\s*<table>.*?</table>\s*</body>\s*</html>)
                    |
                    # case2: <body><table>...</table></body>
                    (?:<body>\s*<table>.*?</table>\s*</body>)
                    |
                    # case3: only <table>...</table>
                    (?:<table>.*?</table>)
                )
                \s*
                (?=\n|$)
                """,
                re.VERBOSE | re.DOTALL | re.IGNORECASE,
            )

            for m in html_table_pattern.finditer(text):
                matches.append(("html", m.start(), m.end()))

        # 4) No tables found -> single text segment
        if not matches:
            return [("text", text)]

        # 5) Sort + merge overlaps to avoid duplicate/overlapping matches
        matches.sort(key=lambda x: (x[1], x[2]))
        merged: list[tuple[str, int, int]] = []
        for kind, s, e in matches:
            if not merged:
                merged.append((kind, s, e))
                continue

            pk, ps, pe = merged[-1]
            if s <= pe:  # overlap / nested
                # Keep the larger span (more complete table)
                if e > pe:
                    merged[-1] = (kind, ps, e)
                # else: ignore smaller span
            else:
                merged.append((kind, s, e))

        # 6) Emit ordered segments (text/table interleaving)
        segments: list[tuple[str, str]] = []
        cursor = 0
        for kind, s, e in merged:
            if s > cursor:
                seg_text = text[cursor:s]
                if seg_text:
                    segments.append(("text", seg_text))

            raw_table = text[s:e]
            if kind == "md" and render_md_table:
                # Convert md table to HTML if requested
                raw_table = markdown(raw_table, extensions=["markdown.extensions.tables"])

            if raw_table:
                segments.append(("table", raw_table))

            cursor = e

        if cursor < len(text):
            tail = text[cursor:]
            if tail:
                segments.append(("text", tail))

        return segments



class MarkdownElementExtractor:
    def __init__(self, segments):
        self.segments = segments

    def get_delimiters(self, delimiters):
        toks = re.findall(r"`([^`]+)`", delimiters)
        toks = sorted(set(toks), key=lambda x: -len(x))
        return "|".join(re.escape(t) for t in toks if t)


    def extract_elements(self, delimiter=None, include_meta=False):
        """
        Input:  self.markdown_content: List[Tuple[str, Any]]
                e.g. [("text", "..."), ("image", {...}), ("table", "<table>...</table>"), ...]

        Output: List[Tuple[str, Any]]
                Same as input, except ("text", ...) segments are split into finer-grained ("text", ...) segments.
                Non-text segments are appended as-is.
        """
        sections = []

        for seg in self.segments:
            seg_type, seg_content = seg

            if seg_type != "text":
                sections.append(seg)
                continue

            self.lines = seg_content.split("\n")

            i = 0
            dels = ""
            if delimiter:
                dels = self.get_delimiters(delimiter)

            if len(dels) > 0:
                text = "\n".join(self.lines)
                if include_meta:
                    pattern = re.compile(dels)
                    last_end = 0
                    for m in pattern.finditer(text):
                        part = text[last_end:m.start()]
                        if part is not None:
                            sections.append(("text", part.strip() if isinstance(part, str) else part))
                        last_end = m.end()

                    part = text[last_end:]
                    if part is not None:
                        sections.append(("text", part.strip() if isinstance(part, str) else part))
                else:
                    parts = re.split(dels, text)
                    for p in parts:
                        sections.append(("text", p.strip() if isinstance(p, str) else p))
                continue

            while i < len(self.lines):
                line = self.lines[i]

                if re.match(r"^#{1,6}\s+.*$", line):
                    element = self._extract_header(i)
                    sections.append(("text", element["content"]))
                    i = element["end_line"] + 1

                elif line.strip().startswith("```"):
                    element = self._extract_code_block(i)
                    sections.append(("text", element["content"]))
                    i = element["end_line"] + 1

                elif re.match(r"^\s*[-*+]\s+.*$", line) or re.match(r"^\s*\d+\.\s+.*$", line):
                    element = self._extract_list_block(i)
                    sections.append(("text", element["content"]))
                    i = element["end_line"] + 1

                elif line.strip().startswith(">"):
                    element = self._extract_blockquote(i)
                    sections.append(("text", element["content"]))
                    i = element["end_line"] + 1

                elif line.strip() or line == "":
                    if line.strip():
                        element = self._extract_text_block(i)
                        sections.append(("text", element["content"]))
                        i = element["end_line"] + 1
                    else:
                        i += 1
                else:
                    i += 1

        return sections


    def _extract_header(self, start_pos):
        return {
            "type": "header",
            "content": self.lines[start_pos],
            "start_line": start_pos,
            "end_line": start_pos,
        }

    def _extract_code_block(self, start_pos):
        end_pos = start_pos
        content_lines = [self.lines[start_pos]]

        # Find the end of the code block
        for i in range(start_pos + 1, len(self.lines)):
            content_lines.append(self.lines[i])
            end_pos = i
            if self.lines[i].strip().startswith("```"):
                break

        return {
            "type": "code_block",
            "content": "\n".join(content_lines),
            "start_line": start_pos,
            "end_line": end_pos,
        }

    def _extract_list_block(self, start_pos):
        end_pos = start_pos
        content_lines = []

        i = start_pos
        while i < len(self.lines):
            line = self.lines[i]
            # check if this line is a list item or continuation of a list
            if (
                re.match(r"^\s*[-*+]\s+.*$", line)
                or re.match(r"^\s*\d+\.\s+.*$", line)
                or (i > start_pos and not line.strip())
                or (i > start_pos and re.match(r"^\s{2,}[-*+]\s+.*$", line))
                or (i > start_pos and re.match(r"^\s{2,}\d+\.\s+.*$", line))
                or (i > start_pos and re.match(r"^\s+\w+.*$", line))
            ):
                content_lines.append(line)
                end_pos = i
                i += 1
            else:
                break

        return {
            "type": "list_block",
            "content": "\n".join(content_lines),
            "start_line": start_pos,
            "end_line": end_pos,
        }

    def _extract_blockquote(self, start_pos):
        end_pos = start_pos
        content_lines = []

        i = start_pos
        while i < len(self.lines):
            line = self.lines[i]
            if line.strip().startswith(">") or (i > start_pos and not line.strip()):
                content_lines.append(line)
                end_pos = i
                i += 1
            else:
                break

        return {
            "type": "blockquote",
            "content": "\n".join(content_lines),
            "start_line": start_pos,
            "end_line": end_pos,
        }

    def _extract_text_block(self, start_pos):
        """Extract a text block (paragraphs, inline elements) until next block element"""
        end_pos = start_pos
        content_lines = [self.lines[start_pos]]

        i = start_pos + 1
        while i < len(self.lines):
            line = self.lines[i]
            # stop if we encounter a block element
            if re.match(r"^#{1,6}\s+.*$", line) or line.strip().startswith("```") or re.match(r"^\s*[-*+]\s+.*$", line) or re.match(r"^\s*\d+\.\s+.*$", line) or line.strip().startswith(">"):
                break
            elif not line.strip():
                # check if the next line is a block element
                if i + 1 < len(self.lines) and (
                    re.match(r"^#{1,6}\s+.*$", self.lines[i + 1])
                    or self.lines[i + 1].strip().startswith("```")
                    or re.match(r"^\s*[-*+]\s+.*$", self.lines[i + 1])
                    or re.match(r"^\s*\d+\.\s+.*$", self.lines[i + 1])
                    or self.lines[i + 1].strip().startswith(">")
                ):
                    break
                else:
                    content_lines.append(line)
                    end_pos = i
                    i += 1
            else:
                content_lines.append(line)
                end_pos = i
                i += 1

        return {
            "type": "text_block",
            "content": "\n".join(content_lines),
            "start_line": start_pos,
            "end_line": end_pos,
        }
