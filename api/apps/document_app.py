#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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

import os
import pathlib
import re

import flask
from elasticsearch_dsl import Q
from flask import request
from flask_login import login_required, current_user

from api.db.db_models import Task, File
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.task_service import TaskService, queue_tasks
from rag.nlp import search
from rag.utils.es_conn import ELASTICSEARCH
from api.db.services import duplicate_name
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request
from api.utils import get_uuid
from api.db import FileType, TaskStatus, ParserType, FileSource
from api.db.services.document_service import DocumentService
from api.settings import RetCode
from api.utils.api_utils import get_json_result
from rag.utils.minio_conn import MINIO
from api.utils.file_utils import filename_type, thumbnail
from api.utils.web_utils import html2pdf, is_valid_url
from api.utils.web_utils import html2pdf, is_valid_url


@manager.route('/upload', methods=['POST'])
@login_required
@validate_request("kb_id")
def upload():
    kb_id = request.form.get("kb_id")
    if not kb_id:
        return get_json_result(
            data=False, retmsg='Lack of "KB ID"', retcode=RetCode.ARGUMENT_ERROR)
    if 'file' not in request.files:
        return get_json_result(
            data=False, retmsg='No file part!', retcode=RetCode.ARGUMENT_ERROR)

    file_objs = request.files.getlist('file')
    for file_obj in file_objs:
        if file_obj.filename == '':
            return get_json_result(
                data=False, retmsg='No file selected!', retcode=RetCode.ARGUMENT_ERROR)

    e, kb = KnowledgebaseService.get_by_id(kb_id)
    if not e:
        raise LookupError("Can't find this knowledgebase!")

    root_folder = FileService.get_root_folder(current_user.id)
    pf_id = root_folder["id"]
    FileService.init_knowledgebase_docs(pf_id, current_user.id)
    kb_root_folder = FileService.get_kb_folder(current_user.id)
    kb_folder = FileService.new_a_file_from_kb(kb.tenant_id, kb.name, kb_root_folder["id"])

    err = []
    for file in file_objs:
        try:
            MAX_FILE_NUM_PER_USER = int(os.environ.get('MAX_FILE_NUM_PER_USER', 0))
            if MAX_FILE_NUM_PER_USER > 0 and DocumentService.get_doc_count(kb.tenant_id) >= MAX_FILE_NUM_PER_USER:
                raise RuntimeError("Exceed the maximum file number of a free user!")

            filename = duplicate_name(
                DocumentService.query,
                name=file.filename,
                kb_id=kb.id)
            filetype = filename_type(filename)
            if filetype == FileType.OTHER.value:
                raise RuntimeError("This type of file has not been supported yet!")

            location = filename
            while MINIO.obj_exist(kb_id, location):
                location += "_"
            blob = file.read()
            MINIO.put(kb_id, location, blob)
            doc = {
                "id": get_uuid(),
                "kb_id": kb.id,
                "parser_id": kb.parser_id,
                "parser_config": kb.parser_config,
                "created_by": current_user.id,
                "type": filetype,
                "name": filename,
                "location": location,
                "size": len(blob),
                "thumbnail": thumbnail(filename, blob)
            }
            if doc["type"] == FileType.VISUAL:
                doc["parser_id"] = ParserType.PICTURE.value
            if doc["type"] == FileType.AURAL:
                doc["parser_id"] = ParserType.AUDIO.value
            if re.search(r"\.(ppt|pptx|pages)$", filename):
                doc["parser_id"] = ParserType.PRESENTATION.value
            DocumentService.insert(doc)

            FileService.add_file_from_kb(doc, kb_folder["id"], kb.tenant_id)
        except Exception as e:
            err.append(file.filename + ": " + str(e))
    if err:
        return get_json_result(
            data=False, retmsg="\n".join(err), retcode=RetCode.SERVER_ERROR)
    return get_json_result(data=True)


