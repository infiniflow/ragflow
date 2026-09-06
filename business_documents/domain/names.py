"""Canonical document-title rules shared by persistence and EVA matching."""

from __future__ import annotations

import hashlib
import re
import unicodedata


_WHITESPACE = re.compile(r"\s+")


def normalize_title(value: str) -> str:
    """Return the canonical form used for exact title comparisons."""

    return _WHITESPACE.sub(" ", unicodedata.normalize("NFKC", value).strip()).casefold()


def title_key(value: str) -> str:
    """Return a fixed-width unique-index key for a canonical title."""

    return hashlib.sha256(normalize_title(value).encode("utf-8")).hexdigest()
