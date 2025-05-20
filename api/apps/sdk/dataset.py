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
#


import logging

from flask import request
from peewee import OperationalError

from api.db import FileSource, StatusEnum
from api.db.db_models import File
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import TenantService
from api.utils import get_uuid
from api.utils.api_utils import (
    deep_merge,
    get_error_argument_result,
    get_error_data_result,
    get_error_operating_result,
    get_error_permission_result,
    get_parser_config,
    get_result,
    remap_dictionary_keys,
    token_required,
    verify_embedding_availability,
)
from api.utils.validation_utils import (
    CreateDatasetReq,
    DeleteDatasetReq,
    ListDatasetReq,
    UpdateDatasetReq,
    validate_and_parse_json_request,
    validate_and_parse_request_args,
)


@manager.route("/datasets", methods=["POST"])  # noqa: F821
@token_required
def create(tenant_id):
    """
    Create a new dataset.
    ---
    tags:
      - Datasets
    security:
      - ApiKeyAuth: []
    parameters:
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Dataset creation parameters.
        required: true
        schema:
          type: object
          required:
            - name
          properties:
            name:
              type: string
              description: Name of the dataset.
            avatar:
              type: string
              description: Base64 encoding of the avatar.
            description:
              type: string
              description: Description of the dataset.
            embedding_model:
              type: string
              description: Embedding model Name.
            permission:
              type: string
              enum: ['me', 'team']
              description: Dataset permission.
            chunk_method:
              type: string
              enum: ["naive", "book", "email", "laws", "manual", "one", "paper",
                     "picture", "presentation", "qa", "table", "tag"
                     ]
              description: Chunking method.
            pagerank:
              type: integer
              description: Set page rank.
            parser_config:
              type: object
              description: Parser configuration.
    responses:
      200:
        description: Successful operation.
        schema:
          type: object
          properties:
            data:
              type: object
    """
    # Field name transformations during model dump:
    # | Original       | Dump Output  |
    # |----------------|-------------|
    # | embedding_model| embd_id     |
    # | chunk_method   | parser_id   |
    req, err = validate_and_parse_json_request(request, CreateDatasetReq)
    if err is not None:
        return get_error_argument_result(err)

    try:
        if KnowledgebaseService.get_or_none(name=req["name"], tenant_id=tenant_id, status=StatusEnum.VALID.value):
            return get_error_operating_result(message=f"Dataset name '{req['name']}' already exists")
    except OperationalError as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")

    req["parser_config"] = get_parser_config(req["parser_id"], req["parser_config"])
    req["id"] = get_uuid()
    req["tenant_id"] = tenant_id
    req["created_by"] = tenant_id

    try:
        ok, t = TenantService.get_by_id(tenant_id)
        if not ok:
            return get_error_permission_result(message="Tenant not found")
    except OperationalError as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")

    if not req.get("embd_id"):
        req["embd_id"] = t.embd_id
    else:
        ok, err = verify_embedding_availability(req["embd_id"], tenant_id)
        if not ok:
            return err

    try:
        if not KnowledgebaseService.save(**req):
            return get_error_data_result(message="Create dataset error.(Database error)")
    except OperationalError as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")

    try:
        ok, k = KnowledgebaseService.get_by_id(req["id"])
        if not ok:
            return get_error_data_result(message="Dataset created failed")
    except OperationalError as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")

    response_data = remap_dictionary_keys(k.to_dict())
    return get_result(data=response_data)


