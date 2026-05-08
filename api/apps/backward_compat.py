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
"""
Backward compatibility layer for deprecated API endpoints.

This module adds support for old API routes that were deprecated during the
RESTful API migration. Each deprecated route forwards to the corresponding
new API implementation.

Deprecated APIs and their replacements:
- POST /api/v1/agents/{agent_id}/completions -> POST /api/v1/agents/chat/completion
- POST /api/v1/chats/{chat_id}/completions -> POST /api/v1/chat/completions
- POST /api/v1/chats_openai/{chat_id}/chat/completions -> POST /api/v1/openai/{chat_id}/chat/completions
- PUT /api/v1/chats/{chat_id}/sessions/{session_id} -> PATCH /api/v1/chats/{chat_id}/sessions/{session_id}
- DELETE /api/v1/chats -> DELETE /api/v1/chats/{chat_id} (with body)
- POST /api/v1/file/convert -> POST /api/v1/files/link-to-datasets
- GET /api/v1/file/* -> GET /api/v1/files*
- POST /api/v1/file/* -> POST /api/v1/files*
- GET /api/v1/document/get/{doc_id} -> GET /api/v1/documents/{doc_id}/preview
- GET /api/v1/document/download/{doc_id} -> GET /api/v1/documents/{doc_id}/download
- POST /api/v1/sessions/related_questions -> POST /api/v1/chat/recommandation
- PUT (chunk update) -> PATCH (chunk update)
"""
import logging

from quart import Blueprint, request

from api.apps import login_required
from api.apps.restful_apis import chat_api, file_api, file2document_api, chunk_api, openai_api, document_api
from api.apps.restful_apis import agent_api
from api.apps.services import file_api_service
from api.utils.api_utils import get_data_error_result, get_json_result, add_tenant_id_to_kwargs

manager = Blueprint("backward_compat", __name__)


# =============================================================================
# Chat Completion APIs
# =============================================================================

@manager.route("/chats/<chat_id>/completions", methods=["POST"])
@login_required
async def deprecated_chat_completions(chat_id):
    """
    Deprecated: Use POST /api/v1/chat/completions instead.

    Old path: POST /api/v1/chats/{chat_id}/completions
    New path: POST /api/v1/chat/completions
    """
    logging.warning(
        "API endpoint /api/v1/chats/%s/completions is deprecated. "
        "Please use /api/v1/chat/completions instead.",
        chat_id,
    )
    # Forward to the new API implementation
    return await chat_api.session_completion(chat_id)


@manager.route("/chats_openai/<chat_id>/chat/completions", methods=["POST"])
@login_required
async def deprecated_openai_chat_completions(chat_id):
    """
    Deprecated: Use POST /api/v1/openai/{chat_id}/chat/completions instead.

    Old path: POST /api/v1/chats_openai/{chat_id}/chat/completions
    New path: POST /api/v1/openai/{chat_id}/chat/completions
    """
    logging.warning(
        "API endpoint /api/v1/chats_openai/%s/chat/completions is deprecated. "
        "Please use /api/v1/openai/%s/chat/completions instead.",
        chat_id, chat_id,
    )
    # Forward to the new API implementation
    return await openai_api.openai_chat_completions(chat_id)


# =============================================================================
# Chat Session APIs
# =============================================================================

@manager.route("/chats/<chat_id>/sessions/<session_id>", methods=["PUT"])
@login_required
async def deprecated_update_session(chat_id, session_id):
    """
    Deprecated: Use PATCH /api/v1/chats/{chat_id}/sessions/{session_id} instead.

    Old path: PUT /api/v1/chats/{chat_id}/sessions/{session_id}
    New path: PATCH /api/v1/chats/{chat_id}/sessions/{session_id}
    """
    logging.warning(
        "API endpoint PUT /api/v1/chats/%s/sessions/%s is deprecated. "
        "Please use PATCH /api/v1/chats/%s/sessions/%s instead.",
        chat_id, session_id, chat_id, session_id,
    )
    # Forward to the new API implementation
    return await chat_api.update_session(chat_id, session_id)


