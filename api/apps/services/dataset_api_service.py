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
from api.db.services.connector_service import Connector2KbService
from api.db.services.task_service import GRAPH_RAPTOR_FAKE_DOC_ID, TaskService
from api.db.services.user_service import TenantService, UserService, UserTenantService
from common.constants import FileSource, StatusEnum
from api.utils.api_utils import deep_merge, get_parser_config, remap_dictionary_keys, verify_embedding_availability

_VALID_INDEX_TYPES = {"graph", "raptor", "mindmap"}

_INDEX_TYPE_TO_TASK_TYPE = {
    "graph": "graphrag",
    "raptor": "raptor",
    "mindmap": "mindmap",
}

_INDEX_TYPE_TO_TASK_ID_FIELD = {
    "graph": "graphrag_task_id",
    "raptor": "raptor_task_id",
    "mindmap": "mindmap_task_id",
}

_INDEX_TYPE_TO_DISPLAY_NAME = {
    "graph": "Graph",
    "raptor": "RAPTOR",
    "mindmap": "Mindmap",
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

    e, create_dict = KnowledgebaseService.create_with_name(
        name=req.pop("name", None),
        tenant_id=tenant_id,
        parser_id=req.pop("parser_id", None),
        **req
    )

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
    kbs, total = KnowledgebaseService.get_list(
        tenant_ids,
        tenant_id,
        page,
        page_size,
        orderby,
        desc,
        kb_id,
        name,
        keywords,
        parser_id
    )
    users = UserService.get_by_ids([m["tenant_id"] for m in kbs])
    user_map = {m.id: m.to_dict() for m in users}
    response_data_list = []
    for kb in kbs:
        user_dict = user_map.get(kb["tenant_id"], {})
        kb.update({
            "nickname": user_dict.get("nickname", ""),
            "tenant_avatar": user_dict.get("avatar", "")
        })
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
        for bucket in settings.retriever.all_tags(kb_tenant_id, kb_ids):
            tag = bucket["value"]
            merged[tag] = merged.get(tag, 0) + bucket["count"]

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
    metadata = parser_cfg.get("metadata") or []
    enabled = parser_cfg.get("enable_metadata", bool(metadata))
    # Normalize to AutoMetadataConfig-like JSON
    fields = []
    for f in metadata:
        if not isinstance(f, dict):
            continue
        fields.append(
            {
                "name": f.get("name", ""),
                "type": f.get("type", ""),
                "description": f.get("description"),
                "examples": f.get("examples"),
                "restrict_values": f.get("restrict_values", False),
            }
        )
    return True, {"enabled": enabled, "fields": fields}


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
    fields = []
    for f in cfg.get("fields", []):
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
    parser_cfg["enable_metadata"] = cfg.get("enabled", True)

    if not KnowledgebaseService.update_by_id(kb.id, {"parser_config": parser_cfg}):
        return False, "Update auto-metadata error.(Database error)"

    return True, {"enabled": parser_cfg["enable_metadata"], "fields": fields}


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
        settings.docStoreConn.update({"tag_kwd": t, "kb_id": [dataset_id]},
                                     {"remove": {"tag_kwd": t}},
                                     search.index_name(kb.tenant_id),
                                     dataset_id)

    return True, {}

def list_ingestion_logs(dataset_id: str, tenant_id: str, page: int, page_size: int, orderby: str, desc: bool, operation_status: list = None, create_date_from: str = None, create_date_to: str = None):
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
    :return: (success, result) or (success, error_message)
    """
    if not dataset_id:
        return False, 'Lack of "Dataset ID"'

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "No authorization."

    from api.db.services.pipeline_operation_log_service import PipelineOperationLogService
    logs, total = PipelineOperationLogService.get_dataset_logs_by_kb_id(
        dataset_id, page, page_size, orderby, desc, operation_status or [], create_date_from, create_date_to
    )
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
    fields = PipelineOperationLogService.get_dataset_logs_fields()
    log = PipelineOperationLogService.model.select(*fields).where(
        (PipelineOperationLogService.model.id == log_id) & (PipelineOperationLogService.model.kb_id == dataset_id)
    ).first()
    if not log:
        return False, "Log not found"

    return True, log.to_dict()


def delete_index(dataset_id: str, tenant_id: str, index_type: str):
    """
    Delete an indexing task (graph/raptor/mindmap) for a dataset.

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
    task_finish_at_field = f"{task_id_field.replace('_task_id', '_task_finish_at')}"
    task_id = getattr(kb, task_id_field, None)

    if task_id:
        from rag.utils.redis_conn import REDIS_CONN
        try:
            REDIS_CONN.set(f"{task_id}-cancel", "x")
        except Exception as e:
            logging.exception(e)
        TaskService.delete_by_id(task_id)

    if index_type == "graph":
        from rag.nlp import search
        settings.docStoreConn.delete({"knowledge_graph_kwd": ["graph", "subgraph", "entity", "relation"]},
                                     search.index_name(kb.tenant_id), dataset_id)
    elif index_type == "raptor":
        from rag.nlp import search
        settings.docStoreConn.delete({"raptor_kwd": ["raptor"]},
                                     search.index_name(kb.tenant_id), dataset_id)

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
    settings.docStoreConn.update({"tag_kwd": from_tag, "kb_id": [dataset_id]},
                                 {"remove": {"tag_kwd": from_tag.strip()}, "add": {"tag_kwd": to_tag}},
                                 search.index_name(kb.tenant_id),
                                 dataset_id)

    return True, {"from": from_tag, "to": to_tag}


async def search(dataset_id: str, tenant_id: str, req: dict):
    """
    Search (retrieval test) within a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :param req: search request
    :return: (success, result) or (success, error_message)
    """
    from api.db.joint_services.tenant_model_service import (
        get_model_config_by_id,
        get_model_config_by_type_and_name,
        get_tenant_default_model_by_type,
    )
    from api.db.services.doc_metadata_service import DocMetadataService
    from api.db.services.llm_service import LLMBundle
    from api.db.services.search_service import SearchService
    from api.db.services.user_service import UserTenantService
    from common.constants import LLMType
    from common.metadata_utils import apply_meta_data_filter
    from rag.app.tag import label_question
    from rag.prompts.generator import cross_languages, keyword_extraction

    page = int(req.get("page", 1))
    size = int(req.get("size", 30))
    question = req.get("question", "")
    doc_ids = req.get("doc_ids", [])
    use_kg = req.get("use_kg", False)
    top = int(req.get("top_k", 1024))
    langs = req.get("cross_languages", [])

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "Only owner of dataset authorized for this operation."

    e, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not e:
        return False, "Dataset not found!"

    local_doc_ids = list(doc_ids) if doc_ids else []

    meta_data_filter = {}
    chat_mdl = None
    if req.get("search_id", ""):
        search_config = SearchService.get_detail(req.get("search_id", "")).get("search_config", {})
        meta_data_filter = search_config.get("meta_data_filter", {})
        if meta_data_filter.get("method") in ["auto", "semi_auto"]:
            chat_id = search_config.get("chat_id", "")
            if chat_id:
                chat_model_config = get_model_config_by_type_and_name(tenant_id, LLMType.CHAT, search_config["chat_id"])
            else:
                chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
            chat_mdl = LLMBundle(tenant_id, chat_model_config)
    else:
        meta_data_filter = req.get("meta_data_filter") or {}
        if meta_data_filter.get("method") in ["auto", "semi_auto"]:
            chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
            chat_mdl = LLMBundle(tenant_id, chat_model_config)

    if meta_data_filter:
        metas = DocMetadataService.get_flatted_meta_by_kbs([dataset_id])
        local_doc_ids = await apply_meta_data_filter(meta_data_filter, metas, question, chat_mdl, local_doc_ids)

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
    if kb.tenant_embd_id:
        embd_model_config = get_model_config_by_id(kb.tenant_embd_id)
    elif kb.embd_id:
        embd_model_config = get_model_config_by_type_and_name(kb.tenant_id, LLMType.EMBEDDING, kb.embd_id)
    else:
        embd_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.EMBEDDING)
    embd_mdl = LLMBundle(kb.tenant_id, embd_model_config)

    rerank_mdl = None
    if req.get("tenant_rerank_id"):
        rerank_model_config = get_model_config_by_id(req["tenant_rerank_id"])
        rerank_mdl = LLMBundle(kb.tenant_id, rerank_model_config)
    elif req.get("rerank_id"):
        rerank_model_config = get_model_config_by_type_and_name(kb.tenant_id, LLMType.RERANK.value, req["rerank_id"])
        rerank_mdl = LLMBundle(kb.tenant_id, rerank_model_config)

    if req.get("keyword", False):
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
                    float(req.get("similarity_threshold", 0.0)),
                    float(req.get("vector_similarity_weight", 0.3)),
                    doc_ids=local_doc_ids,
                    top=top,
                    rerank_mdl=rerank_mdl,
                    rank_feature=labels
                )

    if use_kg:
        default_chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
        ck = await settings.kg_retriever.retrieval(_question,
                                               tenant_ids,
                                               [dataset_id],
                                               embd_mdl,
                                               LLMBundle(kb.tenant_id, default_chat_model_config))
        if ck["content_with_weight"]:
            ranks["chunks"].insert(0, ck)
    ranks["chunks"] = settings.retriever.retrieval_by_children(ranks["chunks"], tenant_ids)
    ranks["total"] = len(ranks["chunks"])

    for c in ranks["chunks"]:
        c.pop("vector", None)
    ranks["labels"] = labels

    return True, ranks