@manager.route("/datasets", methods=["DELETE"])  # noqa: F821
@token_required
def delete(tenant_id):
    """
    Delete datasets.
    ---
    tags:
      - Datasets
    security:
      - ApiKeyAuth: []
    parameters:
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Dataset deletion parameters.
        required: true
        schema:
          type: object
          required:
            - ids
          properties:
            ids:
              type: array or null
              items:
                type: string
              description: |
                Specifies the datasets to delete:
                - If `null`, all datasets will be deleted.
                - If an array of IDs, only the specified datasets will be deleted.
                - If an empty array, no datasets will be deleted.
    responses:
      200:
        description: Successful operation.
        schema:
          type: object
    """
    req, err = validate_and_parse_json_request(request, DeleteDatasetReq)
    if err is not None:
        return get_error_argument_result(err)

    kb_id_instance_pairs = []
    if req["ids"] is None:
        try:
            kbs = KnowledgebaseService.query(tenant_id=tenant_id)
            for kb in kbs:
                kb_id_instance_pairs.append((kb.id, kb))
        except OperationalError as e:
            logging.exception(e)
            return get_error_data_result(message="Database operation failed")
    else:
        error_kb_ids = []
        for kb_id in req["ids"]:
            try:
                kb = KnowledgebaseService.get_or_none(id=kb_id, tenant_id=tenant_id)
                if kb is None:
                    error_kb_ids.append(kb_id)
                    continue
                kb_id_instance_pairs.append((kb_id, kb))
            except OperationalError as e:
                logging.exception(e)
                return get_error_data_result(message="Database operation failed")
        if len(error_kb_ids) > 0:
            return get_error_permission_result(message=f"""User '{tenant_id}' lacks permission for datasets: '{", ".join(error_kb_ids)}'""")

    errors = []
    success_count = 0
    for kb_id, kb in kb_id_instance_pairs:
        try:
            for doc in DocumentService.query(kb_id=kb_id):
                if not DocumentService.remove_document(doc, tenant_id):
                    errors.append(f"Remove document '{doc.id}' error for dataset '{kb_id}'")
                    continue
                f2d = File2DocumentService.get_by_document_id(doc.id)
                FileService.filter_delete(
                    [
                        File.source_type == FileSource.KNOWLEDGEBASE,
                        File.id == f2d[0].file_id,
                    ]
                )
                File2DocumentService.delete_by_document_id(doc.id)
            FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.type == "folder", File.name == kb.name])
            if not KnowledgebaseService.delete_by_id(kb_id):
                errors.append(f"Delete dataset error for {kb_id}")
                continue
            success_count += 1
        except OperationalError as e:
            logging.exception(e)
            return get_error_data_result(message="Database operation failed")

    if not errors:
        return get_result()

    error_message = f"Successfully deleted {success_count} datasets, {len(errors)} failed. Details: {'; '.join(errors)[:128]}..."
    if success_count == 0:
        return get_error_data_result(message=error_message)

    return get_result(data={"success_count": success_count, "errors": errors[:5]}, message=error_message)