@manager.route('/web_crawl', methods=['POST'])
@login_required
@validate_request("kb_id", "name", "url")
def web_crawl():
    kb_id = request.form.get("kb_id")
    if not kb_id:
        return get_json_result(
            data=False, retmsg='Lack of "KB ID"', retcode=RetCode.ARGUMENT_ERROR)
    name = request.form.get("name")
    url = request.form.get("url")
    if not is_valid_url(url):
        return get_json_result(
            data=False, retmsg='The URL format is invalid', retcode=RetCode.ARGUMENT_ERROR)
    e, kb = KnowledgebaseService.get_by_id(kb_id)
    if not e:
        raise LookupError("Can't find this knowledgebase!")

    blob = html2pdf(url)
    if not blob: return server_error_response(ValueError("Download failure."))

    root_folder = FileService.get_root_folder(current_user.id)
    pf_id = root_folder["id"]
    FileService.init_knowledgebase_docs(pf_id, current_user.id)
    kb_root_folder = FileService.get_kb_folder(current_user.id)
    kb_folder = FileService.new_a_file_from_kb(kb.tenant_id, kb.name, kb_root_folder["id"])

    try:
        filename = duplicate_name(
            DocumentService.query,
            name=name+".pdf",
            kb_id=kb.id)
        filetype = filename_type(filename)
        if filetype == FileType.OTHER.value:
            raise RuntimeError("This type of file has not been supported yet!")

        location = filename
        while MINIO.obj_exist(kb_id, location):
            location += "_"
        MINIO.put(kb_id, location, blob)
        doc = {
            "id": get_uuid(),
            "kb_id": kb.id,
            "parser_id": kb.parser_id,
            "parser_config": kb.parser_config,
            "created_by": current_user.id,
            "type": filetype,
            "name": filename,
            "location": location,
            "size": len(blob),
            "thumbnail": thumbnail(filename, blob)
        }
        if doc["type"] == FileType.VISUAL:
            doc["parser_id"] = ParserType.PICTURE.value
        if doc["type"] == FileType.AURAL:
            doc["parser_id"] = ParserType.AUDIO.value
        if re.search(r"\.(ppt|pptx|pages)$", filename):
            doc["parser_id"] = ParserType.PRESENTATION.value
        DocumentService.insert(doc)
        FileService.add_file_from_kb(doc, kb_folder["id"], kb.tenant_id)
    except Exception as e:
        return server_error_response(e)
    return get_json_result(data=True)


@manager.route('/create', methods=['POST'])
@login_required
@validate_request("name", "kb_id")
def create():
    req = request.json
    kb_id = req["kb_id"]
    if not kb_id:
        return get_json_result(
            data=False, retmsg='Lack of "KB ID"', retcode=RetCode.ARGUMENT_ERROR)

    try:
        e, kb = KnowledgebaseService.get_by_id(kb_id)
        if not e:
            return get_data_error_result(
                retmsg="Can't find this knowledgebase!")

        if DocumentService.query(name=req["name"], kb_id=kb_id):
            return get_data_error_result(
                retmsg="Duplicated document name in the same knowledgebase.")

        doc = DocumentService.insert({
            "id": get_uuid(),
            "kb_id": kb.id,
            "parser_id": kb.parser_id,
            "parser_config": kb.parser_config,
            "created_by": current_user.id,
            "type": FileType.VIRTUAL,
            "name": req["name"],
            "location": "",
            "size": 0
        })
        return get_json_result(data=doc.to_json())
    except Exception as e:
        return server_error_response(e)


@manager.route('/list', methods=['GET'])
@login_required
def list_docs():
    kb_id = request.args.get("kb_id")
    if not kb_id:
        return get_json_result(
            data=False, retmsg='Lack of "KB ID"', retcode=RetCode.ARGUMENT_ERROR)
    keywords = request.args.get("keywords", "")

    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 15))
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", True)
    try:
        docs, tol = DocumentService.get_by_kb_id(
            kb_id, page_number, items_per_page, orderby, desc, keywords)
        return get_json_result(data={"total": tol, "docs": docs})
    except Exception as e:
        return server_error_response(e)


