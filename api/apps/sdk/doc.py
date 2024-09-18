import pathlib
import re
import datetime
import json
import traceback

from flask import request
from flask_login import login_required, current_user
from elasticsearch_dsl import Q

from rag.app.qa import rmPrefix, beAdoc
from rag.nlp import search, rag_tokenizer, keyword_extraction
from rag.utils.es_conn import ELASTICSEARCH
from rag.utils import rmSpace
from api.db import LLMType, ParserType
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import TenantLLMService
from api.db.services.user_service import UserTenantService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request
from api.db.services.document_service import DocumentService
from api.settings import RetCode, retrievaler, kg_retrievaler
from api.utils.api_utils import get_json_result
import hashlib
import re
from api.utils.api_utils import get_json_result, token_required, get_data_error_result

from api.db.db_models import Task, File

from api.db.services.task_service import TaskService, queue_tasks
from api.db.services.user_service import TenantService, UserTenantService

from api.utils.api_utils import server_error_response, get_data_error_result, validate_request

from api.utils.api_utils import get_json_result

from functools import partial
from io import BytesIO

from elasticsearch_dsl import Q
from flask import request, send_file
from flask_login import login_required

from api.db import FileSource, TaskStatus, FileType
from api.db.db_models import File
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.settings import RetCode, retrievaler
from api.utils.api_utils import construct_json_result, construct_error_response
from rag.app import book, laws, manual, naive, one, paper, presentation, qa, resume, table, picture, audio, email
from rag.nlp import search
from rag.utils import rmSpace
from rag.utils.es_conn import ELASTICSEARCH
from rag.utils.storage_factory import STORAGE_IMPL

MAXIMUM_OF_UPLOADING_FILES = 256

MAXIMUM_OF_UPLOADING_FILES = 256


@manager.route('/dataset/<dataset_id>/documents/upload', methods=['POST'])
@token_required
def upload(dataset_id, tenant_id):
    if 'file' not in request.files:
        return get_json_result(
            data=False, retmsg='No file part!', retcode=RetCode.ARGUMENT_ERROR)
    file_objs = request.files.getlist('file')
    for file_obj in file_objs:
        if file_obj.filename == '':
            return get_json_result(
                data=False, retmsg='No file selected!', retcode=RetCode.ARGUMENT_ERROR)
    e, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not e:
        raise LookupError(f"Can't find the knowledgebase with ID {dataset_id}!")
    err, _ = FileService.upload_document(kb, file_objs, tenant_id)
    if err:
        return get_json_result(
            data=False, retmsg="\n".join(err), retcode=RetCode.SERVER_ERROR)
    return get_json_result(data=True)


@manager.route('/infos', methods=['GET'])
@token_required
def docinfos(tenant_id):
    req = request.args
    if "id" in req:
        doc_id = req["id"]
        e, doc = DocumentService.get_by_id(doc_id)
        return get_json_result(data=doc.to_json())
    if "name" in req:
        doc_name = req["name"]
        doc_id = DocumentService.get_doc_id_by_doc_name(doc_name)
        e, doc = DocumentService.get_by_id(doc_id)
        return get_json_result(data=doc.to_json())


@manager.route('/save', methods=['POST'])
@token_required
def save_doc(tenant_id):
    req = request.json
    #get doc by id or name
    doc_id = None
    if "id" in req:
        doc_id = req["id"]
    elif "name" in req:
        doc_name = req["name"]
        doc_id = DocumentService.get_doc_id_by_doc_name(doc_name)
    if not doc_id:
        return get_json_result(retcode=400, retmsg="Document ID or name is required")
    e, doc = DocumentService.get_by_id(doc_id)
    if not e:
        return get_data_error_result(retmsg="Document not found!")
    #other value can't be changed
    if "chunk_num" in req:
        if req["chunk_num"] != doc.chunk_num:
            return get_data_error_result(
                retmsg="Can't change chunk_count.")
    if "progress" in req:
        if req['progress'] != doc.progress:
            return get_data_error_result(
                retmsg="Can't change progress.")
    #change name or parse_method
    if "name" in req and req["name"] != doc.name:
        try:
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
                    doc_id, {"name": req["name"]}):
                return get_data_error_result(
                    retmsg="Database error (Document rename)!")

            informs = File2DocumentService.get_by_document_id(doc_id)
            if informs:
                e, file = FileService.get_by_id(informs[0].file_id)
                FileService.update_by_id(file.id, {"name": req["name"]})
        except Exception as e:
            return server_error_response(e)
    if "parser_id" in req:
        try:
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
        except Exception as e:
            return server_error_response(e)
    return get_json_result(data=True)



