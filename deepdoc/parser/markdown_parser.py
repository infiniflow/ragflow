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

import logging
import re

from markdown import markdown


class RAGFlowMarkdownParser:
    def __init__(self, chunk_token_num=128):
        self.chunk_token_num = int(chunk_token_num)

    def extract_tables_and_remainder(self, markdown_text, separate_tables=True):
        tables = []
        working_text = markdown_text

        def replace_tables_with_rendered_html(pattern, table_list, render=True):
            new_text = ""
            last_end = 0
            for match in pattern.finditer(working_text):
                raw_table = match.group()
                table_list.append(raw_table)
                if separate_tables:
                    # Skip this match (i.e., remove it)
                    new_text += working_text[last_end : match.start()] + "\n\n"
                else:
                    # Replace with rendered HTML
                    html_table = markdown(raw_table, extensions=["markdown.extensions.tables"]) if render else raw_table
                    new_text += working_text[last_end : match.start()] + html_table + "\n\n"
                last_end = match.end()
            new_text += working_text[last_end:]
            return new_text

        if "|" in markdown_text:  # for optimize performance
            # Standard Markdown table
            border_table_pattern = re.compile(
                r"""
                (?:\n|^)
                (?:\|.*?\|.*?\|.*?\n)
                (?:\|(?:\s*[:-]+[-| :]*\s*)\|.*?\n)
                (?:\|.*?\|.*?\|.*?\n)+
            """,
                re.VERBOSE,
            )
            working_text = replace_tables_with_rendered_html(border_table_pattern, tables, render=separate_tables)

            # Borderless Markdown table
            no_border_table_pattern = re.compile(
                r"""
                (?:\n|^)
                (?:\S.*?\|.*?\n)
                (?:(?:\s*[:-]+[-| :]*\s*).*?\n)
                (?:\S.*?\|.*?\n)+
                """,
                re.VERBOSE,
            )
            working_text = replace_tables_with_rendered_html(no_border_table_pattern, tables, render=separate_tables)

        # Replace any TAGS e.g. <table ...> to <table>
        TAGS = ["table", "td", "tr", "th", "tbody", "thead", "div"]
        table_with_attributes_pattern = re.compile(rf"<(?:{'|'.join(TAGS)})[^>]*>", re.IGNORECASE)

        def replace_tag(m):
            tag_name = re.match(r"<(\w+)", m.group()).group(1)
            return "<{}>".format(tag_name)

        working_text = re.sub(table_with_attributes_pattern, replace_tag, working_text)

        if "<table>" in working_text.lower():  # for optimize performance
            # HTML table extraction - handle possible html/body wrapper tags
            html_table_pattern = re.compile(
                r"""
            (?:\n|^)
            \s*
            (?:
                # case1: <html><body><table>...</table></body></html>
                (?:<html[^>]*>\s*<body[^>]*>\s*<table[^>]*>.*?</table>\s*</body>\s*</html>)
                |
                # case2: <body><table>...</table></body>
                (?:<body[^>]*>\s*<table[^>]*>.*?</table>\s*</body>)
                |
                # case3: only<table>...</table>
                (?:<table[^>]*>.*?</table>)
            )
            \s*
            (?=\n|$)
            """,
                re.VERBOSE | re.DOTALL | re.IGNORECASE,
            )

            def replace_html_tables():
                nonlocal working_text
                new_text = ""
                last_end = 0
                for match in html_table_pattern.finditer(working_text):
                    raw_table = match.group()
                    tables.append(raw_table)
                    if separate_tables:
                        new_text += working_text[last_end : match.start()] + "\n\n"
                    else:
                        new_text += working_text[last_end : match.start()] + raw_table + "\n\n"
                    last_end = match.end()
                new_text += working_text[last_end:]
                working_text = new_text

            replace_html_tables()

        return working_text, tables


