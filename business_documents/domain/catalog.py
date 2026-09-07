from __future__ import annotations

import json
from pathlib import Path
from typing import Any


_CATALOG_PATH = Path(__file__).with_name("bcm_bank_v15_l5.json")


def load_document_catalog() -> dict[str, Any]:
    """Load and validate the bundled L5 business-document catalog."""

    catalog = json.loads(_CATALOG_PATH.read_text(encoding="utf-8"))
    items = catalog.get("items")
    if not isinstance(items, list) or not items:
        raise RuntimeError("The business-document catalog must contain items")
    item_ids: set[str] = set()
    for item in items:
        if not isinstance(item, dict):
            raise RuntimeError("Every business-document catalog item must be an object")
        item_id = item.get("id")
        title = item.get("title")
        if not isinstance(item_id, str) or not item_id.strip() or item_id in item_ids:
            raise RuntimeError("Business-document catalog IDs must be non-empty and unique")
        if not isinstance(title, str) or not title.strip():
            raise RuntimeError(f"Business-document catalog item {item_id} has no title")
        if item.get("capability_level") != "L5":
            raise RuntimeError(f"Business-document catalog item {item_id} is not L5")
        item_ids.add(item_id)
    return catalog
