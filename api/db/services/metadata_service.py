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
Metadata Management Service for Hierarchical Retrieval.

Provides batch CRUD operations for document metadata to support:
- Efficient metadata filtering in Tier 2 of hierarchical retrieval
- Bulk metadata updates across multiple documents
- Metadata schema management per knowledge base
"""

import logging
from typing import List, Dict, Any, Optional, Tuple

from peewee import fn

from api.db.db_models import DB, Document
from api.db.services.document_service import DocumentService


class MetadataService:
    """
    Service for managing document metadata in batch operations.
    
    Supports the hierarchical retrieval architecture by providing
    efficient metadata management for document filtering.
    """
    
    @classmethod
    @DB.connection_context()
    def batch_get_metadata(
        cls,
        doc_ids: List[str],
        fields: Optional[List[str]] = None
    ) -> Dict[str, Dict[str, Any]]:
        """
        Get metadata for multiple documents.
        
        Args:
            doc_ids: List of document IDs
            fields: Optional list of specific metadata fields to retrieve
            
        Returns:
            Dict mapping doc_id to metadata dict
        """
        if not doc_ids:
            return {}
        
        result = {}
        docs = Document.select(
            Document.id, 
            Document.meta_fields,
            Document.name
        ).where(Document.id.in_(doc_ids))
        
        for doc in docs:
            meta = doc.meta_fields or {}
            if fields:
                # Filter to requested fields only
                meta = {k: v for k, v in meta.items() if k in fields}
            result[doc.id] = {
                "doc_id": doc.id,
                "doc_name": doc.name,
                "metadata": meta
            }
        
        return result
    
    @classmethod
    @DB.connection_context()
    def batch_update_metadata(
        cls,
        updates: List[Dict[str, Any]],
        merge: bool = True
    ) -> Tuple[int, List[str]]:
        """
        Update metadata for multiple documents in batch.
        
        Args:
            updates: List of dicts with 'doc_id' and 'metadata' keys
            merge: If True, merge with existing metadata; if False, replace
            
        Returns:
            Tuple of (success_count, list of failed doc_ids)
        """
        if not updates:
            return 0, []
        
        success_count = 0
        failed_ids = []
        
        for update in updates:
            doc_id = update.get("doc_id")
            new_metadata = update.get("metadata", {})
            
            if not doc_id:
                continue
            
            try:
                if merge:
                    # Get existing metadata and merge
                    doc = Document.get_or_none(Document.id == doc_id)
                    if doc:
                        existing = doc.meta_fields or {}
                        existing.update(new_metadata)
                        new_metadata = existing
                
                DocumentService.update_meta_fields(doc_id, new_metadata)
                success_count += 1
                
            except Exception as e:
                logging.error(f"Failed to update metadata for doc {doc_id}: {e}")
                failed_ids.append(doc_id)
        
        logging.info(f"Batch metadata update: {success_count} succeeded, {len(failed_ids)} failed")
        return success_count, failed_ids
    
    @classmethod
    @DB.connection_context()
    def batch_delete_metadata_fields(
        cls,
        doc_ids: List[str],
        fields: List[str]
    ) -> Tuple[int, List[str]]:
        """
        Delete specific metadata fields from multiple documents.
        
        Args:
            doc_ids: List of document IDs
            fields: List of metadata field names to delete
            
        Returns:
            Tuple of (success_count, list of failed doc_ids)
        """
        if not doc_ids or not fields:
            return 0, []
        
        success_count = 0
        failed_ids = []
        
        docs = Document.select(
            Document.id, 
            Document.meta_fields
        ).where(Document.id.in_(doc_ids))
        
        for doc in docs:
            try:
                meta = doc.meta_fields or {}
                modified = False
                
                for field in fields:
                    if field in meta:
                        del meta[field]
                        modified = True
                
                if modified:
                    DocumentService.update_meta_fields(doc.id, meta)
                    success_count += 1
                    
            except Exception as e:
                logging.error(f"Failed to delete metadata fields for doc {doc.id}: {e}")
                failed_ids.append(doc.id)
        
        return success_count, failed_ids
    
    @classmethod
    @DB.connection_context()
    def batch_set_metadata_field(
        cls,
        doc_ids: List[str],
        field_name: str,
        field_value: Any
    ) -> Tuple[int, List[str]]:
        """
        Set a specific metadata field to the same value for multiple documents.
        
        Useful for bulk categorization or tagging.
        
        Args:
            doc_ids: List of document IDs
            field_name: Name of the metadata field
            field_value: Value to set
            
        Returns:
            Tuple of (success_count, list of failed doc_ids)
        """
        if not doc_ids or not field_name:
            return 0, []
        
        updates = [
            {"doc_id": doc_id, "metadata": {field_name: field_value}}
            for doc_id in doc_ids
        ]
        
        return cls.batch_update_metadata(updates, merge=True)
    
    @classmethod
    @DB.connection_context()
    def get_metadata_schema(cls, kb_id: str) -> Dict[str, Dict[str, Any]]:
        """
        Get the metadata schema for a knowledge base.
        
        Analyzes all documents in the KB to determine available
        metadata fields and their types/values.
        
        Args:
            kb_id: Knowledge base ID
            
        Returns:
            Dict mapping field names to field info (type, sample values, count)
        """
        schema = {}
        
        docs = Document.select(
            Document.meta_fields
        ).where(Document.kb_id == kb_id)
        
        for doc in docs:
            meta = doc.meta_fields or {}
            for field_name, field_value in meta.items():
                if field_name not in schema:
                    schema[field_name] = {
                        "type": type(field_value).__name__,
                        "sample_values": set(),
                        "count": 0
                    }
                
                schema[field_name]["count"] += 1
                
                # Collect sample values (limit to 10)
                if len(schema[field_name]["sample_values"]) < 10:
                    try:
                        schema[field_name]["sample_values"].add(str(field_value)[:100])
                    except Exception:
                        pass
        
        # Convert sets to lists for JSON serialization
        for field_name in schema:
            schema[field_name]["sample_values"] = list(schema[field_name]["sample_values"])
        
        return schema
    
    @classmethod
    @DB.connection_context()
    def search_by_metadata(
        cls,
        kb_id: str,
        filters: Dict[str, Any],
        limit: int = 100
    ) -> List[Dict[str, Any]]:
        """
        Search documents by metadata filters.
        
        Args:
            kb_id: Knowledge base ID
            filters: Dict of field_name -> value or {operator: value}
            limit: Maximum number of results
            
        Returns:
            List of matching documents with their metadata
        """
        docs = Document.select(
            Document.id,
            Document.name,
            Document.meta_fields
        ).where(Document.kb_id == kb_id)
        
        results = []
        for doc in docs:
            meta = doc.meta_fields or {}
            matches = True
            
            for field_name, condition in filters.items():
                doc_value = meta.get(field_name)
                
                if isinstance(condition, dict):
                    # Operator-based condition
                    op = list(condition.keys())[0]
                    val = condition[op]
                    
                    if op == "equals":
                        matches = str(doc_value) == str(val)
                    elif op == "contains":
                        matches = str(val).lower() in str(doc_value).lower()
                    elif op == "starts_with":
                        matches = str(doc_value).lower().startswith(str(val).lower())
                    elif op == "in":
                        matches = doc_value in val
                    elif op == "gt":
                        matches = float(doc_value) > float(val) if doc_value else False
                    elif op == "lt":
                        matches = float(doc_value) < float(val) if doc_value else False
                else:
                    # Simple equality
                    matches = str(doc_value) == str(condition)
                
                if not matches:
                    break
            
            if matches:
                results.append({
                    "doc_id": doc.id,
                    "doc_name": doc.name,
                    "metadata": meta
                })
                
                if len(results) >= limit:
                    break
        
        return results
    
    @classmethod
    @DB.connection_context()
    def get_metadata_statistics(cls, kb_id: str) -> Dict[str, Any]:
        """
        Get statistics about metadata usage in a knowledge base.
        
        Args:
            kb_id: Knowledge base ID
            
        Returns:
            Dict with statistics about metadata fields
        """
        total_docs = Document.select(fn.COUNT(Document.id)).where(
            Document.kb_id == kb_id
        ).scalar()
        
        docs_with_metadata = 0
        field_usage = {}
        
        docs = Document.select(Document.meta_fields).where(Document.kb_id == kb_id)
        
        for doc in docs:
            meta = doc.meta_fields or {}
            if meta:
                docs_with_metadata += 1
                for field_name in meta.keys():
                    field_usage[field_name] = field_usage.get(field_name, 0) + 1
        
        return {
            "total_documents": total_docs,
            "documents_with_metadata": docs_with_metadata,
            "metadata_coverage": docs_with_metadata / total_docs if total_docs > 0 else 0,
            "field_usage": field_usage,
            "unique_fields": len(field_usage)
        }
    
    @classmethod
    @DB.connection_context()
    def copy_metadata(
        cls,
        source_doc_id: str,
        target_doc_ids: List[str],
        fields: Optional[List[str]] = None
    ) -> Tuple[int, List[str]]:
        """
        Copy metadata from one document to multiple target documents.
        
        Args:
            source_doc_id: Source document ID
            target_doc_ids: List of target document IDs
            fields: Optional list of specific fields to copy (all if None)
            
        Returns:
            Tuple of (success_count, list of failed doc_ids)
        """
        source_doc = Document.get_or_none(Document.id == source_doc_id)
        if not source_doc:
            return 0, target_doc_ids
        
        source_meta = source_doc.meta_fields or {}
        
        if fields:
            source_meta = {k: v for k, v in source_meta.items() if k in fields}
        
        if not source_meta:
            return 0, []
        
        updates = [
            {"doc_id": doc_id, "metadata": source_meta.copy()}
            for doc_id in target_doc_ids
        ]
        
        return cls.batch_update_metadata(updates, merge=True)
