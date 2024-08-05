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
#  limitations under the License.
import os
import pathlib
import re
import warnings
from functools import partial
from io import BytesIO

from elasticsearch_dsl import Q
from flask import request, send_file
from flask_login import login_required, current_user
from httpx import HTTPError

from api.contants import NAME_LENGTH_LIMIT
from api.db import FileType, ParserType, FileSource, TaskStatus
from api.db import StatusEnum
from api.db.db_models import File
from api.db.services import duplicate_name
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import TenantService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import construct_json_result, construct_error_response
from api.utils.api_utils import construct_result, validate_request
from api.utils.file_utils import filename_type, thumbnail
from rag.app import book, laws, manual, naive, one, paper, presentation, qa, resume, table, picture, audio
from rag.nlp import search
from rag.utils.es_conn import ELASTICSEARCH
from rag.utils.minio_conn import MINIO

MAXIMUM_OF_UPLOADING_FILES = 256


# ------------------------------ create a dataset ---------------------------------------

@manager.route("/", methods=["POST"])
@login_required  # use login
@validate_request("name")  # check name key
def create_dataset():
    # Check if Authorization header is present
    authorization_token = request.headers.get("Authorization")
    if not authorization_token:
        return construct_json_result(code=RetCode.AUTHENTICATION_ERROR, message="Authorization header is missing.")

    # TODO: Login or API key
    # objs = APIToken.query(token=authorization_token)
    #
    # # Authorization error
    # if not objs:
    #     return construct_json_result(code=RetCode.AUTHENTICATION_ERROR, message="Token is invalid.")
    #
    # tenant_id = objs[0].tenant_id

    tenant_id = current_user.id
    request_body = request.json

    # In case that there's no name
    if "name" not in request_body:
        return construct_json_result(code=RetCode.DATA_ERROR, message="Expected 'name' field in request body")

    dataset_name = request_body["name"]

    # empty dataset_name
    if not dataset_name:
        return construct_json_result(code=RetCode.DATA_ERROR, message="Empty dataset name")

    # In case that there's space in the head or the tail
    dataset_name = dataset_name.strip()

    # In case that the length of the name exceeds the limit
    dataset_name_length = len(dataset_name)
    if dataset_name_length > NAME_LENGTH_LIMIT:
        return construct_json_result(
            code=RetCode.DATA_ERROR,
            message=f"Dataset name: {dataset_name} with length {dataset_name_length} exceeds {NAME_LENGTH_LIMIT}!")

    # In case that there are other fields in the data-binary
    if len(request_body.keys()) > 1:
        name_list = []
        for key_name in request_body.keys():
            if key_name != "name":
                name_list.append(key_name)
        return construct_json_result(code=RetCode.DATA_ERROR,
                                     message=f"fields: {name_list}, are not allowed in request body.")

    # If there is a duplicate name, it will modify it to make it unique
    request_body["name"] = duplicate_name(
        KnowledgebaseService.query,
        name=dataset_name,
        tenant_id=tenant_id,
        status=StatusEnum.VALID.value)
    try:
        request_body["id"] = get_uuid()
        request_body["tenant_id"] = tenant_id
        request_body["created_by"] = tenant_id
        exist, t = TenantService.get_by_id(tenant_id)
        if not exist:
            return construct_result(code=RetCode.AUTHENTICATION_ERROR, message="Tenant not found.")
        request_body["embd_id"] = t.embd_id
        if not KnowledgebaseService.save(**request_body):
            # failed to create new dataset
            return construct_result()
        return construct_json_result(code=RetCode.SUCCESS,
                                     data={"dataset_name": request_body["name"], "dataset_id": request_body["id"]})
    except Exception as e:
        return construct_error_response(e)


# -----------------------------list datasets-------------------------------------------------------

@manager.route("/", methods=["GET"])
@login_required
def list_datasets():
    offset = request.args.get("offset", 0)
    count = request.args.get("count", -1)
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", True)
    try:
        tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
        datasets = KnowledgebaseService.get_by_tenant_ids_by_offset(
            [m["tenant_id"] for m in tenants], current_user.id, int(offset), int(count), orderby, desc)
        return construct_json_result(data=datasets, code=RetCode.SUCCESS, message=f"List datasets successfully!")
    except Exception as e:
        return construct_error_response(e)
    except HTTPError as http_err:
        return construct_json_result(http_err)