@manager.route('/change_parser', methods=['POST'])
@token_required
def change_parser(tenant_id):
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


@manager.route("/<document_id>", methods=["GET"])
@token_required
def download_document(dataset_id, document_id):
    try:
        # Check whether there is this document
        exist, document = DocumentService.get_by_id(document_id)
        if not exist:
            return construct_json_result(message=f"This document '{document_id}' cannot be found!",
                                         code=RetCode.ARGUMENT_ERROR)

        # The process of downloading
        doc_id, doc_location = File2DocumentService.get_minio_address(doc_id=document_id)  # minio address
        file_stream = STORAGE_IMPL.get(doc_id, doc_location)
        if not file_stream:
            return construct_json_result(message="This file is empty.", code=RetCode.DATA_ERROR)

        file = BytesIO(file_stream)

        # Use send_file with a proper filename and MIME type
        return send_file(
            file,
            as_attachment=True,
            download_name=document.name,
            mimetype='application/octet-stream'  # Set a default MIME type
        )

    # Error
    except Exception as e:
        return construct_error_response(e)


@manager.route('/dataset/<dataset_id>/documents', methods=['GET'])
@token_required
def list_docs(dataset_id, tenant_id):
    kb_id = request.args.get("kb_id")
    if not kb_id:
        return get_json_result(
            data=False, retmsg='Lack of "KB ID"', retcode=RetCode.ARGUMENT_ERROR)
    tenants = UserTenantService.query(user_id=tenant_id)
    for tenant in tenants:
        if KnowledgebaseService.query(
                tenant_id=tenant.tenant_id, id=kb_id):
            break
    else:
        return get_json_result(
            data=False, retmsg=f'Only owner of knowledgebase authorized for this operation.',
            retcode=RetCode.OPERATING_ERROR)
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


@manager.route('/delete', methods=['DELETE'])
@token_required
def rm(tenant_id):
    req = request.args
    if "doc_id" not in req:
        return get_data_error_result(
            retmsg="doc_id is required")
    doc_ids = req["doc_id"]
    if isinstance(doc_ids, str): doc_ids = [doc_ids]
    root_folder = FileService.get_root_folder(tenant_id)
    pf_id = root_folder["id"]
    FileService.init_knowledgebase_docs(pf_id, tenant_id)
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

            STORAGE_IMPL.rm(b, n)
        except Exception as e:
            errors += str(e)

    if errors:
        return get_json_result(data=False, retmsg=errors, retcode=RetCode.SERVER_ERROR)

    return get_json_result(data=True, retmsg="success")

@manager.route("/<document_id>/status", methods=["GET"])
@token_required
def show_parsing_status(tenant_id, document_id):
    try:
        # valid document
        exist, _ = DocumentService.get_by_id(document_id)
        if not exist:
            return construct_json_result(code=RetCode.DATA_ERROR,
                                         message=f"This document: '{document_id}' is not a valid document.")

        _, doc = DocumentService.get_by_id(document_id)  # get doc object
        doc_attributes = doc.to_dict()

        return construct_json_result(
            data={"progress": doc_attributes["progress"], "status": TaskStatus(doc_attributes["status"]).name},
            code=RetCode.SUCCESS
        )
    except Exception as e:
        return construct_error_response(e)



@manager.route('/run', methods=['POST'])
@token_required
def run(tenant_id):
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


