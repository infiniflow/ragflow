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
import re

from api.db.joint_services.tenant_model_service import get_model_config_from_provider_instance
from common.constants import PAGERANK_FLD
from common import settings
from api.db.db_models import File
from api.db.services.document_service import DocumentService, queue_raptor_o_graphrag_tasks
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.connector_service import Connector2KbService
from api.db.services.task_service import GRAPH_RAPTOR_FAKE_DOC_ID, TaskService
from api.db.services.user_service import TenantService, UserService, UserTenantService
from common.constants import FileSource, StatusEnum
from api.utils.api_utils import deep_merge, get_parser_config, remap_dictionary_keys, verify_embedding_availability
from common.misc_utils import thread_pool_exec

_VALID_INDEX_TYPES = {"graph", "raptor", "mindmap", "artifact", "skill"}

_INDEX_TYPE_TO_TASK_TYPE = {
    "graph": "graphrag",
    "raptor": "raptor",
    "mindmap": "mindmap",
    "artifact": "artifact",
    "skill": "skill",
}

_INDEX_TYPE_TO_TASK_ID_FIELD = {
    "graph": "graphrag_task_id",
    "raptor": "raptor_task_id",
    "mindmap": "mindmap_task_id",
    "artifact": "artifact_task_id",
    "skill": "skill_task_id",
}

_INDEX_TYPE_TO_DISPLAY_NAME = {
    "graph": "Graph",
    "raptor": "RAPTOR",
    "mindmap": "Mindmap",
    "artifact": "Artifact",
    "skill": "Skill",
}


async def create_dataset(tenant_id: str, req: dict):
    """
    Create a new dataset.

    :param tenant_id: tenant ID
    :param req: dataset creation request
    :return: (success, result) or (success, error_message)
    """
    # Extract ext field for additional parameters
    ext_fields = req.pop("ext", {})

    # Map auto_metadata_config (if provided) into parser_config structure
    auto_meta = req.pop("auto_metadata_config", {})
    if auto_meta:
        parser_cfg = req.get("parser_config") or {}
        fields = []
        for f in auto_meta.get("fields", []):
            fields.append(
                {
                    "name": f.get("name", ""),
                    "type": f.get("type", ""),
                    "description": f.get("description"),
                    "examples": f.get("examples"),
                    "restrict_values": f.get("restrict_values", False),
                }
            )
        parser_cfg["metadata"] = fields
        parser_cfg["enable_metadata"] = auto_meta.get("enabled", True)
        req["parser_config"] = parser_cfg
    req.update(ext_fields)

    e, create_dict = KnowledgebaseService.create_with_name(name=req.pop("name", None), tenant_id=tenant_id, parser_id=req.pop("parser_id", None), **req)

    if not e:
        return False, create_dict

    # Insert embedding model(embd id)
    ok, t = TenantService.get_by_id(tenant_id)
    if not ok:
        return False, "Tenant not found"
    if not create_dict.get("embd_id"):
        create_dict["embd_id"] = t.embd_id
    else:
        ok, err = verify_embedding_availability(create_dict["embd_id"], tenant_id)
        if not ok:
            return False, err

    if not KnowledgebaseService.save(**create_dict):
        return False, "Failed to save dataset"
    ok, k = KnowledgebaseService.get_by_id(create_dict["id"])
    if not ok:
        return False, "Dataset created failed"
    response_data = remap_dictionary_keys(k.to_dict())
    return True, response_data


async def delete_datasets(tenant_id: str, ids: list = None, delete_all: bool = False):
    """
    Delete datasets.

    :param tenant_id: tenant ID
    :param ids: list of dataset IDs
    :param delete_all: whether to delete all datasets of the tenant (if ids is not provided)
    :return: (success, result) or (success, error_message)
    """
    kb_id_instance_pairs = []
    if not ids:
        if not delete_all:
            return True, {"success_count": 0}
        else:
            ids = [kb.id for kb in KnowledgebaseService.query(tenant_id=tenant_id)]

    error_kb_ids = []
    for kb_id in ids:
        kb = KnowledgebaseService.get_or_none(id=kb_id, tenant_id=tenant_id)
        if kb is None:
            error_kb_ids.append(kb_id)
            continue
        kb_id_instance_pairs.append((kb_id, kb))
    if len(error_kb_ids) > 0:
        return False, f"""User '{tenant_id}' lacks permission for datasets: '{", ".join(error_kb_ids)}'"""

    errors = []
    success_count = 0
    for kb_id, kb in kb_id_instance_pairs:
        for doc in DocumentService.query(kb_id=kb_id):
            if not DocumentService.remove_document(doc, tenant_id):
                errors.append(f"Remove document '{doc.id}' error for dataset '{kb_id}'")
                continue
            f2d = File2DocumentService.get_by_document_id(doc.id)
            if f2d:
                FileService.filter_delete(
                    [
                        File.source_type == FileSource.KNOWLEDGEBASE,
                        File.id == f2d[0].file_id,
                    ]
                )
            else:
                # Normal uploads create a File2Document row via FileService.add_file_from_kb.
                # A missing row usually means stale/partial data (e.g. link removed earlier,
                # failed post-insert file linkage, or legacy rows). Deletion still proceeds.
                logging.warning(
                    "delete_datasets: document %s in dataset %s has no File2Document row; skipping linked file delete",
                    doc.id,
                    kb_id,
                )
            File2DocumentService.delete_by_document_id(doc.id)
        FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.type == "folder", File.name == kb.name])

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


def get_dataset(dataset_id: str, tenant_id: str):
    """
    Get a single dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    if not dataset_id:
        return False, 'Lack of "Dataset ID"'

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, f"User '{tenant_id}' lacks permission for dataset '{dataset_id}'"

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    response_data = remap_dictionary_keys(kb.to_dict())
    response_data["size"] = DocumentService.get_total_size_by_kb_id(dataset_id)
    response_data["connectors"] = list(Connector2KbService.list_connectors(dataset_id))
    return True, response_data


def get_ingestion_summary(dataset_id: str, tenant_id: str):
    """
    Get ingestion summary for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    if not dataset_id:
        return False, 'Lack of "Dataset ID"'

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, f"User '{tenant_id}' lacks permission for dataset '{dataset_id}'"

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    status = DocumentService.get_parsing_status_by_kb_ids([dataset_id]).get(dataset_id, {})
    return True, {
        "doc_num": kb.doc_num,
        "chunk_num": kb.chunk_num,
        "token_num": kb.token_num,
        "status": status,
    }


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

    # Extract ext field for additional parameters
    ext_fields = req.pop("ext", {})

    # Map auto_metadata_config into parser_config if present
    auto_meta = req.pop("auto_metadata_config", {})
    if auto_meta:
        parser_cfg = req.get("parser_config") or {}
        fields = []
        for f in auto_meta.get("fields", []):
            fields.append(
                {
                    "name": f.get("name", ""),
                    "type": f.get("type", ""),
                    "description": f.get("description"),
                    "examples": f.get("examples"),
                    "restrict_values": f.get("restrict_values", False),
                }
            )
        parser_cfg["metadata"] = fields
        parser_cfg["enable_metadata"] = auto_meta.get("enabled", True)
        req["parser_config"] = parser_cfg

    # Merge ext fields with req
    req.update(ext_fields)

    # Extract connectors from request
    connectors = []
    if "connectors" in req:
        connectors = req["connectors"]
        del req["connectors"]

    if req.get("parser_config"):
        # Flatten parent_child config into children_delimiter for the execution layer
        pc = req["parser_config"].get("parent_child", {})
        if pc.get("use_parent_child"):
            req["parser_config"]["children_delimiter"] = pc.get("children_delimiter", "\n")
            req["parser_config"]["enable_children"] = pc.get("use_parent_child", True)
        else:
            req["parser_config"]["children_delimiter"] = ""
            req["parser_config"]["enable_children"] = False
            req["parser_config"]["parent_child"] = {}

        parser_config = req["parser_config"]
        req_ext_fields = parser_config.pop("ext", {})
        parser_config.update(req_ext_fields)
        req["parser_config"] = deep_merge(kb.parser_config, parser_config)

    if (chunk_method := req.get("parser_id")) and chunk_method != kb.parser_id:
        if not req.get("parser_config"):
            req["parser_config"] = get_parser_config(chunk_method, None)
    elif "parser_config" in req and not req["parser_config"]:
        del req["parser_config"]

    if kb.pipeline_id and req.get("parser_id") and not req.get("pipeline_id"):
        # shift to use parser_id, delete old pipeline_id
        req["pipeline_id"] = ""

    if "name" in req and req["name"].lower() != kb.name.lower():
        exists = KnowledgebaseService.get_or_none(name=req["name"], tenant_id=tenant_id, status=StatusEnum.VALID.value)
        if exists:
            return False, f"Dataset name '{req['name']}' already exists"

    if "embd_id" in req:
        if not req["embd_id"]:
            req["embd_id"] = kb.embd_id
        ok, err = verify_embedding_availability(req["embd_id"], tenant_id)
        if not ok:
            return False, err

    if "pagerank" in req and req["pagerank"] != kb.pagerank:
        if os.environ.get("DOC_ENGINE", "elasticsearch") == "infinity":
            return False, "'pagerank' can only be set when doc_engine is elasticsearch"

        if req["pagerank"] > 0:
            from rag.nlp import search

            settings.docStoreConn.update({"kb_id": kb.id}, {PAGERANK_FLD: req["pagerank"]}, search.index_name(kb.tenant_id), kb.id)
        else:
            # Elasticsearch requires PAGERANK_FLD be non-zero!
            from rag.nlp import search

            settings.docStoreConn.update({"exists": PAGERANK_FLD}, {"remove": PAGERANK_FLD}, search.index_name(kb.tenant_id), kb.id)
    if "parse_type" in req:
        del req["parse_type"]

    if not KnowledgebaseService.update_by_id(kb.id, req):
        return False, "Update dataset error.(Database error)"

    ok, k = KnowledgebaseService.get_by_id(kb.id)
    if not ok:
        return False, "Dataset updated failed"

    # Link connectors to the dataset
    errors = Connector2KbService.link_connectors(kb.id, [conn for conn in connectors], tenant_id)
    if errors:
        logging.error("Link KB errors: %s", errors)

    response_data = remap_dictionary_keys(k.to_dict())
    response_data["connectors"] = connectors
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
    page = int(args.get("page", 1))
    page_size = int(args.get("page_size", 30))
    ext_fields = args.get("ext", {})
    parser_id = ext_fields.get("parser_id")
    keywords = ext_fields.get("keywords", "")
    orderby = args.get("orderby", "create_time")
    desc_arg = args.get("desc", "true")
    if isinstance(desc_arg, str):
        desc = desc_arg.lower() != "false"
    elif isinstance(desc_arg, bool):
        desc = desc_arg
    else:
        # unknown type, default to True
        desc = True

    if kb_id:
        kbs = KnowledgebaseService.get_kb_by_id(kb_id, tenant_id)
        if not kbs:
            return False, f"User '{tenant_id}' lacks permission for dataset '{kb_id}'"
    if name:
        kbs = KnowledgebaseService.get_kb_by_name(name, tenant_id)
        if not kbs:
            return False, f"User '{tenant_id}' lacks permission for dataset '{name}'"
    if ext_fields.get("owner_ids", []):
        tenant_ids = ext_fields["owner_ids"]
    else:
        tenants = TenantService.get_joined_tenants_by_user_id(tenant_id)
        tenant_ids = [m["tenant_id"] for m in tenants]
    kbs, total = KnowledgebaseService.get_list(tenant_ids, tenant_id, page, page_size, orderby, desc, kb_id, name, keywords, parser_id)
    users = UserService.get_by_ids([m["tenant_id"] for m in kbs])
    user_map = {m.id: m.to_dict() for m in users}
    response_data_list = []
    for kb in kbs:
        user_dict = user_map.get(kb["tenant_id"], {})
        kb.update({"nickname": user_dict.get("nickname", ""), "tenant_avatar": user_dict.get("avatar", "")})
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

    req = {"kb_id": [dataset_id], "knowledge_graph_kwd": ["graph"]}

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
            filtered_edges = [o for o in obj["graph"]["edges"] if o["source"] != o["target"] and o["source"] in node_id_set and o["target"] in node_id_set]
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
    from rag.graphrag.phase_markers import clear_phase_markers

    settings.docStoreConn.delete({"knowledge_graph_kwd": ["graph", "subgraph", "entity", "relation", "community_report"]}, search.index_name(kb.tenant_id), dataset_id)
    # Wiping the graph invalidates any phase-completion markers used to
    # short-circuit resolution / community detection on resume.
    clear_phase_markers(dataset_id)
    KnowledgebaseService.update_by_id(
        kb.id,
        {"graphrag_task_id": "", "graphrag_task_finish_at": None},
    )

    return True, True


