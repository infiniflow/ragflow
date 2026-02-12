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
#  limitations under the License.
#
import logging
import json
import os
from common.constants import PAGERANK_FLD
from common import settings
from api.db.db_models import File
from api.db.services.document_service import DocumentService, queue_raptor_o_graphrag_tasks
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.task_service import GRAPH_RAPTOR_FAKE_DOC_ID, TaskService
from api.db.services.user_service import TenantService
from common.constants import FileSource, StatusEnum
from api.utils.api_utils import deep_merge, get_parser_config, remap_dictionary_keys, verify_embedding_availability


async def create_dataset(tenant_id: str, req: dict):
    """
    Create a new dataset.

    :param tenant_id: tenant ID
    :param req: dataset creation request
    :return: (success, result) or (success, error_message)
    """
    e, req = KnowledgebaseService.create_with_name(
        name=req.pop("name", None),
        tenant_id=tenant_id,
        parser_id=req.pop("parser_id", None),
        **req
    )

    if not e:
        return False, req

    # Insert embedding model(embd id)
    ok, t = TenantService.get_by_id(tenant_id)
    if not ok:
        return False, "Tenant not found"
    if not req.get("embd_id"):
        req["embd_id"] = t.embd_id
    else:
        ok, err = verify_embedding_availability(req["embd_id"], tenant_id)
        if not ok:
            return False, err

    if not KnowledgebaseService.save(**req):
        return False, "Failed to save dataset"
    ok, k = KnowledgebaseService.get_by_id(req["id"])
    if not ok:
        return False, "Dataset created failed"
    response_data = remap_dictionary_keys(k.to_dict())
    return True, response_data


async def delete_datasets(tenant_id: str, ids: list = None):
    """
    Delete datasets.

    :param tenant_id: tenant ID
    :param ids: list of dataset IDs, None means delete all
    :return: (success, result) or (success, error_message)
    """
    kb_id_instance_pairs = []
    if ids is None:
        kbs = KnowledgebaseService.query(tenant_id=tenant_id)
        for kb in kbs:
            kb_id_instance_pairs.append((kb.id, kb))
    else:
        error_kb_ids = []
        for kb_id in ids:
            kb = KnowledgebaseService.get_or_none(id=kb_id, tenant_id=tenant_id)
            if kb is None:
                error_kb_ids.append(kb_id)
                continue
            kb_id_instance_pairs.append((kb_id, kb))
        if len(error_kb_ids) > 0:
            return False, f"User '{tenant_id}' lacks permission for datasets: '{', '.join(error_kb_ids)}'"

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
            errors.append(f"Failed to drop index for dataset {kb_id}: {e}")

        if not KnowledgebaseService.delete_by_id(kb_id):
            errors.append(f"Delete dataset error for {kb_id}")
            continue
        success_count += 1

    if not errors:
        return True, {"success_count": success_count}

    error_message = f"Successfully deleted {success_count} datasets, {len(errors)} failed. Details: {'; '.join(errors)[:128]}..."
    if success_count == 0:
        return False, error_message

    return True, {"success_count": success_count, "errors": errors[:5]}


async def update_dataset(tenant_id: str, dataset_id: str, req: dict):
    """
    Update a dataset.

    :param tenant_id: tenant ID
    :param dataset_id: dataset ID
    :param req: dataset update request
    :return: (success, result) or (success, error_message)
    """
    if not req:
        return False, "No properties were modified"

    kb = KnowledgebaseService.get_or_none(id=dataset_id, tenant_id=tenant_id)
    if kb is None:
        return False, f"User '{tenant_id}' lacks permission for dataset '{dataset_id}'"

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
            return False, f"Dataset name '{req['name']}' already exists"

    if "embd_id" in req:
        if not req["embd_id"]:
            req["embd_id"] = kb.embd_id
        if kb.chunk_num != 0 and req["embd_id"] != kb.embd_id:
            return False, f"When chunk_num ({kb.chunk_num}) > 0, embedding_model must remain {kb.embd_id}"
        ok, err = verify_embedding_availability(req["embd_id"], tenant_id)
        if not ok:
            return False, err

    if "pagerank" in req and req["pagerank"] != kb.pagerank:
        if os.environ.get("DOC_ENGINE", "elasticsearch") == "infinity":
            return False, "'pagerank' can only be set when doc_engine is elasticsearch"

        if req["pagerank"] > 0:
            from rag.nlp import search
            settings.docStoreConn.update({"kb_id": kb.id}, {PAGERANK_FLD: req["pagerank"]},
                                         search.index_name(kb.tenant_id), kb.id)
        else:
            # Elasticsearch requires PAGERANK_FLD be non-zero!
            from rag.nlp import search
            settings.docStoreConn.update({"exists": PAGERANK_FLD}, {"remove": PAGERANK_FLD},
                                         search.index_name(kb.tenant_id), kb.id)

    if not KnowledgebaseService.update_by_id(kb.id, req):
        return False, "Update dataset error.(Database error)"

    ok, k = KnowledgebaseService.get_by_id(kb.id)
    if not ok:
        return False, "Dataset updated failed"

    response_data = remap_dictionary_keys(k.to_dict())
    return True, response_data