# =============================================================================
# File APIs (Old /api/v1/file/* -> New /api/v1/files*)
# =============================================================================

@manager.route("/file/get/<file_id>", methods=["GET"])
@login_required
async def deprecated_file_get(file_id):
    """
    Deprecated: Use GET /api/v1/files/{file_id} instead.

    Old path: GET /api/v1/file/get/{file_id}
    New path: GET /api/v1/files/{file_id}
    """
    logging.warning(
        "API endpoint /api/v1/file/get/%s is deprecated. "
        "Please use /api/v1/files/%s instead.",
        file_id, file_id,
    )
    # Forward to the new API implementation (download)
    return await file_api.download(file_id=file_id)


@manager.route("/file/list", methods=["GET"])
@login_required
async def deprecated_file_list():
    """
    Deprecated: Use GET /api/v1/files instead.

    Old path: GET /api/v1/file/list?...
    New path: GET /api/v1/files?...
    """
    logging.warning(
        "API endpoint /api/v1/file/list is deprecated. "
        "Please use /api/v1/files instead."
    )
    # Forward to the new API implementation
    return await file_api.list_files()


@manager.route("/file/all_parent_folder", methods=["GET"])
@login_required
async def deprecated_file_all_parent_folder():
    """
    Deprecated: Use GET /api/v1/files/{file_id}/ancestors instead.

    Old path: GET /api/v1/file/all_parent_folder?file_id=...
    New path: GET /api/v1/files/{file_id}/ancestors
    """
    file_id = request.args.get("file_id")
    if not file_id:
        return get_data_error_result(message="`file_id` query parameter is required")
    logging.warning(
        "API endpoint /api/v1/file/all_parent_folder is deprecated. "
        "Please use /api/v1/files/%s/ancestors instead.",
        file_id,
    )
    # Forward to the new API implementation
    return await file_api.ancestors(file_id=file_id)


@manager.route("/file/parent_folder", methods=["GET"])
@login_required
async def deprecated_file_parent_folder():
    """
    Deprecated: Use GET /api/v1/files/{file_id}/parent instead.

    Old path: GET /api/v1/file/parent_folder?file_id=...
    New path: GET /api/v1/files/{file_id}/parent
    """
    file_id = request.args.get("file_id")
    if not file_id:
        return get_data_error_result(message="`file_id` query parameter is required")
    logging.warning(
        "API endpoint /api/v1/file/parent_folder is deprecated. "
        "Please use /api/v1/files/%s/parent instead.",
        file_id,
    )
    # Forward to the new API implementation
    return await file_api.parent_folder(file_id=file_id)


@manager.route("/file/root_folder", methods=["GET"])
@login_required
async def deprecated_file_root_folder():
    """
    Deprecated: Root folder is now accessible via GET /api/v1/files with parent_id=...

    Old path: GET /api/v1/file/root_folder
    New path: GET /api/v1/files?parent_id=<root_id>
    """
    logging.warning(
        "API endpoint /api/v1/file/root_folder is deprecated. "
        "Please use /api/v1/files with appropriate parent_id instead."
    )
    # Forward to the new API implementation with empty parent_id to get root
    return await file_api.list_files()


@manager.route("/file/create", methods=["POST"])
@login_required
@add_tenant_id_to_kwargs
async def deprecated_file_create(tenant_id=None):
    """
    Deprecated: Use POST /api/v1/files instead.

    Old path: POST /api/v1/file/create
    New path: POST /api/v1/files
    """
    logging.warning(
        "API endpoint /api/v1/file/create is deprecated. "
        "Please use POST /api/v1/files instead."
    )
    # Forward to the new API implementation
    return await file_api.create_or_upload(tenant_id=tenant_id)


