from __future__ import annotations

"""Section utilities: merge horizontally aligned lines and deduplicate overlaps.

This module provides light-weight, dependency-free helpers to post-process
sections emitted by `MonkeyDocPdfParser` before building final output.

Data model
----------
Sections are represented as tuples (text_with_tag, unused_field) where
`text_with_tag` ends with an inline position tag of the form:
    @@{page}\t{x0}\t{x1}\t{top}\t{bottom}##

These helpers parse that tag to access geometry, perform operations, and then
re-emit text with a single consolidated tag.
"""

import re
from dataclasses import dataclass
from typing import List, Tuple


_TAG_RE = re.compile(r"@@(\d+)\t([0-9.]+)\t([0-9.]+)\t([0-9.]+)\t([0-9.]+)##")


@dataclass
class Section:
    text: str
    page: int
    x0: float
    x1: float
    top: float
    bottom: float

    @property
    def width(self) -> float:
        return max(0.0, self.x1 - self.x0)

    @property
    def height(self) -> float:
        return max(0.0, self.bottom - self.top)


def _parse_section(raw: Tuple[str, str]) -> Section | None:
    text, _ = raw
    m = _TAG_RE.search(text)
    if not m:
        return None
    page, x0, x1, top, bottom = m.groups()
    body = _TAG_RE.sub("", text).strip()
    return Section(
        text=body,
        page=int(page),
        x0=float(x0),
        x1=float(x1),
        top=float(top),
        bottom=float(bottom),
    )


def _tag_text(sec: Section) -> str:
    return f"{sec.text}@@{sec.page}\t{sec.x0:.1f}\t{sec.x1:.1f}\t{sec.top:.1f}\t{sec.bottom:.1f}##"


def _horiz_overlap(a: Section, b: Section) -> float:
    left = max(a.x0, b.x0)
    right = min(a.x1, b.x1)
    if right <= left:
        return 0.0
    inter = right - left
    denom = max(min(a.width, b.width), 1e-6)
    return inter / denom


def _vert_overlap(a: Section, b: Section) -> float:
    top = max(a.top, b.top)
    bottom = min(a.bottom, b.bottom)
    if bottom <= top:
        return 0.0
    inter = bottom - top
    denom = max(min(a.height, b.height), 1e-6)
    return inter / denom


def merge_horizontally(sections: List[Tuple[str, str]], y_tol: float = 2.0, min_horiz_overlap: float = 0.1) -> List[Tuple[str, str]]:
    """Merge sections that sit on the same row (similar top) and touch/overlap horizontally.

    Parameters
    - y_tol: vertical tolerance to consider two sections aligned on the same row
    - min_horiz_overlap: minimum horizontal overlap ratio (relative to min width) to merge;
      if no overlap, adjacency is allowed when abs(a.x1 - b.x0) <= 1.0
    """
    parsed = [s for s in (_parse_section(r) for r in sections) if s is not None]
    if not parsed:
        return sections

    # Group by (page, approx_top)
    groups = {}
    for s in parsed:
        key = (s.page, round(s.top / max(y_tol, 1e-6)))
        groups.setdefault(key, []).append(s)

    merged: List[Section] = []
    for key, items in groups.items():
        items.sort(key=lambda s: (s.top, s.x0))
        row: List[Section] = []
        for s in items:
            if not row:
                row.append(s)
                continue
            last = row[-1]
            same_row = abs(s.top - last.top) <= y_tol
            touches = abs(last.x1 - s.x0) <= 1.0
            horz_ok = _horiz_overlap(last, s) >= min_horiz_overlap or touches
            if same_row and horz_ok:
                # Merge geometries and texts
                last.text = (last.text + " " + s.text).strip()
                last.x0 = min(last.x0, s.x0)
                last.x1 = max(last.x1, s.x1)
                last.top = min(last.top, s.top)
                last.bottom = max(last.bottom, s.bottom)
            else:
                row.append(s)
        merged.extend(row)

    # Convert back to output shape
    return [(_tag_text(s), "") for s in merged]