# ---------------------------------delete a dataset ----------------------------

@manager.route("/<dataset_id>", methods=["DELETE"])
@login_required
def remove_dataset(dataset_id):
    try:
        datasets = KnowledgebaseService.query(created_by=current_user.id, id=dataset_id)

        # according to the id, searching for the dataset
        if not datasets:
            return construct_json_result(message=f"The dataset cannot be found for your current account.",
                                         code=RetCode.OPERATING_ERROR)

        # Iterating the documents inside the dataset
        for doc in DocumentService.query(kb_id=dataset_id):
            if not DocumentService.remove_document(doc, datasets[0].tenant_id):
                # the process of deleting failed
                return construct_json_result(code=RetCode.DATA_ERROR,
                                             message="There was an error during the document removal process. "
                                                     "Please check the status of the RAGFlow server and try the removal again.")
            # delete the other files
            f2d = File2DocumentService.get_by_document_id(doc.id)
            FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
            File2DocumentService.delete_by_document_id(doc.id)

        # delete the dataset
        if not KnowledgebaseService.delete_by_id(dataset_id):
            return construct_json_result(code=RetCode.DATA_ERROR,
                                         message="There was an error during the dataset removal process. "
                                                 "Please check the status of the RAGFlow server and try the removal again.")
        # success
        return construct_json_result(code=RetCode.SUCCESS, message=f"Remove dataset: {dataset_id} successfully")
    except Exception as e:
        return construct_error_response(e)


# ------------------------------ get details of a dataset ----------------------------------------

@manager.route("/<dataset_id>", methods=["GET"])
@login_required
def get_dataset(dataset_id):
    try:
        dataset = KnowledgebaseService.get_detail(dataset_id)
        if not dataset:
            return construct_json_result(code=RetCode.DATA_ERROR, message="Can't find this dataset!")
        return construct_json_result(data=dataset, code=RetCode.SUCCESS)
    except Exception as e:
        return construct_json_result(e)


# ------------------------------ update a dataset --------------------------------------------

@manager.route("/<dataset_id>", methods=["PUT"])
@login_required
def update_dataset(dataset_id):
    req = request.json
    try:
        # the request cannot be empty
        if not req:
            return construct_json_result(code=RetCode.DATA_ERROR, message="Please input at least one parameter that "
                                                                          "you want to update!")
        # check whether the dataset can be found
        if not KnowledgebaseService.query(created_by=current_user.id, id=dataset_id):
            return construct_json_result(message=f"Only the owner of knowledgebase is authorized for this operation!",
                                         code=RetCode.OPERATING_ERROR)

        exist, dataset = KnowledgebaseService.get_by_id(dataset_id)
        # check whether there is this dataset
        if not exist:
            return construct_json_result(code=RetCode.DATA_ERROR, message="This dataset cannot be found!")

        if "name" in req:
            name = req["name"].strip()
            # check whether there is duplicate name
            if name.lower() != dataset.name.lower() \
                    and len(KnowledgebaseService.query(name=name, tenant_id=current_user.id,
                                                       status=StatusEnum.VALID.value)) > 1:
                return construct_json_result(code=RetCode.DATA_ERROR,
                                             message=f"The name: {name.lower()} is already used by other "
                                                     f"datasets. Please choose a different name.")

        dataset_updating_data = {}
        chunk_num = req.get("chunk_num")
        # modify the value of 11 parameters

        # 2 parameters: embedding id and chunk method
        # only if chunk_num is 0, the user can update the embedding id
        if req.get("embedding_model_id"):
            if chunk_num == 0:
                dataset_updating_data["embd_id"] = req["embedding_model_id"]
            else:
                return construct_json_result(code=RetCode.DATA_ERROR,
                                             message="You have already parsed the document in this "
                                                     "dataset, so you cannot change the embedding "
                                                     "model.")
        # only if chunk_num is 0, the user can update the chunk_method
        if "chunk_method" in req:
            type_value = req["chunk_method"]
            if is_illegal_value_for_enum(type_value, ParserType):
                return construct_json_result(message=f"Illegal value {type_value} for 'chunk_method' field.",
                                             code=RetCode.DATA_ERROR)
            if chunk_num != 0:
                construct_json_result(code=RetCode.DATA_ERROR, message="You have already parsed the document "
                                                                       "in this dataset, so you cannot "
                                                                       "change the chunk method.")
            dataset_updating_data["parser_id"] = req["template_type"]

        # convert the photo parameter to avatar
        if req.get("photo"):
            dataset_updating_data["avatar"] = req["photo"]

        # layout_recognize
        if "layout_recognize" in req:
            if "parser_config" not in dataset_updating_data:
                dataset_updating_data['parser_config'] = {}
            dataset_updating_data['parser_config']['layout_recognize'] = req['layout_recognize']

        # TODO: updating use_raptor needs to construct a class

        # 6 parameters
        for key in ["name", "language", "description", "permission", "id", "token_num"]:
            if key in req:
                dataset_updating_data[key] = req.get(key)

        # update
        if not KnowledgebaseService.update_by_id(dataset.id, dataset_updating_data):
            return construct_json_result(code=RetCode.OPERATING_ERROR, message="Failed to update! "
                                                                               "Please check the status of RAGFlow "
                                                                               "server and try again!")

        exist, dataset = KnowledgebaseService.get_by_id(dataset.id)
        if not exist:
            return construct_json_result(code=RetCode.DATA_ERROR, message="Failed to get the dataset "
                                                                          "using the dataset ID.")

        return construct_json_result(data=dataset.to_json(), code=RetCode.SUCCESS)
    except Exception as e:
        return construct_error_response(e)