def run_index(dataset_id: str, tenant_id: str, index_type: str):
    """
    Run an indexing task (graph/raptor/mindmap) for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :param index_type: one of "graph", "raptor", "mindmap"
    :return: (success, result) or (success, error_message)
    """
    if index_type not in _VALID_INDEX_TYPES:
        return False, f"Invalid index type '{index_type}'. Must be one of {sorted(_VALID_INDEX_TYPES)}"

    if not dataset_id:
        return False, 'Lack of "Dataset ID"'
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    task_type = _INDEX_TYPE_TO_TASK_TYPE[index_type]
    task_id_field = _INDEX_TYPE_TO_TASK_ID_FIELD[index_type]
    display_name = _INDEX_TYPE_TO_DISPLAY_NAME[index_type]

    existing_task_id = getattr(kb, task_id_field, None)
    if existing_task_id:
        ok, task = TaskService.get_by_id(existing_task_id)
        if not ok:
            logging.warning(f"A valid {display_name} task id is expected for Dataset {dataset_id}")

        if task and task.progress not in [-1, 1]:
            return False, f"Task {existing_task_id} in progress with status {task.progress}. A {display_name} Task is already running."

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

    task_id = queue_raptor_o_graphrag_tasks(sample_doc=sample_document, ty=task_type, priority=0, fake_doc_id=GRAPH_RAPTOR_FAKE_DOC_ID, doc_ids=list(document_ids))

    if not KnowledgebaseService.update_by_id(kb.id, {task_id_field: task_id}):
        logging.warning(f"Cannot save {task_id_field} for Dataset {dataset_id}")

    return True, {"task_id": task_id}


def trace_index(dataset_id: str, tenant_id: str, index_type: str):
    """
    Trace an indexing task (graph/raptor/mindmap) for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :param index_type: one of "graph", "raptor", "mindmap"
    :return: (success, result) or (success, error_message)
    """
    if index_type not in _VALID_INDEX_TYPES:
        return False, f"Invalid index type '{index_type}'. Must be one of {sorted(_VALID_INDEX_TYPES)}"

    if not dataset_id:
        return False, 'Lack of "Dataset ID"'

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    task_id_field = _INDEX_TYPE_TO_TASK_ID_FIELD[index_type]
    task_id = getattr(kb, task_id_field, None)
    if not task_id:
        return True, {}

    ok, task = TaskService.get_by_id(task_id)
    if not ok:
        return True, {}

    return True, task.to_dict()


def list_tags(dataset_id: str, tenant_id: str):
    """
    List tags for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    if not dataset_id:
        return False, 'Lack of "Dataset ID"'

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    tenants = UserTenantService.get_tenants_by_user_id(tenant_id)
    tags = []
    for tenant in tenants:
        tags += settings.retriever.all_tags(tenant["tenant_id"], [dataset_id])
    return True, tags


def aggregate_tags(dataset_ids: list[str], tenant_id: str):
    """
    Aggregate tags across multiple datasets.

    :param dataset_ids: list of dataset IDs
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    if not dataset_ids:
        return False, 'Lack of "dataset_ids"'

    for dataset_id in dataset_ids:
        if not KnowledgebaseService.accessible(dataset_id, tenant_id):
            return False, f"No authorization for dataset '{dataset_id}'"

    dataset_ids_by_tenant = {}
    for dataset_id in dataset_ids:
        ok, kb = KnowledgebaseService.get_by_id(dataset_id)
        if not ok:
            return False, f"Invalid Dataset ID '{dataset_id}'"
        dataset_ids_by_tenant.setdefault(kb.tenant_id, []).append(dataset_id)

    merged = {}
    for kb_tenant_id, kb_ids in dataset_ids_by_tenant.items():
        for tag, count in settings.retriever.all_tags(kb_tenant_id, kb_ids):
            merged[tag] = merged.get(tag, 0) + count

    return True, [{"value": tag, "count": count} for tag, count in merged.items()]


def get_flattened_metadata(dataset_ids: list[str], tenant_id: str):
    """
    Get flattened metadata for datasets.

    :param dataset_ids: list of dataset IDs
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    if not dataset_ids:
        return False, 'Lack of "dataset_ids"'

    for dataset_id in dataset_ids:
        if not KnowledgebaseService.accessible(dataset_id, tenant_id):
            return False, f"No authorization for dataset '{dataset_id}'"

    from api.db.services.doc_metadata_service import DocMetadataService

    return True, DocMetadataService.get_flatted_meta_by_kbs(dataset_ids)


def get_auto_metadata(dataset_id: str, tenant_id: str):
    """
    Get auto-metadata configuration for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    kb = KnowledgebaseService.get_or_none(id=dataset_id, tenant_id=tenant_id)
    if kb is None:
        return False, f"User '{tenant_id}' lacks permission for dataset '{dataset_id}'"
    parser_cfg = kb.parser_config or {}
    return True, {"metadata": parser_cfg.get("metadata") or [], "built_in_metadata": parser_cfg.get("built_in_metadata") or []}


async def update_auto_metadata(dataset_id: str, tenant_id: str, cfg: dict):
    """
    Update auto-metadata configuration for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :param cfg: auto-metadata configuration
    :return: (success, result) or (success, error_message)
    """
    kb = KnowledgebaseService.get_or_none(id=dataset_id, tenant_id=tenant_id)
    if kb is None:
        return False, f"User '{tenant_id}' lacks permission for dataset '{dataset_id}'"

    parser_cfg = kb.parser_config or {}
    parser_cfg["metadata"] = cfg.get("metadata")
    parser_cfg["built_in_metadata"] = cfg.get("built_in_metadata")

    if not KnowledgebaseService.update_by_id(kb.id, {"parser_config": parser_cfg}):
        return False, "Update auto-metadata error.(Database error)"

    return True, cfg


def delete_tags(dataset_id: str, tenant_id: str, tags: list[str]):
    """
    Delete tags from a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :param tags: list of tags to delete
    :return: (success, result) or (success, error_message)
    """
    if not dataset_id:
        return False, 'Lack of "Dataset ID"'

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    from rag.nlp import search

    for t in tags:
        settings.docStoreConn.update({"tag_kwd": t, "kb_id": [dataset_id]}, {"remove": {"tag_kwd": t}}, search.index_name(kb.tenant_id), dataset_id)

    return True, {}


