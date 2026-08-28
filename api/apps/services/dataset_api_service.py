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
import json
import logging
import os
import re

from api.db.db_models import Connector2Kb, Document, File, SyncLogs
from api.db.joint_services.tenant_model_service import get_composite_model_name_by_ids, resolve_model_config, resolve_model_id
from api.db.services.connector_service import Connector2KbService, SyncLogsService
from api.db.services.document_service import DocumentService, queue_raptor_o_graphrag_tasks
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService, validate_dataset_embedding_models
from api.db.services.task_service import GRAPH_RAPTOR_FAKE_DOC_ID, TaskService
from api.db.services.tenant_model_service import TenantModelService
from api.db.services.user_service import TenantService, UserService, UserTenantService
from api.utils.api_utils import deep_merge, get_parser_config, remap_dictionary_keys, verify_embedding_availability
from common import settings
from common.constants import PAGERANK_FLD, FileSource, LLMType, RetCode, StatusEnum, TaskStatus
from common.misc_utils import thread_pool_exec, thread_pool_exec_long_time
from rag.advanced_rag.knowlege_compile.wiki import WIKI_PAGE_COMPILE_KWD

# KB-wide structure-graph merge index types. Each (re)builds the ``dataset_graph``
# rows for one structure kind via ``rebuild_dataset_structure_graph_json``; the
# task_type equals the index_type and the KB task-id column is ``<type>_task_id``.
# The value is the friendly kind resolvable through ``_resolve_dataset_structure_kind``
# (defined below), except ``structure`` which is the merge-all variant.
_STRUCTURE_INDEX_TYPE_TO_KIND = {
    "structure_graph": "graph",
    "structure_mindmap": "mindmap",
    "timeline": "timeline",
    "session_graph": "session_graph",
    "session_essence": "session_essence",
    "structure": None,  # merge-all: rebuild every dataset-merge kind
}
_STRUCTURE_INDEX_TYPES = frozenset(_STRUCTURE_INDEX_TYPE_TO_KIND)

_VALID_INDEX_TYPES = {"graph", "raptor", "mindmap", "wiki", "skill"} | set(_STRUCTURE_INDEX_TYPES)

_INDEX_TYPE_TO_TASK_TYPE = {
    "graph": "structure_graph",
    "raptor": "raptor",
    "mindmap": "structure_mindmap",
    "wiki": "wiki",
    "skill": "skill",
    # Structure merge types carry their own task_type (== index_type) so the
    # executor can resolve which kind to merge from the task body.
    **{t: t for t in _STRUCTURE_INDEX_TYPES},
}

_INDEX_TYPE_TO_TASK_ID_FIELD = {
    "graph": "graphrag_task_id",
    "raptor": "raptor_task_id",
    "mindmap": "mindmap_task_id",
    "wiki": "wiki_task_id",
    "skill": "skill_task_id",
    **{t: f"{t}_task_id" for t in _STRUCTURE_INDEX_TYPES},
}