@manager.route('/chunk/list', methods=['POST'])
@token_required
@validate_request("doc_id")
def list_chunk(tenant_id):
    req = request.json
    doc_id = req["doc_id"]
    page = int(req.get("page", 1))
    size = int(req.get("size", 30))
    question = req.get("keywords", "")
    try:
        tenant_id = DocumentService.get_tenant_id(req["doc_id"])
        if not tenant_id:
            return get_data_error_result(retmsg="Tenant not found!")
        e, doc = DocumentService.get_by_id(doc_id)
        if not e:
            return get_data_error_result(retmsg="Document not found!")
        query = {
            "doc_ids": [doc_id], "page": page, "size": size, "question": question, "sort": True
        }
        if "available_int" in req:
            query["available_int"] = int(req["available_int"])
        sres = retrievaler.search(query, search.index_name(tenant_id), highlight=True)
        res = {"total": sres.total, "chunks": [], "doc": doc.to_dict()}
        for id in sres.ids:
            d = {
                "chunk_id": id,
                "content_with_weight": rmSpace(sres.highlight[id]) if question and id in sres.highlight else sres.field[
                    id].get(
                    "content_with_weight", ""),
                "doc_id": sres.field[id]["doc_id"],
                "docnm_kwd": sres.field[id]["docnm_kwd"],
                "important_kwd": sres.field[id].get("important_kwd", []),
                "img_id": sres.field[id].get("img_id", ""),
                "available_int": sres.field[id].get("available_int", 1),
                "positions": sres.field[id].get("position_int", "").split("\t")
            }
            if len(d["positions"]) % 5 == 0:
                poss = []
                for i in range(0, len(d["positions"]), 5):
                    poss.append([float(d["positions"][i]), float(d["positions"][i + 1]), float(d["positions"][i + 2]),
                                 float(d["positions"][i + 3]), float(d["positions"][i + 4])])
                d["positions"] = poss
            res["chunks"].append(d)
        return get_json_result(data=res)
    except Exception as e:
        if str(e).find("not_found") > 0:
            return get_json_result(data=False, retmsg=f'No chunk found!',
                                   retcode=RetCode.DATA_ERROR)
        return server_error_response(e)


@manager.route('/chunk/create', methods=['POST'])
@token_required
@validate_request("doc_id", "content_with_weight")
def create(tenant_id):
    req = request.json
    md5 = hashlib.md5()
    md5.update((req["content_with_weight"] + req["doc_id"]).encode("utf-8"))
    chunck_id = md5.hexdigest()
    d = {"id": chunck_id, "content_ltks": rag_tokenizer.tokenize(req["content_with_weight"]),
         "content_with_weight": req["content_with_weight"]}
    d["content_sm_ltks"] = rag_tokenizer.fine_grained_tokenize(d["content_ltks"])
    d["important_kwd"] = req.get("important_kwd", [])
    d["important_tks"] = rag_tokenizer.tokenize(" ".join(req.get("important_kwd", [])))
    d["create_time"] = str(datetime.datetime.now()).replace("T", " ")[:19]
    d["create_timestamp_flt"] = datetime.datetime.now().timestamp()

    try:
        e, doc = DocumentService.get_by_id(req["doc_id"])
        if not e:
            return get_data_error_result(retmsg="Document not found!")
        d["kb_id"] = [doc.kb_id]
        d["docnm_kwd"] = doc.name
        d["doc_id"] = doc.id

        tenant_id = DocumentService.get_tenant_id(req["doc_id"])
        if not tenant_id:
            return get_data_error_result(retmsg="Tenant not found!")

        embd_id = DocumentService.get_embd_id(req["doc_id"])
        embd_mdl = TenantLLMService.model_instance(
            tenant_id, LLMType.EMBEDDING.value, embd_id)

        v, c = embd_mdl.encode([doc.name, req["content_with_weight"]])
        v = 0.1 * v[0] + 0.9 * v[1]
        d["q_%d_vec" % len(v)] = v.tolist()
        ELASTICSEARCH.upsert([d], search.index_name(tenant_id))

        DocumentService.increment_chunk_num(
            doc.id, doc.kb_id, c, 1, 0)
        return get_json_result(data={"chunk": d})
        # return get_json_result(data={"chunk_id": chunck_id})
    except Exception as e:
        return server_error_response(e)


@manager.route('/chunk/rm', methods=['POST'])
@token_required
@validate_request("chunk_ids", "doc_id")
def rm_chunk():
    req = request.json
    try:
        if not ELASTICSEARCH.deleteByQuery(
                Q("ids", values=req["chunk_ids"]), search.index_name(current_user.id)):
            return get_data_error_result(retmsg="Index updating failure")
        e, doc = DocumentService.get_by_id(req["doc_id"])
        if not e:
            return get_data_error_result(retmsg="Document not found!")
        deleted_chunk_ids = req["chunk_ids"]
        chunk_number = len(deleted_chunk_ids)
        DocumentService.decrement_chunk_num(doc.id, doc.kb_id, 1, chunk_number, 0)
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)