def list_ingestion_logs(
    dataset_id: str,
    tenant_id: str,
    page: int,
    page_size: int,
    orderby: str,
    desc: bool,
    operation_status: list = None,
    create_date_from: str = None,
    create_date_to: str = None,
    log_type: str = "dataset",
    keywords: str = None,
):
    """
    List ingestion logs for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :param page: page number
    :param page_size: items per page
    :param orderby: order by field
    :param desc: descending order
    :param operation_status: filter by operation status
    :param create_date_from: filter start date
    :param create_date_to: filter end date
    :param log_type: "dataset" or "file"
    :param keywords: search keywords for file logs
    :return: (success, result) or (success, error_message)
    """
    if not dataset_id:
        return False, 'Lack of "Dataset ID"'

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    from api.db.services.pipeline_operation_log_service import PipelineOperationLogService

    allowed_log_types = {"dataset", "file"}
    if log_type not in allowed_log_types:
        logging.warning(
            "list_ingestion_logs invalid log_type: dataset_id=%s tenant_id=%s log_type=%s",
            dataset_id,
            tenant_id,
            log_type,
        )
        return False, 'Invalid "log_type", expected "dataset" or "file"'

    logging.info(
        "list_ingestion_logs: dataset_id=%s tenant_id=%s log_type=%s page=%s page_size=%s",
        dataset_id,
        tenant_id,
        log_type,
        page,
        page_size,
    )

    if log_type == "file":
        logs, total = PipelineOperationLogService.get_file_logs_by_kb_id(dataset_id, page, page_size, orderby, desc, keywords, operation_status or [], None, None, create_date_from, create_date_to)
    else:
        logs, total = PipelineOperationLogService.get_dataset_logs_by_kb_id(dataset_id, page, page_size, orderby, desc, operation_status or [], create_date_from, create_date_to, keywords)
    return True, {"total": total, "logs": logs}


def get_ingestion_log(dataset_id: str, tenant_id: str, log_id: str):
    """
    Get a single ingestion log.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :param log_id: log ID
    :return: (success, result) or (success, error_message)
    """
    if not dataset_id:
        return False, 'Lack of "Dataset ID"'

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    from api.db.services.pipeline_operation_log_service import PipelineOperationLogService

    # Return the full record (including `dsl`) so the front-end dataflow-result
    # page can render the pipeline timeline and chunks. The file-level field set
    # is a superset of the dataset-level fields, so it is valid for both
    # dataset-level (graph/raptor/mindmap) and per-file logs.
    fields = PipelineOperationLogService.get_file_logs_fields()
    log = PipelineOperationLogService.model.select(*fields).where((PipelineOperationLogService.model.id == log_id) & (PipelineOperationLogService.model.kb_id == dataset_id)).first()
    if not log:
        return False, "Log not found"

    result = log.to_dict()
    # Be explicit here: the dataflow-result page needs the full DSL payload to
    # rebuild the timeline and right-side parser view. Some serialization paths
    # can omit JSON fields from Peewee model dicts, so keep it attached here.
    result["dsl"] = log.dsl or {}
    return True, result


def delete_index(dataset_id: str, tenant_id: str, index_type: str, wipe: bool = True):
    """
    Delete an indexing task (graph/raptor/mindmap) for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :param index_type: one of "graph", "raptor", "mindmap"
    :param wipe: when True (default) the persisted artefacts (graph rows,
        raptor summaries) are removed from the doc store and any GraphRAG
        phase-completion markers are cleared.  Pass False to cancel the
        running task while keeping prior progress so it can be resumed.
    :return: (success, result) or (success, error_message)
    """
    if index_type not in _VALID_INDEX_TYPES:
        return False, f"Invalid index type '{index_type}'. Must be one of {sorted(_VALID_INDEX_TYPES)}"

    if not dataset_id:
        return False, 'Lack of "Dataset ID"'

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    task_id_field = _INDEX_TYPE_TO_TASK_ID_FIELD[index_type]
    task_finish_at_field = f"{task_id_field.replace('_task_id', '_task_finish_at')}"
    task_id = getattr(kb, task_id_field, None)

    logging.info("delete_index: dataset=%s index_type=%s wipe=%s", dataset_id, index_type, wipe)

    if task_id:
        from rag.utils.redis_conn import REDIS_CONN

        try:
            REDIS_CONN.set(f"{task_id}-cancel", "x")
        except Exception as e:
            logging.exception(e)
        TaskService.delete_by_id(task_id)

    if wipe and index_type == "graph":
        from rag.nlp import search
        from rag.graphrag.phase_markers import clear_phase_markers

        settings.docStoreConn.delete({"knowledge_graph_kwd": ["graph", "subgraph", "entity", "relation", "community_report"]}, search.index_name(kb.tenant_id), dataset_id)
        # Wiping the graph invalidates any phase-completion markers used to
        # short-circuit resolution / community detection on resume.
        clear_phase_markers(dataset_id)
        logging.info("delete_index: cleared GraphRAG artefacts and phase markers for dataset=%s", dataset_id)
    elif wipe and index_type == "raptor":
        from rag.nlp import search

        settings.docStoreConn.delete({"raptor_kwd": ["raptor"]}, search.index_name(kb.tenant_id), dataset_id)
    elif wipe and index_type == "skill":
        from rag.nlp import search

        settings.docStoreConn.delete({"compile_kwd": ["skill", "skill_all"]}, search.index_name(kb.tenant_id), dataset_id)

    KnowledgebaseService.update_by_id(kb.id, {task_id_field: "", task_finish_at_field: None})
    return True, {}


