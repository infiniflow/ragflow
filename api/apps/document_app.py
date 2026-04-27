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
import os.path
import re
from pathlib import PurePosixPath, PureWindowsPath

from quart import make_response, request

from api.apps import current_user, login_required
from api.common.check_team_permission import check_kb_team_permission
from api.constants import IMG_BASE64_PREFIX, FILE_NAME_LEN_LIMIT
from api.db import FileType
from api.db.db_models import Task
from api.db.services.document_service import DocumentService, doc_upload_and_parse
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.task_service import TaskService, cancel_all_task_of
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    get_request_json,
    server_error_response,
    validate_request,
)
from api.utils.web_utils import CONTENT_TYPE_MAP, apply_safe_file_response_headers, is_valid_url
from common import settings
from common.constants import SANDBOX_ARTIFACT_BUCKET, RetCode, TaskStatus
from common.file_utils import get_project_base_directory
from common.misc_utils import thread_pool_exec
from common.ssrf_guard import assert_url_is_safe
from deepdoc.parser.html_parser import RAGFlowHtmlParser
from rag.nlp import search


def _is_safe_download_filename(name: str) -> bool:
    if not name or name in {".", ".."}:
        return False
    if "\x00" in name or len(name) > 255:
        return False
    if name != PurePosixPath(name).name:
        return False
    if name != PureWindowsPath(name).name:
        return False
    return True



@manager.route("/change_status", methods=["POST"])  # noqa: F821
@login_required
@validate_request("doc_ids", "status")
async def change_status():
    req = await get_request_json()
    doc_ids = req.get("doc_ids", [])
    status = str(req.get("status", ""))

    if status not in ["0", "1"]:
        return get_json_result(data=False, message='"Status" must be either 0 or 1!', code=RetCode.ARGUMENT_ERROR)

    result = {}
    has_error = False
    for doc_id in doc_ids:
        if not DocumentService.accessible(doc_id, current_user.id):
            result[doc_id] = {"error": "No authorization."}
            has_error = True
            continue

        try:
            e, doc = DocumentService.get_by_id(doc_id)
            if not e:
                result[doc_id] = {"error": "No authorization."}
                has_error = True
                continue
            e, kb = KnowledgebaseService.get_by_id(doc.kb_id)
            if not e:
                result[doc_id] = {"error": "Can't find this dataset!"}
                has_error = True
                continue
            current_status = str(doc.status)
            if current_status == status:
                result[doc_id] = {"status": status}
                continue
            if not DocumentService.update_by_id(doc_id, {"status": str(status)}):
                result[doc_id] = {"error": "Database error (Document update)!"}
                has_error = True
                continue

            status_int = int(status)
            if getattr(doc, "chunk_num", 0) > 0:
                try:
                    ok = settings.docStoreConn.update(
                        {"doc_id": doc_id},
                        {"available_int": status_int},
                        search.index_name(kb.tenant_id),
                        doc.kb_id,
                    )
                except Exception:
                    logging.exception(
                        "Document store update failed in change_status: doc_id=%s kb_id=%s status=%s",
                        doc_id, doc.kb_id, status_int,
                    )
                    result[doc_id] = {"error": "Document store update failed."}
                    has_error = True
                    continue
                if not ok:
                    logging.warning(
                        "Document store update returned False in change_status: doc_id=%s kb_id=%s status=%s",
                        doc_id, doc.kb_id, status_int,
                    )
                    result[doc_id] = {"error": "Document store table missing or update failed."}
                    has_error = True
                    continue
            result[doc_id] = {"status": status}
        except Exception as e:
            result[doc_id] = {"error": f"Internal server error: {str(e)}"}
            has_error = True

    if has_error:
        return get_json_result(data=result, message="Partial failure", code=RetCode.SERVER_ERROR)
    return get_json_result(data=result)


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


ARTIFACT_CONTENT_TYPES = {
    ".png": "image/png",
    ".jpg": "image/jpeg",
    ".jpeg": "image/jpeg",
    ".svg": "image/svg+xml",
    ".pdf": "application/pdf",
    ".csv": "text/csv",
    ".json": "application/json",
    ".html": "text/html",
}


