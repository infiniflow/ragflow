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
import os
import json
from quart import request
from peewee import OperationalError
from api.db.db_models import File
from api.db.services.document_service import DocumentService, queue_raptor_o_graphrag_tasks
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.task_service import GRAPH_RAPTOR_FAKE_DOC_ID, TaskService
from api.db.services.user_service import TenantService
from common.constants import RetCode, FileSource, StatusEnum
from api.utils.api_utils import (
    deep_merge,
    get_error_argument_result,
    get_error_data_result,
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
from rag.nlp import search
from common.constants import PAGERANK_FLD
from common import settings


@manager.route("/datasets", methods=["POST"])  # noqa: F821
@token_required
async def create(tenant_id):
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
              description: Dataset name (required).
            avatar:
              type: string
              description: Optional base64-encoded avatar image.
            description:
              type: string
              description: Optional dataset description.
            embedding_model:
              type: string
              description: Optional embedding model name; if omitted, the tenant's default embedding model is used.
            permission:
              type: string
              enum: ['me', 'team']
              description: Visibility of the dataset (private to me or shared with team).
            chunk_method:
              type: string
              enum: ["naive", "book", "email", "laws", "manual", "one", "paper",
                     "picture", "presentation", "qa", "table", "tag"]
              description: Chunking method; if omitted, defaults to "naive".
            parser_config:
              type: object
              description: Optional parser configuration; server-side defaults will be applied.
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

    req, err = await validate_and_parse_json_request(request, CreateDatasetReq)
    if err is not None:
        return get_error_argument_result(err)
    e, req = KnowledgebaseService.create_with_name(
        name = req.pop("name", None),
        tenant_id = tenant_id,
        parser_id = req.pop("parser_id", None),
        **req
    )

    if not e:
        return req

    # Insert embedding model(embd id)
    ok, t = TenantService.get_by_id(tenant_id)
    if not ok:
        return get_error_permission_result(message="Tenant not found")
    if not req.get("embd_id"):
        req["embd_id"] = t.embd_id
    else:
        ok, err = verify_embedding_availability(req["embd_id"], tenant_id)
        if not ok:
            return err


    try:
      if not KnowledgebaseService.save(**req):
          return get_error_data_result()
      ok, k = KnowledgebaseService.get_by_id(req["id"])
      if not ok:
        return get_error_data_result(message="Dataset created failed")
      response_data = remap_dictionary_keys(k.to_dict())
      return get_result(data=response_data)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")

@manager.route("/datasets", methods=["DELETE"])  # noqa: F821
@token_required
async def delete(tenant_id):
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
    req, err = await validate_and_parse_json_request(request, DeleteDatasetReq)
    if err is not None:
        return get_error_argument_result(err)

    try:
        kb_id_instance_pairs = []
        if req["ids"] is None:
            kbs = KnowledgebaseService.query(tenant_id=tenant_id)
            for kb in kbs:
                kb_id_instance_pairs.append((kb.id, kb))

        else:
            error_kb_ids = []
            for kb_id in req["ids"]:
                kb = KnowledgebaseService.get_or_none(id=kb_id, tenant_id=tenant_id)
                if kb is None:
                    error_kb_ids.append(kb_id)
                    continue
                kb_id_instance_pairs.append((kb_id, kb))
            if len(error_kb_ids) > 0:
                return get_error_permission_result(
                    message=f"""User '{tenant_id}' lacks permission for datasets: '{", ".join(error_kb_ids)}'""")

        errors = []
        success_count = 0
        for kb_id, kb in kb_id_instance_pairs:
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
            FileService.filter_delete(
                [File.source_type == FileSource.KNOWLEDGEBASE, File.type == "folder", File.name == kb.name])

            # Drop index for this dataset
            try:
                from rag.nlp import search
                idxnm = search.index_name(kb.tenant_id)
                settings.docStoreConn.delete_idx(idxnm, kb_id)
            except Exception as e:
                logging.warning(f"Failed to drop index for dataset {kb_id}: {e}")

            if not KnowledgebaseService.delete_by_id(kb_id):
                errors.append(f"Delete dataset error for {kb_id}")
                continue
            success_count += 1

        if not errors:
            return get_result()

        error_message = f"Successfully deleted {success_count} datasets, {len(errors)} failed. Details: {'; '.join(errors)[:128]}..."
        if success_count == 0:
            return get_error_data_result(message=error_message)

        return get_result(data={"success_count": success_count, "errors": errors[:5]}, message=error_message)
    except OperationalError as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")


@manager.route("/datasets/<dataset_id>", methods=["PUT"])  # noqa: F821
@token_required
async def update(tenant_id, dataset_id):
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
    req, err = await validate_and_parse_json_request(request, UpdateDatasetReq, extras=extras, exclude_unset=True)
    if err is not None:
        return get_error_argument_result(err)

    if not req:
        return get_error_argument_result(message="No properties were modified")

    try:
        kb = KnowledgebaseService.get_or_none(id=dataset_id, tenant_id=tenant_id)
        if kb is None:
            return get_error_permission_result(
                message=f"User '{tenant_id}' lacks permission for dataset '{dataset_id}'")

        if req.get("parser_config"):
            req["parser_config"] = deep_merge(kb.parser_config, req["parser_config"])

        if (chunk_method := req.get("parser_id")) and chunk_method != kb.parser_id:
            if not req.get("parser_config"):
                req["parser_config"] = get_parser_config(chunk_method, None)
        elif "parser_config" in req and not req["parser_config"]:
            del req["parser_config"]

        if "name" in req and req["name"].lower() != kb.name.lower():
            exists = KnowledgebaseService.get_or_none(name=req["name"], tenant_id=tenant_id,
                                                      status=StatusEnum.VALID.value)
            if exists:
                return get_error_data_result(message=f"Dataset name '{req['name']}' already exists")

        if "embd_id" in req:
            if not req["embd_id"]:
                req["embd_id"] = kb.embd_id
            if kb.chunk_num != 0 and req["embd_id"] != kb.embd_id:
                return get_error_data_result(
                    message=f"When chunk_num ({kb.chunk_num}) > 0, embedding_model must remain {kb.embd_id}")
            ok, err = verify_embedding_availability(req["embd_id"], tenant_id)
            if not ok:
                return err

        if "pagerank" in req and req["pagerank"] != kb.pagerank:
            if os.environ.get("DOC_ENGINE", "elasticsearch") == "infinity":
                return get_error_argument_result(message="'pagerank' can only be set when doc_engine is elasticsearch")

            if req["pagerank"] > 0:
                settings.docStoreConn.update({"kb_id": kb.id}, {PAGERANK_FLD: req["pagerank"]},
                                             search.index_name(kb.tenant_id), kb.id)
            else:
                # Elasticsearch requires PAGERANK_FLD be non-zero!
                settings.docStoreConn.update({"exists": PAGERANK_FLD}, {"remove": PAGERANK_FLD},
                                             search.index_name(kb.tenant_id), kb.id)

        if not KnowledgebaseService.update_by_id(kb.id, req):
            return get_error_data_result(message="Update dataset error.(Database error)")

        ok, k = KnowledgebaseService.get_by_id(kb.id)
        if not ok:
            return get_error_data_result(message="Dataset created failed")

        response_data = remap_dictionary_keys(k.to_dict())
        return get_result(data=response_data)
    except OperationalError as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")


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

    try:
        kb_id = request.args.get("id")
        name = args.get("name")
        if kb_id:
            kbs = KnowledgebaseService.get_kb_by_id(kb_id, tenant_id)

            if not kbs:
                return get_error_permission_result(message=f"User '{tenant_id}' lacks permission for dataset '{kb_id}'")
        if name:
            kbs = KnowledgebaseService.get_kb_by_name(name, tenant_id)
            if not kbs:
                return get_error_permission_result(message=f"User '{tenant_id}' lacks permission for dataset '{name}'")

        tenants = TenantService.get_joined_tenants_by_user_id(tenant_id)
        kbs, total = KnowledgebaseService.get_list(
            [m["tenant_id"] for m in tenants],
            tenant_id,
            args["page"],
            args["page_size"],
            args["orderby"],
            args["desc"],
            kb_id,
            name,
        )

        response_data_list = []
        for kb in kbs:
            response_data_list.append(remap_dictionary_keys(kb))
        return get_result(data=response_data_list, total=total)
    except OperationalError as e:
        logging.exception(e)
        return get_error_data_result(message="Database operation failed")


@manager.route('/datasets/<dataset_id>/knowledge_graph', methods=['GET'])  # noqa: F821
@token_required
async def knowledge_graph(tenant_id, dataset_id):
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return get_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR
        )
    _, kb = KnowledgebaseService.get_by_id(dataset_id)
    req = {
        "kb_id": [dataset_id],
        "knowledge_graph_kwd": ["graph"]
    }

    obj = {"graph": {}, "mind_map": {}}
    if not settings.docStoreConn.index_exist(search.index_name(kb.tenant_id), dataset_id):
        return get_result(data=obj)
    sres = await settings.retriever.search(req, search.index_name(kb.tenant_id), [dataset_id])
    if not len(sres.ids):
        return get_result(data=obj)

    for id in sres.ids[:1]:
        ty = sres.field[id]["knowledge_graph_kwd"]
        try:
            content_json = json.loads(sres.field[id]["content_with_weight"])
        except Exception:
            continue

        obj[ty] = content_json

    if "nodes" in obj["graph"]:
        obj["graph"]["nodes"] = sorted(obj["graph"]["nodes"], key=lambda x: x.get("pagerank", 0), reverse=True)[:256]
        if "edges" in obj["graph"]:
            node_id_set = {o["id"] for o in obj["graph"]["nodes"]}
            filtered_edges = [o for o in obj["graph"]["edges"] if
                              o["source"] != o["target"] and o["source"] in node_id_set and o["target"] in node_id_set]
            obj["graph"]["edges"] = sorted(filtered_edges, key=lambda x: x.get("weight", 0), reverse=True)[:128]
    return get_result(data=obj)


