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
import re

# Same @@page\tleft\tright\ttop\tbottom## form the PDF TCADP path parses.
_TCADP_POSITION_TAG_RE = re.compile(r"@@([0-9-]+)\t([0-9.]+)\t([0-9.]+)\t([0-9.]+)\t([0-9.]+)##")


def spreadsheet_positions_from_tcadp_tag(position_tag):
    """Map a TCADP position tag onto DeepDOC spreadsheet coordinates.

    DeepDOC JSON items store a 0-based sheet index as the first component so
    ``add_positions`` can persist a 1-based sheet. TCADP tags use a 1-based page.
    Remaining numbers are reused as row/col when the tag carries them.
    """
    fallback = [[0, 1, 1, 1, 1]]
    if not position_tag:
        return fallback
    match = _TCADP_POSITION_TAG_RE.match(str(position_tag).strip())
    if not match:
        return fallback
    pn, left, right, top, bottom = match.groups()
    sheet = max(int(pn.split("-")[0]) - 1, 0)
    return [[sheet, int(float(left)), int(float(right)), int(float(top)), int(float(bottom))]]


def html_table_row_col_span(html):
    """Return 1-based row/col counts from a TCADP HTML table, defaulting to 1x1."""
    if not isinstance(html, str) or not html.strip():
        return 1, 1
    n_rows = len(re.findall(r"<tr\b", html, flags=re.IGNORECASE))
    n_cols = 0
    first_row = re.search(r"<tr\b[^>]*>(.*?)</tr>", html, flags=re.IGNORECASE | re.DOTALL)
    if first_row:
        n_cols = len(re.findall(r"<t[dh]\b", first_row.group(1), flags=re.IGNORECASE))
    return max(n_rows, 1), max(n_cols, 1)


def spreadsheet_positions_from_tcadp_table(sheet_idx, table_html):
    """Attach at least a sheet index; use the HTML row/col span when present."""
    n_rows, n_cols = html_table_row_col_span(table_html)
    return [[sheet_idx, 1, n_rows, 1, n_cols]]


def tcadp_spreadsheet_json_items(sections, tables, flatten_media_to_text=False):
    """Turn mocked TCADP ``(sections, tables)`` into JSON items with ``positions``."""
    result = []
    for section, position_tag in sections:
        if section:
            result.append(
                {
                    "text": section,
                    "doc_type_kwd": "text",
                    "positions": spreadsheet_positions_from_tcadp_tag(position_tag),
                }
            )
    sheet_idx = 0
    for table in tables:
        if table:
            result.append(
                {
                    "text": table,
                    "doc_type_kwd": "text" if flatten_media_to_text else "table",
                    "positions": spreadsheet_positions_from_tcadp_table(sheet_idx, table),
                }
            )
            sheet_idx += 1
    return result