@manager.route("/artifact/<filename>", methods=["GET"])  # noqa: F821
@login_required
async def get_artifact(filename):
    try:
        bucket = SANDBOX_ARTIFACT_BUCKET
        # Validate filename: must be uuid hex + allowed extension, nothing else
        basename = os.path.basename(filename)
        if basename != filename or "/" in filename or "\\" in filename:
            return get_data_error_result(message="Invalid filename.")
        ext = os.path.splitext(basename)[1].lower()
        if ext not in ARTIFACT_CONTENT_TYPES:
            return get_data_error_result(message="Invalid file type.")
        data = await thread_pool_exec(settings.STORAGE_IMPL.get, bucket, basename)
        if not data:
            return get_data_error_result(message="Artifact not found.")
        content_type = ARTIFACT_CONTENT_TYPES.get(ext, "application/octet-stream")
        response = await make_response(data)
        safe_filename = re.sub(r"[^\w.\-]", "_", basename)
        apply_safe_file_response_headers(response, content_type, ext)
        if not response.headers.get("Content-Disposition"):
            response.headers.set("Content-Disposition", f'inline; filename="{safe_filename}"')
        return response
    except Exception as e:
        return server_error_response(e)


@manager.route("/upload_and_parse", methods=["POST"])  # noqa: F821
@login_required
@validate_request("conversation_id")
async def upload_and_parse():
    files = await request.files
    if "file" not in files:
        return get_json_result(data=False, message="No file part!", code=RetCode.ARGUMENT_ERROR)

    file_objs = files.getlist("file")
    for file_obj in file_objs:
        if file_obj.filename == "":
            return get_json_result(data=False, message="No file selected!", code=RetCode.ARGUMENT_ERROR)

    form = await request.form
    doc_ids = doc_upload_and_parse(form.get("conversation_id"), file_objs, current_user.id)
    return get_json_result(data=doc_ids)


@manager.route("/parse", methods=["POST"])  # noqa: F821
@login_required
async def parse():
    req = await get_request_json()
    url = req.get("url", "")
    if url:
        if not is_valid_url(url):
            return get_json_result(data=False, message="The URL format is invalid", code=RetCode.ARGUMENT_ERROR)
        download_path = os.path.join(get_project_base_directory(), "logs/downloads")
        os.makedirs(download_path, exist_ok=True)
        from seleniumwire.webdriver import Chrome, ChromeOptions

        options = ChromeOptions()
        options.add_argument("--headless")
        options.add_argument("--disable-gpu")
        options.add_argument("--no-sandbox")
        options.add_argument("--disable-dev-shm-usage")
        options.add_experimental_option("prefs", {"download.default_directory": download_path, "download.prompt_for_download": False, "download.directory_upgrade": True, "safebrowsing.enabled": True})
        driver = Chrome(options=options)
        driver.get(url)
        res_headers = [r.response.headers for r in driver.requests if r and r.response]
        if len(res_headers) > 1:
            sections = RAGFlowHtmlParser().parser_txt(driver.page_source)
            driver.quit()
            return get_json_result(data="\n".join(sections))

        class File:
            filename: str
            filepath: str

            def __init__(self, filename, filepath):
                self.filename = filename
                self.filepath = filepath

            def read(self):
                with open(self.filepath, "rb") as f:
                    return f.read()

        r = re.search(r"filename=\"([^\"]+)\"", str(res_headers))
        if not r or not r.group(1):
            return get_json_result(data=False, message="Can't not identify downloaded file", code=RetCode.ARGUMENT_ERROR)
        filename = r.group(1).strip()
        if not _is_safe_download_filename(filename):
            return get_json_result(data=False, message="Invalid downloaded filename", code=RetCode.ARGUMENT_ERROR)
        filepath = os.path.join(download_path, filename)
        f = File(filename, filepath)
        txt = FileService.parse_docs([f], current_user.id)
        return get_json_result(data=txt)

    files = await request.files
    if "file" not in files:
        return get_json_result(data=False, message="No file part!", code=RetCode.ARGUMENT_ERROR)

    file_objs = files.getlist("file")
    txt = FileService.parse_docs(file_objs, current_user.id)

    return get_json_result(data=txt)


@manager.route("/upload_info", methods=["POST"])  # noqa: F821
@login_required
async def upload_info():
    files = await request.files
    file_objs = files.getlist("file") if files and files.get("file") else []
    url = request.args.get("url")

    if file_objs and url:
        return get_json_result(
            data=False,
            message="Provide either multipart file(s) or ?url=..., not both.",
            code=RetCode.BAD_REQUEST,
        )

    if not file_objs and not url:
        return get_json_result(
            data=False,
            message="Missing input: provide multipart file(s) or url",
            code=RetCode.BAD_REQUEST,
        )

    try:
        if url and not file_objs:
            assert_url_is_safe(url)
            return get_json_result(data=FileService.upload_info(current_user.id, None, url))

        if len(file_objs) == 1:
            return get_json_result(data=FileService.upload_info(current_user.id, file_objs[0], None))

        results = [FileService.upload_info(current_user.id, f, None) for f in file_objs]
        return get_json_result(data=results)
    except Exception as e:
        return server_error_response(e)