@manager.route('/datasets/<dataset_id>/knowledge_graph', methods=['DELETE'])  # noqa: F821
@token_required
def delete_knowledge_graph(tenant_id, dataset_id):
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return get_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR
        )
    _, kb = KnowledgebaseService.get_by_id(dataset_id)
    settings.docStoreConn.delete({"knowledge_graph_kwd": ["graph", "subgraph", "entity", "relation"]},
                                 search.index_name(kb.tenant_id), dataset_id)

    return get_result(data=True)


@manager.route("/datasets/<dataset_id>/run_graphrag", methods=["POST"])  # noqa: F821
@token_required
def run_graphrag(tenant_id,dataset_id):
    if not dataset_id:
        return get_error_data_result(message='Lack of "Dataset ID"')
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return get_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR
        )

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return get_error_data_result(message="Invalid Dataset ID")

    task_id = kb.graphrag_task_id
    if task_id:
        ok, task = TaskService.get_by_id(task_id)
        if not ok:
            logging.warning(f"A valid GraphRAG task id is expected for Dataset {dataset_id}")

        if task and task.progress not in [-1, 1]:
            return get_error_data_result(message=f"Task {task_id} in progress with status {task.progress}. A Graph Task is already running.")

    documents, _ = DocumentService.get_by_kb_id(
        kb_id=dataset_id,
        page_number=0,
        items_per_page=0,
        orderby="create_time",
        desc=False,
        keywords="",
        run_status=[],
        types=[],
        suffix=[],
    )
    if not documents:
        return get_error_data_result(message=f"No documents in Dataset {dataset_id}")

    sample_document = documents[0]
    document_ids = [document["id"] for document in documents]

    task_id = queue_raptor_o_graphrag_tasks(sample_doc_id=sample_document, ty="graphrag", priority=0, fake_doc_id=GRAPH_RAPTOR_FAKE_DOC_ID, doc_ids=list(document_ids))

    if not KnowledgebaseService.update_by_id(kb.id, {"graphrag_task_id": task_id}):
        logging.warning(f"Cannot save graphrag_task_id for Dataset {dataset_id}")

    return get_result(data={"graphrag_task_id": task_id})