@manager.route("/file/upload", methods=["POST"])
@login_required
@add_tenant_id_to_kwargs
async def deprecated_file_upload(tenant_id=None):
    """
    Deprecated: Use POST /api/v1/files (with multipart/form-data) instead.

    Old path: POST /api/v1/file/upload
    New path: POST /api/v1/files
    """
    logging.warning(
        "API endpoint /api/v1/file/upload is deprecated. "
        "Please use POST /api/v1/files with multipart/form-data instead."
    )
    # Forward to the new API implementation
    return await file_api.create_or_upload(tenant_id=tenant_id)


@manager.route("/file/convert", methods=["POST"])
@login_required
async def deprecated_file_convert():
    """
    Deprecated: Use POST /api/v1/files/link-to-datasets instead.

    Old path: POST /api/v1/file/convert
    New path: POST /api/v1/files/link-to-datasets
    """
    logging.warning(
        "API endpoint /api/v1/file/convert is deprecated. "
        "Please use POST /api/v1/files/link-to-datasets instead."
    )
    return await file2document_api.convert()


@manager.route("/file/mv", methods=["POST"])
@login_required
@add_tenant_id_to_kwargs
async def deprecated_file_mv(tenant_id=None):
    """
    Deprecated: Use POST /api/v1/files/move instead.

    Old path: POST /api/v1/file/mv
    New path: POST /api/v1/files/move
    """
    logging.warning(
        "API endpoint /api/v1/file/mv is deprecated. "
        "Please use POST /api/v1/files/move instead."
    )
    # Forward to the new API implementation
    return await file_api.move(tenant_id=tenant_id)


@manager.route("/file/rename", methods=["POST"])
@login_required
@add_tenant_id_to_kwargs
async def deprecated_file_rename(tenant_id=None):
    """
    Deprecated: Use POST /api/v1/files/move with new_name instead.

    Old path: POST /api/v1/file/rename
    New path: POST /api/v1/files/move
    """
    logging.warning(
        "API endpoint /api/v1/file/rename is deprecated. "
        "Please use POST /api/v1/files/move with `new_name` instead."
    )
    # Transform the old API format to new format
    req = await request.get_json()
    # Old API used `file_id` and `name`, new API uses `src_file_ids` and `new_name`
    src_file_ids = [req.get("file_id")]
    new_name = req.get("name")
    # Call the underlying service directly with transformed data
    try:
        success, result = await file_api_service.move_files(
            tenant_id, src_file_ids, None, new_name
        )
        if success:
            return get_json_result(data=result)
        else:
            return get_data_error_result(message=result)
    except Exception as e:
        logging.exception(e)
        return get_data_error_result(message="Internal server error")


@manager.route("/file/rm", methods=["POST"])
@login_required
@add_tenant_id_to_kwargs
async def deprecated_file_rm(tenant_id=None):
    """
    Deprecated: Use DELETE /api/v1/files instead.

    Old path: POST /api/v1/file/rm
    New path: DELETE /api/v1/files
    """
    logging.warning(
        "API endpoint /api/v1/file/rm is deprecated. "
        "Please use DELETE /api/v1/files instead."
    )
    # Transform POST with body to DELETE behavior
    # The new API expects a JSON body with `ids`
    return await file_api.delete(tenant_id=tenant_id)


# =============================================================================
# Related Questions API
# =============================================================================

@manager.route("/sessions/related_questions", methods=["POST"])
@login_required
async def deprecated_related_questions():
    """
    Deprecated: Use POST /api/v1/chat/recommendation instead.

    Old path: POST /api/v1/sessions/related_questions
    New path: POST /api/v1/chat/recommendation
    """
    logging.warning(
        "API endpoint /api/v1/sessions/related_questions is deprecated. "
        "Please use /api/v1/chat/recommendation instead."
    )
    # Forward to the new API implementation
    return await chat_api.recommendation()