@manager.route("/datasets/<dataset_id>", methods=["PUT"])  # noqa: F821
@token_required
def update(tenant_id, dataset_id):
    """
    Update a dataset.
    ---
    tags:
      - Datasets
    security:
      - ApiKeyAuth: []
    parameters:
      - in: path
        name: dataset_id
        type: string
        required: true
        description: ID of the dataset to update.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
      - in: body
        name: body
        description: Dataset update parameters.
        required: true
        schema:
          type: object
          properties:
            name:
              type: string
              description: New name of the dataset.
            avatar:
              type: string
              description: Updated base64 encoding of the avatar.
            description:
              type: string
              description: Updated description of the dataset.
            embedding_model:
              type: string
              description: Updated embedding model Name.
            permission:
              type: string
              enum: ['me', 'team']
              description: Updated dataset permission.
            chunk_method:
              type: string
              enum: ["naive", "book", "email", "laws", "manual", "one", "paper",
                     "picture", "presentation", "qa", "table", "tag"
                     ]
              description: Updated chunking method.
            pagerank:
              type: integer
              description: Updated page rank.
            parser_config:
              type: object
              description: Updated parser configuration.
    responses:
      200:
        description: Successful operation.
        schema:
          type: object
    """
    # Field name transformations during model dump:
    # | Original       | Dump Output  |
    # |----------------|-------------|
    # | embedding_model| embd_id     |
    # | chunk_method   | parser_id   |
    extras = {"dataset_id": dataset_id}
    req, err = validate_and_parse_json_request(request, UpdateDatasetReq, extras=extras, exclude_unset=True)
    if err is not None:
        return get_error_argument_result(err)

    if not req:
        return get_error_argument_result(message="No properties were modified")

    try:
        kb = KnowledgebaseService.get_or_none(id=dataset_id, tenant_id=tenant_id)
        if kb is None:
            return get_error_permission_result(message=f"User '{tenant_id}' lacks permission for dataset '{dataset_id}'")
    except OperationalError as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")

    if req.get("parser_config"):
        req["parser_config"] = deep_merge(kb.parser_config, req["parser_config"])

    if (chunk_method := req.get("parser_id")) and chunk_method != kb.parser_id:
        if not req.get("parser_config"):
            req["parser_config"] = get_parser_config(chunk_method, None)
    elif "parser_config" in req and not req["parser_config"]:
        del req["parser_config"]

    if "name" in req and req["name"].lower() != kb.name.lower():
        try:
            exists = KnowledgebaseService.get_or_none(name=req["name"], tenant_id=tenant_id, status=StatusEnum.VALID.value)
            if exists:
                return get_error_data_result(message=f"Dataset name '{req['name']}' already exists")
        except OperationalError as e:
            logging.exception(e)
            return get_error_data_result(message="Database operation failed")

    if "embd_id" in req:
        if kb.chunk_num != 0 and req["embd_id"] != kb.embd_id:
            return get_error_data_result(message=f"When chunk_num ({kb.chunk_num}) > 0, embedding_model must remain {kb.embd_id}")
        ok, err = verify_embedding_availability(req["embd_id"], tenant_id)
        if not ok:
            return err

    try:
        if not KnowledgebaseService.update_by_id(kb.id, req):
            return get_error_data_result(message="Update dataset error.(Database error)")
    except OperationalError as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")

    return get_result()


@manager.route("/datasets", methods=["GET"])  # noqa: F821
@token_required
def list_datasets(tenant_id):
    """
    List datasets.
    ---
    tags:
      - Datasets
    security:
      - ApiKeyAuth: []
    parameters:
      - in: query
        name: id
        type: string
        required: false
        description: Dataset ID to filter.
      - in: query
        name: name
        type: string
        required: false
        description: Dataset name to filter.
      - in: query
        name: page
        type: integer
        required: false
        default: 1
        description: Page number.
      - in: query
        name: page_size
        type: integer
        required: false
        default: 30
        description: Number of items per page.
      - in: query
        name: orderby
        type: string
        required: false
        default: "create_time"
        description: Field to order by.
      - in: query
        name: desc
        type: boolean
        required: false
        default: true
        description: Order in descending.
      - in: header
        name: Authorization
        type: string
        required: true
        description: Bearer token for authentication.
    responses:
      200:
        description: Successful operation.
        schema:
          type: array
          items:
            type: object
    """
    args, err = validate_and_parse_request_args(request, ListDatasetReq)
    if err is not None:
        return get_error_argument_result(err)

    kb_id = request.args.get("id")
    name = args.get("name")
    if kb_id:
        try:
            kbs = KnowledgebaseService.get_kb_by_id(kb_id, tenant_id)
        except OperationalError as e:
            logging.exception(e)
            return get_error_data_result(message="Database operation failed")
        if not kbs:
            return get_error_permission_result(message=f"User '{tenant_id}' lacks permission for dataset '{kb_id}'")
    if name:
        try:
            kbs = KnowledgebaseService.get_kb_by_name(name, tenant_id)
        except OperationalError as e:
            logging.exception(e)
            return get_error_data_result(message="Database operation failed")
        if not kbs:
            return get_error_permission_result(message=f"User '{tenant_id}' lacks permission for dataset '{name}'")

    try:
        tenants = TenantService.get_joined_tenants_by_user_id(tenant_id)
        kbs = KnowledgebaseService.get_list(
            [m["tenant_id"] for m in tenants],
            tenant_id,
            args["page"],
            args["page_size"],
            args["orderby"],
            args["desc"],
            kb_id,
            name,
        )
    except OperationalError as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")

    response_data_list = []
    for kb in kbs:
        response_data_list.append(remap_dictionary_keys(kb))
    return get_result(data=response_data_list)
