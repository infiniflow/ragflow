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
Metadata Management API for Hierarchical Retrieval.

Provides REST endpoints for batch CRUD operations on document metadata,
supporting the hierarchical retrieval architecture's Tier 2 document filtering.
"""

from quart import request
from api.apps import current_user, login_required
from api.common.check_team_permission import check_kb_team_permission
from api.db.services.metadata_service import MetadataService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils.api_utils import (
    get_json_result,
    server_error_response,
)
from common.constants import RetCode


@manager.route("/batch/get", methods=["POST"])  # noqa: F821
@login_required
async def batch_get_metadata():
    """
    Get metadata for multiple documents.
    
    Request body:
    {
        "doc_ids": ["doc1", "doc2", ...],
        "fields": ["field1", "field2", ...]  // optional
    }
    
    Returns:
    {
        "doc1": {"doc_id": "doc1", "doc_name": "...", "metadata": {...}},
        ...
    }
    """
    try:
        req = await request.json
        doc_ids = req.get("doc_ids", [])
        fields = req.get("fields")
        
        if not doc_ids:
            return get_json_result(
                data={},
                message="No document IDs provided",
                code=RetCode.ARGUMENT_ERROR
            )
        
        result = MetadataService.batch_get_metadata(doc_ids, fields)
        return get_json_result(data=result)
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/batch/update", methods=["POST"])  # noqa: F821
@login_required
async def batch_update_metadata():
    """
    Update metadata for multiple documents.
    
    Request body:
    {
        "updates": [
            {"doc_id": "doc1", "metadata": {"field1": "value1", ...}},
            {"doc_id": "doc2", "metadata": {"field2": "value2", ...}},
            ...
        ],
        "merge": true  // optional, default true. If false, replaces all metadata
    }
    
    Returns:
    {
        "success_count": 5,
        "failed_ids": ["doc3"]
    }
    """
    try:
        req = await request.json
        updates = req.get("updates", [])
        merge = req.get("merge", True)
        
        if not updates:
            return get_json_result(
                data={"success_count": 0, "failed_ids": []},
                message="No updates provided",
                code=RetCode.ARGUMENT_ERROR
            )
        
        success_count, failed_ids = MetadataService.batch_update_metadata(updates, merge)
        
        return get_json_result(data={
            "success_count": success_count,
            "failed_ids": failed_ids
        })
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/batch/delete-fields", methods=["POST"])  # noqa: F821
@login_required
async def batch_delete_metadata_fields():
    """
    Delete specific metadata fields from multiple documents.
    
    Request body:
    {
        "doc_ids": ["doc1", "doc2", ...],
        "fields": ["field1", "field2", ...]
    }
    
    Returns:
    {
        "success_count": 5,
        "failed_ids": []
    }
    """
    try:
        req = await request.json
        doc_ids = req.get("doc_ids", [])
        fields = req.get("fields", [])
        
        if not doc_ids or not fields:
            return get_json_result(
                data={"success_count": 0, "failed_ids": []},
                message="doc_ids and fields are required",
                code=RetCode.ARGUMENT_ERROR
            )
        
        success_count, failed_ids = MetadataService.batch_delete_metadata_fields(doc_ids, fields)
        
        return get_json_result(data={
            "success_count": success_count,
            "failed_ids": failed_ids
        })
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/batch/set-field", methods=["POST"])  # noqa: F821
@login_required
async def batch_set_metadata_field():
    """
    Set a specific field to the same value for multiple documents.
    
    Useful for bulk categorization or tagging.
    
    Request body:
    {
        "doc_ids": ["doc1", "doc2", ...],
        "field_name": "category",
        "field_value": "Technical"
    }
    
    Returns:
    {
        "success_count": 5,
        "failed_ids": []
    }
    """
    try:
        req = await request.json
        doc_ids = req.get("doc_ids", [])
        field_name = req.get("field_name")
        field_value = req.get("field_value")
        
        if not doc_ids or not field_name:
            return get_json_result(
                data={"success_count": 0, "failed_ids": []},
                message="doc_ids and field_name are required",
                code=RetCode.ARGUMENT_ERROR
            )
        
        success_count, failed_ids = MetadataService.batch_set_metadata_field(
            doc_ids, field_name, field_value
        )
        
        return get_json_result(data={
            "success_count": success_count,
            "failed_ids": failed_ids
        })
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/schema/<kb_id>", methods=["GET"])  # noqa: F821
@login_required
async def get_metadata_schema(kb_id):
    """
    Get the metadata schema for a knowledge base.
    
    Returns available metadata fields, their types, and sample values.
    
    Returns:
    {
        "field1": {"type": "str", "sample_values": ["a", "b"], "count": 10},
        ...
    }
    """
    try:
        # Check KB access permission
        kb = KnowledgebaseService.get_by_id(kb_id)
        if not kb:
            return get_json_result(
                data={},
                message="Knowledge base not found",
                code=RetCode.DATA_ERROR
            )
        
        if not check_kb_team_permission(current_user.id, kb_id):
            return get_json_result(
                data={},
                message="No permission to access this knowledge base",
                code=RetCode.PERMISSION_ERROR
            )
        
        schema = MetadataService.get_metadata_schema(kb_id)
        return get_json_result(data=schema)
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/statistics/<kb_id>", methods=["GET"])  # noqa: F821
@login_required
async def get_metadata_statistics(kb_id):
    """
    Get statistics about metadata usage in a knowledge base.
    
    Returns:
    {
        "total_documents": 100,
        "documents_with_metadata": 80,
        "metadata_coverage": 0.8,
        "field_usage": {"category": 50, "author": 30},
        "unique_fields": 5
    }
    """
    try:
        # Check KB access permission
        kb = KnowledgebaseService.get_by_id(kb_id)
        if not kb:
            return get_json_result(
                data={},
                message="Knowledge base not found",
                code=RetCode.DATA_ERROR
            )
        
        if not check_kb_team_permission(current_user.id, kb_id):
            return get_json_result(
                data={},
                message="No permission to access this knowledge base",
                code=RetCode.PERMISSION_ERROR
            )
        
        stats = MetadataService.get_metadata_statistics(kb_id)
        return get_json_result(data=stats)
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/search", methods=["POST"])  # noqa: F821
@login_required
async def search_by_metadata():
    """
    Search documents by metadata filters.
    
    Request body:
    {
        "kb_id": "kb123",
        "filters": {
            "category": "Technical",
            "author": {"contains": "John"},
            "year": {"gt": 2020}
        },
        "limit": 100
    }
    
    Supported operators: equals, contains, starts_with, in, gt, lt
    
    Returns:
    [
        {"doc_id": "doc1", "doc_name": "...", "metadata": {...}},
        ...
    ]
    """
    try:
        req = await request.json
        kb_id = req.get("kb_id")
        filters = req.get("filters", {})
        limit = req.get("limit", 100)
        
        if not kb_id:
            return get_json_result(
                data=[],
                message="kb_id is required",
                code=RetCode.ARGUMENT_ERROR
            )
        
        # Check KB access permission
        if not check_kb_team_permission(current_user.id, kb_id):
            return get_json_result(
                data=[],
                message="No permission to access this knowledge base",
                code=RetCode.PERMISSION_ERROR
            )
        
        results = MetadataService.search_by_metadata(kb_id, filters, limit)
        return get_json_result(data=results)
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/copy", methods=["POST"])  # noqa: F821
@login_required
async def copy_metadata():
    """
    Copy metadata from one document to multiple target documents.
    
    Request body:
    {
        "source_doc_id": "doc1",
        "target_doc_ids": ["doc2", "doc3", ...],
        "fields": ["field1", "field2"]  // optional, copies all if not specified
    }
    
    Returns:
    {
        "success_count": 5,
        "failed_ids": []
    }
    """
    try:
        req = await request.json
        source_doc_id = req.get("source_doc_id")
        target_doc_ids = req.get("target_doc_ids", [])
        fields = req.get("fields")
        
        if not source_doc_id or not target_doc_ids:
            return get_json_result(
                data={"success_count": 0, "failed_ids": []},
                message="source_doc_id and target_doc_ids are required",
                code=RetCode.ARGUMENT_ERROR
            )
        
        success_count, failed_ids = MetadataService.copy_metadata(
            source_doc_id, target_doc_ids, fields
        )
        
        return get_json_result(data={
            "success_count": success_count,
            "failed_ids": failed_ids
        })
        
    except Exception as e:
        return server_error_response(e)
