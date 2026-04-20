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

import logging


def resolve_reference_metadata_preferences(
    request_payload: dict | None = None,
    config_payload: dict | None = None,
) -> tuple[bool, set[str] | None]:
    """
    Resolve metadata include/fields from request and optional config.
    Request values take precedence over config values.
    Supports legacy request keys: include_metadata / metadata_fields.
    """
    request_payload = request_payload or {}
    config_payload = config_payload or {}

    config_ref = config_payload.get("reference_metadata", {})
    request_ref = request_payload.get("reference_metadata", {})

    resolved: dict = {}
    if isinstance(config_ref, dict):
        resolved.update(config_ref)
    if isinstance(request_ref, dict):
        resolved.update(request_ref)

    if "include_metadata" in request_payload and "include" not in resolved:
        resolved["include"] = bool(request_payload.get("include_metadata"))
    if "metadata_fields" in request_payload and "fields" not in resolved:
        resolved["fields"] = request_payload.get("metadata_fields")

    include_metadata = bool(resolved.get("include", False))
    fields = resolved.get("fields")
    if fields is None:
        return include_metadata, None
    if not isinstance(fields, list):
        return include_metadata, set()
    return include_metadata, {f for f in fields if isinstance(f, str)}


def enrich_chunks_with_document_metadata(
    chunks: list[dict],
    metadata_fields: set[str] | None = None,
    *,
    kb_field: str = "kb_id",
    doc_field: str = "doc_id",
    output_field: str = "document_metadata",
) -> None:
    """
    Mutates chunk payloads in-place by attaching `document_metadata`.
    Field names can be customized for different chunk schemas.
    """
    if metadata_fields is not None and not metadata_fields:
        return

    doc_ids_by_kb: dict[str, set[str]] = {}
    for chunk in chunks:
        kb_id = chunk.get(kb_field)
        doc_id = chunk.get(doc_field)
        if not kb_id or not doc_id:
            continue
        doc_ids_by_kb.setdefault(kb_id, set()).add(doc_id)

    if not doc_ids_by_kb:
        return

    # Resolve service lazily so callers/tests that swap service modules at runtime
    # (e.g. via monkeypatch) don't get stuck with a stale class reference.
    from api.db.services.doc_metadata_service import DocMetadataService
    metadata_getter = getattr(DocMetadataService, "get_metadata_for_documents", None)
    if not callable(metadata_getter):
        logging.warning(
            "DocMetadataService.get_metadata_for_documents is unavailable; "
            "skipping metadata enrichment."
        )
        return

    meta_by_doc: dict[str, dict] = {}
    for kb_id, doc_ids in doc_ids_by_kb.items():
        meta_map = metadata_getter(list(doc_ids), kb_id)
        if meta_map:
            meta_by_doc.update(meta_map)
            logging.debug("Fetched metadata for %d docs in kb_id=%s", len(meta_map), kb_id)

    for chunk in chunks:
        doc_id = chunk.get(doc_field)
        if not doc_id:
            continue
        meta = meta_by_doc.get(doc_id)
        if not meta:
            continue
        if metadata_fields is not None:
            meta = {k: v for k, v in meta.items() if k in metadata_fields}
        if meta:
            chunk[output_field] = meta
            logging.debug("Enriched chunk for doc_id=%s with %d metadata fields: %s", doc_id, len(meta), list(meta.keys()))