# --------------------------------content management ----------------------------------------------

# ----------------------------upload files-----------------------------------------------------
@manager.route("/<dataset_id>/documents/", methods=["POST"])
@login_required
def upload_documents(dataset_id):
    # no files
    if not request.files:
        return construct_json_result(
            message="There is no file!", code=RetCode.ARGUMENT_ERROR)

    # the number of uploading files exceeds the limit
    file_objs = request.files.getlist("file")
    num_file_objs = len(file_objs)

    if num_file_objs > MAXIMUM_OF_UPLOADING_FILES:
        return construct_json_result(code=RetCode.DATA_ERROR, message=f"You try to upload {num_file_objs} files, "
                                                                      f"which exceeds the maximum number of uploading files: {MAXIMUM_OF_UPLOADING_FILES}")

    # no dataset
    exist, dataset = KnowledgebaseService.get_by_id(dataset_id)
    if not exist:
        return construct_json_result(message="Can't find this dataset", code=RetCode.DATA_ERROR)

    for file_obj in file_objs:
        file_name = file_obj.filename
        # no name
        if not file_name:
            return construct_json_result(
                message="There is a file without name!", code=RetCode.ARGUMENT_ERROR)

        # TODO: support the remote files
        if 'http' in file_name:
            return construct_json_result(code=RetCode.ARGUMENT_ERROR, message="Remote files have not unsupported.")

    # get the root_folder
    root_folder = FileService.get_root_folder(current_user.id)
    # get the id of the root_folder
    parent_file_id = root_folder["id"]  # document id
    # this is for the new user, create '.knowledgebase' file
    FileService.init_knowledgebase_docs(parent_file_id, current_user.id)
    # go inside this folder, get the kb_root_folder
    kb_root_folder = FileService.get_kb_folder(current_user.id)
    # link the file management to the kb_folder
    kb_folder = FileService.new_a_file_from_kb(dataset.tenant_id, dataset.name, kb_root_folder["id"])

    # grab all the errs
    err = []
    MAX_FILE_NUM_PER_USER = int(os.environ.get("MAX_FILE_NUM_PER_USER", 0))
    uploaded_docs_json = []
    for file in file_objs:
        try:
            # TODO: get this value from the database as some tenants have this limit while others don't
            if MAX_FILE_NUM_PER_USER > 0 and DocumentService.get_doc_count(dataset.tenant_id) >= MAX_FILE_NUM_PER_USER:
                return construct_json_result(code=RetCode.DATA_ERROR,
                                             message="Exceed the maximum file number of a free user!")
            # deal with the duplicate name
            filename = duplicate_name(
                DocumentService.query,
                name=file.filename,
                kb_id=dataset.id)

            # deal with the unsupported type
            filetype = filename_type(filename)
            if filetype == FileType.OTHER.value:
                return construct_json_result(code=RetCode.DATA_ERROR,
                                             message="This type of file has not been supported yet!")

            # upload to the minio
            location = filename
            while MINIO.obj_exist(dataset_id, location):
                location += "_"

            blob = file.read()

            # the content is empty, raising a warning
            if blob == b'':
                warnings.warn(f"[WARNING]: The content of the file {filename} is empty.")

            MINIO.put(dataset_id, location, blob)

            doc = {
                "id": get_uuid(),
                "kb_id": dataset.id,
                "parser_id": dataset.parser_id,
                "parser_config": dataset.parser_config,
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

            FileService.add_file_from_kb(doc, kb_folder["id"], dataset.tenant_id)
            uploaded_docs_json.append(doc)
        except Exception as e:
            err.append(file.filename + ": " + str(e))

    if err:
        # return all the errors
        return construct_json_result(message="\n".join(err), code=RetCode.SERVER_ERROR)
    # success
    return construct_json_result(data=uploaded_docs_json, code=RetCode.SUCCESS)


# ----------------------------delete a file-----------------------------------------------------
@manager.route("/<dataset_id>/documents/<document_id>", methods=["DELETE"])
@login_required
def delete_document(document_id, dataset_id):  # string
    # get the root folder
    root_folder = FileService.get_root_folder(current_user.id)
    # parent file's id
    parent_file_id = root_folder["id"]
    # consider the new user
    FileService.init_knowledgebase_docs(parent_file_id, current_user.id)
    # store all the errors that may have
    errors = ""
    try:
        # whether there is this document
        exist, doc = DocumentService.get_by_id(document_id)
        if not exist:
            return construct_json_result(message=f"Document {document_id} not found!", code=RetCode.DATA_ERROR)
        # whether this doc is authorized by this tenant
        tenant_id = DocumentService.get_tenant_id(document_id)
        if not tenant_id:
            return construct_json_result(
                message=f"You cannot delete this document {document_id} due to the authorization"
                        f" reason!", code=RetCode.AUTHENTICATION_ERROR)

        # get the doc's id and location
        real_dataset_id, location = File2DocumentService.get_minio_address(doc_id=document_id)

        if real_dataset_id != dataset_id:
            return construct_json_result(message=f"The document {document_id} is not in the dataset: {dataset_id}, "
                                                 f"but in the dataset: {real_dataset_id}.", code=RetCode.ARGUMENT_ERROR)

        # there is an issue when removing
        if not DocumentService.remove_document(doc, tenant_id):
            return construct_json_result(
                message="There was an error during the document removal process. Please check the status of the "
                        "RAGFlow server and try the removal again.", code=RetCode.OPERATING_ERROR)

        # fetch the File2Document record associated with the provided document ID.
        file_to_doc = File2DocumentService.get_by_document_id(document_id)
        # delete the associated File record.
        FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == file_to_doc[0].file_id])
        # delete the File2Document record itself using the document ID. This removes the
        # association between the document and the file after the File record has been deleted.
        File2DocumentService.delete_by_document_id(document_id)

        # delete it from minio
        MINIO.rm(dataset_id, location)
    except Exception as e:
        errors += str(e)
    if errors:
        return construct_json_result(data=False, message=errors, code=RetCode.SERVER_ERROR)

    return construct_json_result(data=True, code=RetCode.SUCCESS)


