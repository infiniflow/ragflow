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
"""Render HTML tables into an LLM-friendly Markdown view.

Why this exists
---------------
The compiled chunks of a Wikipedia-sourced KB carry machine-generated HTML
tables. Passing them raw is both expensive and hard to read: a person infobox is
1-2K chars of ``<table>/<td>`` markup, and a question like "how many children did
all ten nominees have" needs ten of them at once. The 2026-09-16 FRAMES
attribution showed the values ARE in the KB while the model still answers "cannot
be determined" — the numbers were never surfaced in a form the model could
aggregate.

Approach
--------
Matches the established practice for tables in RAG (serialize tables to
Markdown; key-value form for attribute tables):

* two-column table (Wikipedia infobox) -> Markdown-KV lines: ``Children: 3``
* any other table (ranked list, election results, timeline) -> a Markdown pipe
  table with the header and EVERY row, because rank/order/completeness decide
  those answers and row-window narrowing silently drops the answer row.

Values are kept VERBATIM (parentheticals, units, "plus the perpetrators"),
entities/whitespace are collapsed, all-empty rows are dropped. The raw HTML is
never destroyed by this module: callers keep the original chunk in the evidence
pool (for citation) and use the view only for the model-visible text.
"""

from __future__ import annotations

import re

from bs4 import BeautifulSoup

# Generous ceilings. A Wikipedia election / standings table can run to 50K+ raw
# chars across 100+ rows. Capping at 4K chars / 60 rows silently dropped the
# answer row for exactly the "large / multi-column" tables this module exists to
# serve (e.g. a 14.7K-char standings table where the rank-19 row sat at ~62% of
# the table). We keep a ceiling only to stop one pathological table from eating
# the whole evidence budget, not to truncate normal tables; the caller still
# holds the raw chunk and can read it on demand.
_MAX_ROWS = 400
_MAX_CHARS_PER_TABLE = 20000


def _cell_text(cell) -> str:
    """Whitespace-collapsed cell text (tags stripped, entities resolved)."""
    return re.sub(r"\s+", " ", cell.get_text(" ", strip=True)).strip()


def _table_rows(table) -> list[list[str]]:
    """Non-empty rows of ``table`` as lists of cell strings."""
    rows: list[list[str]] = []
    for tr in table.find_all("tr"):
        vals = [_cell_text(c) for c in tr.find_all(["td", "th"])]
        if any(vals):
            rows.append(vals)
    return rows


def _md_cell(value: str) -> str:
    """Escape a value for a Markdown pipe cell (the delimiter must stay intact)."""
    return value.replace("\\", "\\\\").replace("|", "\\|")


# Tokens that appear in a Wikipedia table's "units" row — the row right under the
# header that reads e.g. "No. | % | No. | %". It carries no data and only
# pollutes a Markdown pipe table, so it is dropped before rendering.
_UNIT_TOKENS = {"", "no.", "no", "n", "%", "#", "—", "-", "•", "·"}


def _is_units_row(vals: list[str]) -> bool:
    if not vals:
        return False
    return all(v.strip().lower() in _UNIT_TOKENS for v in vals)


def _render_one(table) -> str | None:
    rows = _table_rows(table)
    if not rows:
        return None
    # Infobox: two columns, a real key in the first cell and a value in the second
    # (the photo-caption row has an empty second cell and is dropped here).
    pairs = [(r[0], r[1]) for r in rows if len(r) == 2 and r[0] and r[1]]
    if len(pairs) >= 3:
        return "\n".join(f"{k}: {v}" for k, v in pairs[:_MAX_ROWS])[:_MAX_CHARS_PER_TABLE]
    width = max(len(r) for r in rows)
    header = rows[0]
    # Drop header-repeat / units rows so the pipe body starts at real data
    # (otherwise a wide election table leads with a "No. | % | No. | %" junk row).
    body = [r for r in rows[1 : _MAX_ROWS + 1] if not _is_units_row(r)]
    if not body:
        return None

    def _line(cells: list[str]) -> str:
        padded = (cells + [""] * width)[:width]
        return "| " + " | ".join(_md_cell(c) for c in padded) + " |"

    caption = table.find("caption")
    cap = (caption.get_text(" ", strip=True).strip() if caption else "") or ""
    out: list[str] = []
    if cap:
        out.append(f"**Table: {cap}**")
    out += [_line(header), "|" + "---|" * width, *(_line(r) for r in body)]
    if len(rows) - 1 > len(body):
        out.append(f"... ({len(rows) - 1 - len(body)} more row(s) omitted)")
    return "\n".join(out)[:_MAX_CHARS_PER_TABLE]


def render_tables(text: str) -> str | None:
    """Replace every HTML table in ``text`` with a Markdown view.

    Returns ``None`` when ``text`` holds no renderable table, so callers can keep
    their previous behaviour untouched. Never raises: a parse failure falls back
    to ``None``.
    """
    if not text or "<table" not in text.lower():
        return None
    try:
        soup = BeautifulSoup(text, "html.parser")
    except Exception:  # noqa: BLE001
        return None
    views: list[str] = []
    for table in soup.find_all("table"):
        try:
            view = _render_one(table)
        except Exception:  # noqa: BLE001
            view = None
        if view:
            views.append(view)
    if not views:
        return None
    return "\n\n".join(views)


def table_view_or_raw(text: str) -> str:
    """``render_tables(text)`` when it produces something, else ``text`` unchanged.

    Small convenience for the two call sites that only want "the best available
    model-visible form of this chunk".
    """
    view = render_tables(text or "")
    return view if view else (text or "")