def run_embedding(dataset_id: str, tenant_id: str):
    """
    Run embedding for all documents in a dataset.

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

    kb_table_num_map = {}
    for doc in documents:
        doc["tenant_id"] = tenant_id
        DocumentService.run(tenant_id, doc, kb_table_num_map)

    return True, {"scheduled_count": len(documents)}


def rename_tag(dataset_id: str, tenant_id: str, from_tag: str, to_tag: str):
    """
    Rename a tag in a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :param from_tag: original tag name
    :param to_tag: new tag name
    :return: (success, result) or (success, error_message)
    """
    if not dataset_id:
        return False, 'Lack of "Dataset ID"'

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    from rag.nlp import search

    settings.docStoreConn.update({"tag_kwd": from_tag, "kb_id": [dataset_id]}, {"remove": {"tag_kwd": from_tag.strip()}, "add": {"tag_kwd": to_tag}}, search.index_name(kb.tenant_id), dataset_id)

    return True, {"from": from_tag, "to": to_tag}


async def search(dataset_id: str, tenant_id: str, req: dict):
    """
    Search (retrieval test) within a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :param req: search request
    :return: (success, result) or (success, error_message)
    """
    from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type
    from api.db.services.doc_metadata_service import DocMetadataService
    from api.db.services.llm_service import LLMBundle
    from api.db.services.search_service import SearchService
    from api.db.services.user_service import UserTenantService
    from common.constants import LLMType
    from common.metadata_utils import apply_meta_data_filter
    from rag.app.tag import label_question
    from rag.prompts.generator import cross_languages, keyword_extraction

    logging.debug(
        "search(dataset=%s, tenant=%s, question_len=%s)",
        dataset_id,
        tenant_id,
        len(req.get("question", "")),
    )

    page = int(req.get("page", 1))
    size = int(req.get("size", 30))
    question = req.get("question", "")
    doc_ids = req.get("doc_ids", [])
    use_kg = req.get("use_kg", False)
    similarity_threshold = float(req.get("similarity_threshold", 0.0))
    vector_similarity_weight = float(req.get("vector_similarity_weight", 0.3))
    top = max(1, min(int(req.get("top_k", 1024)), 2048))
    langs = req.get("cross_languages", [])

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        logging.warning("search access denied: dataset=%s tenant=%s", dataset_id, tenant_id)
        return False, "Only owner of dataset authorized for this operation."

    e, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not e:
        logging.warning("search dataset not found: dataset=%s", dataset_id)
        return False, "Dataset not found!"

    if doc_ids is not None and not isinstance(doc_ids, list):
        return False, "`doc_ids` should be a list"
    local_doc_ids = list(doc_ids) if doc_ids else []

    meta_data_filter = {}
    search_id = req.get("search_id", "")
    search_config = {}
    chat_mdl = None
    if search_id:
        search_detail = SearchService.get_detail(search_id)
        if not search_detail:
            logging.warning("search config not found: search_id=%s", search_id)
            return False, "Invalid search_id"
        search_config = search_detail.get("search_config", {})
        meta_data_filter = search_config.get("meta_data_filter", {})
        similarity_threshold = float(search_config.get("similarity_threshold", similarity_threshold))
        vector_similarity_weight = float(search_config.get("vector_similarity_weight", vector_similarity_weight))
        top = max(1, min(int(search_config.get("top_k", top)), 2048))
        use_kg = search_config.get("use_kg", use_kg)
        langs = search_config.get("cross_languages", langs)
        logging.debug(
            "Dataset search loaded Search config: search_id=%s dataset_id=%s vector_similarity_weight=%s full_text_weight=%s similarity_threshold=%s top_k=%s",
            search_id,
            dataset_id,
            vector_similarity_weight,
            1 - vector_similarity_weight,
            similarity_threshold,
            top,
        )
        if meta_data_filter.get("method") in ["auto", "semi_auto"]:
            chat_id = search_config.get("chat_id", "")
            if chat_id:
                chat_model_config = get_model_config_from_provider_instance(tenant_id, LLMType.CHAT, search_config["chat_id"])
            else:
                chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
            chat_mdl = LLMBundle(tenant_id, chat_model_config)
    else:
        meta_data_filter = req.get("meta_data_filter") or {}
        if meta_data_filter.get("method") in ["auto", "semi_auto"]:
            chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
            chat_mdl = LLMBundle(tenant_id, chat_model_config)

    if meta_data_filter:
        local_doc_ids = await apply_meta_data_filter(
            meta_data_filter,
            None,
            question,
            chat_mdl,
            local_doc_ids,
            kb_ids=[dataset_id],
            metas_loader=lambda: DocMetadataService.get_flatted_meta_by_kbs([dataset_id]),
        )

    tenant_ids = []
    tenants = UserTenantService.query(user_id=tenant_id)
    for tenant in tenants:
        if KnowledgebaseService.query(tenant_id=tenant.tenant_id, id=dataset_id):
            tenant_ids.append(tenant.tenant_id)
            break
    else:
        return False, "Only owner of dataset authorized for this operation."

    _question = question
    if langs:
        _question = await cross_languages(kb.tenant_id, None, _question, langs)
    if kb.embd_id:
        embd_model_config = get_model_config_from_provider_instance(kb.tenant_id, LLMType.EMBEDDING, kb.embd_id)
    else:
        embd_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.EMBEDDING)
    embd_mdl = LLMBundle(kb.tenant_id, embd_model_config)

    rerank_mdl = None
    rerank_id = search_config.get("rerank_id") or req.get("rerank_id")
    if rerank_id:
        rerank_model_config = get_model_config_from_provider_instance(kb.tenant_id, LLMType.RERANK.value, rerank_id)
        rerank_mdl = LLMBundle(kb.tenant_id, rerank_model_config)

    if search_config.get("keyword", req.get("keyword", False)):
        default_chat_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.CHAT)
        chat_mdl = LLMBundle(kb.tenant_id, default_chat_model_config)
        _question += await keyword_extraction(chat_mdl, _question)

    labels = label_question(_question, [kb])
    ranks = await settings.retriever.retrieval(
        _question,
        embd_mdl,
        tenant_ids,
        [dataset_id],
        page,
        size,
        similarity_threshold,
        vector_similarity_weight,
        doc_ids=local_doc_ids,
        top=top,
        rerank_mdl=rerank_mdl,
        rank_feature=labels,
        trace_id=search_id,
    )

    if use_kg:
        try:
            default_chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
            ck = await settings.kg_retriever.retrieval(_question, tenant_ids, [dataset_id], embd_mdl, LLMBundle(kb.tenant_id, default_chat_model_config))
            if ck["content_with_weight"]:
                ranks["chunks"].insert(0, ck)
        except Exception:
            logging.warning("search KG retrieval failed: dataset=%s tenant=%s", dataset_id, tenant_id, exc_info=True)
    ranks["chunks"] = settings.retriever.retrieval_by_children(ranks["chunks"], tenant_ids)
    ranks["total"] = len(ranks["chunks"])

    for c in ranks["chunks"]:
        c.pop("vector", None)
    ranks["labels"] = labels

    return True, ranks


def check_embedding(dataset_id: str, tenant_id: str, req: dict):
    """
    Check embedding model compatibility by sampling random chunks,
    re-embedding them with the new model, and computing cosine similarity.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :param req: request body with embd_id
    :return: (success, result) or (success, error_message)
    """
    import random

    import numpy as np
    from common.constants import RetCode
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search

    from api.db.services.llm_service import LLMBundle
    from common.constants import LLMType

    def _guess_vec_field(src: dict):
        for k in src or {}:
            if k.endswith("_vec"):
                return k
        return None

    def _as_float_vec(v):
        if v is None:
            return []
        if isinstance(v, str):
            return [float(x) for x in v.split("\t") if x != ""]
        if isinstance(v, (list, tuple, np.ndarray)):
            return [float(x) for x in v]
        return []

    def _to_1d(x):
        a = np.asarray(x, dtype=np.float32)
        return a.reshape(-1)

    def _cos_sim(a, b, eps=1e-12):
        a = _to_1d(a)
        b = _to_1d(b)
        na = np.linalg.norm(a)
        nb = np.linalg.norm(b)
        if na < eps or nb < eps:
            return 0.0
        return float(np.dot(a, b) / (na * nb))

    def sample_random_chunks_with_vectors(
        docStoreConn,
        tenant_id: str,
        kb_id: str,
        n: int = 5,
        base_fields=("docnm_kwd", "doc_id", "content_with_weight", "page_num_int", "position_int", "top_int"),
    ):
        index_nm = search.index_name(tenant_id)
        try:
            res0 = docStoreConn.search(
                select_fields=[],
                highlight_fields=[],
                condition={"kb_id": kb_id, "available_int": 1},
                match_expressions=[],
                order_by=OrderByExpr(),
                offset=0,
                limit=1,
                index_names=index_nm,
                knowledgebase_ids=[kb_id],
            )
        except Exception as e:
            if "not_found_exception" in repr(e) or "index_not_found_exception" in repr(e):
                logging.info(
                    "sample_random_chunks_with_vectors: index %s not yet created for tenant %s; "
                    "returning empty sample set",
                    index_nm,
                    tenant_id,
                )
                return []
            raise
        total = docStoreConn.get_total(res0)
        if total <= 0:
            return []

        n = min(n, total)
        offsets = sorted(random.sample(range(min(total, 1000)), n))
        out = []

        for off in offsets:
            res1 = docStoreConn.search(
                select_fields=list(base_fields),
                highlight_fields=[],
                condition={"kb_id": kb_id, "available_int": 1},
                match_expressions=[],
                order_by=OrderByExpr(),
                offset=off,
                limit=1,
                index_names=index_nm,
                knowledgebase_ids=[kb_id],
            )
            ids = docStoreConn.get_doc_ids(res1)
            if not ids:
                continue

            cid = ids[0]
            full_doc = docStoreConn.get(cid, index_nm, [kb_id]) or {}
            vec_field = _guess_vec_field(full_doc)
            vec = _as_float_vec(full_doc.get(vec_field))

            out.append(
                {
                    "chunk_id": cid,
                    "kb_id": kb_id,
                    "doc_id": full_doc.get("doc_id"),
                    "doc_name": full_doc.get("docnm_kwd"),
                    "vector_field": vec_field,
                    "vector_dim": len(vec),
                    "vector": vec,
                    "page_num_int": full_doc.get("page_num_int"),
                    "position_int": full_doc.get("position_int"),
                    "top_int": full_doc.get("top_int"),
                    "content_with_weight": full_doc.get("content_with_weight") or "",
                    "question_kwd": full_doc.get("question_kwd") or [],
                }
            )
        return out

    def _clean(s: str):
        return re.sub(r"</?(table|td|caption|tr|th)( [^<>]{0,12})?>", " ", s or "").strip()

    if not dataset_id:
        return False, 'Lack of "Dataset ID"'

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    embd_id = req.get("embd_id", "")
    if not embd_id:
        return False, "`embd_id` is required."

    logging.info("check_embedding: dataset=%s tenant=%s embd_id=%s", dataset_id, tenant_id, embd_id)

    ok, err = verify_embedding_availability(embd_id, tenant_id)
    if not ok:
        return False, err

    embd_model_config = get_model_config_from_provider_instance(kb.tenant_id, LLMType.EMBEDDING, embd_id)
    emb_mdl = LLMBundle(kb.tenant_id, embd_model_config)

    n = int(req.get("check_num", 5))
    samples = sample_random_chunks_with_vectors(settings.docStoreConn, tenant_id=kb.tenant_id, kb_id=dataset_id, n=n)
    logging.info("check_embedding: dataset=%s sampled=%d chunks", dataset_id, len(samples))

    results, eff_sims = [], []
    mode = "content_only"
    for ck in samples:
        title = ck.get("doc_name") or "Title"

        txt_in = "\n".join(ck.get("question_kwd") or []) or ck.get("content_with_weight") or ""
        txt_in = _clean(txt_in)
        if not txt_in:
            results.append({"chunk_id": ck["chunk_id"], "reason": "no_text"})
            continue

        if not ck.get("vector"):
            results.append({"chunk_id": ck["chunk_id"], "reason": "no_stored_vector"})
            continue

        try:
            v, _ = emb_mdl.encode([title, txt_in])
            assert len(v[1]) == len(ck["vector"]), f"The dimension ({len(v[1])}) of given embedding model is different from the original ({len(ck['vector'])})"
            sim_content = _cos_sim(v[1], ck["vector"])
            title_w = 0.1
            qv_mix = title_w * v[0] + (1 - title_w) * v[1]
            sim_mix = _cos_sim(qv_mix, ck["vector"])
            sim = sim_content
            mode = "content_only"
            if sim_mix > sim:
                sim = sim_mix
                mode = "title+content"
        except Exception as e:
            return False, f"Embedding failure. {e}"

        eff_sims.append(sim)
        results.append(
            {
                "chunk_id": ck["chunk_id"],
                "doc_id": ck["doc_id"],
                "doc_name": ck["doc_name"],
                "vector_field": ck["vector_field"],
                "vector_dim": ck["vector_dim"],
                "cos_sim": round(sim, 6),
            }
        )

    summary = {
        "kb_id": dataset_id,
        "model": embd_id,
        "sampled": len(samples),
        "valid": len(eff_sims),
        "avg_cos_sim": round(float(np.mean(eff_sims)) if eff_sims else 0.0, 6),
        "min_cos_sim": round(float(np.min(eff_sims)) if eff_sims else 0.0, 6),
        "max_cos_sim": round(float(np.max(eff_sims)) if eff_sims else 0.0, 6),
        "match_mode": mode,
    }

    data = {"summary": summary, "results": results}
    if not eff_sims:
        logging.warning("check_embedding: dataset=%s no comparable chunks", dataset_id)
        return False, "No embedded chunks are available to compare."
    if summary["avg_cos_sim"] >= 0.9:
        logging.info("check_embedding: dataset=%s compatible avg_cos_sim=%s valid=%d", dataset_id, summary["avg_cos_sim"], len(eff_sims))
        return True, data
    logging.warning("check_embedding: dataset=%s not_effective avg_cos_sim=%s valid=%d", dataset_id, summary["avg_cos_sim"], len(eff_sims))
    return "not_effective", {
        "code": RetCode.NOT_EFFECTIVE,
        "message": "Embedding model switch failed: the average similarity between old and new vectors is below 0.9, indicating incompatible vector spaces.",
        "data": data,
    }


async def search_datasets(tenant_id: str, req: dict):
    """
    Search (retrieval test) across multiple datasets.

    :param tenant_id: tenant ID
    :param req: search request containing dataset_ids and other params
    :return: (success, result) or (success, error_message)
    """
    from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type, split_model_name
    from api.db.services.doc_metadata_service import DocMetadataService
    from api.db.services.llm_service import LLMBundle
    from api.db.services.search_service import SearchService
    from api.db.services.user_service import UserTenantService
    from common.constants import LLMType
    from common.metadata_utils import apply_meta_data_filter
    from rag.app.tag import label_question
    from rag.prompts.generator import cross_languages, keyword_extraction

    kb_ids = req.get("dataset_ids", [])
    page = int(req.get("page", 1))
    size = int(req.get("size", 30))
    question = req.get("question", "")
    doc_ids = req.get("doc_ids", [])
    use_kg = req.get("use_kg", False)
    similarity_threshold = float(req.get("similarity_threshold", 0.0))
    vector_similarity_weight = float(req.get("vector_similarity_weight", 0.3))
    top = max(1, min(int(req.get("top_k", 1024)), 2048))
    langs = req.get("cross_languages", [])

    logging.debug(
        "search_datasets(datasets=%s, tenant=%s, question_len=%s)",
        kb_ids,
        tenant_id,
        len(question),
    )

    # Access check for all datasets
    for kb_id in kb_ids:
        if not KnowledgebaseService.accessible(kb_id, tenant_id):
            logging.warning("search_datasets access denied: dataset=%s tenant=%s", kb_id, tenant_id)
            return False, f"Only owner of dataset {kb_id} authorized for this operation."

    kbs = KnowledgebaseService.get_by_ids(kb_ids)
    if not kbs:
        return False, "Datasets not found!"

    # All datasets must use the same embedding model
    embd_nms = list(set([split_model_name(kb.embd_id)[0] for kb in kbs]))
    if len(embd_nms) != 1:
        return False, "Datasets use different embedding models."

    if doc_ids is not None and not isinstance(doc_ids, list):
        return False, "`doc_ids` should be a list"
    local_doc_ids = list(doc_ids) if doc_ids else []

    meta_data_filter = {}
    search_id = req.get("search_id", "")
    search_config = {}
    chat_mdl = None
    if search_id:
        search_detail = SearchService.get_detail(search_id)
        if not search_detail:
            logging.warning("search config not found: search_id=%s", search_id)
            return False, "Invalid search_id"
        search_config = search_detail.get("search_config", {})
        meta_data_filter = search_config.get("meta_data_filter", {})
        similarity_threshold = float(search_config.get("similarity_threshold", similarity_threshold))
        vector_similarity_weight = float(search_config.get("vector_similarity_weight", vector_similarity_weight))
        top = max(1, min(int(search_config.get("top_k", top)), 2048))
        use_kg = search_config.get("use_kg", use_kg)
        langs = search_config.get("cross_languages", langs)
        logging.debug(
            "Dataset search loaded Search config: search_id=%s dataset_ids=%s vector_similarity_weight=%s full_text_weight=%s similarity_threshold=%s top_k=%s",
            search_id,
            kb_ids,
            vector_similarity_weight,
            1 - vector_similarity_weight,
            similarity_threshold,
            top,
        )
        if meta_data_filter.get("method") in ["auto", "semi_auto"]:
            chat_id = search_config.get("chat_id", "")
            if chat_id:
                chat_model_config = get_model_config_from_provider_instance(tenant_id, LLMType.CHAT, search_config["chat_id"])
            else:
                chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
            chat_mdl = LLMBundle(tenant_id, chat_model_config)
    else:
        meta_data_filter = req.get("meta_data_filter") or {}
        if meta_data_filter.get("method") in ["auto", "semi_auto"]:
            chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
            chat_mdl = LLMBundle(tenant_id, chat_model_config)

    if meta_data_filter:
        logging.debug("Metadata filter applied: %s, question length: %d, chat_mdl=%s", meta_data_filter, len(question), "None" if chat_mdl is None else "configured")
        local_doc_ids = await apply_meta_data_filter(
            meta_data_filter,
            None,
            question,
            chat_mdl,
            local_doc_ids,
            kb_ids=kb_ids,
            metas_loader=lambda: DocMetadataService.get_flatted_meta_by_kbs(kb_ids),
        )

    tenant_ids = []
    tenants = UserTenantService.query(user_id=tenant_id)
    for tenant in tenants:
        if any(KnowledgebaseService.query(tenant_id=tenant.tenant_id, id=kb_id) for kb_id in kb_ids):
            tenant_ids.append(tenant.tenant_id)
            break
    else:
        return False, "Only owner of datasets authorized for this operation."

    kb = kbs[0]
    _question = question
    if langs:
        _question = await cross_languages(kb.tenant_id, None, _question, langs)
    if kb.embd_id:
        embd_model_config = get_model_config_from_provider_instance(kb.tenant_id, LLMType.EMBEDDING, kb.embd_id)
    else:
        embd_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.EMBEDDING)
    embd_mdl = LLMBundle(kb.tenant_id, embd_model_config)

    rerank_mdl = None
    rerank_id = search_config.get("rerank_id") or req.get("rerank_id")
    if rerank_id:
        rerank_model_config = get_model_config_from_provider_instance(kb.tenant_id, LLMType.RERANK.value, rerank_id)
        rerank_mdl = LLMBundle(kb.tenant_id, rerank_model_config)

    if search_config.get("keyword", req.get("keyword", False)):
        default_chat_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.CHAT)
        chat_mdl = LLMBundle(kb.tenant_id, default_chat_model_config)
        _question += await keyword_extraction(chat_mdl, _question)

    labels = label_question(_question, kbs)
    ranks = await settings.retriever.retrieval(
        _question,
        embd_mdl,
        tenant_ids,
        kb_ids,
        page,
        size,
        similarity_threshold,
        vector_similarity_weight,
        doc_ids=local_doc_ids,
        top=top,
        rerank_mdl=rerank_mdl,
        rank_feature=labels,
        trace_id=search_id,
    )

    if use_kg:
        try:
            default_chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
            ck = await settings.kg_retriever.retrieval(_question, tenant_ids, kb_ids, embd_mdl, LLMBundle(kb.tenant_id, default_chat_model_config))
            if ck["content_with_weight"]:
                ranks["chunks"].insert(0, ck)
        except Exception:
            logging.warning("search_datasets KG retrieval failed: datasets=%s tenant=%s", kb_ids, tenant_id, exc_info=True)
    ranks["chunks"] = settings.retriever.retrieval_by_children(ranks["chunks"], tenant_ids)
    ranks["total"] = len(ranks["chunks"])

    for c in ranks["chunks"]:
        c.pop("vector", None)
    ranks["labels"] = labels

    return True, ranks


# ---------------------------------------------------------------------------
# Artifact (knowledge compilation) page surface
#
# These three helpers power the dataset-level "Artifact" tab. They query rows
# with ``compile_kwd="artifact_page"`` written by TaskHandler's
# ``_persist_wiki_pages_to_es``. The schema fields they rely on are:
#   slug_kwd, title_kwd, page_type_kwd, content_with_weight,
#   entity_names_kwd, outlinks_kwd, related_kb_pages_kwd,
#   source_chunk_ids, source_doc_ids
# ---------------------------------------------------------------------------

_WIKI_COMPILE_KWD = "artifact_page"
_SKILL_COMPILE_KWD = "skill"
_SKILL_ALL_COMPILE_KWD = "skill_all"


def _compiled_index_or_none(tenant_id: str, kb_id: str):
    """Return (index_name, search_module) when the tenant index exists,
    else ``None``. Avoids 500s on brand-new tenants whose ES index hasn't
    been created yet."""
    from rag.nlp import search as _rag_search

    index_nm = _rag_search.index_name(tenant_id)
    if not settings.docStoreConn.index_exist(index_nm, kb_id):
        return None
    return index_nm, _rag_search


def _wiki_index_or_none(tenant_id: str, kb_id: str):
    return _compiled_index_or_none(tenant_id, kb_id)


def _skill_index_or_none(tenant_id: str, kb_id: str):
    return _compiled_index_or_none(tenant_id, kb_id)


async def has_any_wiki(dataset_id: str, tenant_id: str):
    """Fast existence probe for the sidebar tab visibility check.

    Returns ``(True, {"has": bool})`` on success or ``(False, str)`` on
    auth failure. Runs a ``limit=1`` search and reads only the total.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _wiki_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, {"has": False}
    index_nm, _ = pack

    from common.doc_store.doc_store_base import OrderByExpr

    try:
        res = settings.docStoreConn.search(
            select_fields=["id"],
            highlight_fields=[],
            condition={"compile_kwd": [_WIKI_COMPILE_KWD]},
            match_expressions=[],
            order_by=OrderByExpr(),
            offset=0,
            limit=1,
            index_names=index_nm,
            knowledgebase_ids=[dataset_id],
        )
    except Exception:
        logging.exception("has_any_wiki: docStore search failed for kb=%s", dataset_id)
        return True, {"has": False}

    total = settings.docStoreConn.get_total(res)
    return True, {"has": bool(total)}