_INDEX_TYPE_TO_DISPLAY_NAME = {
    "graph": "Graph",
    "raptor": "RAPTOR",
    "mindmap": "Mindmap",
    "wiki": "Wiki",
    "skill": "Skill",
    "structure_graph": "Structure Graph",
    "structure_mindmap": "Structure Mindmap",
    "timeline": "Timeline",
    "session_graph": "Session Graph",
    "session_essence": "Session Essence",
    "structure": "Structure",
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


def _delete_datasets_sync(tenant_id: str, ids: list = None, delete_all: bool = False):
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
        # Cancel this dataset's queued syncs before touching its documents.
        # Tasks are only picked up while they are SCHEDULE, so cancelling stops
        # every run that has not started yet; a sync already in flight is not
        # interruptible, which is what the stranded-row sweep below covers.
        SyncLogsService.filter_update(
            [SyncLogs.kb_id == kb_id, SyncLogs.status.in_([TaskStatus.SCHEDULE, TaskStatus.RUNNING])],
            {"status": TaskStatus.CANCEL},
        )

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

        # Unwire the data sources only once the dataset is really gone, so a
        # failed deletion above leaves a dataset that is still linked and still
        # syncable. Left behind, these rows keep the connector scheduler queueing
        # syncs against a kb_id that no longer resolves, and any document such a
        # run writes outlives its dataset -- an invisible row that later reports
        # a cross-KB id collision against whatever dataset is linked next.
        Connector2KbService.filter_delete([Connector2Kb.kb_id == kb_id])
        SyncLogsService.filter_delete([SyncLogs.kb_id == kb_id])

        # Sweep anything the per-document loop could not see, including rows
        # written by a sync that was already in flight when deletion started.
        stranded = DocumentService.filter_delete([Document.kb_id == kb_id])
        if stranded:
            logging.warning("delete_datasets: removed %s stranded document rows for dataset %s", stranded, kb_id)

        success_count += 1

    if not errors:
        return True, {"success_count": success_count}

    error_message = f"Successfully deleted {success_count} datasets, {len(errors)} failed. Details: {'; '.join(errors)[:128]}..."
    if success_count == 0:
        return False, error_message

    return True, {"success_count": success_count, "errors": errors[:5]}


async def delete_datasets(tenant_id: str, ids: list = None, delete_all: bool = False):
    return await thread_pool_exec_long_time(_delete_datasets_sync, tenant_id, ids, delete_all)


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
        return False, "no properties were modified"

    kbs = KnowledgebaseService.get_kb_by_id(dataset_id, tenant_id)
    if not kbs:
        return False, f"User '{tenant_id}' lacks permission for dataset '{dataset_id}'"

    kb = KnowledgebaseService.get_or_none(id=dataset_id)
    if kb is None:
        return False, "Invalid Dataset ID"

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
        exists = KnowledgebaseService.get_or_none(name=req["name"], tenant_id=kb.tenant_id, status=StatusEnum.VALID.value)
        if exists:
            return False, f"Dataset name '{req['name']}' already exists"

    if "embd_id" in req:
        if not req["embd_id"]:
            req["embd_id"] = kb.embd_id
        ok, err = verify_embedding_availability(req["embd_id"], tenant_id)
        if not ok:
            return False, err
        ok, _ = TenantModelService.get_by_id(req["embd_id"])
        if ok:
            req["tenant_embd_id"] = req["embd_id"]
        else:
            req["tenant_embd_id"] = resolve_model_id(tenant_id, LLMType.EMBEDDING, req["embd_id"])

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
    req.pop("parse_type", None)

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
    kb_ids = args.get("ids")
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

    if kb_id and kb_ids:
        return False, f"Should not provide both 'id':{kb_id} and 'ids'{kb_ids}"
    if kb_id:
        kbs = KnowledgebaseService.get_kb_by_id(kb_id, tenant_id)
        if not kbs:
            return False, f"User '{tenant_id}' lacks permission for dataset '{kb_id}'"

    if name:
        kbs = KnowledgebaseService.get_kb_by_name(name, tenant_id)
        if not kbs:
            return False, f"User '{tenant_id}' lacks permission for dataset '{name}'"
    owner_ids = [owner_id.strip() for owner_id in ext_fields.get("owner_ids", []) if isinstance(owner_id, str) and owner_id.strip()]
    if owner_ids:
        tenants = TenantService.get_joined_tenants_by_user_id(tenant_id)
        allowed_tenant_ids = {m["tenant_id"] for m in tenants}
        allowed_tenant_ids.add(tenant_id)
        tenant_ids = [owner_id for owner_id in owner_ids if owner_id in allowed_tenant_ids]
        query_user_id = tenant_id if tenant_id in tenant_ids else ""
    else:
        tenants = TenantService.get_joined_tenants_by_user_id(tenant_id)
        tenant_ids = [m["tenant_id"] for m in tenants]
        query_user_id = tenant_id
    if kb_ids:
        accessible_ids = KnowledgebaseService.get_accessible_ids([m["tenant_id"] for m in tenants], tenant_id, kb_ids)
        denied_ids = [kb_id for kb_id in kb_ids if kb_id not in accessible_ids]
        if denied_ids:
            logging.warning("User '%s' lacks permission for datasets: '%s'", tenant_id, ", ".join(denied_ids))
        kb_ids = [kb_id for kb_id in kb_ids if kb_id in accessible_ids]
        if not kb_ids:
            return True, {"data": [], "total": 0}
    kbs, total = KnowledgebaseService.get_list(tenant_ids, query_user_id, page, page_size, orderby, desc, kb_id, name, keywords, parser_id, kb_ids)
    users = UserService.get_by_ids([m["tenant_id"] for m in kbs])
    user_map = {m.id: m.to_dict() for m in users}

    ips_arg = args.get("include_parsing_status", False)
    if isinstance(ips_arg, str):
        include_parsing_status = ips_arg.lower() not in ("false", "0", "")
    elif isinstance(ips_arg, bool):
        include_parsing_status = ips_arg
    else:
        include_parsing_status = bool(ips_arg)

    status_by_kb = {}
    if include_parsing_status and kbs:
        status_by_kb = DocumentService.get_parsing_status_by_kb_ids([kb["id"] for kb in kbs])

    response_data_list = []
    for kb in kbs:
        user_dict = user_map.get(kb["tenant_id"], {})
        kb.update({"nickname": user_dict.get("nickname", ""), "tenant_avatar": user_dict.get("avatar", "")})
        if status_by_kb:
            kb["parsing_status"] = status_by_kb.get(kb["id"], {})
        response_data_list.append(remap_dictionary_keys(kb))

    embed_model_names = get_composite_model_name_by_ids([m["embedding_model"] for m in response_data_list])
    for response_data in response_data_list:
        response_data["embedding_model_name"] = embed_model_names.get(response_data["embedding_model"], "")
    return True, {"data": response_data_list, "total": total}


def list_dataset_filters(tenant_id: str):
    tenants = TenantService.get_joined_tenants_by_user_id(tenant_id)
    tenant_ids = [m["tenant_id"] for m in tenants]
    owners = KnowledgebaseService.get_owner_filter(tenant_ids, tenant_id)
    return True, {"filter": {"owner": owners}, "total": sum(owner["count"] for owner in owners)}


async def get_knowledge_graph(dataset_id: str, tenant_id: str):
    """
    Get knowledge graph for a dataset.

    :param dataset_id: dataset ID
    :param tenant_id: tenant ID
    :return: (success, result) or (success, error_message)
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
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
        return False, "no authorization"
    _, kb = KnowledgebaseService.get_by_id(dataset_id)
    from rag.graphrag.phase_markers import clear_phase_markers
    from rag.nlp import search

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
        return False, "no authorization"

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
    # Disabled documents must not participate in a dataset-level structure
    # rebuild. Keep them out of the fan-out task itself; the merger also
    # applies a database-backed filter as a defense in depth.
    documents = [document for document in documents if str(document.get("status", "1")) != "0"]
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
        return False, "no authorization"

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
        return False, "no authorization"

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
        return False, "no authorization"

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    from rag.nlp import search

    for t in tags:
        updated = settings.docStoreConn.update({"tag_kwd": t, "kb_id": [dataset_id]}, {"remove": {"tag_kwd": t}}, search.index_name(kb.tenant_id), dataset_id)
        if callable(getattr(settings.docStoreConn, "db_type", None)) and settings.docStoreConn.db_type() == "gaussdb" and not updated:
            return False, "Failed to update dataset tags in document store"

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
        return False, "no authorization"

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
        return False, "no authorization"

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
        return False, "no authorization"

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
        from rag.graphrag.phase_markers import clear_phase_markers
        from rag.nlp import search

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
    elif wipe and index_type in _STRUCTURE_INDEX_TYPES:
        from rag.nlp import search

        # Wipe the merged KB-wide metadata and entity/relation rows for the
        # requested kind (all kinds for the merge-all "structure" type). The
        # per-document entity/relation rows the merge reads from are left intact.
        friendly = _STRUCTURE_INDEX_TYPE_TO_KIND.get(index_type)
        resolved_kind = _resolve_dataset_structure_kind(friendly) if friendly else None
        conditions = [
            {"knowledge_graph_kwd": ["dataset_graph"]},
            {"knowledge_graph_kwd": ["entity", "relation"], "scope_kwd": ["dataset"]},
        ]
        for condition in conditions:
            if resolved_kind:
                condition["compilation_template_kind_kwd"] = [resolved_kind]
            settings.docStoreConn.delete(condition, search.index_name(kb.tenant_id), dataset_id)

    KnowledgebaseService.update_by_id(kb.id, {task_id_field: "", task_finish_at_field: None})
    return True, {}


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
        return False, "no authorization"

    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    from rag.nlp import search

    updated = settings.docStoreConn.update(
        {"tag_kwd": from_tag, "kb_id": [dataset_id]}, {"remove": {"tag_kwd": from_tag.strip()}, "add": {"tag_kwd": to_tag}}, search.index_name(kb.tenant_id), dataset_id
    )
    if callable(getattr(settings.docStoreConn, "db_type", None)) and settings.docStoreConn.db_type() == "gaussdb" and not updated:
        return False, "Failed to update dataset tags in document store"

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
    size = int(req.get("page_size") or req.get("size", 30))
    rerank_candidates_count = int(req.get("rerank_candidates_count", 64))
    question = req.get("question", "")
    doc_ids = req.get("doc_ids", [])
    use_kg = req.get("use_kg", False)
    similarity_threshold = float(req.get("similarity_threshold", 0.0))
    vector_similarity_weight = float(req.get("vector_similarity_weight", 0.3))
    knn_top_k = max(1, min(int(req.get("knn_top_k", 1024)), 2048))
    knn_num_candidates = int(req.get("knn_num_candidates", 2048))
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
        knn_top_k = max(1, min(int(search_config.get("top_k", knn_top_k)), 2048))
        rerank_candidates_count = int(search_config.get("rerank_candidates_count", 100))
        use_kg = search_config.get("use_kg", use_kg)
        langs = search_config.get("cross_languages", langs)
        logging.debug(
            "Dataset search loaded Search config: search_id=%s dataset_id=%s vector_similarity_weight=%s full_text_weight=%s similarity_threshold=%s knn_top_k=%s",
            search_id,
            dataset_id,
            vector_similarity_weight,
            1 - vector_similarity_weight,
            similarity_threshold,
            knn_top_k,
        )
        if meta_data_filter.get("method") in ["auto", "semi_auto"]:
            chat_id = search_config.get("chat_id", "")
            if chat_id:
                chat_model_config = resolve_model_config(tenant_id, LLMType.CHAT, search_config["chat_id"])
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
        embd_model_config = resolve_model_config(kb.tenant_id, LLMType.EMBEDDING, kb.embd_id)
    else:
        embd_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.EMBEDDING)
    embd_mdl = LLMBundle(kb.tenant_id, embd_model_config)

    rerank_mdl = None
    rerank_id = req.get("rerank_id") or search_config.get("rerank_id")
    if rerank_id:
        rerank_model_config = resolve_model_config(kb.tenant_id, LLMType.RERANK.value, rerank_id)
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
        knn_top_k=knn_top_k,
        knn_num_candidates=knn_num_candidates,
        rerank_mdl=rerank_mdl,
        rank_feature=labels,
        trace_id=search_id,
        rerank_candidates_count=rerank_candidates_count,
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

    from api.db.services.llm_service import LLMBundle
    from common.constants import LLMType, RetCode
    from common.doc_store.doc_store_base import OrderByExpr
    from rag.nlp import search

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
                    "sample_random_chunks_with_vectors: index %s not yet created for tenant %s; returning empty sample set",
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
            vec_valid = full_doc.get(f"{vec_field}_valid") if vec_field else None
            if callable(getattr(docStoreConn, "db_type", None)) and docStoreConn.db_type() == "gaussdb" and vec_valid is False:
                vec = []
            else:
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
        return False, "no authorization"

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

    embd_model_config = resolve_model_config(kb.tenant_id, LLMType.EMBEDDING, embd_id)
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
    from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type
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
    size = int(req.get("page_size") or req.get("size", 30))
    rerank_candidates_count = int(req.get("rerank_candidates_count", 64))
    question = req.get("question", "")
    doc_ids = req.get("doc_ids", [])
    use_kg = req.get("use_kg", False)
    similarity_threshold = float(req.get("similarity_threshold", 0.0))
    vector_similarity_weight = float(req.get("vector_similarity_weight", 0.3))
    knn_top_k = max(1, min(int(req.get("knn_top_k", 1024)), 2048))
    knn_num_candidates = int(req.get("knn_num_candidates", 2048))
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

    err = validate_dataset_embedding_models(kbs)
    if err:
        return False, err

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
        knn_top_k = max(1, min(int(search_config.get("top_k", knn_top_k)), 2048))
        rerank_candidates_count = int(search_config.get("rerank_candidates_count", 100))
        use_kg = search_config.get("use_kg", use_kg)
        langs = search_config.get("cross_languages", langs)
        logging.debug(
            "Dataset search loaded Search config: search_id=%s dataset_ids=%s vector_similarity_weight=%s full_text_weight=%s similarity_threshold=%s knn_top_k=%s",
            search_id,
            kb_ids,
            vector_similarity_weight,
            1 - vector_similarity_weight,
            similarity_threshold,
            knn_top_k,
        )
        if meta_data_filter.get("method") in ["auto", "semi_auto"]:
            chat_id = search_config.get("chat_id", "")
            if chat_id:
                chat_model_config = resolve_model_config(tenant_id, LLMType.CHAT, search_config["chat_id"])
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

    embd_mdl = None
    if kb.embd_id:
        embd_model_config = resolve_model_config(kb.tenant_id, LLMType.EMBEDDING, kb.embd_id)
    else:
        embd_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.EMBEDDING)
    embd_mdl = LLMBundle(kb.tenant_id, embd_model_config)

    rerank_mdl = None
    rerank_id = req.get("rerank_id") or search_config.get("rerank_id")
    if rerank_id:
        rerank_model_config = resolve_model_config(kb.tenant_id, LLMType.RERANK.value, rerank_id)
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
        knn_top_k=knn_top_k,
        knn_num_candidates=knn_num_candidates,
        rerank_mdl=rerank_mdl,
        rank_feature=labels,
        trace_id=search_id,
        must_not=None if req.get("include_knowledge_compilation", True) else {"exists": "compile_kwd"},
        rerank_candidates_count=rerank_candidates_count,
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

    for c in ranks["chunks"]:
        c.pop("vector", None)
    ranks["labels"] = labels

    return True, ranks


# ---------------------------------------------------------------------------
# Artifact (knowledge compilation) page surface
#
# These three helpers power the dataset-level "Artifact" tab. They query rows
# with ``compile_kwd="wiki_page"`` written by TaskHandler's
# ``persist_wiki_pages``. The schema fields they rely on are:
#   slug_kwd, title_kwd, page_type_kwd, content_with_weight,
#   topic_kwd, entity_names_kwd, outlinks_kwd, related_kb_pages_kwd,
#   source_chunk_ids, source_doc_ids
# ---------------------------------------------------------------------------

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


def _compilation_template_kind(kind) -> str:
    if not isinstance(kind, str):
        return ""
    normalized = kind.strip().lower().replace("-", "_")
    if normalized in {"pageindex", "page_index", "knowledge_graph"}:
        return "timeline"
    return normalized


def _scalar(raw, default=""):
    """Infinity ``get_fields`` returns every ``*_kwd`` field as a list (split
    on ``###``), even single scalar values like ``slug_kwd=["entity/foo"]``.
    Normalize a field value that is expected to be a scalar identifier back to
    the first non-empty element."""
    if isinstance(raw, (list, tuple)):
        for item in raw:
            if item not in (None, ""):
                return item
        return default
    return raw if raw not in (None, "") else default


def _string_list(raw) -> list[str]:
    """Normalize native arrays and legacy JSON/Infinity string fields."""
    if isinstance(raw, (list, tuple, set)):
        values = raw
    elif isinstance(raw, str):
        value = raw.strip()
        if not value:
            return []
        try:
            decoded = json.loads(value)
        except (json.JSONDecodeError, TypeError):
            decoded = None
        if isinstance(decoded, list):
            values = decoded
        else:
            values = value.split("###")
    else:
        return []

    result: list[str] = []
    seen: set[str] = set()
    for item in values:
        if not isinstance(item, str):
            continue
        item = item.strip()
        if item and item not in seen:
            seen.add(item)
            result.append(item)
    return result


def _normalize_compilation_template_group_ids(raw) -> list[str]:
    if isinstance(raw, str):
        raw = [raw]
    if not isinstance(raw, list):
        return []
    ids: list[str] = []
    seen: set[str] = set()
    for group_id in raw:
        if not isinstance(group_id, str):
            continue
        group_id = group_id.strip()
        if group_id and group_id not in seen:
            seen.add(group_id)
            ids.append(group_id)
    return ids


def _extract_pipeline_compiler_group_ids(dsl) -> list[str]:
    if isinstance(dsl, str):
        try:
            dsl = json.loads(dsl)
        except Exception:
            return []
    if not isinstance(dsl, dict):
        return []
    components = dsl.get("components")
    if not isinstance(components, dict):
        return []

    group_ids: list[str] = []
    seen: set[str] = set()
    for component in components.values():
        if not isinstance(component, dict):
            continue
        obj = component.get("obj") if isinstance(component.get("obj"), dict) else {}
        component_name = obj.get("component_name") or component.get("component_name") or component.get("name")
        if not isinstance(component_name, str) or component_name.lower() != "compiler":
            continue
        candidates = [
            obj.get("params") if isinstance(obj.get("params"), dict) else {},
            obj,
            component.get("params") if isinstance(component.get("params"), dict) else {},
            component,
        ]
        for candidate in candidates:
            for key in ("compilation_template_group_ids", "compilation_template_group_id"):
                for group_id in _normalize_compilation_template_group_ids(candidate.get(key)):
                    if group_id not in seen:
                        seen.add(group_id)
                        group_ids.append(group_id)
    return group_ids


def _normalize_template_kind(raw) -> str:
    """Lowercase a template's raw kind WITHOUT folding.

    Unlike :func:`_compilation_template_kind` (which folds ``page_index`` /
    ``knowledge_graph`` into ``timeline``), this keeps kinds distinct so the
    alteration API can tell ``graph`` / ``timeline`` / ``tree`` apart.
    """
    if not isinstance(raw, str):
        return ""
    return raw.strip().lower().replace("-", "_")


def _template_matches_kind(template: dict | None, accepted: set[str]) -> bool:
    if not isinstance(template, dict):
        return False
    config = template.get("config") if isinstance(template.get("config"), dict) else {}
    raw_kind = config.get("kind") or template.get("kind") or ""
    return _normalize_template_kind(raw_kind) in accepted


def _group_has_template_of_kind(group_id: str, tenant_id: str, accepted: set[str], group_cache: dict[str, bool]) -> bool:
    if group_id in group_cache:
        return group_cache[group_id]
    from api.db.services.compilation_template_group_service import CompilationTemplateGroupService

    group = CompilationTemplateGroupService.get_saved(group_id, tenant_id)
    matched = any(_template_matches_kind(template, accepted) for template in (group or {}).get("templates") or [])
    group_cache[group_id] = matched
    return matched


def _parser_config_has_template_of_kind(parser_config, tenant_id: str, accepted: set[str], template_cache: dict[str, bool]) -> bool:
    from api.db.services.compilation_template_service import CompilationTemplateService
    from rag.svr.task_executor_refactor.chunk_post_processor import _parser_config_compilation_template_ids

    for template_id in _parser_config_compilation_template_ids(parser_config, tenant_id):
        if template_id not in template_cache:
            template_cache[template_id] = _template_matches_kind(CompilationTemplateService.get_saved(template_id, tenant_id), accepted)
        if template_cache[template_id]:
            return True
    return False


def _pipeline_has_compiler_of_kind(
    pipeline_id: str,
    tenant_id: str,
    accepted: set[str],
    pipeline_cache: dict[str, bool],
    group_cache: dict[str, bool],
) -> bool:
    pipeline_id = (pipeline_id or "").strip()
    if not pipeline_id:
        return False
    if pipeline_id in pipeline_cache:
        return pipeline_cache[pipeline_id]

    from api.db.services.canvas_service import UserCanvasService

    ok, canvas = UserCanvasService.get_by_id(pipeline_id)
    if not ok or not canvas:
        pipeline_cache[pipeline_id] = False
        return False

    group_ids = _extract_pipeline_compiler_group_ids(getattr(canvas, "dsl", None))
    matched = any(_group_has_template_of_kind(group_id, tenant_id, accepted, group_cache) for group_id in group_ids)
    pipeline_cache[pipeline_id] = matched
    return matched


def _skill_index_or_none(tenant_id: str, kb_id: str):
    return _compiled_index_or_none(tenant_id, kb_id)


async def has_any_wiki(dataset_id: str, tenant_id: str):
    """Fast existence probe for the sidebar tab visibility check.

    Returns ``(True, {"has": bool})`` on success or ``(False, str)`` on
    auth failure. Runs a ``limit=1`` search and reads only the total.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
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
            condition={"compile_kwd": [WIKI_PAGE_COMPILE_KWD]},
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


# Dataset-scope structure kinds the artifacts_structure API serves. Keys are the
# friendly names the frontend passes; values are the template's *top-level* kind
# as stamped on ``dataset_graph`` rows. The canonical names map to themselves so
# a caller can pass either form. Note this deliberately does NOT reuse
# ``_compilation_template_kind`` — that helper folds ``knowledge_graph`` into
# ``timeline`` and would merge distinct kinds here.
_DATASET_STRUCTURE_ROW_KWD = "dataset_graph"
_DATASET_STRUCTURE_KIND_ALIASES = {
    "graph": "knowledge_graph",
    "knowledge_graph": "knowledge_graph",
    "mindmap": "mind_map",
    "mind_map": "mind_map",
    "timeline": "timeline",
    "session_essence": "session_essence",
    "session_graph": "session_graph",
}
_DATASET_STRUCTURE_KIND_TO_INDEX_TYPE = {
    "knowledge_graph": "structure_graph",
    "mind_map": "structure_mindmap",
    "timeline": "timeline",
    "session_essence": "session_essence",
    "session_graph": "session_graph",
}


def _resolve_dataset_structure_kind(kind) -> str | None:
    """Map a friendly/canonical kind string to the stored top-level kind."""
    if not isinstance(kind, str):
        return None
    return _DATASET_STRUCTURE_KIND_ALIASES.get(kind.strip().lower().replace("-", "_"))


def delete_dataset_structure(dataset_id: str, tenant_id: str, kind: str, wipe: bool = True):
    """Delete the merged KB-wide structure rows for one artifacts_structure kind."""
    resolved_kind = _resolve_dataset_structure_kind(kind)
    if not resolved_kind:
        return False, f"Unsupported structure kind: {kind!r}. Expected one of: graph, mindmap, timeline, session_essence, session_graph."
    index_type = _DATASET_STRUCTURE_KIND_TO_INDEX_TYPE[resolved_kind]
    return delete_index(dataset_id, tenant_id, index_type, wipe=wipe)


async def get_dataset_structure(dataset_id: str, tenant_id: str, kind: str, keywords: str = ""):
    """Load the dataset-scope (KB-wide) structure graph for one ``kind``.

    ``kind`` is one of ``graph`` / ``mindmap`` / ``timeline`` /
    ``session_essence`` / ``session_graph``. The ``knowledge_graph_kwd="dataset_graph"``
    blob rows (one per template, written by ``rebuild_dataset_structure_graph_json``)
    are used only to DISCOVER the requested kind's template buckets; each bucket's
    entities/relations are then fetched from the raw KB-wide ``entity`` / ``relation``
    rows with subgraph sampling — identical to the per-document endpoint but scoped
    KB-wide (no ``doc_id`` filter), since dataset-merge templates dedup those rows
    across documents.

    ``keywords`` (optional): return the matching entities' subgraph, including
    neighbors and touching relations, across the kind.

    Returns ``(True, {"kind": <kind>, "templates": [...]})`` or
    ``(False, message)`` on auth/validation failure.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"

    resolved_kind = _resolve_dataset_structure_kind(kind)
    if not resolved_kind:
        return False, f"Unsupported structure kind: {kind!r}. Expected one of: graph, mindmap, timeline, session_essence, session_graph."

    _, kb = KnowledgebaseService.get_by_id(dataset_id)
    empty = {"kind": kind, "templates": []}

    pack = _compiled_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, empty
    index_nm, _ = pack

    from api.apps.services import structure_graph_common as sgc
    from api.db.services.compilation_template_service import CompilationTemplateService
    from api.db.services.tenant_llm_service import TenantLLMService
    from common.doc_store.doc_store_base import OrderByExpr

    keywords = (keywords or "").strip()
    _, active_doc_ids = await _current_dataset_docs(dataset_id)
    disabled_doc_ids = await _disabled_dataset_doc_ids(dataset_id)
    if not active_doc_ids:
        return True, empty
    # Dataset rows use the KB id as ``doc_id``. If a historical row has no
    # source provenance, exclude it while documents are disabled and fall back
    # to active document rows instead of returning potentially stale content.
    dataset_excluded_doc_ids = disabled_doc_ids | ({dataset_id} if disabled_doc_ids else set())

    def _row_template_id(row: dict) -> str | None:
        raw = row.get("compilation_template_ids")
        if isinstance(raw, list):
            for v in raw:
                if isinstance(v, str) and v.strip():
                    return v.strip()
        if isinstance(raw, str) and raw.strip():
            return raw.strip()
        return None

    # Resolve a template's top-level kind + display name, memoized. Used both to
    # label buckets and as a fallback for rows written before the kind stamp
    # (they carry ``compilation_template_ids`` but no ``compilation_template_kind_kwd``).
    template_kind_cache: dict[str, str | None] = {}
    template_name_cache: dict[str, str] = {}

    def _template_meta(tid: str | None) -> str | None:
        if not tid:
            return None
        if tid in template_kind_cache:
            return template_kind_cache[tid]
        top_kind = None
        try:
            saved = CompilationTemplateService.get_saved(tid, tenant_id)
            if saved:
                top_kind = (saved.get("kind") or "").strip() or None
                template_name_cache[tid] = saved.get("name") or tid
        except Exception:
            logging.exception("get_dataset_structure: template lookup failed for %s", tid)
        template_kind_cache[tid] = top_kind
        return top_kind

    def _bucket_meta_for(tid: str, row_kind: str = "") -> dict:
        if tid not in template_name_cache:
            _template_meta(tid)
        return {
            "template_id": tid,
            "template_name": template_name_cache.get(tid, tid),
            "kind": row_kind or template_kind_cache.get(tid) or resolved_kind,
        }

    # ── Discovery: the dataset-scoped rows the structure merge writes. ──
    # ``run_structure_merge`` (rag.svr.task_executor_refactor.dataset_structure_merger)
    # writes merged ``knowledge_graph_kwd="entity"/"relation"`` rows with
    # ``scope_kwd="dataset"``, stamped with ``compilation_template_ids`` and the
    # top-level ``compilation_template_kind_kwd``. Page through them (metadata
    # fields only) to enumerate the distinct template ids whose top-level kind
    # matches the request; ``build_bucket`` below then reads each template's rows.
    meta_fields = ["id", "compile_kwd", "compilation_template_ids", "compilation_template_kind_kwd"]
    page_size = 1000

    async def _discover_scope_templates(scope_kwd: str) -> tuple[list[str], bool, int]:
        scope_template_ids: list[str] = []
        seen_scope_tid: set[str] = set()
        scope_has_templateless = False
        offset = 0
        pages = 0
        while True:
            pages += 1
            try:
                res = await thread_pool_exec(
                    settings.docStoreConn.search,
                    meta_fields,
                    [],
                    {"knowledge_graph_kwd": ["entity", "relation"], "scope_kwd": [scope_kwd]},
                    [],
                    OrderByExpr(),
                    offset,
                    page_size,
                    index_nm,
                    [dataset_id],
                )
                meta_rows = settings.docStoreConn.get_fields(res, meta_fields) or {}
            except Exception:
                logging.exception("get_dataset_structure: docStore discovery failed for kb=%s scope=%s", dataset_id, scope_kwd)
                return [], False, pages
            if not meta_rows:
                break
            for row in meta_rows.values():
                tid = _row_template_id(row)
                stamped_kind = row.get("compilation_template_kind_kwd") or ""
                if isinstance(stamped_kind, list):
                    stamped_kind = stamped_kind[0].strip()
                row_kind = stamped_kind or _template_meta(tid) or ""
                if _resolve_dataset_structure_kind(row_kind) != resolved_kind:
                    continue
                if tid:
                    if tid not in seen_scope_tid:
                        seen_scope_tid.add(tid)
                        scope_template_ids.append(tid)
                else:
                    scope_has_templateless = True
            if len(meta_rows) < page_size:
                break
            offset += page_size
        return scope_template_ids, scope_has_templateless, pages

    dataset_template_ids, has_templateless, dataset_pages = await _discover_scope_templates("dataset")
    kind_template_ids: list[str] = list(dataset_template_ids)
    template_scope_by_id: dict[str, str] = {tid: "dataset" for tid in dataset_template_ids}

    doc_template_ids, _, doc_pages = await _discover_scope_templates("doc")
    for tid in doc_template_ids:
        if tid not in template_scope_by_id:
            template_scope_by_id[tid] = "doc"
            kind_template_ids.append(tid)

    # Detect datasets that have ONLY the legacy dataset_graph blob (no
    # entity/relation rows yet) so the fallback path below handles them.
    if not kind_template_ids and not has_templateless:
        try:
            legacy_check = await thread_pool_exec(
                settings.docStoreConn.search,
                ["id"],
                [],
                {"knowledge_graph_kwd": [_DATASET_STRUCTURE_ROW_KWD]},
                [],
                OrderByExpr(),
                0,
                1,
                index_nm,
                [dataset_id],
            )
            legacy_fm = settings.docStoreConn.get_fields(legacy_check, ["id"]) or {}
            if legacy_fm:
                has_templateless = True
        except Exception:
            pass

    logging.debug(
        "get_dataset_structure: discovered %d dataset template(s) and %d doc template(s) in %d/%d page(s) for kb=%s kind=%s",
        len(dataset_template_ids),
        len(doc_template_ids),
        dataset_pages,
        doc_pages,
        dataset_id,
        resolved_kind,
    )

    # ── keywords mode: name matching/KNN across the kind → matched subgraph. ──
    if keywords:
        if not kind_template_ids:
            return True, empty
        try:
            model_config = resolve_model_config(kb.tenant_id, LLMType.EMBEDDING.value, kb.embd_id)
            embd_mdl = TenantLLMService.model_instance(model_config)
        except Exception:
            logging.exception("get_dataset_structure: embedding bind failed for kb=%s", dataset_id)
            return True, empty

        def _scope_for_template(row: dict):
            tid = _row_template_id(row) or ""
            stamped = row.get("compilation_template_kind_kwd") or ""
            if isinstance(stamped, list):
                stamped = stamped[0].strip()
            scope_kwd = template_scope_by_id.get(tid, "dataset") if tid else "dataset"
            meta = _bucket_meta_for(tid, stamped) if tid else {"template_id": f"kind:{resolved_kind}", "template_name": f"kind:{resolved_kind}", "kind": resolved_kind}
            if tid:
                return meta, {"compilation_template_ids": [tid], "scope_kwd": [scope_kwd]}
            return meta, {"compilation_template_kind_kwd": [stamped], "scope_kwd": [scope_kwd]}

        bucket_meta, kw_entities, kw_relations = await sgc.keyword_subgraph(
            index_nm,
            dataset_id,
            embd_mdl,
            {"compilation_template_ids": kind_template_ids, "knowledge_graph_kwd": ["entity"], "scope_kwd": ["dataset", "doc"]},
            keywords,
            _scope_for_template,
            log_ctx=f"kb={dataset_id}",
            excluded_doc_ids=dataset_excluded_doc_ids,
        )
        if resolved_kind in {"knowledge_graph", "mind_map", "timeline"}:
            kw_entities = sgc.filter_entities_with_relations(kw_entities, kw_relations)
            if not kw_entities:
                return True, empty
        if not bucket_meta or (not kw_entities and not kw_relations):
            return True, empty
        bucket = dict(bucket_meta)
        bucket["entities"] = kw_entities
        bucket["relations"] = kw_relations
        return True, {"kind": kind, "templates": [bucket]}

    # ── normal mode: per-template subgraph sampling from raw KB-wide rows. ──
    templates_out: list[dict] = []
    for tid in kind_template_ids:
        scope_kwd = template_scope_by_id.get(tid, "dataset")
        try:
            scope = {"compilation_template_ids": [tid], "scope_kwd": [scope_kwd]}
            if scope_kwd == "doc":
                scope["doc_id"] = sorted(active_doc_ids)
            entities, relations = await sgc.build_bucket(
                index_nm,
                dataset_id,
                scope,
                excluded_doc_ids=dataset_excluded_doc_ids if scope_kwd == "dataset" else disabled_doc_ids,
            )
        except Exception:
            logging.exception("get_dataset_structure: bucket build failed for kb=%s template=%s", dataset_id, tid)
            continue
        if resolved_kind in {"knowledge_graph", "mind_map", "timeline"}:
            entities = sgc.filter_entities_with_relations(entities, relations)
            if not entities:
                continue
        if not entities and not relations:
            continue
        meta = _bucket_meta_for(tid)
        templates_out.append({**meta, "entities": entities, "relations": relations})

    # Legacy template-less dataset_graph rows have no template id to scope raw
    # rows by, so fall back to their blob content directly (no sampling). Rare —
    # rebuild_dataset_structure_graph_json always stamps a template id.
    if has_templateless:
        try:
            res_l = await thread_pool_exec(
                settings.docStoreConn.search,
                ["content_with_weight", "compilation_template_kind_kwd", "compile_kwd"],
                [],
                {"knowledge_graph_kwd": [_DATASET_STRUCTURE_ROW_KWD], "must_not": {"exists": "compilation_template_ids"}},
                [],
                OrderByExpr(),
                0,
                1000,
                index_nm,
                [dataset_id],
            )
            legacy_rows = settings.docStoreConn.get_fields(res_l, ["content_with_weight", "compilation_template_kind_kwd", "compile_kwd"]) or {}
        except Exception:
            logging.exception("get_dataset_structure: legacy blob fetch failed for kb=%s", dataset_id)
            legacy_rows = {}
        legacy_bucket = {"template_id": f"kind:{resolved_kind}", "template_name": f"kind:{resolved_kind}", "kind": resolved_kind, "entities": [], "relations": []}
        reconstructed_compile_kwds: set[str] = set()
        for row in legacy_rows.values():
            stamped_kind = row.get("compilation_template_kind_kwd") or ""
            if isinstance(stamped_kind, list):
                stamped_kind = stamped_kind[0].strip()
            if _resolve_dataset_structure_kind((stamped_kind or "").strip()) != resolved_kind:
                continue
            if disabled_doc_ids:
                compile_kwd = _scalar(row.get("compile_kwd"))
                if not compile_kwd or compile_kwd in reconstructed_compile_kwds:
                    continue
                reconstructed_compile_kwds.add(compile_kwd)
                try:
                    entities, relations = await sgc.build_bucket(
                        index_nm,
                        dataset_id,
                        {"compile_kwd": [compile_kwd], "doc_id": sorted(active_doc_ids)},
                        excluded_doc_ids=disabled_doc_ids,
                    )
                except Exception:
                    continue
                legacy_bucket["entities"].extend(entities)
                legacy_bucket["relations"].extend(relations)
                continue
            try:
                graph = json.loads(row.get("content_with_weight") or "{}")
            except Exception:
                continue
            if not isinstance(graph, dict):
                continue
            legacy_bucket["entities"].extend(item for item in (graph.get("entities") or []) if isinstance(item, dict))
            legacy_bucket["relations"].extend(item for item in (graph.get("relations") or []) if isinstance(item, dict))
        if resolved_kind in {"knowledge_graph", "mind_map", "timeline"}:
            legacy_bucket["entities"] = sgc.filter_entities_with_relations(legacy_bucket["entities"], legacy_bucket["relations"])
        if legacy_bucket["entities"] or legacy_bucket["relations"]:
            if resolved_kind not in {"knowledge_graph", "mind_map", "timeline"} or legacy_bucket["entities"]:
                templates_out.append(legacy_bucket)

    return True, {"kind": kind, "templates": templates_out}


# ── artifacts/alteration: per-``kind`` provenance & eligibility mapping ──
#
# Alteration is a pure set operation shared by every kind:
#   removed        = involved (product provenance) − current (dataset docs)
#   newly_uploaded = eligible (should be compiled) − involved
# Only two inputs vary by kind: how ``involved`` provenance is gathered, and
# which template kinds make a doc ``eligible``.

# Non-folded template kinds that make a doc eligible for each API kind.
_ALTERATION_ELIGIBLE_TEMPLATE_KINDS = {
    "wiki": {"wiki"},
    "graph": {"knowledge_graph"},
    "mindmap": {"mind_map"},
    "timeline": {"timeline"},
    "tree": {"tree", "page_index"},
}

# Dataset-merged structure kinds carry provenance in ``source_doc_ids`` or
# ``doc_ids_kwd`` on ``scope_kwd="dataset"`` rows, discovered by their stamped
# top-level kind.
_ALTERATION_KIND_TO_MERGED_ROW_KIND = {
    "graph": "knowledge_graph",
    "mindmap": "mind_map",
    "timeline": "timeline",
}

# Per-document structure kinds keep one product row per document; provenance is
# the row's own ``doc_id`` (no dataset merge, no ``source_doc_ids``). ``tree`` and
# ``page_index`` write distinct ``compile_kwd`` values.
_ALTERATION_TREE_COMPILE_KWDS = ["tree", "page_index"]


async def _current_dataset_docs(dataset_id: str):
    """Return ``(docs, current_doc_id_set)`` for the dataset."""
    docs, _ = await thread_pool_exec(
        DocumentService.get_by_kb_id,
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
    docs = [doc for doc in docs if str(doc.get("status", "1")) != "0"]
    docs = docs or []
    current_doc_ids = {str(doc.get("id")) for doc in docs if doc.get("id")}
    return docs, current_doc_ids


async def _disabled_dataset_doc_ids(dataset_id: str) -> set[str]:
    return await thread_pool_exec(DocumentService.get_disabled_doc_ids_by_kb_id, dataset_id)


def _alteration_result(current_doc_ids: set, involved_doc_ids: set, eligible_doc_ids: set) -> dict:
    """Build the drift response dict shared by every ``kind``."""
    removed_doc_ids = sorted(involved_doc_ids - current_doc_ids)
    newly_uploaded_doc_ids = sorted(eligible_doc_ids - involved_doc_ids)
    return {
        "removed": len(removed_doc_ids),
        "newly_uploaded": len(newly_uploaded_doc_ids),
        "removed_doc_ids": removed_doc_ids,
        "newly_uploaded_doc_ids": newly_uploaded_doc_ids,
        "involved_doc_ids": sorted(involved_doc_ids),
        "eligible_doc_ids": sorted(eligible_doc_ids),
    }


async def _wiki_chunk_alteration(
    tenant_id: str,
    dataset_id: str,
    eligible_doc_ids: set[str],
    involved_doc_ids: set[str],
) -> dict:
    """Compare active source chunks with the hashes used by Wiki MAP.

    Document-level provenance cannot detect an edited, added, or removed
    chunk.  The MAP resume rows already contain the exact chunk hash used by
    compilation, so they are the authoritative compiled-side snapshot.
    A previously involved document is changed when a chunk is added, removed,
    or has a different hash. Documents removed or disabled at document level
    remain represented by the existing ``removed`` fields rather than being
    duplicated in ``changed``.
    """
    empty = {
        "changed": 0,
        "changed_doc_ids": [],
    }
    if not eligible_doc_ids and not involved_doc_ids:
        return empty

    from rag.advanced_rag.knowlege_compile.wiki import (
        _wiki_compare_chunk_states,
        _wiki_load_active_map_state,
        _wiki_scan_current_chunk_state,
    )

    try:
        current = await _wiki_scan_current_chunk_state(tenant_id, dataset_id, eligible_doc_ids)
        previous = await _wiki_load_active_map_state(tenant_id, dataset_id)
    except Exception as exc:
        logging.exception("alteration: failed to compare Wiki chunk state for kb=%s", dataset_id)
        raise RuntimeError(f"Failed to compare Wiki chunk state for alteration (kb={dataset_id})") from exc

    delta = _wiki_compare_chunk_states(previous, current)
    changed_doc_ids = {
        str((current.get(chunk_id) or previous.get(chunk_id) or {}).get("doc_id") or "") for chunk_id in delta["new_chunk_ids"] | delta["changed_chunk_ids"] | delta["deleted_chunk_ids"]
    }
    # ``newly_uploaded`` owns eligible documents which have not contributed to
    # the current Wiki; ``removed`` owns previously involved documents which
    # are no longer eligible. Keep ``changed`` disjoint from both categories.
    changed_doc_ids &= eligible_doc_ids & involved_doc_ids

    return {
        "changed": len(changed_doc_ids),
        "changed_doc_ids": sorted(changed_doc_ids),
    }


def _flatten_provenance_doc_ids(value) -> set[str]:
    """Normalize source_doc_ids stored as JSON strings, lists, or scalars."""
    if value is None:
        return set()
    if isinstance(value, str):
        raw = value.strip()
        if not raw:
            return set()
        try:
            return _flatten_provenance_doc_ids(json.loads(raw))
        except (json.JSONDecodeError, TypeError):
            return {raw}
    if isinstance(value, (list, tuple, set)):
        result: set[str] = set()
        for item in value:
            result.update(_flatten_provenance_doc_ids(item))
        return result
    return {str(value)}


def _eligible_doc_ids_for_kind(docs, tenant_id: str, kind: str) -> set:
    """Doc ids whose parser_config or pipeline carries a template of ``kind``."""
    accepted = _ALTERATION_ELIGIBLE_TEMPLATE_KINDS.get(kind) or set()
    template_cache: dict[str, bool] = {}
    group_cache: dict[str, bool] = {}
    pipeline_cache: dict[str, bool] = {}
    eligible: set[str] = set()
    for doc in docs or []:
        if str(doc.get("status", "1")) == "0":
            continue
        doc_id = str(doc.get("id") or "")
        if not doc_id:
            continue
        parser_config = doc.get("parser_config") or {}
        if _parser_config_has_template_of_kind(parser_config, tenant_id, accepted, template_cache):
            eligible.add(doc_id)
            continue
        if _pipeline_has_compiler_of_kind(doc.get("pipeline_id") or "", tenant_id, accepted, pipeline_cache, group_cache):
            eligible.add(doc_id)
    return eligible


async def _involved_doc_ids_paged(index_nm, dataset_id: str, condition: dict, field: str | list[str], from_list: bool) -> set:
    """Page a docStore search, folding provenance fields into a doc-id set.

    ``from_list`` reads the selected fields as lists of provenance document IDs;
    otherwise the selected field is the row's own scalar document ID.
    """
    from common.doc_store.doc_store_base import OrderByExpr

    involved: set[str] = set()
    fields = [field] if isinstance(field, str) else field
    select_fields = ["id", *fields]
    offset = 0
    page_size = 1000
    while True:
        try:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                select_fields=select_fields,
                highlight_fields=[],
                condition=condition,
                match_expressions=[],
                order_by=OrderByExpr(),
                offset=offset,
                limit=page_size,
                index_names=index_nm,
                knowledgebase_ids=[dataset_id],
            )
            rows = settings.docStoreConn.get_fields(res, select_fields) or {}
        except Exception:
            logging.exception("alteration: docStore search failed for kb=%s cond=%s", dataset_id, condition)
            rows = {}

        if not rows:
            break
        for row in rows.values():
            if from_list:
                for field_name in fields:
                    involved.update(_flatten_provenance_doc_ids(row.get(field_name)))
            else:
                involved.update(_flatten_provenance_doc_ids(row.get(fields[0])))

        offset += page_size
        total = settings.docStoreConn.get_total(res)
        if not total or offset >= int(total):
            break
    return involved


async def _involved_doc_ids_for_kind(index_nm, dataset_id: str, kind: str) -> set:
    """Gather the doc ids baked into the compiled product for ``kind``."""
    if kind == "wiki":
        return await _involved_doc_ids_paged(index_nm, dataset_id, {"compile_kwd": [WIKI_PAGE_COMPILE_KWD]}, "source_doc_ids", from_list=True)
    if kind in _ALTERATION_KIND_TO_MERGED_ROW_KIND:
        condition = {
            "knowledge_graph_kwd": ["entity", "relation"],
            "scope_kwd": ["dataset"],
            "compilation_template_kind_kwd": [_ALTERATION_KIND_TO_MERGED_ROW_KIND[kind]],
        }
        involved = await _involved_doc_ids_paged(index_nm, dataset_id, condition, ["source_doc_ids", "doc_ids_kwd"], from_list=True)
        if involved:
            return involved

        # Historical dataset rows may predate provenance columns. Recover the
        # source document set from their template/compile identity so alteration
        # still reports disabled documents instead of treating every active
        # document as newly uploaded.
        template_ids = await _involved_doc_ids_paged(index_nm, dataset_id, condition, "compilation_template_ids", from_list=True)
        compile_kwds = await _involved_doc_ids_paged(index_nm, dataset_id, condition, "compile_kwd", from_list=True)
        if not template_ids and not compile_kwds:
            legacy_condition = {
                "knowledge_graph_kwd": [_DATASET_STRUCTURE_ROW_KWD],
                "compilation_template_kind_kwd": [_ALTERATION_KIND_TO_MERGED_ROW_KIND[kind]],
            }
            template_ids = await _involved_doc_ids_paged(index_nm, dataset_id, legacy_condition, "compilation_template_ids", from_list=True)
            compile_kwds = await _involved_doc_ids_paged(index_nm, dataset_id, legacy_condition, "compile_kwd", from_list=True)
        source_condition: dict = {"knowledge_graph_kwd": ["entity", "relation"]}
        if template_ids:
            source_condition["compilation_template_ids"] = sorted(template_ids)
        elif compile_kwds:
            source_condition["compile_kwd"] = sorted(compile_kwds)
        else:
            return set()
        involved = await _involved_doc_ids_paged(index_nm, dataset_id, source_condition, "doc_id", from_list=False)
        involved.discard(dataset_id)
        return involved
    if kind == "tree":
        return await _involved_doc_ids_paged(index_nm, dataset_id, {"compile_kwd": list(_ALTERATION_TREE_COMPILE_KWDS)}, "doc_id", from_list=False)
    return set()


async def _get_alteration(dataset_id: str, tenant_id: str, kind: str):
    """Shared driver: doc-level drift between the ``kind`` product and the dataset."""
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
    ok, kb = KnowledgebaseService.get_by_id(dataset_id)
    if not ok:
        return False, "Invalid Dataset ID"

    docs, current_doc_ids = await _current_dataset_docs(dataset_id)
    eligible_doc_ids = _eligible_doc_ids_for_kind(docs, kb.tenant_id, kind)

    involved_doc_ids: set[str] = set()
    chunk_changes = None
    pack = _compiled_index_or_none(kb.tenant_id, dataset_id)
    if pack is not None:
        index_nm, _ = pack
        involved_doc_ids = await _involved_doc_ids_for_kind(index_nm, dataset_id, kind)
        if kind == "wiki":
            chunk_changes = await _wiki_chunk_alteration(
                kb.tenant_id,
                dataset_id,
                eligible_doc_ids,
                involved_doc_ids,
            )

    # Wiki membership follows compilation eligibility. Disabling a document or
    # removing its Wiki template is therefore a removal; enabling it again or
    # restoring the template after a rebuild makes it newly uploaded.
    alteration_current_doc_ids = eligible_doc_ids if kind == "wiki" else current_doc_ids
    result = _alteration_result(alteration_current_doc_ids, involved_doc_ids, eligible_doc_ids)
    if chunk_changes is not None:
        result.update(chunk_changes)
    return True, result


async def get_wiki_alteration(dataset_id: str, tenant_id: str):
    """Return doc-level drift between current dataset docs and compiled wiki provenance."""
    return await _get_alteration(dataset_id, tenant_id, "wiki")


async def get_structure_alteration(dataset_id: str, tenant_id: str, kind: str):
    """Return doc-level drift for a non-wiki structure ``kind``.

    ``kind`` is one of ``graph`` / ``mindmap`` / ``timeline`` (dataset-merged
    products, provenance from ``source_doc_ids``) or ``tree`` (per-document
    products covering ``tree`` + ``page_index``, provenance from ``doc_id``).
    """
    kind = (kind or "").strip().lower()
    if kind not in _ALTERATION_KIND_TO_MERGED_ROW_KIND and kind != "tree":
        return False, f"Unsupported structure kind: {kind!r}. Expected one of: graph, mindmap, timeline, tree."
    return await _get_alteration(dataset_id, tenant_id, kind)


async def list_wiki_pages(
    dataset_id: str,
    tenant_id: str,
    page: int = 1,
    page_size: int = 200,
    page_type: str | None = None,
    topic: str | None = None,
    keywords: str = "",
):
    """List artifact pages for the left-hand 2-column list.

    Returns ``(True, {"total", "items": [{slug, title, page_type}, ...]})``.
    Ordering: ``page_type`` ascending, then ``title`` ascending — keeps
    pages of the same type grouped together visually.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _wiki_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, {"total": 0, "items": []}
    index_nm, _ = pack

    from common.doc_store.doc_store_base import OrderByExpr

    page = max(1, int(page or 1))
    page_size = max(1, min(int(page_size or 200), 1000))
    offset = (page - 1) * page_size
    page_type = page_type.strip() if isinstance(page_type, str) else page_type
    topic = topic.strip() if isinstance(topic, str) else topic
    keywords = (keywords or "").strip().casefold()

    condition: dict = {"compile_kwd": [WIKI_PAGE_COMPILE_KWD]}
    if page_type:
        condition["page_type_kwd"] = [page_type]
    if topic:
        condition["topic_kwd"] = [topic]

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
        "topic_kwd",
        "outlinks_int",
        "summary_with_weight",
    ]

    def _to_item(row):
        slug = _scalar(row.get("slug_kwd"))
        if not slug:
            return None
        return {
            "slug": slug,
            "title": _scalar(row.get("title_kwd")) or slug,
            "page_type": _scalar(row.get("page_type_kwd")) or "concept",
            "topic": _scalar(row.get("topic_kwd")) or "",
            "summary": row.get("summary_with_weight") or "",
        }

    try:
        if not keywords:
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
            items = [item for row in (field_map or {}).values() if (item := _to_item(row)) is not None]
            total = settings.docStoreConn.get_total(res)
        else:
            # Wiki list search is intentionally metadata-based. These fields
            # are available on existing rows and behave consistently across
            # every document-store backend, unlike a full-text query over
            # backend-specific token fields.
            matched_items = []
            batch_size = 1000
            search_offset = 0
            while True:
                res = settings.docStoreConn.search(
                    select_fields=select_fields,
                    highlight_fields=[],
                    condition=condition,
                    match_expressions=[],
                    order_by=order_by,
                    offset=search_offset,
                    limit=batch_size,
                    index_names=index_nm,
                    knowledgebase_ids=[dataset_id],
                )
                field_map = settings.docStoreConn.get_fields(res, select_fields)
                rows = list((field_map or {}).values())
                if not rows:
                    break
                for row in rows:
                    item = _to_item(row)
                    if item is not None and any(keywords in str(item[field]).casefold() for field in ("title", "slug", "summary")):
                        matched_items.append(item)
                if len(rows) < batch_size:
                    break
                search_offset += batch_size
            total = len(matched_items)
            items = matched_items[offset : offset + page_size]
    except Exception:
        logging.exception("list_wiki_pages: docStore search failed for kb=%s", dataset_id)
        return True, {"total": 0, "items": []}

    return True, {"total": int(total or 0), "items": items}


async def list_wiki_topics(
    dataset_id: str,
    tenant_id: str,
    page: int = 1,
    page_size: int = 200,
    keywords: str = "",
):
    """List wiki topics for the dataset Artifact tab."""
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _wiki_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, {"total": 0, "items": []}
    index_nm, _ = pack

    from common.doc_store.doc_store_base import OrderByExpr

    page = max(1, int(page or 1))
    page_size = max(1, min(int(page_size or 200), 1000))
    offset = (page - 1) * page_size
    keywords = (keywords or "").strip().casefold()

    try:
        agg_res = settings.docStoreConn.search(
            select_fields=["id"],
            highlight_fields=[],
            condition={"compile_kwd": [WIKI_PAGE_COMPILE_KWD], "page_type_kwd": ["concept", "entity"]},
            match_expressions=[],
            order_by=OrderByExpr(),
            offset=0,
            limit=0,
            index_names=index_nm,
            knowledgebase_ids=[dataset_id],
            agg_fields=["topic_kwd"],
        )
        buckets = settings.docStoreConn.get_aggregation(agg_res, "topic_kwd")
    except Exception:
        logging.exception("list_wiki_topics: docStore aggregation failed for kb=%s", dataset_id)
        return True, {"total": 0, "items": []}

    counts = {t: int(c) for t, c in (buckets or []) if isinstance(t, str) and t and int(c or 0) > 0}
    if not counts:
        return True, {"total": 0, "items": []}

    # Rank topics by page count (descending), then title for a stable order.
    ranked = sorted(
        (
            {
                "topic": t,
                "title": t.rsplit("/", 1)[-1],
                "slug": t,
                "page_count": c,
            }
            for t, c in counts.items()
        ),
        key=lambda x: (-x["page_count"], x["title"].lower()),
    )

    if keywords:
        matching_topics = {item["topic"] for item in ranked if any(keywords in str(item[field]).casefold() for field in ("topic", "title", "slug"))}

        child_fields = ["topic_kwd", "title_kwd", "slug_kwd", "summary_with_weight"]
        batch_size = 1000
        child_offset = 0
        try:
            while True:
                child_res = settings.docStoreConn.search(
                    select_fields=child_fields,
                    highlight_fields=[],
                    condition={"compile_kwd": [WIKI_PAGE_COMPILE_KWD], "page_type_kwd": ["concept", "entity"]},
                    match_expressions=[],
                    order_by=OrderByExpr(),
                    offset=child_offset,
                    limit=batch_size,
                    index_names=index_nm,
                    knowledgebase_ids=[dataset_id],
                )
                child_map = settings.docStoreConn.get_fields(child_res, child_fields)
                child_rows = list((child_map or {}).values())
                if not child_rows:
                    break
                for row in child_rows:
                    topic_name = _scalar(row.get("topic_kwd"))
                    if topic_name and any(keywords in str(_scalar(row.get(field)) or "").casefold() for field in ("title_kwd", "slug_kwd", "summary_with_weight")):
                        matching_topics.add(topic_name)
                if len(child_rows) < batch_size:
                    break
                child_offset += batch_size
        except Exception:
            logging.exception("list_wiki_topics: child-page keyword lookup failed for kb=%s", dataset_id)

        ranked = [item for item in ranked if item["topic"] in matching_topics]

    total = len(ranked)
    items = ranked[offset : offset + page_size]
    return True, {"total": total, "items": items}


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
        return False, "no authorization"
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
        "topic_kwd",
        "md_with_weight",
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
                "compile_kwd": [WIKI_PAGE_COMPILE_KWD],
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
    # The incremental writer stores page body in md_with_weight; fall back to
    # content_with_weight for any rows written by the legacy path.
    content_md = row.get("md_with_weight") or row.get("content_with_weight") or ""
    summary = row.get("summary_with_weight") or ""
    return True, {
        "slug": _scalar(row.get("slug_kwd")) or full_slug,
        "title": _scalar(row.get("title_kwd")) or full_slug,
        "page_type": _scalar(row.get("page_type_kwd")) or page_type,
        "topic": _scalar(row.get("topic_kwd")) or "",
        "content_md_rendered": content_md,
        "summary": summary,
        "entity_names": _string_list(row.get("entity_names_kwd")),
        "outlinks": _string_list(row.get("outlinks_kwd")),
        "related_kb_pages": _string_list(row.get("related_kb_pages_kwd")),
        "source_chunk_ids": _string_list(row.get("source_chunk_ids")),
        "source_doc_ids": _string_list(row.get("source_doc_ids")),
    }


async def has_any_skill(dataset_id: str, tenant_id: str):
    """Fast existence probe for the dataset Skills sidebar entry."""
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
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
        return False, "no authorization"
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


async def delete_skills(dataset_id: str, tenant_id: str):
    """Delete every compiled skill row (``skill`` + ``skill_all``) for a dataset.

    Returns ``(True, {"deleted": <n>})`` on success. When the tenant index does
    not exist yet there is nothing to delete, so it succeeds with ``0``.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _skill_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, {"deleted": 0}
    index_nm, _ = pack

    try:
        deleted = settings.docStoreConn.delete(
            {"compile_kwd": [_SKILL_COMPILE_KWD, _SKILL_ALL_COMPILE_KWD]},
            index_nm,
            dataset_id,
        )
    except Exception:
        logging.exception("delete_skills: docStore delete failed for kb=%s", dataset_id)
        return False, "Failed to delete skills."

    # Clear the skill compilation markers so the dataset reflects "no skill"
    # (and a later re-compile isn't short-circuited by a stale task id).
    try:
        KnowledgebaseService.update_by_id(kb.id, {"skill_task_id": "", "skill_task_finish_at": None})
    except Exception:
        logging.exception("delete_skills: failed clearing skill task markers for kb=%s", dataset_id)

    return True, {"deleted": int(deleted or 0)}


def _parse_list_field(value):
    """Normalize a search-result ``_kwd`` field to a Python list.

    Infinity stores keyword fields as ``###``-joined strings;
    ES / OpenSearch / OceanBase keep native lists.
    """
    if isinstance(value, list):
        return [v for v in value if v]
    if isinstance(value, str) and value:
        return [v for v in value.split("###") if v]
    return []


def _walk_skill_tree(tree: list[dict], target_kwd: str, parent_kwd: str | None = None):
    """Depth-first search through the ``skill_all`` tree JSON.

    Returns ``(parent_kwd, found_node)`` or ``(None, None)`` when *target_kwd*
    is not present in the tree.
    """
    for node in tree:
        if node.get("skill_kwd") == target_kwd:
            return parent_kwd, node
        children = node.get("children_kwd", [])
        if children:
            result = _walk_skill_tree(children, target_kwd, node.get("skill_kwd"))
            p, n = result
            if n is not None:
                return p, n
    return None, None


def _collect_tree_descendants(node: dict) -> set[str]:
    """Return all ``skill_kwd`` values from *node*'s subtree (recursive)."""
    result: set[str] = set()
    for child in node.get("children_kwd", []):
        kwd = child.get("skill_kwd", "")
        if kwd:
            result.add(kwd)
        result.update(_collect_tree_descendants(child))
    return result


def _prune_skill_tree(tree: list[dict], deleted_kwds: set[str]) -> list[dict]:
    """Remove nodes whose ``skill_kwd`` is in *deleted_kwds* (and their subtrees)."""
    result = []
    for node in tree:
        if node.get("skill_kwd") in deleted_kwds:
            continue
        children = node.get("children_kwd", [])
        node["children_kwd"] = _prune_skill_tree(children, deleted_kwds)
        result.append(node)
    return result


async def delete_skill(dataset_id: str, tenant_id: str, skill_kwd: str):
    """Delete a compiled skill node and its descendants.

    1. Walk the ``skill_all`` tree to identify the target node, its parent,
       and all descendant ``skill_kwd`` values.
    2. Delete every matching ``compile_kwd="skill"`` row (self + subtree).
    3. If a parent exists, remove the deleted node from its ``children_kwd``.
    4. Prune the ``skill_all`` tree and rewrite the aggregate row.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _skill_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, {"deleted": 0}
    index_nm, _ = pack

    # ------------------------------------------------------------------
    # 1. Read current skill_all tree
    # ------------------------------------------------------------------
    from common.doc_store.doc_store_base import OrderByExpr

    _ALL_SELECT = ["id", "kb_id", "doc_id", "compile_kwd", "skill_with_weight", "available_int"]
    try:
        res = settings.docStoreConn.search(
            select_fields=_ALL_SELECT,
            highlight_fields=[],
            condition={"compile_kwd": [_SKILL_ALL_COMPILE_KWD]},
            match_expressions=[],
            order_by=OrderByExpr(),
            offset=0,
            limit=1,
            index_names=index_nm,
            knowledgebase_ids=[dataset_id],
        )
        field_map = settings.docStoreConn.get_fields(res, _ALL_SELECT)
    except Exception:
        logging.exception("delete_skill: read skill_all failed kb=%s skill=%s", dataset_id, skill_kwd)
        return False, "Failed to read skill tree."

    if not field_map:
        return True, {"deleted": 0}
    _, skill_all_row = next(iter(field_map.items()))

    raw_tree = skill_all_row.get("skill_with_weight", "[]")
    tree = json.loads(raw_tree) if isinstance(raw_tree, str) else raw_tree
    tree = tree if isinstance(tree, list) else [tree]

    # ------------------------------------------------------------------
    # 2. Locate target node and collect descendants
    # ------------------------------------------------------------------
    parent_kwd, found_node = _walk_skill_tree(tree, skill_kwd)
    if found_node is None:
        # Fallback: node not tracked in the tree; try direct delete anyway.
        try:
            deleted = settings.docStoreConn.delete(
                {"compile_kwd": [_SKILL_COMPILE_KWD], "skill_kwd": [skill_kwd]},
                index_nm,
                dataset_id,
            )
        except Exception:
            logging.exception("delete_skill: fallback delete failed kb=%s skill=%s", dataset_id, skill_kwd)
            return False, "Failed to delete skill."
        return True, {"deleted": int(deleted or 0)}

    descendant_kwds = _collect_tree_descendants(found_node)
    all_kwds = [skill_kwd] + sorted(descendant_kwds)

    # ------------------------------------------------------------------
    # 3. Delete compile_kwd="skill" rows (self + all descendants)
    # ------------------------------------------------------------------
    try:
        deleted = settings.docStoreConn.delete(
            {"compile_kwd": [_SKILL_COMPILE_KWD], "skill_kwd": all_kwds},
            index_nm,
            dataset_id,
        )
    except Exception:
        logging.exception("delete_skill: delete failed kb=%s skill=%s kwds=%s", dataset_id, skill_kwd, all_kwds)
        return False, "Failed to delete skill."

    # ------------------------------------------------------------------
    # 4. Update parent's children_kwd
    # ------------------------------------------------------------------
    if parent_kwd:
        _NODE_SELECT = [
            "id",
            "kb_id",
            "doc_id",
            "compile_kwd",
            "skill_kwd",
            "depth_int",
            "children_kwd",
            "source_doc_ids",
            "md_with_weight",
            "available_int",
            "parent_kwd",
        ]
        try:
            res = settings.docStoreConn.search(
                select_fields=_NODE_SELECT,
                highlight_fields=[],
                condition={"compile_kwd": [_SKILL_COMPILE_KWD], "skill_kwd": [parent_kwd]},
                match_expressions=[],
                order_by=OrderByExpr(),
                offset=0,
                limit=1,
                index_names=index_nm,
                knowledgebase_ids=[dataset_id],
            )
            fm = settings.docStoreConn.get_fields(res, _NODE_SELECT)
        except Exception:
            logging.exception("delete_skill: read parent failed kb=%s parent=%s", dataset_id, parent_kwd)
        else:
            if fm:
                _, parent_row = next(iter(fm.items()))
                children = [c for c in _parse_list_field(parent_row.get("children_kwd", [])) if c != skill_kwd]
                parent_row["children_kwd"] = children
                try:
                    settings.docStoreConn.insert([parent_row], index_nm, dataset_id)
                except Exception:
                    logging.exception("delete_skill: update parent children_kwd failed kb=%s parent=%s", dataset_id, parent_kwd)

    # ------------------------------------------------------------------
    # 5. Prune and rewrite skill_all tree
    # ------------------------------------------------------------------
    try:
        deleted_set = set(all_kwds)
        pruned_tree = _prune_skill_tree(tree, deleted_set)
        skill_all_row["skill_with_weight"] = json.dumps(pruned_tree, ensure_ascii=False, indent=2)
        settings.docStoreConn.insert([skill_all_row], index_nm, dataset_id)
    except Exception:
        logging.exception("delete_skill: rewrite skill_all failed kb=%s skill=%s", dataset_id, skill_kwd)
        return False, "Failed to update skill tree."

    return True, {"deleted": int(deleted or 0)}


async def get_skill_page(dataset_id: str, tenant_id: str, skill_kwd: str):
    """Fetch the full markdown body for a single skill node."""
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
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


# ---------------------------------------------------------------------------
# Dataset navigation tree (written by rag/advanced_rag/knowlege_compile/
# dataset_nav.py as nav_cluster / nav_doc rows). Loaded hierarchically: the
# first call returns the top clusters (parent = "root"); clicking a cluster
# returns its direct children (sub-clusters + document leaves). These rows are
# ``available_int=0`` so they're read via docStoreConn.search directly.
# ---------------------------------------------------------------------------

_NAV_COMPILE_KWD = "dataset_nav"
_NAV_ROOT_PARENT = "root"
_NAV_FIELDS = [
    "id",
    "name",
    "type_kwd",
    "content_with_weight",
    "doc_count_int",
    "doc_ids_kwd",
    "doc_id",
    "depth_int",
    "parent_kwd",
]


def _first_str(val) -> str:
    """Extract the first string from a value that may be str, list, tuple, or set."""
    if isinstance(val, (list, tuple, set)):
        return str(next(iter(val), "") or "")
    return str(val or "")


def _resolve_embd_mdl(kb):
    """Resolve the embedding model for a knowledge base, or None on failure."""
    from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type
    from api.db.services.llm_service import LLMBundle
    from common.constants import LLMType

    try:
        if kb.embd_id:
            embd_model_config = resolve_model_config(kb.tenant_id, LLMType.EMBEDDING, kb.embd_id)
        else:
            embd_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.EMBEDDING)
        if embd_model_config is None:
            return None
        return LLMBundle(kb.tenant_id, embd_model_config)
    except Exception:
        logging.exception("Failed to resolve embedding model for kb=%s", kb.id)
        return None


def _nav_item(row: dict) -> dict:
    """Shape one nav row into a UI node: name, description, doc count, type."""
    try:
        payload = json.loads(row.get("content_with_weight") or "{}")
    except Exception:
        payload = {}
    row_type = row.get("type_kwd")
    if isinstance(row_type, (list, tuple, set)):
        row_type = next(iter(row_type), None)
    is_cluster = (row_type or payload.get("type")) == "nav_cluster"
    return {
        "name": row.get("name") or "",
        "description": payload.get("description") or "",
        "keywords": list(payload.get("keywords") or []),
        "entities": list(payload.get("entities") or []),
        "graph_content": payload.get("graph_content") or "",
        # doc_id count under this node: the cluster`s tally, or 1 for a leaf.
        "doc_count": int(row.get("doc_count_int") or 0) if is_cluster else 1,
        "type": "cluster" if is_cluster else "doc",
        "doc_id": None if is_cluster else (row.get("doc_id") or row.get("name")),
        "has_children": is_cluster,
    }


async def _nav_search(dataset_id: str, tenant_id: str, condition: dict, page: int, page_size: int):
    """Run one nav-tree search and shape the hits into UI nodes."""
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _compiled_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, {"total": 0, "items": []}
    index_nm, _ = pack

    from common.doc_store.doc_store_base import OrderByExpr

    page = max(1, int(page or 1))
    page_size = max(1, min(int(page_size or 1000), 2000))
    offset = (page - 1) * page_size

    order_by = OrderByExpr()
    try:
        # Biggest clusters first; leaves (no doc_count_int) fall to the end.
        order_by.desc("doc_count_int")
    except Exception:
        order_by = OrderByExpr()

    try:
        res = settings.docStoreConn.search(
            select_fields=_NAV_FIELDS,
            highlight_fields=[],
            condition=condition,
            match_expressions=[],
            order_by=order_by,
            offset=offset,
            limit=page_size,
            index_names=index_nm,
            knowledgebase_ids=[dataset_id],
        )
        field_map = settings.docStoreConn.get_fields(res, _NAV_FIELDS)
    except Exception:
        logging.exception("dataset_nav: docStore search failed for kb=%s", dataset_id)
        return True, {"total": 0, "items": []}

    total = settings.docStoreConn.get_total(res)
    items = [_nav_item(row) for row in (field_map or {}).values()]
    return True, {"total": int(total or 0), "items": items}


async def list_nav_clusters(dataset_id: str, tenant_id: str, page: int = 1, page_size: int = 1000, q: str | None = None, top_k: int | None = None):
    """First level of the nav tree: the clusters with no parent.

    When ``q`` is provided, runs a tree-structured search (mode="navigation_tree")
    and returns enriched nav node items in the same ``_nav_item`` shape so the
    frontend tree search can reuse this endpoint.
    """
    if q and q.strip():
        success, result = await search_dataset_layers(dataset_id, tenant_id, q.strip(), "navigation_tree", top_k=top_k or 1000)
        if not success:
            return success, result
        result["items"] = await _enrich_nav_items(dataset_id, tenant_id, result.get("items", []))
        return True, {"total": result.get("total", 0), "items": result["items"]}
    condition = {
        "compile_kwd": [_NAV_COMPILE_KWD],
        "type_kwd": ["nav_cluster"],
        "parent_kwd": [_NAV_ROOT_PARENT],
    }
    return await _nav_search(dataset_id, tenant_id, condition, page, page_size)


async def list_nav_children(dataset_id: str, tenant_id: str, name: str, page: int = 1, page_size: int = 1000):
    """Direct children of the node ``name`` — sub-clusters and document leaves.

    One level at a time (lazy) so the tree loads hierarchically as the user
    expands each node.
    """
    if not isinstance(name, str) or not name.strip():
        return True, {"total": 0, "items": []}
    condition = {
        "compile_kwd": [_NAV_COMPILE_KWD],
        "parent_kwd": [name.strip()],
    }
    return await _nav_search(dataset_id, tenant_id, condition, page, page_size)


async def delete_nav(dataset_id: str, tenant_id: str):
    """Delete the entire dataset navigation tree for a dataset.

    Returns ``(True, {"deleted": <n>})``; succeeds with ``0`` when there is no
    index yet.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _compiled_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, {"deleted": 0}
    index_nm, _ = pack

    try:
        deleted = await thread_pool_exec(
            settings.docStoreConn.delete,
            {"compile_kwd": [_NAV_COMPILE_KWD]},
            index_nm,
            dataset_id,
        )
    except Exception:
        logging.exception("delete_nav: docStore delete failed for kb=%s", dataset_id)
        return False, "Failed to delete the navigation tree."

    return True, {"deleted": int(deleted or 0)}


async def delete_nav_node(dataset_id: str, tenant_id: str, name: str):
    """Delete one navigation node (identified by ``name``) and its whole subtree.

    Children reference their parent by ``name`` (``parent_kwd``), so removing a
    cluster without its descendants would leave them orphaned in the tree view.
    We therefore walk the subtree top-down and delete every node in it.
    """
    if not isinstance(name, str) or not name.strip():
        return True, {"deleted": 0}
    name = name.strip()

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _compiled_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, {"deleted": 0}
    index_nm, _ = pack

    from common.doc_store.doc_store_base import OrderByExpr
    from rag.advanced_rag.knowlege_compile.dataset_nav import (
        _LOCK_BLOCKING_TIMEOUT_S,
        _LOCK_TIMEOUT_S,
        _nav_lock_key,
        _remove_dataset_nav_doc_locked,
    )
    from rag.utils.redis_conn import RedisDistributedLock

    async def search_rows(condition):
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            ["id", "name", "type_kwd", "parent_kwd", "doc_id"],
            [],
            condition,
            [],
            OrderByExpr(),
            0,
            10000,
            index_nm,
            [dataset_id],
        )
        return list((settings.docStoreConn.get_fields(res, ["id", "name", "type_kwd", "parent_kwd", "doc_id"]) or {}).values())

    lock = RedisDistributedLock(
        _nav_lock_key(dataset_id),
        timeout=_LOCK_TIMEOUT_S,
        blocking_timeout=_LOCK_BLOCKING_TIMEOUT_S,
    )
    try:
        await lock.spin_acquire()
    except Exception:
        logging.exception("delete_nav_node: lock acquire failed for kb=%s", dataset_id)
        return False, "Failed to acquire the navigation tree lock."

    try:
        # ES requires the keyword subfield for exact node-name matching. Infinity
        # has a scalar varchar name field, while parent_kwd is exact-matchable.
        name_field = "name.keyword" if settings.DOC_ENGINE.lower() in {"elasticsearch", "opensearch"} else "name"
        target_condition = {"compile_kwd": [_NAV_COMPILE_KWD], name_field: [name]}
        rows = await search_rows(target_condition)
        if not rows and name_field != "name":
            # Keep this fallback for installations with an older/missing mapping.
            rows = [row for row in await search_rows({"compile_kwd": [_NAV_COMPILE_KWD]}) if row.get("name") == name]
        if not rows:
            return True, {"deleted": 0}

        # Collect the target and descendants by stable row IDs, not by names.
        # Names are display values and may collide across malformed/old data.
        rows_by_id = {row.get("id"): row for row in rows if row.get("id")}
        frontier = [row.get("name") for row in rows if row.get("name")]
        for _ in range(64):
            if not frontier:
                break
            children = await search_rows(
                {"compile_kwd": [_NAV_COMPILE_KWD], "parent_kwd": frontier},
            )
            frontier = []
            for child in children:
                row_id = child.get("id")
                child_name = child.get("name")
                if row_id and row_id not in rows_by_id:
                    rows_by_id[row_id] = child
                    if child_name:
                        frontier.append(child_name)

        deleted = 0
        # Reuse the existing document-removal path so direct parents are
        # updated and empty ancestor clusters are cleaned consistently.
        for row in rows_by_id.values():
            if row.get("type_kwd") == "nav_doc" and row.get("doc_id"):
                await _remove_dataset_nav_doc_locked(tenant_id, dataset_id, row["doc_id"])
                deleted += 1

        cluster_ids = [row_id for row_id, row in rows_by_id.items() if row.get("type_kwd") == "nav_cluster" and row_id]
        if cluster_ids:
            deleted_rows = await thread_pool_exec(
                settings.docStoreConn.delete,
                {"compile_kwd": [_NAV_COMPILE_KWD], "id": cluster_ids},
                index_nm,
                dataset_id,
            )
            deleted += int(deleted_rows or 0)

        return True, {"deleted": deleted}
    except Exception:
        logging.exception("delete_nav_node: deletion failed for kb=%s name=%s", dataset_id, name)
        return False, "Failed to delete the navigation node."
    finally:
        try:
            lock.release()
        except Exception:
            logging.exception("delete_nav_node: lock release failed for kb=%s", dataset_id)