# ----------------------------list files-----------------------------------------------------
@manager.route('/<dataset_id>/documents/', methods=['GET'])
@login_required
def list_documents(dataset_id):
    if not dataset_id:
        return construct_json_result(
            data=False, message="Lack of 'dataset_id'", code=RetCode.ARGUMENT_ERROR)

    # searching keywords
    keywords = request.args.get("keywords", "")

    offset = request.args.get("offset", 0)
    count = request.args.get("count", -1)
    order_by = request.args.get("order_by", "create_time")
    descend = request.args.get("descend", True)
    try:
        docs, total = DocumentService.list_documents_in_dataset(dataset_id, int(offset), int(count), order_by,
                                                                descend, keywords)

        return construct_json_result(data={"total": total, "docs": docs}, message=RetCode.SUCCESS)
    except Exception as e:
        return construct_error_response(e)


# ----------------------------update: enable rename-----------------------------------------------------
@manager.route("/<dataset_id>/documents/<document_id>", methods=["PUT"])
@login_required
def update_document(dataset_id, document_id):
    req = request.json
    try:
        legal_parameters = set()
        legal_parameters.add("name")
        legal_parameters.add("enable")
        legal_parameters.add("template_type")

        for key in req.keys():
            if key not in legal_parameters:
                return construct_json_result(code=RetCode.ARGUMENT_ERROR, message=f"{key} is an illegal parameter.")

        # The request body cannot be empty
        if not req:
            return construct_json_result(
                code=RetCode.DATA_ERROR,
                message="Please input at least one parameter that you want to update!")

        # Check whether there is this dataset
        exist, dataset = KnowledgebaseService.get_by_id(dataset_id)
        if not exist:
            return construct_json_result(code=RetCode.DATA_ERROR, message=f"This dataset {dataset_id} cannot be found!")

        # The document does not exist
        exist, document = DocumentService.get_by_id(document_id)
        if not exist:
            return construct_json_result(message=f"This document {document_id} cannot be found!",
                                         code=RetCode.ARGUMENT_ERROR)

        # Deal with the different keys
        updating_data = {}
        if "name" in req:
            new_name = req["name"]
            updating_data["name"] = new_name
            # Check whether the new_name is suitable
            # 1. no name value
            if not new_name:
                return construct_json_result(code=RetCode.DATA_ERROR, message="There is no new name.")

            # 2. In case that there's space in the head or the tail
            new_name = new_name.strip()

            # 3. Check whether the new_name has the same extension of file as before
            if pathlib.Path(new_name.lower()).suffix != pathlib.Path(
                    document.name.lower()).suffix:
                return construct_json_result(
                    data=False,
                    message="The extension of file cannot be changed",
                    code=RetCode.ARGUMENT_ERROR)

            # 4. Check whether the new name has already been occupied by other file
            for d in DocumentService.query(name=new_name, kb_id=document.kb_id):
                if d.name == new_name:
                    return construct_json_result(
                        message="Duplicated document name in the same dataset.",
                        code=RetCode.ARGUMENT_ERROR)

        if "enable" in req:
            enable_value = req["enable"]
            if is_illegal_value_for_enum(enable_value, StatusEnum):
                return construct_json_result(message=f"Illegal value {enable_value} for 'enable' field.",
                                             code=RetCode.DATA_ERROR)
            updating_data["status"] = enable_value

        # TODO: Chunk-method - update parameters inside the json object parser_config
        if "template_type" in req:
            type_value = req["template_type"]
            if is_illegal_value_for_enum(type_value, ParserType):
                return construct_json_result(message=f"Illegal value {type_value} for 'template_type' field.",
                                             code=RetCode.DATA_ERROR)
            updating_data["parser_id"] = req["template_type"]

        # The process of updating
        if not DocumentService.update_by_id(document_id, updating_data):
            return construct_json_result(
                code=RetCode.OPERATING_ERROR,
                message="Failed to update document in the database! "
                        "Please check the status of RAGFlow server and try again!")

        # name part: file service
        if "name" in req:
            # Get file by document id
            file_information = File2DocumentService.get_by_document_id(document_id)
            if file_information:
                exist, file = FileService.get_by_id(file_information[0].file_id)
                FileService.update_by_id(file.id, {"name": req["name"]})

        exist, document = DocumentService.get_by_id(document_id)

        # Success
        return construct_json_result(data=document.to_json(), message="Success", code=RetCode.SUCCESS)
    except Exception as e:
        return construct_error_response(e)