class MarkdownElementExtractor:
    def __init__(self, markdown_content):
        self.markdown_content = markdown_content
        self.lines = markdown_content.split("\n")

    def get_delimiters(self, delimiters):
        toks = re.findall(r"`([^`]+)`", delimiters)
        toks = sorted(set(toks), key=lambda x: -len(x))
        return "|".join(re.escape(t) for t in toks if t)

    def _get_fence_marker(self, line):
        match = re.match(r"^[ \t]{0,3}(?P<fence>`{3,}|~{3,})(?:.*)$", line)
        if not match:
            return None
        fence = match.group("fence")
        return fence[0], len(fence)

    def _is_closing_fence(self, line, fence_char, fence_len):
        pattern = r"^[ \t]{0,3}" + re.escape(fence_char) + r"{" + str(fence_len) + r",}\s*$"
        return re.match(pattern, line) is not None

    def _line_start_offsets(self, text):
        offsets = []
        offset = 0
        for line in self.lines:
            offsets.append(offset)
            offset += len(line) + 1
        return offsets

    def _fenced_code_ranges(self, text):
        ranges = []
        line_offsets = self._line_start_offsets(text)

        i = 0
        while i < len(self.lines):
            marker = self._get_fence_marker(self.lines[i])
            if not marker:
                i += 1
                continue

            fence_char, fence_len = marker
            start_pos = line_offsets[i]
            end_line = len(self.lines) - 1
            for j in range(i + 1, len(self.lines)):
                if self._is_closing_fence(self.lines[j], fence_char, fence_len):
                    end_line = j
                    break

            end_pos = min(len(text), line_offsets[end_line] + len(self.lines[end_line]))
            ranges.append((start_pos, end_pos))
            i = end_line + 1

        return ranges

    def _table_cells(self, line):
        stripped = line.strip()
        if "|" not in stripped:
            return []
        if stripped.startswith("|"):
            stripped = stripped[1:]
        if stripped.endswith("|"):
            stripped = stripped[:-1]
        return [cell.strip() for cell in stripped.split("|")]

    def _is_table_row(self, line):
        cells = self._table_cells(line)
        return len(cells) >= 2 and any(cell for cell in cells)

    def _is_table_separator_row(self, line):
        cells = self._table_cells(line)
        return len(cells) >= 2 and all(re.match(r"^:?-+:?$", cell.replace(" ", "")) for cell in cells)

    def _markdown_table_ranges(self, text):
        ranges = []
        line_offsets = self._line_start_offsets(text)

        i = 0
        while i < len(self.lines) - 1:
            if not self._is_table_row(self.lines[i]) or not self._is_table_separator_row(self.lines[i + 1]):
                i += 1
                continue

            end_line = i + 1
            j = i + 2
            while j < len(self.lines) and self._is_table_row(self.lines[j]):
                end_line = j
                j += 1

            end_pos = min(len(text), line_offsets[end_line] + len(self.lines[end_line]))
            ranges.append((line_offsets[i], end_pos))
            i = end_line + 1

        return ranges

    def _html_table_ranges(self, text):
        table_pattern = re.compile(
            r"""
            (?:
                (?:<html[^>]*>\s*<body[^>]*>\s*<table[^>]*>.*?</table>\s*</body>\s*</html>)
                |
                (?:<body[^>]*>\s*<table[^>]*>.*?</table>\s*</body>)
                |
                (?:<table[^>]*>.*?</table>)
            )
            """,
            re.VERBOSE | re.DOTALL | re.IGNORECASE,
        )
        return [(match.start(), match.end()) for match in table_pattern.finditer(text)]

    def _merge_ranges(self, ranges):
        if not ranges:
            return []

        merged = []
        for start, end in sorted(ranges):
            if not merged or start > merged[-1][1]:
                merged.append((start, end))
            else:
                merged[-1] = (merged[-1][0], max(merged[-1][1], end))
        return merged

    def _protected_ranges(self, text):
        return self._merge_ranges(self._fenced_code_ranges(text) + self._markdown_table_ranges(text) + self._html_table_ranges(text))

    def _append_delimited_section(self, sections, text, start, end, include_meta):
        part = text[start:end]
        if not part or not part.strip():
            return
        if include_meta:
            sections.append(
                {
                    "content": part.strip(),
                    "start_line": text.count("\n", 0, start),
                    "end_line": text.count("\n", 0, end),
                }
            )
        else:
            sections.append(part.strip())

    def _extract_delimited_elements(self, text, delimiters, include_meta=False):
        sections = []
        pattern = re.compile(delimiters)
        protected_ranges = self._protected_ranges(text)
        if protected_ranges:
            logging.debug("markdown_parser: detected %d protected ranges for delimiter extraction", len(protected_ranges))
        protected_idx = 0
        last_end = 0

        for match in pattern.finditer(text):
            while protected_idx < len(protected_ranges) and protected_ranges[protected_idx][1] <= match.start():
                protected_idx += 1

            if protected_idx < len(protected_ranges):
                start, end = protected_ranges[protected_idx]
                if start <= match.start() < end:
                    logging.debug(
                        "markdown_parser: skipped delimiter match at pos=%d delimiter=%r inside fenced range %s",
                        match.start(),
                        match.group(),
                        (start, end),
                    )
                    continue

            self._append_delimited_section(sections, text, last_end, match.start(), include_meta)
            last_end = match.end()

        self._append_delimited_section(sections, text, last_end, len(text), include_meta)
        return sections

    def extract_elements(self, delimiter=None, include_meta=False):
        """Extract individual elements (headers, code blocks, lists, etc.)"""
        sections = []

        i = 0
        dels = ""
        if delimiter:
            dels = self.get_delimiters(delimiter)
        if len(dels) > 0:
            text = "\n".join(self.lines)
            sections = self._extract_delimited_elements(text, dels, include_meta)

            # Attach lone header lines to the section that follows them so that
            # "## Title\n" never becomes an isolated chunk when the delimiter
            # splits at every newline.  A header is "lone" when it occupies a
            # single line (no embedded newline after stripping).
            def _is_lone_header(section_content):
                stripped = section_content.strip()
                return bool(re.match(r"^#{1,6}\s+\S", stripped)) and "\n" not in stripped

            def _is_attachable_body(section_content):
                """True when the following chunk is prose body, not code/table/list/etc."""
                stripped = section_content.strip()
                if not stripped:
                    return False
                first_line = stripped.split("\n", 1)[0]
                if self._get_fence_marker(first_line):
                    return False
                if first_line.lstrip().startswith("|"):
                    return False
                if re.match(r"^\S+\s*\|", first_line):
                    return False
                if first_line.lstrip().startswith("<"):
                    return False
                if re.match(r"^\s*[-*+]\s+", first_line) or re.match(r"^\s*\d+\.\s+", first_line):
                    return False
                if first_line.lstrip().startswith(">"):
                    return False
                return True

            merged = []
            merged_header_count = 0
            i = 0
            while i < len(sections):
                content = sections[i]["content"] if include_meta else sections[i]
                if _is_lone_header(content):
                    header_parts = [content.strip()]
                    j = i + 1
                    while j < len(sections):
                        next_content = sections[j]["content"] if include_meta else sections[j]
                        if not _is_lone_header(next_content):
                            break
                        header_parts.append(next_content.strip())
                        j += 1
                    if j < len(sections):
                        body_content = sections[j]["content"] if include_meta else sections[j]
                        if _is_attachable_body(body_content):
                            combined = "\n".join(header_parts) + "\n" + body_content
                            if include_meta:
                                merged.append(
                                    {
                                        **sections[i],
                                        "content": combined,
                                        "end_line": sections[j]["end_line"],
                                    }
                                )
                            else:
                                merged.append(combined)
                            merged_header_count += len(header_parts)
                            i = j + 1
                            continue
                    for k in range(i, j):
                        merged.append(sections[k])
                    i = j
                    continue
                merged.append(sections[i])
                i += 1
            if merged_header_count:
                logging.debug(
                    "markdown_parser: merged %d lone header line(s) into following sections",
                    merged_header_count,
                )
            return merged
        while i < len(self.lines):
            line = self.lines[i]

            if re.match(r"^#{1,6}\s+.*$", line):
                # header
                element = self._extract_header(i)
                sections.append(element if include_meta else element["content"])
                i = element["end_line"] + 1
            elif self._get_fence_marker(line):
                # code block
                element = self._extract_code_block(i)
                sections.append(element if include_meta else element["content"])
                i = element["end_line"] + 1
            elif re.match(r"^\s*[-*+]\s+.*$", line) or re.match(r"^\s*\d+\.\s+.*$", line):
                # list block
                element = self._extract_list_block(i)
                sections.append(element if include_meta else element["content"])
                i = element["end_line"] + 1
            elif line.strip().startswith(">"):
                # blockquote
                element = self._extract_blockquote(i)
                sections.append(element if include_meta else element["content"])
                i = element["end_line"] + 1
            elif line.strip():
                # text block (paragraphs and inline elements until next block element)
                element = self._extract_text_block(i)
                sections.append(element if include_meta else element["content"])
                i = element["end_line"] + 1
            else:
                i += 1

        if include_meta:
            sections = [section for section in sections if section["content"].strip()]
        else:
            sections = [section for section in sections if section.strip()]
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
        fence_char, fence_len = self._get_fence_marker(self.lines[start_pos])

        # Find the end of the code block
        for i in range(start_pos + 1, len(self.lines)):
            content_lines.append(self.lines[i])
            end_pos = i
            if self._is_closing_fence(self.lines[i], fence_char, fence_len):
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
            if re.match(r"^#{1,6}\s+.*$", line) or self._get_fence_marker(line) or re.match(r"^\s*[-*+]\s+.*$", line) or re.match(r"^\s*\d+\.\s+.*$", line) or line.strip().startswith(">"):
                break
            elif not line.strip():
                # check if the next line is a block element
                if i + 1 < len(self.lines) and (
                    re.match(r"^#{1,6}\s+.*$", self.lines[i + 1])
                    or self._get_fence_marker(self.lines[i + 1])
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
