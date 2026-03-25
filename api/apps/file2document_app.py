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

import asyncio
from pathlib import Path

from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService

from api.apps import login_required, current_user
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils.api_utils import get_data_error_result, get_json_result, get_request_json, server_error_response, validate_request
from common.misc_utils import get_uuid
from common.constants import RetCode
from api.db import FileType
from api.db.services.document_service import DocumentService


def _convert_files(file_ids, kb_ids, user_id):
    """Synchronous worker: delete old docs and insert new ones for the given file/kb pairs."""
    for id in file_ids:
        informs = File2DocumentService.get_by_file_id(id)
        for inform in informs:
            doc_id = inform.document_id
            e, doc = DocumentService.get_by_id(doc_id)
            if not e:
                continue
            tenant_id = DocumentService.get_tenant_id(doc_id)
            if tenant_id:
                DocumentService.remove_document(doc, tenant_id)
        File2DocumentService.delete_by_file_id(id)

        for kb_id in kb_ids:
            e, kb = KnowledgebaseService.get_by_id(kb_id)
            if not e:
                continue
            e, file = FileService.get_by_id(id)
            if not e:
                continue
            doc = DocumentService.insert({
                "id": get_uuid(),
                "kb_id": kb.id,
                "parser_id": FileService.get_parser(file.type, file.name, kb.parser_id),
                "pipeline_id": kb.pipeline_id,
                "parser_config": kb.parser_config,
                "created_by": user_id,
                "type": file.type,
                "name": file.name,
                "suffix": Path(file.name).suffix.lstrip("."),
                "location": file.location,
                "size": file.size
            })
            File2DocumentService.insert({
                "id": get_uuid(),
                "file_id": id,
                "document_id": doc.id,
            })


@manager.route('/convert', methods=['POST'])  # noqa: F821
@login_required
@validate_request("file_ids", "kb_ids")
async def convert():
    req = await get_request_json()
    kb_ids = req["kb_ids"]
    file_ids = req["file_ids"]

    try:
        files = FileService.get_by_ids(file_ids)
        files_set = {file.id: file for file in files}

        # Validate all files exist before starting any work
        for file_id in file_ids:
            if file_id not in files_set:
                return get_data_error_result(message="File not found!")

        # Expand folders to their innermost file IDs
        all_file_ids = []
        for file_id in file_ids:
            file = files_set[file_id]
            if file.type == FileType.FOLDER.value:
                all_file_ids.extend(FileService.get_all_innermost_file_ids(file_id, []))
            else:
                all_file_ids.append(file_id)

        user_id = current_user.id
        # Run the blocking DB work in a thread so the event loop is not blocked.
        # For large folders this prevents 504 Gateway Timeout by returning as
        # soon as the background task is scheduled.
        asyncio.get_running_loop().run_in_executor(
            None, _convert_files, all_file_ids, kb_ids, user_id
        )
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['POST'])  # noqa: F821
@login_required
@validate_request("file_ids")
async def rm():
    req = await get_request_json()
    file_ids = req["file_ids"]
    if not file_ids:
        return get_json_result(
            data=False, message='Lack of "Files ID"', code=RetCode.ARGUMENT_ERROR)
    try:
        for file_id in file_ids:
            informs = File2DocumentService.get_by_file_id(file_id)
            if not informs:
                return get_data_error_result(message="Inform not found!")
            for inform in informs:
                if not inform:
                    return get_data_error_result(message="Inform not found!")
                File2DocumentService.delete_by_file_id(file_id)
                doc_id = inform.document_id
                e, doc = DocumentService.get_by_id(doc_id)
                if not e:
                    return get_data_error_result(message="Document not found!")
                tenant_id = DocumentService.get_tenant_id(doc_id)
                if not tenant_id:
                    return get_data_error_result(message="Tenant not found!")
                if not DocumentService.remove_document(doc, tenant_id):
                    return get_data_error_result(
                        message="Database error (Document removal)!")
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)