# Helper method to judge whether it's an illegal value
def is_illegal_value_for_enum(value, enum_class):
    return value not in enum_class.__members__.values()


# ----------------------------download a file-----------------------------------------------------
@manager.route("/<dataset_id>/documents/<document_id>", methods=["GET"])
@login_required
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
        file_stream = MINIO.get(doc_id, doc_location)
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


# ----------------------------start parsing a document-----------------------------------------------------
# helper method for parsing
# callback method
def doc_parse_callback(doc_id, prog=None, msg=""):
    cancel = DocumentService.do_cancel(doc_id)
    if cancel:
        raise Exception("The parsing process has been cancelled!")

"""
def doc_parse(binary, doc_name, parser_name, tenant_id, doc_id):
    match parser_name:
        case "book":
            book.chunk(doc_name, binary=binary, callback=partial(doc_parse_callback, doc_id))
        case "laws":
            laws.chunk(doc_name, binary=binary, callback=partial(doc_parse_callback, doc_id))
        case "manual":
            manual.chunk(doc_name, binary=binary, callback=partial(doc_parse_callback, doc_id))
        case "naive":
            # It's the mode by default, which is general in the front-end
            naive.chunk(doc_name, binary=binary, callback=partial(doc_parse_callback, doc_id))
        case "one":
            one.chunk(doc_name, binary=binary, callback=partial(doc_parse_callback, doc_id))
        case "paper":
            paper.chunk(doc_name, binary=binary, callback=partial(doc_parse_callback, doc_id))
        case "picture":
            picture.chunk(doc_name, binary=binary, tenant_id=tenant_id, lang="Chinese",
                          callback=partial(doc_parse_callback, doc_id))
        case "presentation":
            presentation.chunk(doc_name, binary=binary, callback=partial(doc_parse_callback, doc_id))
        case "qa":
            qa.chunk(doc_name, binary=binary, callback=partial(doc_parse_callback, doc_id))
        case "resume":
            resume.chunk(doc_name, binary=binary, callback=partial(doc_parse_callback, doc_id))
        case "table":
            table.chunk(doc_name, binary=binary, callback=partial(doc_parse_callback, doc_id))
        case "audio":
            audio.chunk(doc_name, binary=binary, callback=partial(doc_parse_callback, doc_id))
        case _:
            return False

    return True
    """