@manager.route('/thumbnails', methods=['GET'])
@login_required
def thumbnails():
    doc_ids = request.args.get("doc_ids").split(",")
    if not doc_ids:
        return get_json_result(
            data=False, retmsg='Lack of "Document ID"', retcode=RetCode.ARGUMENT_ERROR)

    try:
        docs = DocumentService.get_thumbnails(doc_ids)
        return get_json_result(data={d["id"]: d["thumbnail"] for d in docs})
    except Exception as e:
        return server_error_response(e)


@manager.route('/change_status', methods=['POST'])
@login_required
@validate_request("doc_id", "status")
def change_status():
    req = request.json
    if str(req["status"]) not in ["0", "1"]:
        get_json_result(
            data=False,
            retmsg='"Status" must be either 0 or 1!',
            retcode=RetCode.ARGUMENT_ERROR)

    try:
        e, doc = DocumentService.get_by_id(req["doc_id"])
        if not e:
            return get_data_error_result(retmsg="Document not found!")
        e, kb = KnowledgebaseService.get_by_id(doc.kb_id)
        if not e:
            return get_data_error_result(
                retmsg="Can't find this knowledgebase!")

        if not DocumentService.update_by_id(
                req["doc_id"], {"status": str(req["status"])}):
            return get_data_error_result(
                retmsg="Database error (Document update)!")

        if str(req["status"]) == "0":
            ELASTICSEARCH.updateScriptByQuery(Q("term", doc_id=req["doc_id"]),
                                              scripts="ctx._source.available_int=0;",
                                              idxnm=search.index_name(
                                                  kb.tenant_id)
                                              )
        else:
            ELASTICSEARCH.updateScriptByQuery(Q("term", doc_id=req["doc_id"]),
                                              scripts="ctx._source.available_int=1;",
                                              idxnm=search.index_name(
                                                  kb.tenant_id)
                                              )
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['POST'])
@login_required
@validate_request("doc_id")
def rm():
    req = request.json
    doc_ids = req["doc_id"]
    if isinstance(doc_ids, str): doc_ids = [doc_ids]
    root_folder = FileService.get_root_folder(current_user.id)
    pf_id = root_folder["id"]
    FileService.init_knowledgebase_docs(pf_id, current_user.id)
    errors = ""
    for doc_id in doc_ids:
        try:
            e, doc = DocumentService.get_by_id(doc_id)
            if not e:
                return get_data_error_result(retmsg="Document not found!")
            tenant_id = DocumentService.get_tenant_id(doc_id)
            if not tenant_id:
                return get_data_error_result(retmsg="Tenant not found!")

            b, n = File2DocumentService.get_minio_address(doc_id=doc_id)

            if not DocumentService.remove_document(doc, tenant_id):
                return get_data_error_result(
                    retmsg="Database error (Document removal)!")

            f2d = File2DocumentService.get_by_document_id(doc_id)
            FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
            File2DocumentService.delete_by_document_id(doc_id)

            MINIO.rm(b, n)
        except Exception as e:
            errors += str(e)

    if errors:
        return get_json_result(data=False, retmsg=errors, retcode=RetCode.SERVER_ERROR)

    return get_json_result(data=True)


@manager.route('/run', methods=['POST'])
@login_required
@validate_request("doc_ids", "run")
def run():
    req = request.json
    try:
        for id in req["doc_ids"]:
            info = {"run": str(req["run"]), "progress": 0}
            if str(req["run"]) == TaskStatus.RUNNING.value:
                info["progress_msg"] = ""
                info["chunk_num"] = 0
                info["token_num"] = 0
            DocumentService.update_by_id(id, info)
            # if str(req["run"]) == TaskStatus.CANCEL.value:
            tenant_id = DocumentService.get_tenant_id(id)
            if not tenant_id:
                return get_data_error_result(retmsg="Tenant not found!")
            ELASTICSEARCH.deleteByQuery(
                Q("match", doc_id=id), idxnm=search.index_name(tenant_id))

            if str(req["run"]) == TaskStatus.RUNNING.value:
                TaskService.filter_delete([Task.doc_id == id])
                e, doc = DocumentService.get_by_id(id)
                doc = doc.to_dict()
                doc["tenant_id"] = tenant_id
                bucket, name = File2DocumentService.get_minio_address(doc_id=doc["id"])
                queue_tasks(doc, bucket, name)

        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/rename', methods=['POST'])
