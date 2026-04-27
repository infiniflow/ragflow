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
#  limitations under the License
#
import re

from quart import make_response, request

from api.apps import current_user, login_required
from api.constants import IMG_BASE64_PREFIX
from api.db import FileType
from api.db.db_models import Task
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.task_service import TaskService, cancel_all_task_of
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    get_request_json,
    server_error_response,
    validate_request,
)
from api.utils.web_utils import CONTENT_TYPE_MAP, apply_safe_file_response_headers
from common import settings
from common.constants import RetCode, TaskStatus
from common.misc_utils import thread_pool_exec
from rag.nlp import search


@manager.route("/get/<doc_id>", methods=["GET"])  # noqa: F821
@login_required
async def get(doc_id):
    try:
        e, doc = DocumentService.get_by_id(doc_id)
        if not e:
            return get_data_error_result(message="Document not found!")

        b, n = File2DocumentService.get_storage_address(doc_id=doc_id)
        data = await thread_pool_exec(settings.STORAGE_IMPL.get, b, n)
        response = await make_response(data)

        ext = re.search(r"\.([^.]+)$", doc.name.lower())
        ext = ext.group(1) if ext else None
        content_type = None
        if ext:
            fallback_prefix = "image" if doc.type == FileType.VISUAL.value else "application"
            content_type = CONTENT_TYPE_MAP.get(ext, f"{fallback_prefix}/{ext}")
        apply_safe_file_response_headers(response, content_type, ext)
        return response
    except Exception as e:
        return server_error_response(e)


@manager.route("/download/<attachment_id>", methods=["GET"])  # noqa: F821
@login_required
async def download_attachment(attachment_id):
    try:
        ext = request.args.get("ext", "markdown")
        data = await thread_pool_exec(settings.STORAGE_IMPL.get, current_user.id, attachment_id)
        response = await make_response(data)
        content_type = CONTENT_TYPE_MAP.get(ext, f"application/{ext}")
        apply_safe_file_response_headers(response, content_type, ext)

        return response

    except Exception as e:
        return server_error_response(e)