@manager.route("/<dataset_id>/documents/<document_id>/status", methods=["POST"])
@login_required
def parse_document(dataset_id, document_id):
    try:
        # valid dataset
        exist, _ = KnowledgebaseService.get_by_id(dataset_id)
        if not exist:
            return construct_json_result(code=RetCode.DATA_ERROR,
                                         message=f"This dataset '{dataset_id}' cannot be found!")

        return parsing_document_internal(document_id)

    except Exception as e:
        return construct_error_response(e)


# ----------------------------start parsing documents-----------------------------------------------------
@manager.route("/<dataset_id>/documents/status", methods=["POST"])
@login_required
def parse_documents(dataset_id):
    doc_ids = request.json["doc_ids"]
    try:
        exist, _ = KnowledgebaseService.get_by_id(dataset_id)
        if not exist:
            return construct_json_result(code=RetCode.DATA_ERROR,
                                         message=f"This dataset '{dataset_id}' cannot be found!")
        # two conditions
        if not doc_ids:
            # documents inside the dataset
            docs, total = DocumentService.list_documents_in_dataset(dataset_id, 0, -1, "create_time",
                                                                    True, "")
            doc_ids = [doc["id"] for doc in docs]

        message = ""
        # for loop
        for id in doc_ids:
            res = parsing_document_internal(id)
            res_body = res.json
            if res_body["code"] == RetCode.SUCCESS:
                message += res_body["message"]
            else:
                return res
        return construct_json_result(data=True, code=RetCode.SUCCESS, message=message)

    except Exception as e:
        return construct_error_response(e)


# helper method for parsing the document
def parsing_document_internal(id):
    message = ""
    try:
        # Check whether there is this document
        exist, document = DocumentService.get_by_id(id)
        if not exist:
            return construct_json_result(message=f"This document '{id}' cannot be found!",
                                         code=RetCode.ARGUMENT_ERROR)

        tenant_id = DocumentService.get_tenant_id(id)
        if not tenant_id:
            return construct_json_result(message="Tenant not found!", code=RetCode.AUTHENTICATION_ERROR)

        info = {"run": "1", "progress": 0}
        info["progress_msg"] = ""
        info["chunk_num"] = 0
        info["token_num"] = 0

        DocumentService.update_by_id(id, info)

        ELASTICSEARCH.deleteByQuery(Q("match", doc_id=id), idxnm=search.index_name(tenant_id))

        _, doc_attributes = DocumentService.get_by_id(id)
        doc_attributes = doc_attributes.to_dict()
        doc_id = doc_attributes["id"]

        bucket, doc_name = File2DocumentService.get_minio_address(doc_id=doc_id)
        binary = MINIO.get(bucket, doc_name)
        parser_name = doc_attributes["parser_id"]
        if binary:
            res = doc_parse(binary, doc_name, parser_name, tenant_id, doc_id)
            if res is False:
                message += f"The parser id: {parser_name} of the document {doc_id} is not supported; "
        else:
            message += f"Empty data in the document: {doc_name}; "
        # failed in parsing
        if doc_attributes["status"] == TaskStatus.FAIL.value:
            message += f"Failed in parsing the document: {doc_id}; "
        return construct_json_result(code=RetCode.SUCCESS, message=message)
    except Exception as e:
        return construct_error_response(e)