# =============================================================================
# Chunk Update API (PUT -> PATCH)
# =============================================================================

@manager.route("/datasets/<dataset_id>/documents/<document_id>/chunks/<chunk_id>", methods=["PUT"])
@login_required
async def deprecated_update_chunk(dataset_id, document_id, chunk_id):
    """
    Deprecated: Use PATCH /api/v1/datasets/{dataset_id}/documents/{document_id}/chunks/{chunk_id} instead.

    Old path: PUT /api/v1/datasets/{dataset_id}/documents/{document_id}/chunks/{chunk_id}
    New path: PATCH /api/v1/datasets/{dataset_id}/documents/{document_id}/chunks/{chunk_id}
    """
    logging.warning(
        "API endpoint PUT /api/v1/datasets/%s/documents/%s/chunks/%s is deprecated. "
        "Please use PATCH instead.",
        dataset_id, document_id, chunk_id,
    )
    # Forward to the new API implementation
    return await chunk_api.update_chunk(dataset_id, document_id, chunk_id)


# =============================================================================
# File Upload Info API
# =============================================================================

@manager.route("/file/upload_info", methods=["POST"])
@login_required
async def deprecated_file_upload_info():
    """
    Deprecated: Use POST /api/v1/documents/upload instead.

    Old path: POST /api/v1/file/upload_info
    New path: POST /api/v1/documents/upload
    """
    from api.apps import current_user

    logging.warning(
        "API endpoint /api/v1/file/upload_info is deprecated. "
        "Please use POST /api/v1/documents/upload instead."
    )
    # Forward to the new API implementation
    # Need to pass tenant_id explicitly since we're calling the function directly
    tenant_id = current_user.id
    return await document_api.upload_info(tenant_id=tenant_id)


# =============================================================================
# Document APIs
# =============================================================================

@manager.route("/document/get/<doc_id>", methods=["GET"])
@login_required
async def deprecated_document_get(doc_id):
    """
    Deprecated: Use GET /api/v1/documents/{doc_id}/preview instead.

    Old path: GET /api/v1/document/get/{doc_id}
    New path: GET /api/v1/documents/{doc_id}/preview
    """
    logging.warning(
        "API endpoint /api/v1/document/get/%s is deprecated. "
        "Please use /api/v1/documents/%s/preview instead.",
        doc_id, doc_id,
    )
    return await document_api.get(doc_id)


@manager.route("/document/download/<doc_id>", methods=["GET"])
@login_required
async def deprecated_document_download(doc_id):
    """
    Deprecated: Use GET /api/v1/documents/{doc_id}/download instead.

    Old path: GET /api/v1/document/download/{doc_id}
    New path: GET /api/v1/documents/{doc_id}/download
    """
    logging.warning(
        "API endpoint /api/v1/document/download/%s is deprecated. "
        "Please use /api/v1/documents/%s/download instead.",
        doc_id, doc_id,
    )
    return await document_api.download_attachment(doc_id=doc_id)

# =============================================================================
# Agent Chat API
# =============================================================================

@manager.route("/agents/<agent_id>/completions", methods=["POST"])
@login_required
@add_tenant_id_to_kwargs
async def deprecated_agent_completions(agent_id, tenant_id=None):
    """
    Deprecated: Use POST /api/v1/agents/chat/completions instead.

    Old path: POST /api/v1/agents/{agent_id}/completions
    New path: POST /api/v1/agents/chat/completions
    """
    logging.warning(
        "API endpoint /api/v1/agents/%s/completions is deprecated. "
        "Please use /api/v1/agents/chat/completions instead.",
        agent_id,
    )
    return await agent_api.agent_chat_completion(tenant_id=tenant_id, agent_id=agent_id)

def register_backward_compat_routes(app_instance):
    """
    Register all backward compatibility routes with the app.
    """
    app_instance.register_blueprint(manager, url_prefix="/api/v1")
    logging.info("Backward compatibility routes registered successfully.")