@manager.route("/datasets/<dataset_id>/trace_graphrag", methods=["GET"])  # noqa: F821
@token_required
def trace_graphrag(tenant_id,dataset_id):
    if not dataset_id:
        return get_error_data_result(message='Lack of "Dataset ID"')
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return get_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR
        )

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return get_error_data_result(message="Invalid Dataset ID")

    task_id = kb.graphrag_task_id
    if not task_id:
        return get_result(data={})

    ok, task = TaskService.get_by_id(task_id)
    if not ok:
        return get_result(data={})

    return get_result(data=task.to_dict())


@manager.route("/datasets/<dataset_id>/run_raptor", methods=["POST"])  # noqa: F821
@token_required
def run_raptor(tenant_id,dataset_id):
    if not dataset_id:
        return get_error_data_result(message='Lack of "Dataset ID"')
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return get_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR
        )

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return get_error_data_result(message="Invalid Dataset ID")

    task_id = kb.raptor_task_id
    if task_id:
        ok, task = TaskService.get_by_id(task_id)
        if not ok:
            logging.warning(f"A valid RAPTOR task id is expected for Dataset {dataset_id}")

        if task and task.progress not in [-1, 1]:
            return get_error_data_result(message=f"Task {task_id} in progress with status {task.progress}. A RAPTOR Task is already running.")

    documents, _ = DocumentService.get_by_kb_id(
        kb_id=dataset_id,
        page_number=0,
        items_per_page=0,
        orderby="create_time",
        desc=False,
        keywords="",
        run_status=[],
        types=[],
        suffix=[],
    )
    if not documents:
        return get_error_data_result(message=f"No documents in Dataset {dataset_id}")

    sample_document = documents[0]
    document_ids = [document["id"] for document in documents]

    task_id = queue_raptor_o_graphrag_tasks(sample_doc_id=sample_document, ty="raptor", priority=0, fake_doc_id=GRAPH_RAPTOR_FAKE_DOC_ID, doc_ids=list(document_ids))

    if not KnowledgebaseService.update_by_id(kb.id, {"raptor_task_id": task_id}):
        logging.warning(f"Cannot save raptor_task_id for Dataset {dataset_id}")

    return get_result(data={"raptor_task_id": task_id})


@manager.route("/datasets/<dataset_id>/trace_raptor", methods=["GET"])  # noqa: F821
@token_required
def trace_raptor(tenant_id,dataset_id):
    if not dataset_id:
        return get_error_data_result(message='Lack of "Dataset ID"')

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return get_result(
            data=False,
            message='No authorization.',
            code=RetCode.AUTHENTICATION_ERROR
        )
    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return get_error_data_result(message="Invalid Dataset ID")

    task_id = kb.raptor_task_id
    if not task_id:
        return get_result(data={})

    ok, task = TaskService.get_by_id(task_id)
    if not ok:
        return get_error_data_result(message="RAPTOR Task Not Found or Error Occurred")

    return get_result(data=task.to_dict())