async def list_wiki_pages(
    dataset_id: str,
    tenant_id: str,
    page: int = 1,
    page_size: int = 200,
    page_type: str | None = None,
):
    """List artifact pages for the left-hand 2-column list.

    Returns ``(True, {"total", "items": [{slug, title, page_type}, ...]})``.
    Ordering: ``page_type`` ascending, then ``title`` ascending — keeps
    pages of the same type grouped together visually.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _wiki_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, {"total": 0, "items": []}
    index_nm, _ = pack

    from common.doc_store.doc_store_base import OrderByExpr

    page = max(1, int(page or 1))
    page_size = max(1, min(int(page_size or 200), 1000))
    offset = (page - 1) * page_size

    condition: dict = {"compile_kwd": [_WIKI_COMPILE_KWD]}
    if page_type:
        condition["page_type_kwd"] = [page_type]

    order_by = OrderByExpr()
    try:
        # Most-connected pages first: outlinks_int = len(outlinks_kwd) is
        # written by the persistence layer for exactly this query.
        order_by.desc("outlinks_int").asc("title_kwd")
    except Exception:
        # OrderByExpr API differs across doc-store backends; degrade to
        # default order rather than 500.
        order_by = OrderByExpr()

    select_fields = [
        "id",
        "slug_kwd",
        "title_kwd",
        "page_type_kwd",
        "outlinks_int",
        "summary_with_weight",
    ]
    try:
        res = settings.docStoreConn.search(
            select_fields=select_fields,
            highlight_fields=[],
            condition=condition,
            match_expressions=[],
            order_by=order_by,
            offset=offset,
            limit=page_size,
            index_names=index_nm,
            knowledgebase_ids=[dataset_id],
        )
        field_map = settings.docStoreConn.get_fields(res, select_fields)
    except Exception:
        logging.exception("list_wiki_pages: docStore search failed for kb=%s", dataset_id)
        return True, {"total": 0, "items": []}

    total = settings.docStoreConn.get_total(res)
    items = []
    for row in (field_map or {}).values():
        slug = row.get("slug_kwd")
        if not isinstance(slug, str) or not slug:
            continue
        items.append(
            {
                "slug": slug,
                "title": row.get("title_kwd") or slug,
                "page_type": row.get("page_type_kwd") or "concept",
                "summary": row.get("summary_with_weight") or "",
            }
        )

    return True, {"total": int(total or 0), "items": items}


async def get_wiki_page(
    dataset_id: str,
    tenant_id: str,
    page_type: str,
    slug: str,
):
    """Fetch a single artifact page for the right-hand markdown viewer.

    ``slug`` is the tail after ``<page_type>/`` — i.e. the URL component
    that came from the markdown link ``artifact/<kb_id>/<page_type>/<slug>``.
    The stored ``slug_kwd`` is the full ``<page_type>/<slug>`` form, so we
    reconstruct it before the lookup.

    Returns ``(True, page_dict)`` or ``(True, None)`` when no row matches.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _wiki_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, None
    index_nm, _ = pack

    from common.doc_store.doc_store_base import OrderByExpr

    full_slug = f"{page_type}/{slug}" if "/" not in slug else slug
    select_fields = [
        "id",
        "slug_kwd",
        "title_kwd",
        "page_type_kwd",
        "content_with_weight",
        "summary_with_weight",
        "entity_names_kwd",
        "outlinks_kwd",
        "related_kb_pages_kwd",
        "source_chunk_ids",
        "source_doc_ids",
    ]
    try:
        res = settings.docStoreConn.search(
            select_fields=select_fields,
            highlight_fields=[],
            condition={
                "compile_kwd": [_WIKI_COMPILE_KWD],
                "page_type_kwd": [page_type],
                "slug_kwd": [full_slug],
            },
            match_expressions=[],
            order_by=OrderByExpr(),
            offset=0,
            limit=1,
            index_names=index_nm,
            knowledgebase_ids=[dataset_id],
        )
        field_map = settings.docStoreConn.get_fields(res, select_fields)
    except Exception:
        logging.exception(
            "get_wiki_page: search failed for kb=%s slug=%s",
            dataset_id,
            full_slug,
        )
        return True, None

    if not field_map:
        return True, None

    _, row = next(iter(field_map.items()))
    content_md = row.get("content_with_weight") or ""
    summary = row.get("summary_with_weight") or ""
    return True, {
        "slug": row.get("slug_kwd") or full_slug,
        "title": row.get("title_kwd") or full_slug,
        "page_type": row.get("page_type_kwd") or page_type,
        "content_md_rendered": content_md,
        "summary": summary,
        "entity_names": row.get("entity_names_kwd") or [],
        "outlinks": row.get("outlinks_kwd") or [],
        "related_kb_pages": row.get("related_kb_pages_kwd") or [],
        "source_chunk_ids": row.get("source_chunk_ids") or [],
        "source_doc_ids": row.get("source_doc_ids") or [],
    }


