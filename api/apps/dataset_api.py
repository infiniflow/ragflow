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


from flask import request
from flask_login import login_required, current_user
from httpx import HTTPError

from api.contants import NAME_LENGTH_LIMIT
from api.db import FileSource, StatusEnum
from api.db.db_models import File
from api.db.services import duplicate_name
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import TenantService
from api.settings import RetCode
from api.utils import get_uuid
from api.utils.api_utils import construct_json_result, construct_result, construct_error_response, validate_request


# ------------------------------ create a dataset ---------------------------------------

@manager.route('/', methods=['POST'])
@login_required  # use login
@validate_request("name")  # check name key
def create_dataset():
    # Check if Authorization header is present
    authorization_token = request.headers.get('Authorization')
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
        return construct_json_result(code=RetCode.DATA_ERROR,
                                     message=f"Dataset name: {dataset_name} with length {dataset_name_length} exceeds {NAME_LENGTH_LIMIT}!")

    # In case that there are other fields in the data-binary
    if len(request_body.keys()) > 1:
        name_list = []
        for key_name in request_body.keys():
            if key_name != 'name':
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

@manager.route('/', methods=['GET'])
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

@manager.route('/<dataset_id>', methods=['DELETE'])
@login_required
def remove_dataset(dataset_id):
    try:
        datasets = KnowledgebaseService.query(created_by=current_user.id, id=dataset_id)

        # according to the id, searching for the dataset
        if not datasets:
            return construct_json_result(message=f'The dataset cannot be found for your current account.',
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
            return construct_json_result(code=RetCode.DATA_ERROR, message="There was an error during the dataset removal process. "
                                                                          "Please check the status of the RAGFlow server and try the removal again.")
        # success
        return construct_json_result(code=RetCode.SUCCESS, message=f"Remove dataset: {dataset_id} successfully")
    except Exception as e:
        return construct_error_response(e)

# ------------------------------ get details of a dataset ----------------------------------------

@manager.route('/<dataset_id>', methods=['GET'])
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

@manager.route('/<dataset_id>', methods=['PUT'])
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
            return construct_json_result(message=f'Only the owner of knowledgebase is authorized for this operation!',
                                         code=RetCode.OPERATING_ERROR)

        exist, dataset = KnowledgebaseService.get_by_id(dataset_id)
        # check whether there is this dataset
        if not exist:
            return construct_json_result(code=RetCode.DATA_ERROR, message="This dataset cannot be found!")

        if 'name' in req:
            name = req["name"].strip()
            # check whether there is duplicate name
            if name.lower() != dataset.name.lower() \
                    and len(KnowledgebaseService.query(name=name, tenant_id=current_user.id,
                                                       status=StatusEnum.VALID.value)) > 1:
                return construct_json_result(code=RetCode.DATA_ERROR, message=f"The name: {name.lower()} is already used by other "
                                                                              f"datasets. Please choose a different name.")

        dataset_updating_data = {}
        chunk_num = req.get("chunk_num")
        # modify the value of 11 parameters

        # 2 parameters: embedding id and chunk method
        # only if chunk_num is 0, the user can update the embedding id
        if req.get('embedding_model_id'):
            if chunk_num == 0:
                dataset_updating_data['embd_id'] = req['embedding_model_id']
            else:
                construct_json_result(code=RetCode.DATA_ERROR, message="You have already parsed the document in this "
                                                                       "dataset, so you cannot change the embedding "
                                                                       "model.")
        # only if chunk_num is 0, the user can update the chunk_method
        if req.get("chunk_method"):
            if chunk_num == 0:
                dataset_updating_data['parser_id'] = req["chunk_method"]
            else:
                construct_json_result(code=RetCode.DATA_ERROR, message="You have already parsed the document "
                                                                       "in this dataset, so you cannot "
                                                                       "change the chunk method.")
        # convert the photo parameter to avatar
        if req.get("photo"):
            dataset_updating_data['avatar'] = req["photo"]

        # layout_recognize
        if 'layout_recognize' in req:
            if 'parser_config' not in dataset_updating_data:
                dataset_updating_data['parser_config'] = {}
            dataset_updating_data['parser_config']['layout_recognize'] = req['layout_recognize']

        # TODO: updating use_raptor needs to construct a class

        # 6 parameters
        for key in ['name', 'language', 'description', 'permission', 'id', 'token_num']:
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