# ----------------------------stop parsing a doc-----------------------------------------------------
@manager.route("<dataset_id>/documents/<document_id>/status", methods=["DELETE"])
@login_required
def stop_parsing_document(dataset_id, document_id):
    try:
        # valid dataset
        exist, _ = KnowledgebaseService.get_by_id(dataset_id)
        if not exist:
            return construct_json_result(code=RetCode.DATA_ERROR,
                                         message=f"This dataset '{dataset_id}' cannot be found!")

        return stop_parsing_document_internal(document_id)

    except Exception as e:
        return construct_error_response(e)


# ----------------------------stop parsing docs-----------------------------------------------------
@manager.route("<dataset_id>/documents/status", methods=["DELETE"])
@login_required
def stop_parsing_documents(dataset_id):
    doc_ids = request.json["doc_ids"]
    try:
        # valid dataset?
        exist, _ = KnowledgebaseService.get_by_id(dataset_id)
        if not exist:
            return construct_json_result(code=RetCode.DATA_ERROR,
                                         message=f"This dataset '{dataset_id}' cannot be found!")
        if not doc_ids:
            # documents inside the dataset
            docs, total = DocumentService.list_documents_in_dataset(dataset_id, 0, -1, "create_time",
                                                                        True, "")
            doc_ids = [doc["id"] for doc in docs]

        message = ""
        # for loop
        for id in doc_ids:
            res = stop_parsing_document_internal(id)
            res_body = res.json
            if res_body["code"] == RetCode.SUCCESS:
                message += res_body["message"]
            else:
                return res
        return construct_json_result(data=True, code=RetCode.SUCCESS, message=message)

    except Exception as e:
        return construct_error_response(e)


# Helper method
def stop_parsing_document_internal(document_id):
    try:
        # valid doc?
        exist, doc = DocumentService.get_by_id(document_id)
        if not exist:
            return construct_json_result(message=f"This document '{document_id}' cannot be found!",
                                         code=RetCode.ARGUMENT_ERROR)
        doc_attributes = doc.to_dict()

        # only when the status is parsing, we need to stop it
        if doc_attributes["status"] == TaskStatus.RUNNING.value:
            tenant_id = DocumentService.get_tenant_id(document_id)
            if not tenant_id:
                return construct_json_result(message="Tenant not found!", code=RetCode.AUTHENTICATION_ERROR)

            # update successfully?
            if not DocumentService.update_by_id(document_id, {"status": "2"}):  # cancel
                return construct_json_result(
                    code=RetCode.OPERATING_ERROR,
                    message="There was an error during the stopping parsing the document process. "
                            "Please check the status of the RAGFlow server and try the update again."
                )

            _, doc_attributes = DocumentService.get_by_id(document_id)
            doc_attributes = doc_attributes.to_dict()

            # failed in stop parsing
            if doc_attributes["status"] == TaskStatus.RUNNING.value:
                return construct_json_result(message=f"Failed in parsing the document: {document_id}; ", code=RetCode.SUCCESS)
        return construct_json_result(code=RetCode.SUCCESS, message="")
    except Exception as e:
        return construct_error_response(e)


# ----------------------------show the status of the file-----------------------------------------------------
@manager.route("/<dataset_id>/documents/<document_id>/status", methods=["GET"])
@login_required
def show_parsing_status(dataset_id, document_id):
    try:
        # valid dataset
        exist, _ = KnowledgebaseService.get_by_id(dataset_id)
        if not exist:
            return construct_json_result(code=RetCode.DATA_ERROR,
                                         message=f"This dataset: '{dataset_id}' cannot be found!")
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

# ----------------------------list the chunks of the file-----------------------------------------------------

# -- --------------------------delete the chunk-----------------------------------------------------

# ----------------------------edit the status of the chunk-----------------------------------------------------

# ----------------------------insert a new chunk-----------------------------------------------------

# ----------------------------upload a file-----------------------------------------------------

# ----------------------------get a specific chunk-----------------------------------------------------

# ----------------------------retrieval test-----------------------------------------------------
