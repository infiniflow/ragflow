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
import logging
import re

from quart import make_response, request

from api.apps import current_user, login_required
from api.constants import IMG_BASE64_PREFIX
from api.db import FileType
from api.db.db_models import Task
from api.db.services.document_service import DocumentService, doc_upload_and_parse
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


@manager.route("/run", methods=["POST"])  # noqa: F821
@login_required
@validate_request("doc_ids", "run")
async def run():
    req = await get_request_json()
    uid = current_user.id
    try:

        def _run_sync():
            for doc_id in req["doc_ids"]:
                if not DocumentService.accessible(doc_id, uid):
                    return get_json_result(data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)

            kb_table_num_map = {}
            for id in req["doc_ids"]:
                info = {"run": str(req["run"]), "progress": 0}
                if str(req["run"]) == TaskStatus.RUNNING.value and req.get("delete", False):
                    info["progress_msg"] = ""
                    info["chunk_num"] = 0
                    info["token_num"] = 0

                tenant_id = DocumentService.get_tenant_id(id)
                if not tenant_id:
                    return get_data_error_result(message="Tenant not found!")
                e, doc = DocumentService.get_by_id(id)
                if not e:
                    return get_data_error_result(message="Document not found!")

                if str(req["run"]) == TaskStatus.CANCEL.value:
                    tasks = list(TaskService.query(doc_id=id))
                    has_unfinished_task = any((task.progress or 0) < 1 for task in tasks)
                    if str(doc.run) in [TaskStatus.RUNNING.value, TaskStatus.CANCEL.value] or has_unfinished_task:
                        cancel_all_task_of(id)
                    else:
                        return get_data_error_result(message="Cannot cancel a task that is not in RUNNING status")
                if all([("delete" not in req or req["delete"]), str(req["run"]) == TaskStatus.RUNNING.value, str(doc.run) == TaskStatus.DONE.value]):
                    DocumentService.clear_chunk_num_when_rerun(doc.id)

                DocumentService.update_by_id(id, info)
                if req.get("delete", False):
                    TaskService.filter_delete([Task.doc_id == id])
                    if settings.docStoreConn.index_exist(search.index_name(tenant_id), doc.kb_id):
                        settings.docStoreConn.delete({"doc_id": id}, search.index_name(tenant_id), doc.kb_id)

                if str(req["run"]) == TaskStatus.RUNNING.value:
                    if req.get("apply_kb"):
                        e, kb = KnowledgebaseService.get_by_id(doc.kb_id)
                        if not e:
                            raise LookupError("Can't find this dataset!")
                        doc.parser_config["llm_id"] = kb.parser_config.get("llm_id")
                        doc.parser_config["enable_metadata"] = kb.parser_config.get("enable_metadata", False)
                        doc.parser_config["metadata"] = kb.parser_config.get("metadata", {})
                        DocumentService.update_parser_config(doc.id, doc.parser_config)
                    doc_dict = doc.to_dict()
                    DocumentService.run(tenant_id, doc_dict, kb_table_num_map)

            return get_json_result(data=True)

        return await thread_pool_exec(_run_sync)
    except Exception as e:
        return server_error_response(e)

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


@manager.route("/change_parser", methods=["POST"])  # noqa: F821
@login_required
@validate_request("doc_id")
async def change_parser():
    req = await get_request_json()
    if not DocumentService.accessible(req["doc_id"], current_user.id):
        return get_json_result(data=False, message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)

    e, doc = DocumentService.get_by_id(req["doc_id"])
    if not e:
        return get_data_error_result(message="Document not found!")

    def reset_doc():
        nonlocal doc
        e = DocumentService.update_by_id(doc.id, {"pipeline_id": req["pipeline_id"], "parser_id": req["parser_id"], "progress": 0, "progress_msg": "", "run": TaskStatus.UNSTART.value})
        if not e:
            return get_data_error_result(message="Document not found!")
        if doc.token_num > 0:
            e = DocumentService.increment_chunk_num(doc.id, doc.kb_id, doc.token_num * -1, doc.chunk_num * -1, doc.process_duration * -1)
            if not e:
                return get_data_error_result(message="Document not found!")
            tenant_id = DocumentService.get_tenant_id(req["doc_id"])
            if not tenant_id:
                return get_data_error_result(message="Tenant not found!")
            DocumentService.delete_chunk_images(doc, tenant_id)
            if settings.docStoreConn.index_exist(search.index_name(tenant_id), doc.kb_id):
                settings.docStoreConn.delete({"doc_id": doc.id}, search.index_name(tenant_id), doc.kb_id)
        return None

    try:
        if "pipeline_id" in req and req["pipeline_id"] != "":
            if doc.pipeline_id == req["pipeline_id"]:
                return get_json_result(data=True)
            DocumentService.update_by_id(doc.id, {"pipeline_id": req["pipeline_id"]})
            reset_doc()
            return get_json_result(data=True)

        if doc.parser_id.lower() == req["parser_id"].lower():
            if "parser_config" in req:
                if req["parser_config"] == doc.parser_config:
                    return get_json_result(data=True)
            else:
                return get_json_result(data=True)

        if (doc.type == FileType.VISUAL and req["parser_id"] != "picture") or (re.search(r"\.(ppt|pptx|pages)$", doc.name) and req["parser_id"] != "presentation"):
            return get_data_error_result(message="Not supported yet!")
        if "parser_config" in req:
            DocumentService.update_parser_config(doc.id, req["parser_config"])
        reset_doc()
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)