async def generate_nav(
    dataset_id: str,
    tenant_id: str,
    documents: list[dict] | None = None,
):
    """Create the entire navigation tree.

    Deletes any existing navigation tree first, then rebuilds it from
    scratch. When ``documents`` is provided, only those doc→summary pairs
    are used; otherwise all documents in the dataset are auto-discovered
    and inserted into the tree.

    ``documents`` is a list of ``{"doc_id": str, "summary": str,
    "doc_title": str (optional), "source_type": str (optional)}``.

    Returns ``(True, {"deleted": <n>, "upserted": <n>})`` on success.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"

    _, kb = KnowledgebaseService.get_by_id(dataset_id)
    if kb is None:
        return False, "Dataset not found."

    # Resolve models.
    from api.db.joint_services.tenant_model_service import (
        get_tenant_default_model_by_type,
    )
    from api.db.services.llm_service import LLMBundle
    from common.constants import LLMType
    from rag.advanced_rag.knowlege_compile.dataset_nav import (
        build_nav_graph_text,
        upsert_dataset_nav_doc,
    )

    embd_mdl = _resolve_embd_mdl(kb)

    chat_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.CHAT)
    chat_mdl = LLMBundle(kb.tenant_id, chat_model_config)

    # Step 0: auto-discover documents when not explicitly provided.
    if not documents:
        try:
            # Query all documents in the dataset directly without
            # File / File2Document JOINs so that every document
            # participates in the navigation tree (including docs
            # created via API that have no file record).
            from api.db.db_utils import DB
            from api.db.services.doc_metadata_service import DocMetadataService

            with DB.connection_context():
                doc_rows = list(
                    DocumentService.model.select(
                        DocumentService.model.id,
                        DocumentService.model.name,
                    )
                    .where(
                        DocumentService.model.kb_id == dataset_id,
                    )
                    .dicts()
                )
            doc_ids = [str(row["id"]) for row in doc_rows]
            metadata_map = DocMetadataService.get_metadata_for_documents(doc_ids, dataset_id) if doc_ids else {}
            all_docs = []
            for row in doc_rows:
                did = str(row["id"])
                all_docs.append(
                    {
                        "id": did,
                        "name": (row.get("name") or ""),
                        "meta_fields": metadata_map.get(did, {}),
                    }
                )

            # Prefer RAPTOR-generated summaries stored in the knowledge
            # graph (compile_kwd="tree", knowledge_graph_kwd="graph")
            # so that rebuilt nav descriptions match the original
            # tree-compilation output.  Fall back to meta_fields.title
            # or filename when the graph is absent.
            raptor_summaries: dict[str, str] = {}
            pack = _compiled_index_or_none(kb.tenant_id, dataset_id)
            if pack is not None:
                try:
                    index_nm, _ = pack
                    from common.doc_store.doc_store_base import OrderByExpr

                    graph_res = settings.docStoreConn.search(
                        select_fields=["doc_id", "content_with_weight"],
                        highlight_fields=[],
                        condition={"compile_kwd": ["tree"], "knowledge_graph_kwd": ["graph"]},
                        match_expressions=[],
                        order_by=OrderByExpr(),
                        offset=0,
                        limit=10000,
                        index_names=index_nm,
                        knowledgebase_ids=[dataset_id],
                    )
                    graph_map = settings.docStoreConn.get_fields(graph_res, ["doc_id", "content_with_weight"])
                    for row in (graph_map or {}).values():
                        gid = str(row.get("doc_id") or "")
                        if not gid:
                            continue
                        try:
                            graph = json.loads(row.get("content_with_weight") or "{}")
                        except Exception:
                            continue
                        # Build both:
                        #   - root_summary: first line of root desc (short,
                        #     for the display title / description field)
                        #   - graph_text: structured text from ALL entities
                        #     and relations (for embedding, keyword extraction,
                        #     entity extraction, and stored as graph_content).
                        # Shared with the parse-time path (run_tree_templates)
                        # so both produce identical, complete nav_doc content.
                        root_summary, graph_text = build_nav_graph_text(graph)

                        if root_summary:
                            raptor_summaries[gid] = {
                                "title": root_summary,
                                "graph_text": graph_text or root_summary,
                            }
                except Exception:
                    logging.exception("generate_nav: failed to read RAPTOR graph summaries for kb=%s", dataset_id)

            documents = []
            for d in all_docs:
                doc_id = str(d.get("id", ""))
                if not doc_id:
                    continue
                if doc_id in raptor_summaries:
                    summary = raptor_summaries[doc_id]
                else:
                    meta = d.get("meta_fields") or {}
                    summary = (meta.get("title") or "").strip() or d.get("name", "")
                documents.append(
                    {
                        "doc_id": doc_id,
                        "summary": summary,
                    }
                )

        except Exception:
            logging.exception("generate_nav: failed to auto-discover docs for kb=%s", dataset_id)
            return False, "Failed to auto-discover documents."

    if not documents:
        return False, "No documents found in dataset."

    # Step 1: delete the entire existing navigation tree so we start clean.
    # ``deleted`` reports the number of *clusters* removed (not nav_doc leaves),
    # since that is the meaningful unit for the navigation tree.
    deleted = 0
    pack = _compiled_index_or_none(kb.tenant_id, dataset_id)
    if pack is not None:
        index_nm, _ = pack
        try:
            from common.doc_store.doc_store_base import OrderByExpr

            # Count existing clusters before wiping the tree.
            count_res = await thread_pool_exec(
                settings.docStoreConn.search,
                ["id"],
                [],
                {"compile_kwd": [_NAV_COMPILE_KWD], "type_kwd": ["nav_cluster"]},
                [],
                OrderByExpr(),
                0,
                10000,
                index_nm,
                [dataset_id],
            )
            deleted = len(settings.docStoreConn.get_fields(count_res, ["id"]) or {})

            await thread_pool_exec(
                settings.docStoreConn.delete,
                {"compile_kwd": [_NAV_COMPILE_KWD]},
                index_nm,
                dataset_id,
            )
        except Exception:
            logging.exception("generate_nav: failed to clear existing nav for kb=%s", dataset_id)
            return False, "Failed to clear existing navigation tree."

    # Step 2: rebuild the tree from the provided doc→summary pairs.
    upserted = 0
    for doc in documents or []:
        doc_id = (doc.get("doc_id") or "").strip()
        summary = doc.get("summary")
        # summary can be a plain string or a RAPTOR tree dict.
        if isinstance(summary, str):
            summary = summary.strip()
        if not doc_id or not summary:
            continue
        try:
            await upsert_dataset_nav_doc(
                tenant_id=tenant_id,
                kb_id=dataset_id,
                doc_id=doc_id,
                summary_or_tree=summary,
                embd_mdl=embd_mdl,
                chat_mdl=chat_mdl,
            )
            upserted += 1
        except Exception:
            logging.exception("generate_nav: failed for doc=%s kb=%s", doc_id, dataset_id)
            return True, {"deleted": deleted, "upserted": upserted, "failed_doc_id": doc_id}

    return True, {"deleted": deleted, "upserted": upserted}


# ---------------------------------------------------------------------------
# Unified Explore Search
# ---------------------------------------------------------------------------

_LAYERS_HANDLERS: dict[str, str] = {
    "nav_doc": "_search_layers_nav_docs",
    "nav_cluster": "_search_layers_nav_clusters",
    "navigation_tree": "_search_layers_navigation_tree",
    "chunk": "_search_layers_chunks",
    "all": "_search_layers_all",
}


async def search_dataset_layers(
    dataset_id: str,
    tenant_id: str,
    query: str,
    mode: str,
    *,
    top_k: int | None = None,
    doc_scope: list[str] | None = None,
) -> tuple[bool, dict]:
    """Unified search across different knowledge layers of a dataset.

    Args:
        mode: One of ``"chunk"``, ``"nav_doc"``, ``"nav_cluster"``, ``"navigation_tree"``, ``"all"``.
            - chunk: raw document chunks (via the main retrieval pipeline)
            - nav_doc: navigation tree document leaves
            - nav_cluster: navigation tree cluster nodes
            - navigation_tree: tree-structured BFS beam descent
            - all: union of all modes, deduplicated by doc_id with best score
        doc_scope: Optional set of documents to restrict the search to.  None or
            empty means all documents of the dataset.  Forwarded to every mode:
            nav modes filter the compiled nav rows by doc, ``chunk`` restricts
            the retriever via ``doc_ids``.  Applied query-time so scoped rows
            are never dropped by the ``top_k`` truncation.

        Items are shaped as ``{"doc_id": str, "score": float}``.
    """
    from rag.advanced_rag.knowlege_compile.dataset_nav import search_dataset_nav

    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, {"error": "no authorization", "code": RetCode.PERMISSION_ERROR}
    if mode not in _LAYERS_HANDLERS:
        return False, {"error": f"unknown mode: {mode}, expected one of {list(_LAYERS_HANDLERS.keys())}", "code": RetCode.ARGUMENT_ERROR}
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    try:
        from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type
        from api.db.services.llm_service import LLMBundle

        if kb.embd_id:
            embd_model_config = resolve_model_config(kb.tenant_id, LLMType.EMBEDDING, kb.embd_id)
        else:
            embd_model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.EMBEDDING)
        embd_mdl = LLMBundle(kb.tenant_id, embd_model_config)
    except Exception as e:
        logging.warning(
            "search_dataset_layers: failed to create LLMBundle(EMBEDDING) for tenant=%s: %s: %s",
            kb.tenant_id,
            type(e).__name__,
            e,
        )
        logging.exception("Full traceback for LLMBundle(EMBEDDING) failure")
        embd_mdl = None

    logging.debug(
        "search_dataset_layers: dispatching scoped mode=%s for dataset=%s, scoped_docs=%d",
        mode,
        dataset_id,
        len([d for d in (doc_scope or []) if str(d).strip()]),
    )
    if mode == "nav_doc":
        return await _search_layers_nav_docs(tenant_id, dataset_id, query, top_k, embd_mdl, search_dataset_nav, doc_scope=doc_scope)
    elif mode == "nav_cluster":
        return await _search_layers_nav_clusters(tenant_id, dataset_id, query, top_k, embd_mdl, search_dataset_nav, doc_scope=doc_scope)
    elif mode == "navigation_tree":
        return await _search_layers_navigation_tree(tenant_id, dataset_id, query, top_k, embd_mdl, doc_scope=doc_scope)
    elif mode == "chunk":
        return await _search_layers_chunks(tenant_id, dataset_id, query, top_k, embd_mdl, kb, doc_scope=doc_scope)
    elif mode == "all":
        return await _search_layers_all(tenant_id, dataset_id, query, top_k, embd_mdl, kb, search_dataset_nav, doc_scope=doc_scope)
    else:
        return False, {"error": f"unknown mode: {mode}", "code": RetCode.ARGUMENT_ERROR}


async def _search_layers_nav_docs(tenant_id, dataset_id, query, top_k, embd_mdl, search_fn, *, doc_scope=None):
    items = await _nav_search_result(
        tenant_id,
        dataset_id,
        query,
        top_k,
        embd_mdl,
        search_fn,
        type_kwd="nav_doc",
        doc_scope=doc_scope,
    )
    return True, {"mode": "nav_doc", "total": len(items), "items": items}


async def _search_layers_nav_clusters(tenant_id, dataset_id, query, top_k, embd_mdl, search_fn, *, doc_scope=None):
    items = await _nav_search_result(
        tenant_id,
        dataset_id,
        query,
        top_k,
        embd_mdl,
        search_fn,
        type_kwd="nav_cluster",
        doc_scope=doc_scope,
    )
    return True, {"mode": "nav_cluster", "total": len(items), "items": items}


async def _search_layers_navigation_tree(tenant_id, dataset_id, query, top_k, embd_mdl, *, doc_scope=None):
    from rag.advanced_rag.knowlege_compile.dataset_nav import search_nav_tree_descent

    items = await search_nav_tree_descent(
        tenant_id,
        dataset_id,
        query,
        embd_mdl,
        top_k=top_k,
        doc_scope=doc_scope,
    )
    return True, {"mode": "navigation_tree", "total": len(items), "items": items}


async def _nav_search_result(tenant_id, dataset_id, query, top_k, embd_mdl, search_fn, **kwargs):
    doc_scope = kwargs.pop("doc_scope", None)
    results = await search_fn(
        tenant_id,
        dataset_id,
        query,
        embd_mdl=embd_mdl,
        top_k=top_k,
        doc_scope=doc_scope,
        **kwargs,
    )
    scope_set = {str(d).strip() for d in doc_scope if str(d).strip()} if doc_scope else None
    items: list[dict] = []
    for r in results:
        raw_doc_id = r.get("doc_id")
        if isinstance(raw_doc_id, str) and raw_doc_id.strip():
            doc_id = raw_doc_id.strip()
        else:
            # Cluster row: pick the first doc_id that's in scope (if a scope
            # is set) so we never leak an out-of-scope doc_id into the results.
            doc_ids = r.get("doc_ids") or []
            if scope_set:
                doc_id = next((str(d).strip() for d in doc_ids if str(d).strip() in scope_set), "")
            else:
                doc_id = str(doc_ids[0]).strip() if doc_ids else ""
        if not doc_id:
            continue
        item = {
            "doc_id": doc_id,
            "score": round(float(r.get("score", 0.0)), 4),
        }
        # Preserve the full nav node info so callers can enrich without a
        # second ES round-trip.  Clusters have doc_id="" but carry name.
        if r.get("name") or r.get("description"):
            item["_nav"] = r
        items.append(item)
    return items


async def _search_layers_chunks(tenant_id, dataset_id, query, top_k, embd_mdl, kb, *, doc_scope=None):
    from common import settings

    tenant_ids = [tenant_id]

    kwargs = {}
    if top_k is not None:
        kwargs["knn_top_k"] = top_k
    if doc_scope:
        kwargs["doc_ids"] = [str(d) for d in doc_scope if str(d).strip()]

    fetch_k = max(top_k, 10) * 3 if top_k is not None else 1024
    try:
        ranks = await settings.retriever.retrieval(
            query,
            embd_mdl,
            tenant_ids,
            [dataset_id],
            1,
            fetch_k,
            0.0,
            0.3,
            **kwargs,
        )
    except Exception:
        return False, {"error": "chunk retrieval failed", "code": RetCode.SERVER_ERROR}

    doc_scores: dict[str, float] = {}
    for c in ranks.get("chunks", []):
        doc_id = (c.get("doc_id") or "").strip()
        score = float(c.get("similarity") or c.get("score") or 0.0)
        if doc_id and score > doc_scores.get(doc_id, -1.0):
            doc_scores[doc_id] = score

    items = sorted(
        ({"doc_id": d, "score": round(s, 4)} for d, s in doc_scores.items()),
        key=lambda x: x["score"],
        reverse=True,
    )
    if top_k is not None and top_k > 0:
        items = items[:top_k]

    return True, {"mode": "chunk", "total": len(items), "items": items}


async def _search_layers_all(tenant_id, dataset_id, query, top_k, embd_mdl, kb, search_fn, *, doc_scope=None):
    """Run all modes and return the union of doc_ids, with best score per doc."""
    import asyncio as _asyncio

    result_lists = await _asyncio.gather(
        _search_layers_nav_docs(tenant_id, dataset_id, query, top_k, embd_mdl, search_fn, doc_scope=doc_scope),
        _search_layers_nav_clusters(tenant_id, dataset_id, query, top_k, embd_mdl, search_fn, doc_scope=doc_scope),
        _search_layers_navigation_tree(tenant_id, dataset_id, query, top_k, embd_mdl, doc_scope=doc_scope),
        _search_layers_chunks(tenant_id, dataset_id, query, top_k, embd_mdl, kb, doc_scope=doc_scope),
        return_exceptions=True,
    )

    doc_scores: dict[str, dict] = {}
    for result in result_lists:
        if isinstance(result, Exception):
            continue
        ok, data = result
        if not ok:
            continue
        for item in data.get("items", []):
            doc_id = item.get("doc_id", "")
            score = float(item.get("score", 0.0))
            # Clusters have no doc_id; key by name so they survive dedup.
            key = doc_id or item.get("_nav", {}).get("name", "")
            if key and score > doc_scores.get(key, {}).get("score", -1.0):
                doc_scores[key] = item

    items = sorted(
        doc_scores.values(),
        key=lambda x: x["score"],
        reverse=True,
    )
    if top_k is not None and top_k > 0:
        items = items[:top_k]

    return True, {"mode": "all", "total": len(items), "items": items}


async def _enrich_nav_items(dataset_id: str, tenant_id: str, items: list[dict]) -> list[dict]:
    """Enrich ``{doc_id, score}`` items with full nav node info.

    Items that already carry a ``_nav`` payload (from nav_doc/nav_cluster/
    navigation_tree search) are shaped in-process.  Items without it (e.g.
    chunk hits) are batch-fetched from ES by ``doc_id``.

    For every matched doc, its parent cluster is also resolved and prepended
    to the result set (cluster score = max child score) so the frontend can
    highlight both the doc and its containing cluster in the tree.
    """
    from common.doc_store.doc_store_base import OrderByExpr

    if not items:
        return items

    _, kb = KnowledgebaseService.get_by_id(dataset_id)
    pack = _compiled_index_or_none(kb.tenant_id, dataset_id) if kb else None
    if pack is None:
        return items
    index_nm, _ = pack

    # Phase 1: shape items that already have _nav; collect doc_ids for chunk-only hits.
    doc_items: list[dict] = []
    missing_doc_ids: list[str] = []
    for it in items:
        nav = it.get("_nav")
        if nav:
            is_cluster = nav.get("type") == "nav_cluster"
            item = {
                "name": nav.get("name") or "",
                "description": nav.get("description") or "",
                "keywords": nav.get("keywords") or [],
                "entities": nav.get("entities") or [],
                "graph_content": nav.get("graph_content") or "",
                "doc_count": nav.get("doc_count") or (0 if is_cluster else 1),
                "type": "cluster" if is_cluster else "doc",
                "doc_id": nav.get("doc_id"),
                "has_children": is_cluster,
                "score": it.get("score", 0.0),
            }
            # Capture parent_kwd for docs so we can fetch the parent cluster.
            if not is_cluster:
                pn = nav.get("parent_kwd") or []
                if isinstance(pn, (list, tuple, set)):
                    pn = next(iter(pn), "")
                if pn:
                    item["_parent_kwd"] = str(pn)
            doc_items.append(item)
        else:
            doc_id = it.get("doc_id", "")
            if doc_id:
                missing_doc_ids.append(doc_id)
            doc_items.append(it)

    # Phase 2: batch-fetch nav_doc rows for chunk-only hits (need parent_kwd).
    all_doc_ids = missing_doc_ids + [d["doc_id"] for d in doc_items if d.get("type") == "doc" and d.get("doc_id") and not d.get("_parent_kwd")]
    nav_rows_by_doc_id: dict[str, dict] = {}
    if all_doc_ids:
        try:
            res = settings.docStoreConn.search(
                select_fields=_NAV_FIELDS,
                highlight_fields=[],
                condition={"compile_kwd": [_NAV_COMPILE_KWD], "doc_id": all_doc_ids},
                match_expressions=[],
                order_by=OrderByExpr(),
                offset=0,
                limit=len(all_doc_ids),
                index_names=index_nm,
                knowledgebase_ids=[dataset_id],
            )
            field_map = settings.docStoreConn.get_fields(res, _NAV_FIELDS)
        except Exception:
            logging.exception("_enrich_nav_items: docStore search failed for kb=%s", dataset_id)
            field_map = None

        for row in (field_map or {}).values():
            did = row.get("doc_id") or row.get("name") or ""
            if isinstance(did, (list, tuple, set)):
                did = next(iter(did), "")
            if did:
                nav_rows_by_doc_id[str(did)] = row

    # Enrich chunk-only items with nav info + parent_kwd.
    for i, it in enumerate(doc_items):
        if it.get("type"):
            continue
        doc_id = it.get("doc_id", "")
        row = nav_rows_by_doc_id.get(doc_id)
        if row:
            nav_item = _nav_item(row)
            nav_item["score"] = it.get("score", 0.0)
            nav_item["_parent_kwd"] = _first_str(row.get("parent_kwd"))
            doc_items[i] = nav_item

    # Phase 3: get parent_kwd for _nav doc items (search_dataset_nav strips it).
    nav_doc_names = [d["name"] for d in doc_items if d.get("type") == "doc" and not d.get("_parent_kwd") and d.get("name")]
    if nav_doc_names:
        name_field = "name.keyword" if settings.DOC_ENGINE.lower() in {"elasticsearch", "opensearch"} else "name"
        try:
            res = settings.docStoreConn.search(
                select_fields=["name", "parent_kwd"],
                highlight_fields=[],
                condition={"compile_kwd": [_NAV_COMPILE_KWD], "type_kwd": ["nav_doc"], name_field: nav_doc_names},
                match_expressions=[],
                order_by=OrderByExpr(),
                offset=0,
                limit=len(nav_doc_names),
                index_names=index_nm,
                knowledgebase_ids=[dataset_id],
            )
            field_map = settings.docStoreConn.get_fields(res, ["name", "parent_kwd"])
        except Exception:
            field_map = None

        parent_by_name: dict[str, str] = {}
        for row in (field_map or {}).values():
            nm = _first_str(row.get("name"))
            pn = _first_str(row.get("parent_kwd"))
            if nm and pn:
                parent_by_name[nm] = pn

        for it in doc_items:
            if it.get("type") == "doc" and not it.get("_parent_kwd"):
                it["_parent_kwd"] = parent_by_name.get(it.get("name", ""), "")

    # Phase 4: batch-fetch parent cluster rows.
    parent_names = {it["_parent_kwd"] for it in doc_items if it.get("_parent_kwd")}
    cluster_by_name: dict[str, dict] = {}
    if parent_names:
        name_field = "name.keyword" if settings.DOC_ENGINE.lower() in {"elasticsearch", "opensearch"} else "name"
        try:
            res = settings.docStoreConn.search(
                select_fields=_NAV_FIELDS,
                highlight_fields=[],
                condition={"compile_kwd": [_NAV_COMPILE_KWD], "type_kwd": ["nav_cluster"], name_field: list(parent_names)},
                match_expressions=[],
                order_by=OrderByExpr(),
                offset=0,
                limit=len(parent_names),
                index_names=index_nm,
                knowledgebase_ids=[dataset_id],
            )
            field_map = settings.docStoreConn.get_fields(res, _NAV_FIELDS)
        except Exception:
            field_map = None

        for row in (field_map or {}).values():
            nm = _first_str(row.get("name"))
            if nm:
                cluster_by_name[nm] = _nav_item(row)

    # Phase 5: build result — clusters (max child score) then docs.
    cluster_scores: dict[str, float] = {}
    doc_parent: dict[int, str] = {}
    for i, it in enumerate(doc_items):
        pn = it.pop("_parent_kwd", "")
        doc_parent[i] = pn
        if pn and pn in cluster_by_name:
            cluster_scores[pn] = max(cluster_scores.get(pn, 0.0), it.get("score", 0.0))

    clusters = [{**nav_item, "score": round(cluster_scores.get(name, 0.0), 4)} for name, nav_item in cluster_by_name.items()]
    cluster_names = {c["name"] for c in clusters}

    # A matched doc whose parent cluster is also in the result is already
    # represented by that cluster's tree — don't surface it as a second,
    # separate root.  This keeps a search that hits both a cluster and its
    # child doc down to a single nav_cluster tree instead of two roots.
    standalone_docs = [it for i, it in enumerate(doc_items) if it.get("type") != "cluster" and doc_parent.get(i) not in cluster_names]
    return clusters + standalone_docs


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
    ``wiki_page_graph`` / ``wiki_entity`` / ``wiki_relation``
    rows stay stale until the next full artifact compile.

    Side effect: when the rendered post-save markdown differs from the
    prior stored content, one ``wiki_commit`` row is recorded
    (git-style audit). No-op saves are silently skipped — empty diff,
    no row.

    Returns ``(True, page_dict)`` mirroring ``get_wiki_page``, or
    ``(True, None)`` when the row is missing, or
    ``(False, message)`` on authorization failure.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _wiki_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, None
    index_nm, _ = pack

    from api.db.services.file_commit_service import FileCommitService
    from rag.advanced_rag.knowlege_compile.wiki import (
        _wiki_extract_summary,
        _wiki_transform_links,
    )

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
            select_fields=["id", "md_with_weight", "content_with_weight"],
            highlight_fields=[],
            condition={
                "compile_kwd": [WIKI_PAGE_COMPILE_KWD],
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
            ["id", "md_with_weight", "content_with_weight"],
        )
        if field_map:
            row_id, row = next(iter(field_map.items()))
            content_before = row.get("md_with_weight") or row.get("content_with_weight") or ""
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
                "md_with_weight": rendered,
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

    refresh_idx = getattr(settings.docStoreConn, "refresh_idx", None)
    if callable(refresh_idx):
        try:
            await thread_pool_exec(refresh_idx, index_nm)
        except Exception:
            logging.exception(
                "update_wiki_page: index refresh failed for kb=%s slug=%s",
                dataset_id,
                full_slug,
            )

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


# All row types the artifact pipeline writes. Listed in dependency
# order so partial failures of earlier deletes don't leave behind state
# that downstream phases would silently reuse. ``wiki_page_graph``
# is the materialized canvas graph derived from the refined pages —
# the dataset Artifact tab's graph view reads exactly this row.
_WIKI_COMPILE_KWDS = (
    "wiki_map_extract",
    "wiki_map_state",
    "wiki_map_state_meta",
    "wiki_reduce_result",
    "wiki_compilation_plan",
    "wiki_page_draft",
    "wiki_page",
    "wiki_page_topic",
    "wiki_canonical_entity",
    "wiki_plan_group",
    "wiki_doc_page_source",
    "wiki_mode_meta",
    "wiki_entity",
    "wiki_relation",
    "wiki_page_graph",
)

# Tunables for the incremental graph loader. See ``get_wiki_graph``.
_WIKI_GRAPH_ENTITY_KWD = "wiki_entity"
_WIKI_GRAPH_RELATION_KWD = "wiki_relation"
_WIKI_GRAPH_ENTITY_PAGE_SIZE = 32
_WIKI_GRAPH_MAX_LOADING_ENTITY = 512


def _wiki_entity_payload(row: dict) -> dict | None:
    """Project one ``wiki_entity`` ES row onto the canvas entity shape.

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
    slug = payload.get("slug") or _scalar(row.get("slug_kwd"))
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
    keywords: str = "",
):
    """One page of wiki_entity rows.

    Without ``keywords``: ordered by ``weight_int DESC`` (heaviest nodes first).
    With ``keywords``: BM25 full-text match over ``content_ltks`` (slug +
    summary), ordered by relevance. ``wiki_entity`` rows are BM25-only (no
    embedding vector), so this is a lexical search, not a dense KNN.
    """
    from common.doc_store.doc_store_base import OrderByExpr

    select_fields = [
        "id",
        "slug_kwd",
        "weight_int",
        "source_chunk_ids",
        "content_with_weight",
    ]

    keywords = (keywords or "").strip()
    match_expressions: list = []
    order_by = OrderByExpr()
    if keywords:
        try:
            match_text, _ = settings.retriever.qryr.question(keywords, min_match=0.1)
            match_expressions = [match_text]
        except Exception:
            logging.exception("get_wiki_graph: failed to build keyword query for kb=%s", dataset_id)
            match_expressions = []
    if not match_expressions:
        # No keywords (or query build failed) → heaviest-weighted first. When a
        # text match is present the store ranks by BM25 score instead.
        try:
            order_by.desc("weight_int")
        except Exception:
            order_by = OrderByExpr()

    res = await thread_pool_exec(
        settings.docStoreConn.search,
        select_fields,
        [],
        {"compile_kwd": [_WIKI_GRAPH_ENTITY_KWD]},
        match_expressions,
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
    """Fetch entity rows whose ``slug_kwd`` is in ``slugs``. Unordered.

    Like :func:`_wiki_search_relations_from`, we avoid pushing ``slug_kwd`` (a
    *_kwd analysed field) into the search filter — a `slug_kwd: [..]` with ~20
    entries triggers TOO_MANY_CONNECTIONS. Pull all entity rows once and filter
    in memory.
    """
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
    wanted = set(slugs)
    results = {}
    offset, page_size = 0, 1000
    while True:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            select_fields,
            [],
            {"compile_kwd": [_WIKI_GRAPH_ENTITY_KWD]},
            [],
            OrderByExpr(),
            offset,
            page_size,
            index_nm,
            [dataset_id],
        )
        rows = settings.docStoreConn.get_fields(res, select_fields)
        if not rows:
            break
        for row in rows.values():
            slug = row.get("slug_kwd")
            if isinstance(slug, list):
                slug = slug[0] if slug else ""
            if slug in wanted:
                results[row.get("id", len(results))] = row
        if len(rows) < page_size:
            break
        offset += page_size
    return results