async def has_any_skill(dataset_id: str, tenant_id: str):
    """Fast existence probe for the dataset Skills sidebar entry."""
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _skill_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, {"has": False}
    index_nm, _ = pack

    from common.doc_store.doc_store_base import OrderByExpr

    try:
        res = settings.docStoreConn.search(
            select_fields=["id"],
            highlight_fields=[],
            condition={"compile_kwd": [_SKILL_ALL_COMPILE_KWD]},
            match_expressions=[],
            order_by=OrderByExpr(),
            offset=0,
            limit=1,
            index_names=index_nm,
            knowledgebase_ids=[dataset_id],
        )
    except Exception:
        logging.exception("has_any_skill: docStore search failed for kb=%s", dataset_id)
        return True, {"has": False}

    total = settings.docStoreConn.get_total(res)
    return True, {"has": bool(total)}


async def get_skill_tree(dataset_id: str, tenant_id: str):
    """Fetch the one-shot recursive skill tree for this dataset."""
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _skill_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, None
    index_nm, _ = pack

    from common.doc_store.doc_store_base import OrderByExpr

    select_fields = ["id", "kb_id", "doc_id", "compile_kwd", "skill_with_weight"]
    try:
        res = settings.docStoreConn.search(
            select_fields=select_fields,
            highlight_fields=[],
            condition={"compile_kwd": [_SKILL_ALL_COMPILE_KWD]},
            match_expressions=[],
            order_by=OrderByExpr(),
            offset=0,
            limit=1,
            index_names=index_nm,
            knowledgebase_ids=[dataset_id],
        )
        field_map = settings.docStoreConn.get_fields(res, select_fields)
    except Exception:
        logging.exception("get_skill_tree: docStore search failed for kb=%s", dataset_id)
        return True, None

    if not field_map:
        return True, None

    _, row = next(iter(field_map.items()))
    return True, {
        "id": row.get("id"),
        "kb_id": row.get("kb_id") or dataset_id,
        "doc_id": row.get("doc_id") or dataset_id,
        "compile_kwd": row.get("compile_kwd") or _SKILL_ALL_COMPILE_KWD,
        "skill_with_weight": json.loads(row.get("skill_with_weight")) or [],
    }


async def get_skill_page(dataset_id: str, tenant_id: str, skill_kwd: str):
    """Fetch the full markdown body for a single skill node."""
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _skill_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, None
    index_nm, _ = pack

    from common.doc_store.doc_store_base import OrderByExpr

    select_fields = [
        "id",
        "kb_id",
        "doc_id",
        "compile_kwd",
        "skill_kwd",
        "depth_int",
        "children_kwd",
        "source_doc_ids",
        "md_with_weight",
    ]
    try:
        res = settings.docStoreConn.search(
            select_fields=select_fields,
            highlight_fields=[],
            condition={
                "compile_kwd": [_SKILL_COMPILE_KWD],
                "skill_kwd": [skill_kwd],
            },
            match_expressions=[],
            order_by=OrderByExpr(),
            offset=0,
            limit=1,
            index_names=index_nm,
            knowledgebase_ids=[dataset_id],
        )
        field_map = settings.docStoreConn.get_fields(res, select_fields)
    except Exception:
        logging.exception(
            "get_skill_page: docStore search failed for kb=%s skill=%s",
            dataset_id,
            skill_kwd,
        )
        return True, None

    if not field_map:
        return True, None

    _, row = next(iter(field_map.items()))
    return True, {
        "id": row.get("id"),
        "kb_id": row.get("kb_id") or dataset_id,
        "doc_id": row.get("doc_id") or dataset_id,
        "compile_kwd": row.get("compile_kwd") or _SKILL_COMPILE_KWD,
        "skill_kwd": row.get("skill_kwd") or skill_kwd,
        "depth_int": row.get("depth_int") or 0,
        "children_kwd": row.get("children_kwd") or [],
        "source_doc_ids": row.get("source_doc_ids") or [],
        "md_with_weight": row.get("md_with_weight") or "",
    }


