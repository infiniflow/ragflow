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
"""
Document Metadata Service

Manages document-level metadata storage in ES/Infinity.
This is the SOLE source of truth for document metadata - MySQL meta_fields column has been removed.
"""

import json
import logging
import re
from copy import deepcopy
from typing import Dict, List, Optional

from api.db.db_models import DB, Document
from common import settings
from common.metadata_utils import dedupe_list
from api.db.db_models import Knowledgebase
from common.doc_store.doc_store_base import OrderByExpr


class DocMetadataService:
    """Service for managing document metadata in ES/Infinity"""

    @staticmethod
    def _get_doc_meta_index_name(tenant_id: str) -> str:
        """
        Get the index name for document metadata.

        Args:
            tenant_id: Tenant ID

        Returns:
            Index name for document metadata
        """
        return f"ragflow_doc_meta_{tenant_id}"

    @staticmethod
    def _extract_metadata(flat_meta: Dict) -> Dict:
        """
        Extract metadata from ES/Infinity document format.

        Args:
            flat_meta: Raw document from ES/Infinity with meta_fields field

        Returns:
            Simple metadata dictionary
        """
        if not flat_meta or not isinstance(flat_meta, dict):
            return {}

        meta_fields = flat_meta.get('meta_fields')
        if not meta_fields:
            return {}

        # Parse JSON string if needed
        if isinstance(meta_fields, str):
            import json
            try:
                return json.loads(meta_fields)
            except json.JSONDecodeError:
                return {}

        # Already a dict, return as-is
        if isinstance(meta_fields, dict):
            return meta_fields

        return {}

    @staticmethod
    def _extract_doc_id(doc: Dict, hit: Dict = None) -> str:
        """
        Extract document ID from various formats.

        Args:
            doc: Document dictionary (from DataFrame or list format)
            hit: Hit dictionary (from ES format with _id field)

        Returns:
            Document ID or empty string
        """
        if hit:
            # ES format: doc is in _source, id is in _id
            return hit.get('_id', '')
        # DataFrame or list format: check multiple possible fields
        return doc.get("doc_id") or doc.get("_id") or doc.get("id", "")

    @classmethod
    def _iter_search_results(cls, results):
        """
        Iterate over search results in various formats (DataFrame, ES, list).

        Yields:
            Tuple of (doc_id, doc_dict) for each document

        Args:
            results: Search results from ES/Infinity in any format
        """
        # Handle tuple return from Infinity: (DataFrame, int)
        # Check this FIRST because pandas DataFrames also have __getitem__
        if isinstance(results, tuple) and len(results) == 2:
            results = results[0]  # Extract DataFrame from tuple

        # Check if results is a pandas DataFrame (from Infinity)
        if hasattr(results, 'iterrows'):
            # Handle pandas DataFrame - use iterrows() to iterate over rows
            for _, row in results.iterrows():
                doc = dict(row)  # Convert Series to dict
                doc_id = cls._extract_doc_id(doc)
                if doc_id:
                    yield doc_id, doc

        # Check if ES format (has 'hits' key)
        # Note: ES returns ObjectApiResponse which is dict-like but not isinstance(dict)
        elif hasattr(results, '__getitem__') and 'hits' in results:
            # ES format: {"hits": {"hits": [{"_source": {...}, "_id": "..."}]}}
            hits = results.get('hits', {}).get('hits', [])
            for hit in hits:
                doc = hit.get('_source', {})
                doc_id = cls._extract_doc_id(doc, hit)
                if doc_id:
                    yield doc_id, doc

        # Handle list of dicts or other formats
        elif isinstance(results, list):
            for res in results:
                if isinstance(res, dict):
                    docs = [res]
                else:
                    docs = res

                for doc in docs:
                    doc_id = cls._extract_doc_id(doc)
                    if doc_id:
                        yield doc_id, doc

    @classmethod
    def _search_metadata(cls, kb_id: str, condition: Dict = None, limit: int = 10000):
        """
        Common search logic for metadata queries.

        Args:
            kb_id: Knowledge base ID
            condition: Optional search condition (defaults to {"kb_id": kb_id})
            limit: Max results to return

        Returns:
            Search results from ES/Infinity, or empty list if index doesn't exist
        """
        kb = Knowledgebase.get_by_id(kb_id)
        if not kb:
            return []

        tenant_id = kb.tenant_id
        index_name = cls._get_doc_meta_index_name(tenant_id)

        # Check if metadata index exists, create if it doesn't
        if not settings.docStoreConn.index_exist(index_name, ""):
            logging.debug(f"Metadata index {index_name} does not exist, creating it")
            result = settings.docStoreConn.create_doc_meta_idx(index_name)
            if result is False:
                logging.error(f"Failed to create metadata index {index_name}")
                return []
            logging.debug(f"Successfully created metadata index {index_name}")

        if condition is None:
            condition = {"kb_id": kb_id}

        order_by = OrderByExpr()

        return settings.docStoreConn.search(
            select_fields=["*"],
            highlight_fields=[],
            condition=condition,
            match_expressions=[],
            order_by=order_by,
            offset=0,
            limit=limit,
            index_names=index_name,
            knowledgebase_ids=[kb_id]
        )

    @classmethod
    def _split_combined_values(cls, meta_fields: Dict) -> Dict:
        """
        Post-process metadata to split combined values by common delimiters.

        For example: "关羽、孙权、张辽" -> ["关羽", "孙权", "张辽"]
        This fixes LLM extraction where multiple values are extracted as one combined value.
        Also removes duplicates after splitting.

        Args:
            meta_fields: Metadata dictionary

        Returns:
            Processed metadata with split values
        """
        if not meta_fields or not isinstance(meta_fields, dict):
            return meta_fields

        processed = {}
        for key, value in meta_fields.items():
            if isinstance(value, list):
                # Process each item in the list
                new_values = []
                for item in value:
                    if isinstance(item, str):
                        # Split by common delimiters: Chinese comma (、), regular comma (,), pipe (|), semicolon (;), Chinese semicolon (；)
                        # Also handle mixed delimiters and spaces
                        split_items = re.split(r'[、,，;；|]+', item.strip())
                        # Trim whitespace and filter empty strings
                        split_items = [s.strip() for s in split_items if s.strip()]
                        if split_items:
                            new_values.extend(split_items)
                        else:
                            # Keep original if no split happened
                            new_values.append(item)
                    else:
                        new_values.append(item)
                # Remove duplicates while preserving order
                processed[key] = list(dict.fromkeys(new_values))
            else:
                processed[key] = value

        if processed != meta_fields:
            logging.debug(f"[METADATA SPLIT] Split combined values: {meta_fields} -> {processed}")
        return processed

    @classmethod
    @DB.connection_context()
    def insert_document_metadata(cls, doc_id: str, meta_fields: Dict) -> bool:
        """
        Insert document metadata into ES/Infinity.

        Args:
            doc_id: Document ID
            meta_fields: Metadata dictionary

        Returns:
            True if successful, False otherwise
        """
        try:
            # Get document with tenant_id (need to join with Knowledgebase)
            doc_query = Document.select(Document, Knowledgebase.tenant_id).join(
                Knowledgebase, on=(Knowledgebase.id == Document.kb_id)
            ).where(Document.id == doc_id)

            doc = doc_query.first()
            if not doc:
                logging.warning(f"Document {doc_id} not found for metadata insertion")
                return False

            # Extract document fields
            doc_obj = doc  # This is the Document object
            tenant_id = doc.knowledgebase.tenant_id  # Get tenant_id from joined Knowledgebase
            kb_id = doc_obj.kb_id

            # Prepare metadata document
            doc_meta = {
                "id": doc_obj.id,
                "kb_id": kb_id,
            }

            # Store metadata as JSON object in meta_fields column (same as MySQL structure)
            if meta_fields:
                # Post-process to split combined values by common delimiters
                meta_fields = cls._split_combined_values(meta_fields)
                doc_meta["meta_fields"] = meta_fields
            else:
                doc_meta["meta_fields"] = {}

            # Ensure index/table exists (per-tenant for both ES and Infinity)
            index_name = cls._get_doc_meta_index_name(tenant_id)

            # Check if table exists
            table_exists = settings.docStoreConn.index_exist(index_name, kb_id)
            logging.debug(f"Metadata table exists check: {index_name} -> {table_exists}")

            # Create index if it doesn't exist
            if not table_exists:
                logging.debug(f"Creating metadata table: {index_name}")
                # Both ES and Infinity now use per-tenant metadata tables
                result = settings.docStoreConn.create_doc_meta_idx(index_name)
                logging.debug(f"Table creation result: {result}")
                if result is False:
                    logging.error(f"Failed to create metadata table {index_name}")
                    return False
            else:
                logging.debug(f"Metadata table already exists: {index_name}")

            # Insert into ES/Infinity
            result = settings.docStoreConn.insert(
                [doc_meta],
                index_name,
                kb_id
            )

            if result:
                logging.error(f"Failed to insert metadata for document {doc_id}: {result}")
                return False
            # Force ES refresh to make metadata immediately available for search
            if not settings.DOC_ENGINE_INFINITY:
                try:
                    settings.docStoreConn.es.indices.refresh(index=index_name)
                    logging.debug(f"Refreshed metadata index: {index_name}")
                except Exception as e:
                    logging.warning(f"Failed to refresh metadata index {index_name}: {e}")
            
            logging.debug(f"Successfully inserted metadata for document {doc_id}")
            return True

        except Exception as e:
            logging.error(f"Error inserting metadata for document {doc_id}: {e}")
            return False

    @classmethod
    @DB.connection_context()
    def update_document_metadata(cls, doc_id: str, meta_fields: Dict) -> bool:
        """
        Update document metadata in ES/Infinity.

        For Elasticsearch: Uses partial update to directly update the meta_fields field.
        For Infinity: Falls back to delete+insert (Infinity doesn't support partial updates well).

        Args:
            doc_id: Document ID
            meta_fields: Metadata dictionary

        Returns:
            True if successful, False otherwise
        """
        try:
            # Get document with tenant_id
            doc_query = Document.select(Document, Knowledgebase.tenant_id).join(
                Knowledgebase, on=(Knowledgebase.id == Document.kb_id)
            ).where(Document.id == doc_id)

            doc = doc_query.first()
            if not doc:
                logging.warning(f"Document {doc_id} not found for metadata update")
                return False

            # Extract fields
            doc_obj = doc
            tenant_id = doc.knowledgebase.tenant_id
            kb_id = doc_obj.kb_id
            index_name = cls._get_doc_meta_index_name(tenant_id)

            # Post-process to split combined values
            processed_meta = cls._split_combined_values(meta_fields)

            logging.debug(f"[update_document_metadata] Updating doc_id: {doc_id}, kb_id: {kb_id}, meta_fields: {processed_meta}")

            # For Elasticsearch, use efficient partial update
            if not settings.DOC_ENGINE_INFINITY:
                try:
                    # Use ES partial update API - much more efficient than delete+insert
                    settings.docStoreConn.es.update(
                        index=index_name,
                        id=doc_id,
                        refresh=True,  # Make changes immediately visible
                        doc={"meta_fields": processed_meta}
                    )
                    logging.debug(f"Successfully updated metadata for document {doc_id} using ES partial update")
                    return True
                except Exception as e:
                    logging.error(f"ES partial update failed for document {doc_id}: {e}")
                    # Fall back to delete+insert if partial update fails
                    logging.info(f"Falling back to delete+insert for document {doc_id}")

            # For Infinity or as fallback: use delete+insert
            logging.debug(f"[update_document_metadata] Using delete+insert method for doc_id: {doc_id}")
            cls.delete_document_metadata(doc_id, skip_empty_check=True)
            return cls.insert_document_metadata(doc_id, processed_meta)

        except Exception as e:
            logging.error(f"Error updating metadata for document {doc_id}: {e}")
            return False

    @classmethod
    @DB.connection_context()
    def delete_document_metadata(cls, doc_id: str, skip_empty_check: bool = False) -> bool:
        """
        Delete document metadata from ES/Infinity.
        Also drops the metadata table if it becomes empty (efficiently).
        If document has no metadata in the table, this is a no-op.

        Args:
            doc_id: Document ID
            skip_empty_check: If True, skip checking/dropping empty table (for bulk deletions)

        Returns:
            True if successful (or no metadata to delete), False otherwise
        """
        try:
            logging.debug(f"[METADATA DELETE] Starting metadata deletion for document: {doc_id}")
            # Get document with tenant_id
            doc_query = Document.select(Document, Knowledgebase.tenant_id).join(
                Knowledgebase, on=(Knowledgebase.id == Document.kb_id)
            ).where(Document.id == doc_id)

            doc = doc_query.first()
            if not doc:
                logging.warning(f"Document {doc_id} not found for metadata deletion")
                return False

            tenant_id = doc.knowledgebase.tenant_id
            kb_id = doc.kb_id
            index_name = cls._get_doc_meta_index_name(tenant_id)
            logging.debug(f"[delete_document_metadata] Deleting doc_id: {doc_id}, kb_id: {kb_id}, index: {index_name}")

            # Check if metadata table exists before attempting deletion
            # This is the key optimization - no table = no metadata = nothing to delete
            if not settings.docStoreConn.index_exist(index_name, ""):
                logging.debug(f"Metadata table {index_name} does not exist, skipping metadata deletion for document {doc_id}")
                return True  # No metadata to delete is considered success

            # Try to get the metadata to confirm it exists before deleting
            # This is more efficient than attempting delete on non-existent records
            try:
                existing_metadata = settings.docStoreConn.get(
                    doc_id,
                    index_name,
                    [""]  # Empty list for metadata tables
                )
                logging.debug(f"[METADATA DELETE] Get result: {existing_metadata is not None}")
                if not existing_metadata:
                    logging.debug(f"[METADATA DELETE] Document {doc_id} has no metadata in table, skipping deletion")
                    # Only check/drop table if not skipped (tenant deletion will handle it)
                    if not skip_empty_check:
                        cls._drop_empty_metadata_table(index_name, tenant_id)
                    return True  # No metadata to delete is success
            except Exception as e:
                # If get fails, document might not exist in metadata table, which is fine
                logging.error(f"[METADATA DELETE] Get failed: {e}")
                # Continue to check/drop table if needed

            # Delete from ES/Infinity (only if metadata exists)
            # For metadata tables, pass kb_id for the delete operation
            # The delete() method will detect it's a metadata table and skip the kb_id filter
            logging.debug(f"[METADATA DELETE] Deleting metadata with condition: {{'id': '{doc_id}'}}")
            deleted_count = settings.docStoreConn.delete(
                {"id": doc_id},
                index_name,
                kb_id  # Pass actual kb_id (delete() will handle metadata tables correctly)
            )
            logging.debug(f"[METADATA DELETE] Deleted count: {deleted_count}")

            # Only check if table should be dropped if not skipped (for bulk operations)
            # Note: delete operation already uses refresh=True, so data is immediately available
            if not skip_empty_check:
                # Check by querying the actual metadata table (not MySQL)
                cls._drop_empty_metadata_table(index_name, tenant_id)

            logging.debug(f"Successfully deleted metadata for document {doc_id}")
            return True

        except Exception as e:
            logging.error(f"Error deleting metadata for document {doc_id}: {e}")
            return False

    @classmethod
    def _drop_empty_metadata_table(cls, index_name: str, tenant_id: str) -> None:
        """
        Check if metadata table is empty and drop it if so.
        Uses optimized count query instead of full search.
        This prevents accumulation of empty metadata tables.

        Args:
            index_name: Metadata table/index name
            tenant_id: Tenant ID
        """
        try:
            logging.debug(f"[DROP EMPTY TABLE] Starting empty table check for: {index_name}")

            # Check if table exists first (cheap operation)
            if not settings.docStoreConn.index_exist(index_name, ""):
                logging.debug(f"[DROP EMPTY TABLE] Metadata table {index_name} does not exist, skipping")
                return

            logging.debug(f"[DROP EMPTY TABLE] Table {index_name} exists, checking if empty...")

            # Use ES count API for accurate count
            # Note: No need to refresh since delete operation already uses refresh=True
            try:
                count_response = settings.docStoreConn.es.count(index=index_name)
                total_count = count_response['count']
                logging.debug(f"[DROP EMPTY TABLE] ES count API result: {total_count} documents")
                is_empty = (total_count == 0)
            except Exception as e:
                logging.warning(f"[DROP EMPTY TABLE] Count API failed, falling back to search: {e}")
                # Fallback to search if count fails
                results = settings.docStoreConn.search(
                    select_fields=["id"],
                    highlight_fields=[],
                    condition={},
                    match_expressions=[],
                    order_by=OrderByExpr(),
                    offset=0,
                    limit=1,  # Only need 1 result to know if table is non-empty
                    index_names=index_name,
                    knowledgebase_ids=[""]  # Metadata tables don't filter by KB
                )

                logging.debug(f"[DROP EMPTY TABLE] Search results type: {type(results)}, results: {results}")

                # Check if empty based on return type (fallback search only)
                if isinstance(results, tuple) and len(results) == 2:
                    # Infinity returns (DataFrame, int)
                    df, total = results
                    logging.debug(f"[DROP EMPTY TABLE] Infinity format - total: {total}, df length: {len(df) if hasattr(df, '__len__') else 'N/A'}")
                    is_empty = (total == 0 or (hasattr(df, '__len__') and len(df) == 0))
                elif hasattr(results, 'get') and 'hits' in results:
                    # ES format - MUST check this before hasattr(results, '__len__')
                    # because ES response objects also have __len__
                    total = results.get('hits', {}).get('total', {})
                    hits = results.get('hits', {}).get('hits', [])

                    # ES 7.x+: total is a dict like {'value': 0, 'relation': 'eq'}
                    # ES 6.x: total is an int
                    if isinstance(total, dict):
                        total_count = total.get('value', 0)
                    else:
                        total_count = total

                    logging.debug(f"[DROP EMPTY TABLE] ES format - total: {total_count}, hits count: {len(hits)}")
                    is_empty = (total_count == 0 or len(hits) == 0)
                elif hasattr(results, '__len__'):
                    # DataFrame or list (check this AFTER ES format)
                    result_len = len(results)
                    logging.debug(f"[DROP EMPTY TABLE] List/DataFrame format - length: {result_len}")
                    is_empty = result_len == 0
                else:
                    logging.warning(f"[DROP EMPTY TABLE] Unknown result format: {type(results)}")
                    is_empty = False

            if is_empty:
                logging.debug(f"[DROP EMPTY TABLE] Metadata table {index_name} is empty, dropping it")
                drop_result = settings.docStoreConn.delete_idx(index_name, "")
                logging.debug(f"[DROP EMPTY TABLE] Drop result: {drop_result}")
            else:
                logging.debug(f"[DROP EMPTY TABLE] Metadata table {index_name} still has documents, keeping it")

        except Exception as e:
            # Log but don't fail - metadata deletion was successful
            logging.error(f"[DROP EMPTY TABLE] Failed to check/drop empty metadata table {index_name}: {e}")

    @classmethod
    @DB.connection_context()
    def get_document_metadata(cls, doc_id: str) -> Dict:
        """
        Get document metadata from ES/Infinity.

        Args:
            doc_id: Document ID

        Returns:
            Metadata dictionary, empty dict if not found
        """
        try:
            # Get document with tenant_id
            doc_query = Document.select(Document, Knowledgebase.tenant_id).join(
                Knowledgebase, on=(Knowledgebase.id == Document.kb_id)
            ).where(Document.id == doc_id)

            doc = doc_query.first()
            if not doc:
                logging.warning(f"Document {doc_id} not found")
                return {}

            # Extract fields
            doc_obj = doc
            tenant_id = doc.knowledgebase.tenant_id
            kb_id = doc_obj.kb_id
            index_name = cls._get_doc_meta_index_name(tenant_id)

            # Try to get metadata from ES/Infinity
            metadata_doc = settings.docStoreConn.get(
                doc_id,
                index_name,
                [kb_id]
            )

            if metadata_doc:
                # Extract and unflatten metadata
                return cls._extract_metadata(metadata_doc)

            return {}

        except Exception as e:
            logging.error(f"Error getting metadata for document {doc_id}: {e}")
            return {}

    @classmethod
    @DB.connection_context()
    def get_meta_by_kbs(cls, kb_ids: List[str]) -> Dict:
        """
        Get metadata for documents in knowledge bases (Legacy).

        Legacy metadata aggregator (backward-compatible).
        - Does NOT expand list values and a list is kept as one string key.
          Example: {"tags": ["foo","bar"]} -> meta["tags"]["['foo', 'bar']"] = [doc_id]
        - Expects meta_fields is a dict.
        Use when existing callers rely on the old list-as-string semantics.

        Args:
            kb_ids: List of knowledge base IDs

        Returns:
            Metadata dictionary in format: {field_name: {value: [doc_ids]}}
        """
        try:
            # Get tenant_id from first KB
            kb = Knowledgebase.get_by_id(kb_ids[0])
            if not kb:
                return {}

            tenant_id = kb.tenant_id
            index_name = cls._get_doc_meta_index_name(tenant_id)

            condition = {"kb_id": kb_ids}
            order_by = OrderByExpr()

            # Query with large limit
            results = settings.docStoreConn.search(
                select_fields=["*"],
                highlight_fields=[],
                condition=condition,
                match_expressions=[],
                order_by=order_by,
                offset=0,
                limit=10000,
                index_names=index_name,
                knowledgebase_ids=kb_ids
            )

            logging.debug(f"[get_meta_by_kbs] index_name: {index_name}, kb_ids: {kb_ids}")

            # Aggregate metadata (legacy: keeps lists as string keys)
            meta = {}

            # Use helper to iterate over results in any format
            for doc_id, doc in cls._iter_search_results(results):
                # Extract metadata fields (exclude system fields)
                doc_meta = cls._extract_metadata(doc)

                # Legacy: Keep lists as string keys (do NOT expand)
                for k, v in doc_meta.items():
                    if k not in meta:
                        meta[k] = {}
                    # If not list, make it a list
                    if not isinstance(v, list):
                        v = [v]
                    # Legacy: Use the entire list as a string key
                    # Skip nested lists/dicts
                    if isinstance(v, list) and any(isinstance(x, (list, dict)) for x in v):
                        continue
                    list_key = str(v)
                    if list_key not in meta[k]:
                        meta[k][list_key] = []
                    meta[k][list_key].append(doc_id)

            logging.debug(f"[get_meta_by_kbs] KBs: {kb_ids}, Returning metadata: {meta}")
            return meta

        except Exception as e:
            logging.error(f"Error getting metadata for KBs {kb_ids}: {e}")
            return {}

    @classmethod
    @DB.connection_context()
    def get_flatted_meta_by_kbs(cls, kb_ids: List[str]) -> Dict:
        """
        Get flattened metadata for documents in knowledge bases.

        - Parses stringified JSON meta_fields when possible and skips non-dict or unparsable values.
        - Expands list values into individual entries.
          Example: {"tags": ["foo","bar"], "author": "alice"} ->
            meta["tags"]["foo"] = [doc_id], meta["tags"]["bar"] = [doc_id], meta["author"]["alice"] = [doc_id]
        Prefer for metadata_condition filtering and scenarios that must respect list semantics.

        Args:
            kb_ids: List of knowledge base IDs

        Returns:
            Metadata dictionary in format: {field_name: {value: [doc_ids]}}
        """
        try:
            # Get tenant_id from first KB
            kb = Knowledgebase.get_by_id(kb_ids[0])
            if not kb:
                return {}

            tenant_id = kb.tenant_id
            index_name = cls._get_doc_meta_index_name(tenant_id)

            condition = {"kb_id": kb_ids}
            order_by = OrderByExpr()

            # Query with large limit
            results = settings.docStoreConn.search(
                select_fields=["*"],  # Get all fields
                highlight_fields=[],
                condition=condition,
                match_expressions=[],
                order_by=order_by,
                offset=0,
                limit=10000,
                index_names=index_name,
                knowledgebase_ids=kb_ids
            )

            logging.debug(f"[get_flatted_meta_by_kbs] index_name: {index_name}, kb_ids: {kb_ids}")
            logging.debug(f"[get_flatted_meta_by_kbs] results type: {type(results)}")

            # Aggregate metadata
            meta = {}

            # Use helper to iterate over results in any format
            for doc_id, doc in cls._iter_search_results(results):
                # Extract metadata fields (exclude system fields)
                doc_meta = cls._extract_metadata(doc)

                for k, v in doc_meta.items():
                    if k not in meta:
                        meta[k] = {}

                    values = v if isinstance(v, list) else [v]
                    for vv in values:
                        if vv is None:
                            continue
                        sv = str(vv)
                        if sv not in meta[k]:
                            meta[k][sv] = []
                        meta[k][sv].append(doc_id)

            logging.debug(f"[get_flatted_meta_by_kbs] KBs: {kb_ids}, Returning metadata: {meta}")
            return meta

        except Exception as e:
            logging.error(f"Error getting flattened metadata for KBs {kb_ids}: {e}")
            return {}

    @classmethod
    def get_metadata_for_documents(cls, doc_ids: Optional[List[str]], kb_id: str) -> Dict[str, Dict]:
        """
        Get metadata fields for specific documents.
        Returns a mapping of doc_id -> meta_fields

        Args:
            doc_ids: List of document IDs (if None, gets all documents with metadata for the KB)
            kb_id: Knowledge base ID

        Returns:
            Dictionary mapping doc_id to meta_fields dict
        """
        try:
            results = cls._search_metadata(kb_id, condition={"kb_id": kb_id})
            if not results:
                return {}

            # Build mapping: doc_id -> meta_fields
            meta_mapping = {}

            # If doc_ids is provided, create a set for efficient lookup
            doc_ids_set = set(doc_ids) if doc_ids else None

            # Use helper to iterate over results in any format
            for doc_id, doc in cls._iter_search_results(results):
                # Filter by doc_ids if provided
                if doc_ids_set is not None and doc_id not in doc_ids_set:
                    continue

                # Extract metadata (handles both JSON strings and dicts)
                doc_meta = cls._extract_metadata(doc)
                if doc_meta:
                    meta_mapping[doc_id] = doc_meta

            logging.debug(f"[get_metadata_for_documents] Found metadata for {len(meta_mapping)}/{len(doc_ids) if doc_ids else 'all'} documents")
            return meta_mapping

        except Exception as e:
            logging.error(f"Error getting metadata for documents: {e}")
            return {}

    @classmethod
    @DB.connection_context()
    def get_metadata_summary(cls, kb_id: str, doc_ids=None) -> Dict:
        """
        Get metadata summary for documents in a knowledge base.

        Args:
            kb_id: Knowledge base ID
            doc_ids: Optional list of document IDs to filter by

        Returns:
            Dictionary with metadata field statistics in format:
            {
                "field_name": {
                    "type": "string" | "number" | "list" | "time",
                    "values": [("value1", count1), ("value2", count2), ...]  # sorted by count desc
                }
            }
        """
        def _is_time_string(value: str) -> bool:
            """Check if a string value is an ISO 8601 datetime (e.g., '2026-02-03T00:00:00')."""
            if not isinstance(value, str):
                return False
            return bool(re.match(r'^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$', value))

        def _meta_value_type(value):
            """Determine the type of a metadata value."""
            if value is None:
                return None
            if isinstance(value, list):
                return "list"
            if isinstance(value, bool):
                return "string"
            if isinstance(value, (int, float)):
                return "number"
            if isinstance(value, str) and _is_time_string(value):
                return "time"
            return "string"

        try:
            results = cls._search_metadata(kb_id, condition={"kb_id": kb_id})
            if not results:
                return {}

            # If doc_ids are provided, we'll filter after the search
            doc_ids_set = set(doc_ids) if doc_ids else None

            # Aggregate metadata
            summary = {}
            type_counter = {}

            logging.debug(f"[METADATA SUMMARY] KB: {kb_id}, doc_ids: {doc_ids}")

            # Use helper to iterate over results in any format
            for doc_id, doc in cls._iter_search_results(results):
                # Check doc_ids filter
                if doc_ids_set and doc_id not in doc_ids_set:
                    continue

                doc_meta = cls._extract_metadata(doc)

                for k, v in doc_meta.items():
                    # Track type counts for this field
                    value_type = _meta_value_type(v)
                    if value_type:
                        if k not in type_counter:
                            type_counter[k] = {}
                        type_counter[k][value_type] = type_counter[k].get(value_type, 0) + 1

                    # Aggregate value counts
                    values = v if isinstance(v, list) else [v]
                    for vv in values:
                        if vv is None:
                            continue
                        sv = str(vv)
                        if k not in summary:
                            summary[k] = {}
                        summary[k][sv] = summary[k].get(sv, 0) + 1

            # Build result with type information and sorted values
            result = {}
            for k, v in summary.items():
                values = sorted([(val, cnt) for val, cnt in v.items()], key=lambda x: x[1], reverse=True)
                type_counts = type_counter.get(k, {})
                value_type = "string"
                if type_counts:
                    value_type = max(type_counts.items(), key=lambda item: item[1])[0]
                result[k] = {"type": value_type, "values": values}

            logging.debug(f"[METADATA SUMMARY] Final result: {result}")
            return result

        except Exception as e:
            logging.error(f"Error getting metadata summary for KB {kb_id}: {e}")
            return {}

    @classmethod
    @DB.connection_context()
    def batch_update_metadata(cls, kb_id: str, doc_ids: List[str], updates=None, deletes=None) -> int:
        """
        Batch update metadata for documents in a knowledge base.

        Args:
            kb_id: Knowledge base ID
            doc_ids: List of document IDs to update
            updates: List of update operations, each with:
                - key: field name to update
                - value: new value
                - match (optional): only update if current value matches this
            deletes: List of delete operations, each with:
                - key: field name to delete from
                - value (optional): specific value to delete (if not provided, deletes the entire field)

        Returns:
            Number of documents updated

        Examples:
            updates = [{"key": "author", "value": "John"}]
            updates = [{"key": "tags", "value": "new", "match": "old"}]  # Replace "old" with "new" in tags list
            deletes = [{"key": "author"}]  # Delete entire author field
            deletes = [{"key": "tags", "value": "obsolete"}]  # Remove "obsolete" from tags list
        """
        updates = updates or []
        deletes = deletes or []
        if not doc_ids:
            return 0

        def _normalize_meta(meta):
            """Normalize metadata to a dict."""
            if isinstance(meta, str):
                try:
                    meta = json.loads(meta)
                except Exception:
                    return {}
            if not isinstance(meta, dict):
                return {}
            return deepcopy(meta)

        def _str_equal(a, b):
            """Compare two values as strings."""
            return str(a) == str(b)

        def _apply_updates(meta):
            """Apply update operations to metadata."""
            changed = False
            for upd in updates:
                key = upd.get("key")
                if not key:
                    continue

                new_value = upd.get("value")
                match_value = upd.get("match", None)
                match_provided = match_value is not None and match_value != ""

                if key not in meta:
                    if match_provided:
                        continue
                    meta[key] = dedupe_list(new_value) if isinstance(new_value, list) else new_value
                    changed = True
                    continue

                if isinstance(meta[key], list):
                    if not match_provided:
                        # No match provided, append new_value to the list
                        if isinstance(new_value, list):
                            meta[key] = dedupe_list(meta[key] + new_value)
                        else:
                            meta[key] = dedupe_list(meta[key] + [new_value])
                        changed = True
                    else:
                        # Replace items matching match_value with new_value
                        replaced = False
                        new_list = []
                        for item in meta[key]:
                            if _str_equal(item, match_value):
                                new_list.append(new_value)
                                replaced = True
                            else:
                                new_list.append(item)
                        if replaced:
                            meta[key] = dedupe_list(new_list)
                            changed = True
                else:
                    if not match_provided:
                        meta[key] = new_value
                        changed = True
                    else:
                        if _str_equal(meta[key], match_value):
                            meta[key] = new_value
                            changed = True
            return changed

        def _apply_deletes(meta):
            """Apply delete operations to metadata."""
            changed = False
            for d in deletes:
                key = d.get("key")
                if not key or key not in meta:
                    continue
                value = d.get("value", None)
                if isinstance(meta[key], list):
                    if value is None:
                        del meta[key]
                        changed = True
                        continue
                    new_list = [item for item in meta[key] if not _str_equal(item, value)]
                    if len(new_list) != len(meta[key]):
                        if new_list:
                            meta[key] = new_list
                        else:
                            del meta[key]
                        changed = True
                else:
                    if value is None or _str_equal(meta[key], value):
                        del meta[key]
                        changed = True
            return changed

        try:
            results = cls._search_metadata(kb_id, condition=None)
            if not results:
                results = []  # Treat as empty list if None

            updated_docs = 0
            doc_ids_set = set(doc_ids)
            found_doc_ids = set()

            logging.debug(f"[batch_update_metadata] Searching for doc_ids: {doc_ids}")

            # Use helper to iterate over results in any format
            for doc_id, doc in cls._iter_search_results(results):
                # Filter to only process requested doc_ids
                if doc_id not in doc_ids_set:
                    continue

                found_doc_ids.add(doc_id)

                # Get current metadata
                current_meta = cls._extract_metadata(doc)
                meta = _normalize_meta(current_meta)
                original_meta = deepcopy(meta)

                logging.debug(f"[batch_update_metadata] Doc {doc_id}: current_meta={current_meta}, meta={meta}")
                logging.debug(f"[batch_update_metadata] Updates to apply: {updates}, Deletes: {deletes}")

                # Apply updates and deletes
                changed = _apply_updates(meta)
                logging.debug(f"[batch_update_metadata] After _apply_updates: changed={changed}, meta={meta}")
                changed = _apply_deletes(meta) or changed
                logging.debug(f"[batch_update_metadata] After _apply_deletes: changed={changed}, meta={meta}")

                # Update if changed
                if changed and meta != original_meta:
                    logging.debug(f"[batch_update_metadata] Updating doc_id: {doc_id}, meta: {meta}")
                    # If metadata is empty, delete the row entirely instead of keeping empty metadata
                    if not meta:
                        cls.delete_document_metadata(doc_id, skip_empty_check=True)
                    else:
                        cls.update_document_metadata(doc_id, meta)
                    updated_docs += 1

            # Handle documents that don't have metadata rows yet
            # These documents weren't in the search results, so we need to insert new metadata for them
            missing_doc_ids = doc_ids_set - found_doc_ids
            if missing_doc_ids and updates:
                logging.debug(f"[batch_update_metadata] Inserting new metadata for documents without metadata rows: {missing_doc_ids}")
                for doc_id in missing_doc_ids:
                    # Apply updates to create new metadata
                    meta = {}
                    _apply_updates(meta)
                    if meta:
                        # Only insert if there's actual metadata to add
                        cls.update_document_metadata(doc_id, meta)
                        updated_docs += 1
                        logging.debug(f"[batch_update_metadata] Inserted metadata for doc_id: {doc_id}, meta: {meta}")

            logging.debug(f"[batch_update_metadata] KB: {kb_id}, doc_ids: {doc_ids}, updated: {updated_docs}")
            return updated_docs

        except Exception as e:
            logging.error(f"Error in batch_update_metadata for KB {kb_id}: {e}")
            return 0