def list_datasets(tenant_id: str, args: dict):
    """
    List datasets.

    :param tenant_id: tenant ID
    :param args: query arguments
    :return: (success, result) or (success, error_message)
    """
    kb_id = args.get("id")
    name = args.get("name")
    if kb_id:
        kbs = KnowledgebaseService.get_kb_by_id(kb_id, tenant_id)
        if not kbs:
            return False, f"User '{tenant_id}' lacks permission for dataset '{kb_id}'"
    if name:
        kbs = KnowledgebaseService.get_kb_by_name(name, tenant_id)
        if not kbs:
            return False, f"User '{tenant_id}' lacks permission for dataset '{name}'"

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
    return True, {"data": response_data_list, "total": total}


async def get_knowledge_graph(dataset_id: str, tenant_id: str):
    """
    Get knowledge graph for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."
    _, kb = KnowledgebaseService.get_by_id(dataset_id)
    req = {
        "kb_id": [dataset_id],
        "knowledge_graph_kwd": ["graph"]
    }

    obj = {"graph": {}, "mind_map": {}}
    from rag.nlp import search
    if not settings.docStoreConn.index_exist(search.index_name(kb.tenant_id), dataset_id):
        return True, obj
    sres = await settings.retriever.search(req, search.index_name(kb.tenant_id), [dataset_id])
    if not len(sres.ids):
        return True, obj

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
    return True, obj


def delete_knowledge_graph(dataset_id: str, tenant_id: str):
    """
    Delete knowledge graph for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."
    _, kb = KnowledgebaseService.get_by_id(dataset_id)
    from rag.nlp import search
    settings.docStoreConn.delete({"knowledge_graph_kwd": ["graph", "subgraph", "entity", "relation"]},
                                 search.index_name(kb.tenant_id), dataset_id)

    return True, True


def run_graphrag(dataset_id: str, tenant_id: str):
    """
    Run GraphRAG for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    if not dataset_id:
        return False, 'Lack of "Dataset ID"'
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    task_id = kb.graphrag_task_id
    if task_id:
        ok, task = TaskService.get_by_id(task_id)
        if not ok:
            logging.warning(f"A valid GraphRAG task id is expected for Dataset {dataset_id}")

        if task and task.progress not in [-1, 1]:
            return False, f"Task {task_id} in progress with status {task.progress}. A Graph Task is already running."

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
        return False, f"No documents in Dataset {dataset_id}"

    sample_document = documents[0]
    document_ids = [document["id"] for document in documents]

    task_id = queue_raptor_o_graphrag_tasks(sample_doc_id=sample_document, ty="graphrag", priority=0, fake_doc_id=GRAPH_RAPTOR_FAKE_DOC_ID, doc_ids=list(document_ids))

    if not KnowledgebaseService.update_by_id(kb.id, {"graphrag_task_id": task_id}):
        logging.warning(f"Cannot save graphrag_task_id for Dataset {dataset_id}")

    return True, {"graphrag_task_id": task_id}


def trace_graphrag(dataset_id: str, tenant_id: str):
    """
    Trace GraphRAG task for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    if not dataset_id:
        return False, 'Lack of "Dataset ID"'
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    task_id = kb.graphrag_task_id
    if not task_id:
        return True, {}

    ok, task = TaskService.get_by_id(task_id)
    if not ok:
        return True, {}

    return True, task.to_dict()


def run_raptor(dataset_id: str, tenant_id: str):
    """
    Run RAPTOR for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    if not dataset_id:
        return False, 'Lack of "Dataset ID"'
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    task_id = kb.raptor_task_id
    if task_id:
        ok, task = TaskService.get_by_id(task_id)
        if not ok:
            logging.warning(f"A valid RAPTOR task id is expected for Dataset {dataset_id}")

        if task and task.progress not in [-1, 1]:
            return False, f"Task {task_id} in progress with status {task.progress}. A RAPTOR Task is already running."

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
        return False, f"No documents in Dataset {dataset_id}"

    sample_document = documents[0]
    document_ids = [document["id"] for document in documents]

    task_id = queue_raptor_o_graphrag_tasks(sample_doc_id=sample_document, ty="raptor", priority=0, fake_doc_id=GRAPH_RAPTOR_FAKE_DOC_ID, doc_ids=list(document_ids))

    if not KnowledgebaseService.update_by_id(kb.id, {"raptor_task_id": task_id}):
        logging.warning(f"Cannot save raptor_task_id for Dataset {dataset_id}")

    return True, {"raptor_task_id": task_id}


async def trace_raptor(dataset_id: str, tenant_id: str):
    """
    Trace RAPTOR task for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    if not dataset_id:
        return False, 'Lack of "Dataset ID"'

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    task_id = kb.raptor_task_id
    if not task_id:
        return True, {}

    ok, task = TaskService.get_by_id(task_id)
    if not ok:
        return False, "RAPTOR Task Not Found or Error Occurred"

    return True, task.to_dict()
