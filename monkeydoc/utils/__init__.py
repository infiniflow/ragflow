"""Utility helpers for MonkeyDoc.

This package contains reusable utilities that can be imported from parsing
pipelines or services without introducing heavy dependencies.
"""

from .sections import (
    merge_horizontally,
    deduplicate_overlaps,
    merge_and_dedup,
    pack_by_token_limit,
)

__all__ = [
    "merge_horizontally",
    "deduplicate_overlaps",
    "merge_and_dedup",
    "pack_by_token_limit",
]