@login_required
@validate_request("doc_id", "name")
def rename():
    req = request.json
    try:
        e, doc = DocumentService.get_by_id(req["doc_id"])
        if not e:
            return get_data_error_result(retmsg="Document not found!")
        if pathlib.Path(req["name"].lower()).suffix != pathlib.Path(
                doc.name.lower()).suffix:
            return get_json_result(
                data=False,
                retmsg="The extension of file can't be changed",
                retcode=RetCode.ARGUMENT_ERROR)
        for d in DocumentService.query(name=req["name"], kb_id=doc.kb_id):
            if d.name == req["name"]:
                return get_data_error_result(
                    retmsg="Duplicated document name in the same knowledgebase.")

        if not DocumentService.update_by_id(
                req["doc_id"], {"name": req["name"]}):
            return get_data_error_result(
                retmsg="Database error (Document rename)!")

        informs = File2DocumentService.get_by_document_id(req["doc_id"])
        if informs:
            e, file = FileService.get_by_id(informs[0].file_id)
            FileService.update_by_id(file.id, {"name": req["name"]})

        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/get/<doc_id>', methods=['GET'])
# @login_required
def get(doc_id):
    try:
        e, doc = DocumentService.get_by_id(doc_id)
        if not e:
            return get_data_error_result(retmsg="Document not found!")

        b,n = File2DocumentService.get_minio_address(doc_id=doc_id)
        response = flask.make_response(MINIO.get(b, n))

        ext = re.search(r"\.([^.]+)$", doc.name)
        if ext:
            if doc.type == FileType.VISUAL.value:
                response.headers.set('Content-Type', 'image/%s' % ext.group(1))
            else:
                response.headers.set(
                    'Content-Type',
                    'application/%s' %
                    ext.group(1))
        return response
    except Exception as e:
        return server_error_response(e)


@manager.route('/change_parser', methods=['POST'])
@login_required
@validate_request("doc_id", "parser_id")
def change_parser():
    req = request.json
    try:
        e, doc = DocumentService.get_by_id(req["doc_id"])
        if not e:
            return get_data_error_result(retmsg="Document not found!")
        if doc.parser_id.lower() == req["parser_id"].lower():
            if "parser_config" in req:
                if req["parser_config"] == doc.parser_config:
                    return get_json_result(data=True)
            else:
                return get_json_result(data=True)

        if doc.type == FileType.VISUAL or re.search(
                r"\.(ppt|pptx|pages)$", doc.name):
            return get_data_error_result(retmsg="Not supported yet!")

        e = DocumentService.update_by_id(doc.id,
                                         {"parser_id": req["parser_id"], "progress": 0, "progress_msg": "",
                                          "run": TaskStatus.UNSTART.value})
        if not e:
            return get_data_error_result(retmsg="Document not found!")
        if "parser_config" in req:
            DocumentService.update_parser_config(doc.id, req["parser_config"])
        if doc.token_num > 0:
            e = DocumentService.increment_chunk_num(doc.id, doc.kb_id, doc.token_num * -1, doc.chunk_num * -1,
                                                    doc.process_duation * -1)
            if not e:
                return get_data_error_result(retmsg="Document not found!")
            tenant_id = DocumentService.get_tenant_id(req["doc_id"])
            if not tenant_id:
                return get_data_error_result(retmsg="Tenant not found!")
            ELASTICSEARCH.deleteByQuery(
                Q("match", doc_id=doc.id), idxnm=search.index_name(tenant_id))

        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/image/<image_id>', methods=['GET'])
# @login_required
def get_image(image_id):
    try:
        bkt, nm = image_id.split("-")
        response = flask.make_response(MINIO.get(bkt, nm))
        response.headers.set('Content-Type', 'image/JPEG')
        return response
    except Exception as e:
        return server_error_response(e)