async def _wiki_search_relations_from(
    index_nm,
    dataset_id: str,
    from_slugs: list[str],
):
    """Fetch relation rows whose ``from_kwd`` is in ``from_slugs``.

    IMPORTANT: we do NOT push ``from_slugs`` into the search filter. ``from_kwd``
    is a *_kwd (whitespace-# analysed) field, and the generic search path turns
    `from_kwd: [v1, v2, ...]` into one ``filter_fulltext`` clause per value. A
    batch of only ~20 slugs already blows past Infinity's per-query connection
    budget and surfaces as TOO_MANY_CONNECTIONS (the incremental writer emits
    many relations, so sub_slugs easily exceeds 20). Instead we pull ALL relation
    rows for the dataset in ONE cheap query (relations are short and few) and
    filter in memory.
    """
    if not from_slugs:
        return {}

    from common.doc_store.doc_store_base import OrderByExpr

    select_fields = ["id", "from_kwd", "to_kwd", "content_with_weight"]
    wanted = set(from_slugs)
    # Single query without the huge from_kwd IN-filter. Page over results in
    # case a dataset has more than 10000 relations.
    results = {}
    offset, page_size = 0, 1000
    while True:
        res = await thread_pool_exec(
            settings.docStoreConn.search,
            select_fields,
            [],
            {"compile_kwd": [_WIKI_GRAPH_RELATION_KWD]},
            [],
            OrderByExpr(),
            offset,
            page_size,
            index_nm,
            [dataset_id],
        )
        rows = settings.docStoreConn.get_fields(res, select_fields)
        if not rows:
            break
        for row in rows.values():
            frm = row.get("from_kwd")
            if isinstance(frm, list):
                frm = frm[0] if frm else ""
            if frm in wanted:
                results[row.get("id", len(results))] = row
        if len(rows) < page_size:
            break
        offset += page_size
    return results


