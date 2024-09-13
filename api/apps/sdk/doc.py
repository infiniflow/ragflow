from io import BytesIO

from flask import request,send_file
from api.utils.api_utils import get_json_result, construct_json_result, server_error_response
from api.utils.api_utils import get_json_result, token_required, get_data_error_result
from api.db import FileType, ParserType, FileSource, TaskStatus
from api.db.db_models import File
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import TenantService, UserTenantService
from api.settings import RetCode
from api.utils.api_utils import construct_json_result, construct_error_response
from rag.utils.storage_factory import STORAGE_IMPL


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
    req = request.json  # Expecting JSON input
    if "id" in req:
        doc_id = req["id"]
    if "name" in req:
        doc_name = req["name"]
        doc_id = DocumentService.get_doc_id_by_doc_name(doc_name)
    data = request.json
    # Call the update method with the provided id and data
    try:
        num = DocumentService.update_by_id(doc_id, data)
        if num > 0:
            return get_json_result(retmsg="success", data={"updated_count": num})
        else:
            return get_json_result(retcode=404, retmsg="Document not found")
    except Exception as e:
        return get_json_result(retmsg=f"Error occurred: {str(e)}")


@manager.route("/<dataset_id>/documents/<document_id>", methods=["GET"])
@token_required
def download_document(dataset_id, document_id):
    try:
        # Check whether there is this dataset
        exist, _ = KnowledgebaseService.get_by_id(dataset_id)
        if not exist:
            return construct_json_result(code=RetCode.DATA_ERROR,
                                         message=f"This dataset '{dataset_id}' cannot be found!")

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
def list_docs(dataset_id,tenant_id):
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

    return get_json_result(data=True,retmsg="success")