async def update_wiki_page(
    dataset_id: str,
    tenant_id: str,
    page_type: str,
    slug: str,
    content_md: str,
    *,
    user_id: str | None = None,
    title: str | None = None,
    comments: str | None = None,
):
    """Edit an artifact page in place from the canvas double-click dialog.

    Body must contain ``content_md`` — the (possibly edited) page markdown.
    We run it through ``_wiki_transform_links`` so any newly typed
    ``[[slug]]`` references upgrade to clickable artifact URLs (and pre-rendered
    links pass through unchanged — the transform is idempotent on already-
    rendered markdown). ``summary`` is re-derived from the new rendered text.
    ``outlinks_kwd`` is rebuilt from the link-transform pass.

    Per the v1 contract, only the page row is updated. The canvas
    ``artifact_page_graph`` / ``artifact_entity`` / ``artifact_relation``
    rows stay stale until the next full artifact compile.

    Side effect: when the rendered post-save markdown differs from the
    prior stored content, one ``artifact_commit`` row is recorded
    (git-style audit). No-op saves are silently skipped — empty diff,
    no row.

    Returns ``(True, page_dict)`` mirroring ``get_wiki_page``, or
    ``(True, None)`` when the row is missing, or
    ``(False, message)`` on authorization failure.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _wiki_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, None
    index_nm, _ = pack

    from rag.advanced_rag.knowlege_compile.wiki import (
        _wiki_transform_links,
        _wiki_extract_summary,
    )
    from api.db.services.file_commit_service import FileCommitService

    full_slug = f"{page_type}/{slug}" if "/" not in slug else slug

    # Capture the pre-edit rendered content + the row id. Both come from
    # the same search: the row id is the dict key returned by
    # docStoreConn.get_fields. We need the id specifically because the
    # generic non-id update path (ESConnection.update slow branch) routes
    # through a Painless script that scrubs newlines / single quotes /
    # backslash escapes from string values — which would collapse every
    # paragraph in the saved markdown to one line. Passing the row id in
    # ``condition`` selects the fast partial-update branch which preserves
    # the JSON value verbatim.
    from common.doc_store.doc_store_base import OrderByExpr

    row_id: str | None = None
    content_before = ""
    try:
        res = settings.docStoreConn.search(
            select_fields=["id", "content_with_weight"],
            highlight_fields=[],
            condition={
                "compile_kwd": [_WIKI_COMPILE_KWD],
                "page_type_kwd": [page_type],
                "slug_kwd": [full_slug],
            },
            match_expressions=[],
            order_by=OrderByExpr(),
            offset=0,
            limit=1,
            index_names=index_nm,
            knowledgebase_ids=[dataset_id],
        )
        field_map = settings.docStoreConn.get_fields(
            res,
            ["id", "content_with_weight"],
        )
        if field_map:
            row_id, row = next(iter(field_map.items()))
            content_before = row.get("content_with_weight") or ""
    except Exception:
        logging.exception(
            "update_wiki_page: lookup failed for kb=%s slug=%s",
            dataset_id,
            full_slug,
        )
    if not row_id:
        return True, None

    content_md = content_md or ""
    rendered, outlinks = _wiki_transform_links(content_md, dataset_id)
    summary = _wiki_extract_summary(rendered) or ""

    try:
        # id-keyed condition forces the partial-update fast path — no
        # newline scrubbing. See the comment above the lookup for the
        # full reasoning.
        ok = settings.docStoreConn.update(
            {"id": row_id},
            {
                "content_with_weight": rendered,
                "summary_with_weight": summary,
                "outlinks_kwd": list(outlinks),
            },
            index_nm,
            dataset_id,
        )
    except Exception:
        logging.exception(
            "update_wiki_page: docStore update failed for kb=%s slug=%s",
            dataset_id,
            full_slug,
        )
        return True, None

    if not ok:
        return True, None

    # Record a file_commit row on every real change. ``record_page_edit``
    # returns None for empty-diff saves, which we silently swallow.
    try:
        FileCommitService.record_page_edit(
            tenant_id=tenant_id,
            kb_id=dataset_id,
            page_type=page_type,
            slug=full_slug,
            content_before=content_before,
            content_after=rendered,
            title=title,
            comments=comments,
            user_id=user_id,
        )
    except Exception:
        logging.exception(
            "update_wiki_page: file_commit record failed for kb=%s slug=%s",
            dataset_id,
            full_slug,
        )

    # Re-read the row so the dialog gets the canonical post-update state.
    return await get_wiki_page(dataset_id, tenant_id, page_type, slug)


# ``list_wiki_commits`` / ``get_wiki_commit`` retired — the two
# ``/datasets/<id>/artifacts/.../commits`` REST endpoints now go through
# the generic file-commit routes (``/datasets/<id>/commits`` with an
# optional ``?slug=`` filter), backed by
# :meth:`FileCommitService.list_page_commits` and
# :meth:`FileCommitService.get_page_commit_detail`.


# All six row types the artifact pipeline writes. Listed in dependency
# order so partial failures of earlier deletes don't leave behind state
# that downstream phases would silently reuse. ``artifact_page_graph``
# is the materialized canvas graph derived from the refined pages —
# the dataset Artifact tab's graph view reads exactly this row.
_WIKI_COMPILE_KWDS = (
    "artifact_map_extract",
    "artifact_reduce_result",
    "artifact_compilation_plan",
    "artifact_page_draft",
    "artifact_page",
    "artifact_entity",
    "artifact_relation",
)

# Tunables for the incremental graph loader. See ``get_wiki_graph``.
_WIKI_GRAPH_ENTITY_KWD = "artifact_entity"
_WIKI_GRAPH_RELATION_KWD = "artifact_relation"
_WIKI_GRAPH_ENTITY_PAGE_SIZE = 32
_WIKI_GRAPH_MAX_LOADING_ENTITY = 128


def _wiki_entity_payload(row: dict) -> dict | None:
    """Project one ``artifact_entity`` ES row onto the canvas entity shape.

    The row stores the canvas payload pre-built as JSON in
    ``content_with_weight``; we parse it back and overlay the columns
    the writer set independently (weight_int, source_chunk_ids) so the
    frontend gets the authoritative numbers regardless of any
    JSON-vs-column drift.
    """
    raw = row.get("content_with_weight") or ""
    payload: dict = {}
    if isinstance(raw, str) and raw.strip():
        try:
            parsed = json.loads(raw)
            if isinstance(parsed, dict):
                payload = parsed
        except Exception:
            pass
    slug = payload.get("slug") or row.get("slug_kwd")
    if not isinstance(slug, str) or not slug:
        return None
    out = {
        "slug": slug,
        "name": payload.get("name") or slug,
        "aliases": list(payload.get("aliases") or []),
        "description": payload.get("description") or "",
        "type": payload.get("type") or "concept",
        "weight": int(row.get("weight_int") or payload.get("weight") or 0),
    }
    source_chunk_ids = row.get("source_chunk_ids") or []
    if isinstance(source_chunk_ids, list):
        out["source_chunk_ids"] = [c for c in source_chunk_ids if isinstance(c, str) and c]
    return out


def _wiki_relation_payload(row: dict) -> dict | None:
    raw = row.get("content_with_weight") or ""
    payload: dict = {}
    if isinstance(raw, str) and raw.strip():
        try:
            parsed = json.loads(raw)
            if isinstance(parsed, dict):
                payload = parsed
        except Exception:
            pass
    src = payload.get("from") or row.get("from_kwd")
    tgt = payload.get("to") or row.get("to_kwd")
    if not isinstance(src, str) or not src or not isinstance(tgt, str) or not tgt:
        return None
    return {"from": src, "to": tgt}


async def _wiki_search_entity_page(
    index_nm,
    dataset_id: str,
    offset: int,
    limit: int,
):
    """One page of artifact_entity rows, ordered by weight_int DESC."""
    from common.doc_store.doc_store_base import OrderByExpr

    order_by = OrderByExpr()
    try:
        order_by.desc("weight_int")
    except Exception:
        order_by = OrderByExpr()

    select_fields = [
        "id",
        "slug_kwd",
        "weight_int",
        "source_chunk_ids",
        "content_with_weight",
    ]
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        select_fields,
        [],
        {"compile_kwd": [_WIKI_GRAPH_ENTITY_KWD]},
        [],
        order_by,
        offset,
        limit,
        index_nm,
        [dataset_id],
    )
    return settings.docStoreConn.get_fields(res, select_fields)


async def _wiki_search_entities_by_slugs(
    index_nm,
    dataset_id: str,
    slugs: list[str],
):
    """Fetch entity rows whose ``slug_kwd`` is in ``slugs``. Unordered."""
    if not slugs:
        return {}

    from common.doc_store.doc_store_base import OrderByExpr

    select_fields = [
        "id",
        "slug_kwd",
        "weight_int",
        "source_chunk_ids",
        "content_with_weight",
    ]
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        select_fields,
        [],
        {
            "compile_kwd": [_WIKI_GRAPH_ENTITY_KWD],
            "slug_kwd": list(slugs),
        },
        [],
        OrderByExpr(),
        0,
        max(len(slugs), 1),
        index_nm,
        [dataset_id],
    )
    return settings.docStoreConn.get_fields(res, select_fields)


async def _wiki_search_relations_from(
    index_nm,
    dataset_id: str,
    from_slugs: list[str],
):
    """Fetch all relation rows with ``from_kwd`` in ``from_slugs``."""
    if not from_slugs:
        return {}

    from common.doc_store.doc_store_base import OrderByExpr

    select_fields = ["id", "from_kwd", "to_kwd", "content_with_weight"]
    # Generous upper bound: relations are short; bulk-pull all matching at
    # once rather than paging.
    res = await thread_pool_exec(
        settings.docStoreConn.search,
        select_fields,
        [],
        {
            "compile_kwd": [_WIKI_GRAPH_RELATION_KWD],
            "from_kwd": list(from_slugs),
        },
        [],
        OrderByExpr(),
        0,
        10000,
        index_nm,
        [dataset_id],
    )
    return settings.docStoreConn.get_fields(res, select_fields)


async def get_wiki_graph(
    dataset_id: str,
    tenant_id: str,
    node: str | None = None,
):
    """Load the canvas graph payload incrementally from per-row data.

    Two modes:

    * **Overview** (``node`` is None) — paginate ``artifact_entity`` rows
      ordered by ``weight_int DESC`` in pages of
      ``_WIKI_GRAPH_ENTITY_PAGE_SIZE``. For each page, append entities
      to a running set while the **cumulative** weight stays within
      ``_WIKI_GRAPH_MAX_LOADING_ENTITY``. Pull ``artifact_relation``
      rows whose ``from_kwd`` is in the just-added entities; pull the
      ``to`` targets that we haven't seen yet (they count toward the same
      cap). Stop once the cap is hit, or the page is empty, or no entry
      from the page fit under the budget.

    * **Click** (``node`` is a slug) — load the centre entity (always
      included), pull every ``artifact_relation`` with ``from_kwd=node``,
      then pull the ``to`` entities. Capped at
      ``_WIKI_GRAPH_MAX_LOADING_ENTITY`` for hub-node safety.

    Returns ``(True, {"entities": [...], "relations": [...]})`` shaped
    exactly as the frontend ``ForceGraph`` adapter consumes, or
    ``(False, message)`` on authorization failure.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    empty = {"entities": [], "relations": []}

    pack = _wiki_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, empty
    index_nm, _ = pack

    cap = _WIKI_GRAPH_MAX_LOADING_ENTITY
    page_size = _WIKI_GRAPH_ENTITY_PAGE_SIZE

    # ``entities`` preserves first-seen order so the canvas paints the
    # heaviest-weighted nodes first (or, in click mode, the centre node
    # first). The dict-keyed-by-slug structure also deduplicates the
    # "B is a to-target AND later a high-weight entity in its own right"
    # case cheaply.
    entities: dict[str, dict] = {}
    relations: list[dict] = []
    relation_keys: set[tuple[str, str]] = set()

    def _add_entity(payload: dict) -> bool:
        slug = payload.get("slug")
        if not isinstance(slug, str) or not slug or slug in entities:
            return False
        entities[slug] = payload
        return True

    def _add_relation(payload: dict) -> None:
        key = (payload["from"], payload["to"])
        if key in relation_keys:
            return
        relation_keys.add(key)
        relations.append(payload)

    # ---- Flow B — click expansion centred on ``node``. ----------------
    if isinstance(node, str) and node.strip():
        center_slug = node.strip()
        try:
            field_map = await _wiki_search_entities_by_slugs(
                index_nm,
                dataset_id,
                [center_slug],
            )
        except Exception:
            logging.exception(
                "get_wiki_graph: centre lookup failed kb=%s node=%s",
                dataset_id,
                center_slug,
            )
            return True, empty

        for row in (field_map or {}).values():
            payload = _wiki_entity_payload(row)
            if payload:
                _add_entity(payload)
                break

        if center_slug not in entities:
            # Caller pointed at a slug that doesn't exist; return empty
            # rather than a confusing partial graph.
            return True, empty

        # Outgoing edges from the centre, capped by MAX_LOADING_ENTITY.
        try:
            rel_map = await _wiki_search_relations_from(
                index_nm,
                dataset_id,
                [center_slug],
            )
        except Exception:
            logging.exception(
                "get_wiki_graph: relation lookup failed kb=%s node=%s",
                dataset_id,
                center_slug,
            )
            return True, {"entities": list(entities.values()), "relations": []}

        to_slugs: list[str] = []
        for row in (rel_map or {}).values():
            payload = _wiki_relation_payload(row)
            if payload is None:
                continue
            if payload["from"] != center_slug:
                continue
            # Hub-node cap: stop accepting more relations once the
            # to-target set would push us over the entity budget.
            if payload["to"] not in entities and len(entities) + len(to_slugs) >= cap:
                continue
            _add_relation(payload)
            if payload["to"] != center_slug and payload["to"] not in entities:
                if payload["to"] not in to_slugs:
                    to_slugs.append(payload["to"])

        if to_slugs:
            try:
                to_map = await _wiki_search_entities_by_slugs(
                    index_nm,
                    dataset_id,
                    to_slugs,
                )
            except Exception:
                logging.exception(
                    "get_wiki_graph: neighbour lookup failed kb=%s node=%s",
                    dataset_id,
                    center_slug,
                )
                to_map = {}
            for row in (to_map or {}).values():
                payload = _wiki_entity_payload(row)
                if payload and len(entities) < cap:
                    _add_entity(payload)

        return True, {
            "entities": list(entities.values()),
            "relations": relations,
        }

    # ---- Flow A — overview, top-weight paged with cumulative budget. ---
    cumulative_weight = 0
    page = 1
    while len(entities) < cap:
        offset = (page - 1) * page_size
        try:
            field_map = await _wiki_search_entity_page(
                index_nm,
                dataset_id,
                offset,
                page_size,
            )
        except Exception:
            logging.exception(
                "get_wiki_graph: entity page fetch failed kb=%s page=%d",
                dataset_id,
                page,
            )
            break
        if not field_map:
            break

        # Preserve weight_int DESC order from ES. Iteration over a dict
        # produced by get_fields keeps insertion order; ES returned them
        # sorted, so we can rely on that.
        page_rows = list(field_map.values())

        e_sub: list[dict] = []
        for row in page_rows:
            payload = _wiki_entity_payload(row)
            if payload is None:
                continue
            if payload["slug"] in entities:
                continue
            w = max(0, int(payload.get("weight") or 0))
            # Step 2: cumulative across the whole flow (per the spec).
            # Stop when adding this entry would push the budget over.
            # If even the first entity on a page can't fit, we exit the
            # outer loop below; this preserves the "least-weight first
            # excluded" semantics.
            # if cumulative_weight + w > cap and len(entities) + len(e_sub) > 0:
            #    break
            cumulative_weight += w
            e_sub.append(payload)
            if len(entities) + len(e_sub) >= cap:
                break

        if not e_sub:
            break

        for payload in e_sub:
            _add_entity(payload)

        # Step 3: relations originating in E_sub.
        sub_slugs = [p["slug"] for p in e_sub]
        try:
            rel_map = await _wiki_search_relations_from(
                index_nm,
                dataset_id,
                sub_slugs,
            )
        except Exception:
            logging.exception(
                "get_wiki_graph: relation page fetch failed kb=%s",
                dataset_id,
            )
            rel_map = {}

        missing_to: list[str] = []
        for row in (rel_map or {}).values():
            payload = _wiki_relation_payload(row)
            if payload is None:
                continue
            _add_relation(payload)
            if payload["to"] not in entities and payload["to"] not in missing_to:
                missing_to.append(payload["to"])

        # Step 4: hydrate the to-targets (they count toward the cap).
        if missing_to:
            try:
                to_map = await _wiki_search_entities_by_slugs(
                    index_nm,
                    dataset_id,
                    missing_to,
                )
            except Exception:
                logging.exception(
                    "get_wiki_graph: to-target hydrate failed kb=%s",
                    dataset_id,
                )
                to_map = {}
            for row in (to_map or {}).values():
                if len(entities) >= cap:
                    break
                payload = _wiki_entity_payload(row)
                if payload:
                    _add_entity(payload)

        # Step 5: page forward only if the cap allows another iteration.
        if len(entities) >= cap or len(page_rows) < page_size:
            break
        page += 1

    return True, {
        "entities": list(entities.values()),
        "relations": relations,
    }


async def clear_wiki(dataset_id: str, tenant_id: str):
    """Wipe every artifact-related row from ES for this KB.

    Touches all five ``compile_kwd`` row types the artifact pipeline writes
    (MAP extracts, REDUCE results, PLAN output, page drafts, and the
    searchable artifact_page rows). After this completes the next "Artifact"
    run starts from a clean slate — no resume cache to short-circuit MAP, no
    prior pages to reconcile against in PLAN.

    Returns ``(True, {"deleted": {kwd: count_or_True}})`` on success or
    ``(False, str)`` on auth failure.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _wiki_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, {"deleted": {}}
    index_nm, _ = pack

    deleted: dict[str, object] = {}
    for kwd in _WIKI_COMPILE_KWDS:
        try:
            res = settings.docStoreConn.delete(
                {"compile_kwd": kwd},
                index_nm,
                dataset_id,
            )
            # Different backends return different shapes (int count, dict,
            # bool). Surface whatever we got so the caller can log it.
            deleted[kwd] = res if res is not None else True
        except Exception:
            logging.exception(
                "clear_wiki: delete failed for kwd=%s kb=%s",
                kwd,
                dataset_id,
            )
            deleted[kwd] = False

    return True, {"deleted": deleted}