def deduplicate_overlaps(sections: List[Tuple[str, str]], cover_ratio: float = 0.8) -> List[Tuple[str, str]]:
    """Remove smaller sections that are largely covered by a larger section on the same page.

    Two-way rule: if A covers B by >= cover_ratio (based on B's area), drop B.
    """
    parsed = [s for s in (_parse_section(r) for r in sections) if s is not None]
    if not parsed:
        return sections

    kept: List[Section] = []
    by_page = {}
    for s in parsed:
        by_page.setdefault(s.page, []).append(s)

    def area(sec: Section) -> float:
        return sec.width * sec.height

    for page, items in by_page.items():
        items.sort(key=lambda s: area(s), reverse=True)
        dropped = [False] * len(items)
        for i, a in enumerate(items):
            if dropped[i]:
                continue
            for j in range(i + 1, len(items)):
                if dropped[j]:
                    continue
                b = items[j]
                # compute overlap area over area(b)
                ix0 = max(a.x0, b.x0)
                iy0 = max(a.top, b.top)
                ix1 = min(a.x1, b.x1)
                iy1 = min(a.bottom, b.bottom)
                if ix1 <= ix0 or iy1 <= iy0:
                    continue
                inter = (ix1 - ix0) * (iy1 - iy0)
                if inter / max(area(b), 1e-6) >= cover_ratio:
                    dropped[j] = True
        for i, s in enumerate(items):
            if not dropped[i]:
                kept.append(s)

    kept.sort(key=lambda s: (s.page, s.top, s.x0))
    return [(_tag_text(s), "") for s in kept]


def merge_and_dedup(sections: List[Tuple[str, str]], y_tol: float = 2.0, min_horiz_overlap: float = 0.1, cover_ratio: float = 0.8) -> List[Tuple[str, str]]:
    """Convenience: merge same-row fragments then drop fully-covered small chunks.

    Order matters: horizontal merge first to build coherent lines, then dedup to
    remove redundant fragments contained by larger blocks.
    """
    merged = merge_horizontally(sections, y_tol=y_tol, min_horiz_overlap=min_horiz_overlap)
    return deduplicate_overlaps(merged, cover_ratio=cover_ratio)


def pack_by_token_limit(
    sections: List[Tuple[str, str]],
    chunk_token_num: int,
    token_counter: "callable[[str], int] | None" = None,
    separator: str = "\n",
) -> List[Tuple[str, str]]:
    """Pack adjacent small sections into larger chunks under a token threshold.

    Behavior:
    - Iterate sections in order. If a section's token length >= chunk_token_num, flush any
      pending buffer first, then append this section as-is.
    - Otherwise, try to accumulate consecutive small sections until adding the next would
      exceed chunk_token_num; emit the accumulated buffer, then continue.

    Token counting removes inline position tags before counting.
    """

    # Choose token counter
    if token_counter is None:
        try:
            from rag.utils import num_tokens_from_string as _ntfs  # type: ignore

            def token_counter(txt: str) -> int:  # type: ignore[no-redef]
                return int(_ntfs(txt))
        except Exception:
            def token_counter(txt: str) -> int:  # type: ignore[no-redef]
                return max(1, len(txt.split()))

    def strip_tag(s_with_tag: str) -> str:
        m = _TAG_RE.search(s_with_tag)
        return _TAG_RE.sub("", s_with_tag).strip() if m else s_with_tag

    packed: List[Tuple[str, str]] = []
    buf_texts: List[str] = []
    buf_tokens = 0

    def flush_buffer():
        nonlocal buf_texts, buf_tokens
        if buf_texts:
            combined = separator.join(buf_texts)
            packed.append((combined, ""))
            buf_texts = []
            buf_tokens = 0

    for t, meta in sections:
        body = strip_tag(t)
        tok = token_counter(body)
        if tok >= chunk_token_num:
            # Large chunk stands alone
            flush_buffer()
            packed.append((t, meta))
            continue

        # Try to fit into buffer
        if buf_tokens == 0:
            buf_texts.append(t)
            buf_tokens = tok
        else:
            if buf_tokens + tok <= chunk_token_num:
                buf_texts.append(t)
                buf_tokens += tok
            else:
                # Emit current buffer, start a new one with current
                flush_buffer()
                buf_texts.append(t)
                buf_tokens = tok

    flush_buffer()
    return packed