async def get_wiki_graph(
    dataset_id: str,
    tenant_id: str,
    node: str | None = None,
    keywords: str | None = None,
    top_n: int | None = None,
):
    """Load the canvas graph payload incrementally from per-row data.

    ``top_n`` overrides the entity budget (default ``_WIKI_GRAPH_MAX_LOADING_ENTITY``).
    ``keywords`` (overview mode only; ignored when ``node`` is given) seeds the
    graph from the best BM25 matches on ``wiki_entity`` rows instead of the
    heaviest-weighted ones. Only entities referenced by a relation are returned.

    Two modes:

    * **Overview** (``node`` is None) — paginate ``wiki_entity`` rows
      ordered by ``weight_int DESC`` in pages of
      ``_WIKI_GRAPH_ENTITY_PAGE_SIZE``. For each page, append entities
      to a running set while the **cumulative** weight stays within
      ``_WIKI_GRAPH_MAX_LOADING_ENTITY``. Pull ``wiki_relation``
      rows whose ``from_kwd`` is in the just-added entities; pull the
      ``to`` targets that we haven't seen yet (they count toward the same
      cap). Stop once the cap is hit, or the page is empty, or no entry
      from the page fit under the budget.

    * **Click** (``node`` is a slug) — load the centre entity (always
      included), pull every ``wiki_relation`` with ``from_kwd=node``,
      then pull the ``to`` entities. Capped at
      ``_WIKI_GRAPH_MAX_LOADING_ENTITY`` for hub-node safety.

    Returns ``(True, {"entities": [...], "relations": [...]})`` shaped
    exactly as the frontend ``ForceGraph`` adapter consumes, or
    ``(False, message)`` on authorization failure.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    empty = {"entities": [], "relations": []}

    pack = _wiki_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, empty
    index_nm, _ = pack

    keywords = (keywords or "").strip()
    # Entity budget: caller-overridable, clamped to a sane range so a bad param
    # can neither disable the cap nor blow up the response.
    if top_n is not None:
        try:
            cap = max(1, min(int(top_n), 1024))
        except (TypeError, ValueError):
            cap = _WIKI_GRAPH_MAX_LOADING_ENTITY
    else:
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
                if payload and len(entities) < cap * 2:
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
                keywords=keywords,
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

    Touches all artifact ``compile_kwd`` row types the artifact pipeline writes
    (MAP extracts, REDUCE results, PLAN output, drafts, pages, topics, and graph
    rows). After this completes the next "Artifact" run starts from a clean
    slate: no resume cache to short-circuit MAP, no prior pages to reconcile
    against in PLAN.

    Returns ``(True, {"deleted": {kwd: count_or_True}})`` on success or
    ``(False, str)`` on auth failure.
    """
    if not KnowledgebaseService.accessible(dataset_id, tenant_id):
        return False, "no authorization"
    _, kb = KnowledgebaseService.get_by_id(dataset_id)

    pack = _wiki_index_or_none(kb.tenant_id, dataset_id)
    if pack is None:
        return True, {"deleted": {}}
    index_nm, _ = pack

    # Repair rows damaged by the former doc-page-source upsert before deleting
    # that bucket. It updated by ``doc_id`` and could stamp ordinary source
    # chunks as ``wiki_doc_page_source``. Those rows retain chunk content;
    # genuine tracking rows do not. Removing only the bad marker preserves the
    # source chunks while allowing the real tracking rows to be cleared below.
    try:
        from common.doc_store.doc_store_base import OrderByExpr

        fields = ["id", "content_with_weight"]
        offset = 0
        page_size = 1000
        damaged_row_ids: list[str] = []
        while True:
            res = await thread_pool_exec(
                settings.docStoreConn.search,
                fields,
                [],
                {"compile_kwd": ["wiki_doc_page_source"]},
                [],
                OrderByExpr(),
                offset,
                page_size,
                index_nm,
                [dataset_id],
            )
            rows = settings.docStoreConn.get_fields(res, fields) or {}
            for row_id, row in rows.items():
                if row.get("content_with_weight"):
                    damaged_row_ids.append(row_id)
            if len(rows) < page_size:
                break
            offset += page_size
        for row_id in damaged_row_ids:
            await thread_pool_exec(
                settings.docStoreConn.update,
                {"id": row_id},
                {"remove": "compile_kwd"},
                index_nm,
                dataset_id,
            )
        if damaged_row_ids:
            logging.warning(
                "clear_wiki: repaired %d source chunk(s) mislabeled as wiki_doc_page_source kb=%s",
                len(damaged_row_ids),
                dataset_id,
            )
    except Exception:
        logging.exception("clear_wiki: failed to repair mislabeled source chunks kb=%s", dataset_id)
        return False, "Failed to repair legacy Wiki state before clearing"

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

    from api.db.services.file_commit_service import FileCommitService

    if all(result is not False for result in deleted.values()):
        try:
            deleted["file_commit_history"] = FileCommitService.delete_all_page_history(dataset_id)
        except Exception:
            logging.exception(
                "clear_wiki: failed to delete page version history for kb=%s",
                dataset_id,
            )
            deleted["file_commit_history"] = False
    else:
        deleted["file_commit_history"] = False

    return True, {"deleted": deleted}
