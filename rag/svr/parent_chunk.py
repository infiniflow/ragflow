"""Shared parent-child index identifiers."""

import xxhash


def parent_chunk_id(doc_id: str, mom: str) -> str:
    """Return the document-scoped parent row ID used by Go and Python."""
    payload = doc_id.encode("utf-8") + b"\x00" + mom.encode("utf-8")
    return xxhash.xxh64(payload).hexdigest()
