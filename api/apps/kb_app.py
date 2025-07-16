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
import json
import os
import logging
import networkx as nx
import trio
from typing import Dict, Any

from flask import request, Blueprint
from flask_login import login_required, current_user

from api.db.services import duplicate_name
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.file_service import FileService
from api.db.services.user_service import TenantService, UserTenantService
from api.utils.api_utils import server_error_response, get_data_error_result, validate_request, not_allowed_parameters
from api.utils import get_uuid
from api.db import StatusEnum, FileSource
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.db_models import File
from api.utils.api_utils import get_json_result
from api import settings
from rag.nlp import search
from api.constants import DATASET_NAME_LIMIT
from rag.settings import PAGERANK_FLD
from rag.utils.storage_factory import STORAGE_IMPL
from graphrag.general.index import resolve_entities as graphrag_resolve_entities_impl, extract_community
from api.db.services.llm_service import LLMBundle
from api.db import LLMType

community_detection_progress = {}
entity_resolution_progress = {}
entity_extraction_progress = {}
graph_building_progress = {}

manager = Blueprint('kb', __name__)

@manager.route('/create', methods=['post'])  # noqa: F821
@login_required
@validate_request("name")
def create():
    req = request.json
    dataset_name = req["name"]
    if not isinstance(dataset_name, str):
        return get_data_error_result(message="Dataset name must be string.")
    if dataset_name.strip() == "":
        return get_data_error_result(message="Dataset name can't be empty.")
    if len(dataset_name.encode("utf-8")) > DATASET_NAME_LIMIT:
        return get_data_error_result(
            message=f"Dataset name length is {len(dataset_name)} which is larger than {DATASET_NAME_LIMIT}")

    dataset_name = dataset_name.strip()
    dataset_name = duplicate_name(
        KnowledgebaseService.query,
        name=dataset_name,
        tenant_id=current_user.id,
        status=StatusEnum.VALID.value)
    try:
        req["id"] = get_uuid()
        req["name"] = dataset_name
        req["tenant_id"] = current_user.id
        req["created_by"] = current_user.id
        e, t = TenantService.get_by_id(current_user.id)
        if not e:
            return get_data_error_result(message="Tenant not found.")
        req["embd_id"] = t.embd_id
        if not KnowledgebaseService.save(**req):
            return get_data_error_result()
        return get_json_result(data={"kb_id": req["id"]})
    except Exception as e:
        return server_error_response(e)


@manager.route('/update', methods=['post'])  # noqa: F821
@login_required
@validate_request("kb_id", "name", "description", "parser_id")
@not_allowed_parameters("id", "tenant_id", "created_by", "create_time", "update_time", "create_date", "update_date", "created_by")
def update():
    req = request.json
    if not isinstance(req["name"], str):
        return get_data_error_result(message="Dataset name must be string.")
    if req["name"].strip() == "":
        return get_data_error_result(message="Dataset name can't be empty.")
    if len(req["name"].encode("utf-8")) > DATASET_NAME_LIMIT:
        return get_data_error_result(
            message=f"Dataset name length is {len(req['name'])} which is large than {DATASET_NAME_LIMIT}")
    req["name"] = req["name"].strip()

    if not KnowledgebaseService.accessible4deletion(req["kb_id"], current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    try:
        if not KnowledgebaseService.query(
                created_by=current_user.id, id=req["kb_id"]):
            return get_json_result(
                data=False, message='Only owner of knowledgebase authorized for this operation.',
                code=settings.RetCode.OPERATING_ERROR)

        e, kb = KnowledgebaseService.get_by_id(req["kb_id"])
        if not e:
            return get_data_error_result(
                message="Can't find this knowledgebase!")

        if req.get("parser_id", "") == "tag" and os.environ.get('DOC_ENGINE', "elasticsearch") == "infinity":
            return get_json_result(
                data=False,
                message='The chunking method Tag has not been supported by Infinity yet.',
                code=settings.RetCode.OPERATING_ERROR
            )

        if req["name"].lower() != kb.name.lower() \
                and len(
            KnowledgebaseService.query(name=req["name"], tenant_id=current_user.id, status=StatusEnum.VALID.value)) >= 1:
            return get_data_error_result(
                message="Duplicated knowledgebase name.")

        if "parser_config" in req and "graphrag" in req.get("parser_config", {}):
            graphrag_config = req["parser_config"]["graphrag"]
            
            if "graphrag_mode" in graphrag_config:
                valid_modes = ["none", "extract_only", "full_auto"]
                if graphrag_config["graphrag_mode"] not in valid_modes:
                    return get_data_error_result(
                        message=f"Invalid graphrag_mode. Must be one of: {', '.join(valid_modes)}")
            
            if "use_graphrag" in graphrag_config:
                use_graphrag = graphrag_config["use_graphrag"]
                if isinstance(use_graphrag, bool):
                    new_mode = "full_auto" if use_graphrag else "none"
                    graphrag_config["graphrag_mode"] = new_mode
                    graphrag_config.pop("use_graphrag", None)

        del req["kb_id"]
        if not KnowledgebaseService.update_by_id(kb.id, req):
            return get_data_error_result()

        if kb.pagerank != req.get("pagerank", 0):
            if os.environ.get("DOC_ENGINE", "elasticsearch") != "elasticsearch":
                return get_data_error_result(message="'pagerank' can only be set when doc_engine is elasticsearch")
            
            if req.get("pagerank", 0) > 0:
                settings.docStoreConn.update({"kb_id": kb.id}, {PAGERANK_FLD: req["pagerank"]},
                                         search.index_name(kb.tenant_id), kb.id)
            else:
                # Elasticsearch requires PAGERANK_FLD be non-zero!
                settings.docStoreConn.update({"exists": PAGERANK_FLD}, {"remove": PAGERANK_FLD},
                                         search.index_name(kb.tenant_id), kb.id)

        e, kb = KnowledgebaseService.get_by_id(kb.id)
        if not e:
            return get_data_error_result(
                message="Database error (Knowledgebase rename)!")
        kb = kb.to_dict()
        kb.update(req)

        return get_json_result(data=kb)
    except Exception as e:
        return server_error_response(e)


@manager.route('/detail', methods=['GET'])  # noqa: F821
@login_required
def detail():
    kb_id = request.args["kb_id"]
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        for tenant in tenants:
            if KnowledgebaseService.query(
                    tenant_id=tenant.tenant_id, id=kb_id):
                break
        else:
            return get_json_result(
                data=False, message='Only owner of knowledgebase authorized for this operation.',
                code=settings.RetCode.OPERATING_ERROR)
        kb = KnowledgebaseService.get_detail(kb_id)
        if not kb:
            return get_data_error_result(
                message="Can't find this knowledgebase!")
        kb["size"] = DocumentService.get_total_size_by_kb_id(kb_id=kb["id"],keywords="", run_status=[], types=[])
        return get_json_result(data=kb)
    except Exception as e:
        return server_error_response(e)


@manager.route('/list', methods=['POST'])  # noqa: F821
@login_required
def list_kbs():
    keywords = request.args.get("keywords", "")
    page_number = int(request.args.get("page", 0))
    items_per_page = int(request.args.get("page_size", 0))
    parser_id = request.args.get("parser_id")
    orderby = request.args.get("orderby", "create_time")
    if request.args.get("desc", "true").lower() == "false":
        desc = False
    else:
        desc = True

    req = request.get_json()
    owner_ids = req.get("owner_ids", [])
    try:
        if not owner_ids:
            tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
            tenants = [m["tenant_id"] for m in tenants]
            kbs, total = KnowledgebaseService.get_by_tenant_ids(
                tenants, current_user.id, page_number,
                items_per_page, orderby, desc, keywords, parser_id)
        else:
            tenants = owner_ids
            kbs, total = KnowledgebaseService.get_by_tenant_ids(
                tenants, current_user.id, 0,
                0, orderby, desc, keywords, parser_id)
            kbs = [kb for kb in kbs if kb["tenant_id"] in tenants]
            total = len(kbs)
            if page_number and items_per_page:
                kbs = kbs[(page_number-1)*items_per_page:page_number*items_per_page]
        return get_json_result(data={"kbs": kbs, "total": total})
    except Exception as e:
        return server_error_response(e)

@manager.route('/rm', methods=['post'])  # noqa: F821
@login_required
@validate_request("kb_id")
def rm():
    req = request.json
    if not KnowledgebaseService.accessible4deletion(req["kb_id"], current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    try:
        kbs = KnowledgebaseService.query(
            created_by=current_user.id, id=req["kb_id"])
        if not kbs:
            return get_json_result(
                data=False, message='Only owner of knowledgebase authorized for this operation.',
                code=settings.RetCode.OPERATING_ERROR)

        for doc in DocumentService.query(kb_id=req["kb_id"]):
            if not DocumentService.remove_document(doc, kbs[0].tenant_id):
                return get_data_error_result(
                    message="Database error (Document removal)!")
            f2d = File2DocumentService.get_by_document_id(doc.id)
            if f2d:
                FileService.filter_delete([File.source_type == FileSource.KNOWLEDGEBASE, File.id == f2d[0].file_id])
            File2DocumentService.delete_by_document_id(doc.id)
        FileService.filter_delete(
            [File.source_type == FileSource.KNOWLEDGEBASE, File.type == "folder", File.name == kbs[0].name])
        if not KnowledgebaseService.delete_by_id(req["kb_id"]):
            return get_data_error_result(
                message="Database error (Knowledgebase removal)!")
        for kb in kbs:
            settings.docStoreConn.delete({"kb_id": kb.id}, search.index_name(kb.tenant_id), kb.id)
            settings.docStoreConn.deleteIdx(search.index_name(kb.tenant_id), kb.id)
            if hasattr(STORAGE_IMPL, 'remove_bucket'):
                STORAGE_IMPL.remove_bucket(kb.id)
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/<kb_id>/tags', methods=['GET'])  # noqa: F821
@login_required
def list_tags(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )

    tags = settings.retrievaler.all_tags(current_user.id, [kb_id])
    return get_json_result(data=tags)


@manager.route('/tags', methods=['GET'])  # noqa: F821
@login_required
def list_tags_from_kbs():
    kb_ids = request.args.get("kb_ids", "").split(",")
    for kb_id in kb_ids:
        if not KnowledgebaseService.accessible(kb_id, current_user.id):
            return get_json_result(
                data=False,
                message='No authorization.',
                code=settings.RetCode.AUTHENTICATION_ERROR
            )

    tags = settings.retrievaler.all_tags(current_user.id, kb_ids)
    return get_json_result(data=tags)


@manager.route('/<kb_id>/rm_tags', methods=['POST'])  # noqa: F821
@login_required
def rm_tags(kb_id):
    req = request.json
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    e, kb = KnowledgebaseService.get_by_id(kb_id)

    for t in req["tags"]:
        settings.docStoreConn.update({"tag_kwd": t, "kb_id": [kb_id]},
                                     {"remove": {"tag_kwd": t}},
                                     search.index_name(kb.tenant_id),
                                     kb_id)
    return get_json_result(data=True)


@manager.route('/<kb_id>/rename_tag', methods=['POST'])  # noqa: F821
@login_required
def rename_tags(kb_id):
    req = request.json
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    e, kb = KnowledgebaseService.get_by_id(kb_id)

    settings.docStoreConn.update({"tag_kwd": req["from_tag"], "kb_id": [kb_id]},
                                     {"remove": {"tag_kwd": req["from_tag"].strip()}, "add": {"tag_kwd": req["to_tag"]}},
                                     search.index_name(kb.tenant_id),
                                     kb_id)
    return get_json_result(data=True)


@manager.route('/<kb_id>/knowledge_graph', methods=['GET'])  # noqa: F821
@login_required
def knowledge_graph(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    _, kb = KnowledgebaseService.get_by_id(kb_id)
    req = {
        "kb_id": [kb_id],
        "knowledge_graph_kwd": ["graph"]
    }

    obj = {"graph": {}, "mind_map": {}}
    if not settings.docStoreConn.indexExist(search.index_name(kb.tenant_id), kb_id):
        return get_json_result(data=obj)
    sres = settings.retrievaler.search(req, search.index_name(kb.tenant_id), [kb_id])
    if not len(sres.ids):
        return get_json_result(data=obj)

    for id in sres.ids[:1]:
        ty = sres.field[id]["knowledge_graph_kwd"]
        try:
            content_json = json.loads(sres.field[id]["content_with_weight"])
        except Exception:
            continue

        obj[ty] = content_json

    if "nodes" in obj["graph"]:
        total_nodes = len(obj["graph"]["nodes"])
        obj["graph"]["nodes"] = sorted(obj["graph"]["nodes"], key=lambda x: x.get("pagerank", 0), reverse=True)[:500]
        if "edges" in obj["graph"]:
            total_edges = len(obj["graph"]["edges"])
            node_id_set = { o["id"] for o in obj["graph"]["nodes"] }
            filtered_edges = [o for o in obj["graph"]["edges"] if o["source"] != o["target"] and o["source"] in node_id_set and o["target"] in node_id_set]
            obj["graph"]["edges"] = sorted(filtered_edges, key=lambda x: x.get("weight", 0), reverse=True)[:300]
            obj["graph"]["total_edges"] = total_edges
        obj["graph"]["total_nodes"] = total_nodes
    return get_json_result(data=obj)

@manager.route('/<kb_id>/knowledge_graph', methods=['DELETE'])  # noqa: F821
@login_required
def delete_knowledge_graph(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    _, kb = KnowledgebaseService.get_by_id(kb_id)
    settings.docStoreConn.delete({"knowledge_graph_kwd": ["graph", "subgraph", "entity", "relation"]}, search.index_name(kb.tenant_id), kb_id)

    return get_json_result(data=True)


@manager.route('/<kb_id>/knowledge_graph/resolve_entities', methods=['POST'])  # noqa: F821
@login_required
def resolve_entities(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    
    try:
        _, kb = KnowledgebaseService.get_by_id(kb_id)
        
        if DocumentService.has_documents_parsing(kb_id):
            return get_json_result(
                data=False,
                message='Cannot perform entity resolution while documents are being parsed. Please wait for parsing to complete.',
                code=423  # HTTP 423 Locked
            )
        
        # Check if knowledge graph exists
        if not settings.docStoreConn.indexExist(search.index_name(kb.tenant_id), kb_id):
            return get_json_result(
                data=False,
                message='Knowledge graph not found.',
                code=settings.RetCode.DATA_ERROR
            )
        
        # Retrieve existing knowledge graph
        req = {
            "kb_id": [kb_id],
            "knowledge_graph_kwd": ["graph"]
        }
        
        sres = settings.retrievaler.search(req, search.index_name(kb.tenant_id), [kb_id])
        if not len(sres.ids):
            return get_json_result(
                data=False,
                message='Knowledge graph not found.',
                code=settings.RetCode.DATA_ERROR
            )
        
        # Get graph data
        graph_data = None
        for id in sres.ids[:1]:
            try:
                graph_data = json.loads(sres.field[id]["content_with_weight"])
                break
            except Exception:
                continue
        
        if not graph_data or "nodes" not in graph_data or "edges" not in graph_data:
            return get_json_result(
                data=False,
                message='Invalid knowledge graph data.',
                code=settings.RetCode.DATA_ERROR
            )
        
        # Create NetworkX graph from stored data
        graph = nx.Graph()
        
        # Add nodes
        for node in graph_data["nodes"]:
            graph.add_node(node["id"], **{k: v for k, v in node.items() if k != "id"})
        
        # Add edges
        for edge in graph_data["edges"]:
            graph.add_edge(edge["source"], edge["target"], **{k: v for k, v in edge.items() if k not in ["source", "target"]})
        
        # Set source_id for the graph (required by GraphRAG functions)
        graph.graph["source_id"] = ["api_call"]
        
        # Get all nodes as subgraph nodes for entity resolution
        subgraph_nodes = set(graph.nodes())
        
        # Progress tracking variables
        progress_data = {
            "total_pairs": 0,
            "processed_pairs": 0,
            "remaining_pairs": 0,
            "current_status": "initializing"
        }
        
        # Initialize progress data in global storage
        entity_resolution_progress[kb_id] = progress_data
        
        # Run entity resolution using the existing GraphRAG functions
        def progress_callback(msg=""):
            import re
            logging.info(f"Entity resolution progress: {msg}")
            
            # Parse progress messages to extract metrics
            # Format: "Identified X candidate pairs"
            if "identified" in msg.lower() and "candidate pairs" in msg.lower():
                match = re.search(r'Identified (\d+) candidate pairs', msg, re.IGNORECASE)
                if match:
                    total_pairs = int(match.group(1))
                    progress_data["total_pairs"] = total_pairs
                    progress_data["processed_pairs"] = 0
                    progress_data["remaining_pairs"] = total_pairs
                    progress_data["current_status"] = "processing"
            
            # Format: "Resolved X pairs, Y are remained to resolve"
            elif "resolved" in msg.lower() and "remained to resolve" in msg.lower():
                match = re.search(r'Resolved (\d+) pairs, (\d+) are remained to resolve', msg, re.IGNORECASE)
                if match:
                    processed_pairs = int(match.group(1))
                    remaining_pairs = int(match.group(2))
                    total_pairs = processed_pairs + remaining_pairs
                    progress_data["total_pairs"] = total_pairs
                    progress_data["processed_pairs"] = processed_pairs
                    progress_data["remaining_pairs"] = remaining_pairs
                    progress_data["current_status"] = "processing"
            
            # Format: "Resolved X candidate pairs, Y of them are selected to merge"
            elif "resolved" in msg.lower() and "candidate pairs" in msg.lower() and "selected to merge" in msg.lower():
                match = re.search(r'Resolved (\d+) candidate pairs', msg, re.IGNORECASE)
                if match:
                    total_pairs = int(match.group(1))
                    progress_data["total_pairs"] = total_pairs
                    progress_data["processed_pairs"] = total_pairs
                    progress_data["remaining_pairs"] = 0
                    progress_data["current_status"] = "completed"
            
            # Update status based on message content
            if "done" in msg.lower():
                progress_data["current_status"] = "completed"
            
            # Update the global progress storage
            entity_resolution_progress[kb_id] = progress_data.copy()
        
        async def run_entity_resolution():
            from rag.utils.redis_conn import RedisDistributedLock
            
            # Acquire the same lock used by document parsing
            graphrag_task_lock = RedisDistributedLock(
                f"graphrag_task_{kb_id}", 
                lock_value="api_entity_resolution", 
                timeout=1200
            )
            
            try:
                await graphrag_task_lock.spin_acquire()
                
                chat_model = LLMBundle(kb.tenant_id, LLMType.CHAT, llm_name=None, lang=kb.language)
                embedding_model = LLMBundle(kb.tenant_id, LLMType.EMBEDDING, llm_name=None, lang=kb.language)
                
                # Get all nodes as subgraph nodes for entity resolution
                subgraph_nodes = set(graph.nodes())
                
                # Call the existing resolve_entities function
                await graphrag_resolve_entities_impl(
                    graph,
                    subgraph_nodes,
                    kb.tenant_id,
                    kb_id,
                    "api_call",  # Use placeholder since this is a manual API call
                    chat_model,
                    embedding_model,
                    progress_callback
                )
                
                # Convert updated graph back to JSON format
                updated_nodes = []
                for node_id, node_data in graph.nodes(data=True):
                    node_dict = {"id": node_id, **node_data}
                    updated_nodes.append(node_dict)
                
                updated_edges = []
                for source, target, edge_data in graph.edges(data=True):
                    edge_dict = {"source": source, "target": target, **edge_data}
                    updated_edges.append(edge_dict)
                
                updated_graph_data = {
                    "nodes": updated_nodes,
                    "edges": updated_edges
                }
                
                # Update stored knowledge graph
                settings.docStoreConn.update(
                    {"knowledge_graph_kwd": ["graph"], "kb_id": [kb_id]},
                    {"content_with_weight": json.dumps(updated_graph_data)},
                    search.index_name(kb.tenant_id),
                    kb_id
                )
                
                progress_data["current_status"] = "completed"
                entity_resolution_progress[kb_id] = progress_data.copy()
            except Exception as e:
                logging.exception(f"Entity resolution failed for kb {kb_id}: {str(e)}")
                entity_resolution_progress[kb_id]["current_status"] = "failed"
                raise
            finally:
                graphrag_task_lock.release()
        
        # Run entity resolution directly with trio
        trio.run(run_entity_resolution)
        
        return get_json_result(
            data=True,
            message='Entity resolution started successfully.',
            code=settings.RetCode.SUCCESS
        )
        
    except Exception as e:
        logging.error(f"Entity resolution failed: {str(e)}")
        if kb_id in entity_resolution_progress:
            del entity_resolution_progress[kb_id]
        return get_json_result(
            data=False,
            message=f'Entity resolution failed: {str(e)}',
            code=settings.RetCode.SERVER_ERROR
        )


@manager.route('/<kb_id>/knowledge_graph/detect_communities', methods=['POST'])  # noqa: F821
@login_required
def detect_communities(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    
    try:
        _, kb = KnowledgebaseService.get_by_id(kb_id)
        
        # Check if documents are currently being parsed
        if DocumentService.has_documents_parsing(kb_id):
            return get_json_result(
                data=False,
                message='Cannot perform community detection while documents are being parsed. Please wait for parsing to complete.',
                code=423  # HTTP 423 Locked
            )
        
        # Check if knowledge graph exists
        if not settings.docStoreConn.indexExist(search.index_name(kb.tenant_id), kb_id):
            return get_json_result(
                data=False,
                message='Knowledge graph not found.',
                code=settings.RetCode.DATA_ERROR
            )
        
        # Retrieve existing knowledge graph
        req = {
            "kb_id": [kb_id],
            "knowledge_graph_kwd": ["graph"]
        }
        
        sres = settings.retrievaler.search(req, search.index_name(kb.tenant_id), [kb_id])
        if not len(sres.ids):
            return get_json_result(
                data=False,
                message='Knowledge graph not found.',
                code=settings.RetCode.DATA_ERROR
            )
        
        # Get graph data
        graph_data = None
        for id in sres.ids[:1]:
            try:
                graph_data = json.loads(sres.field[id]["content_with_weight"])
                break
            except Exception:
                continue
        
        if not graph_data or "nodes" not in graph_data or "edges" not in graph_data:
            return get_json_result(
                data=False,
                message='Invalid knowledge graph data.',
                code=settings.RetCode.DATA_ERROR
            )
        
        # Create NetworkX graph from stored data
        graph = nx.Graph()
        
        # Add nodes
        for node in graph_data["nodes"]:
            graph.add_node(node["id"], **{k: v for k, v in node.items() if k != "id"})
        
        # Add edges
        for edge in graph_data["edges"]:
            graph.add_edge(edge["source"], edge["target"], **{k: v for k, v in edge.items() if k not in ["source", "target"]})
        
        # Set source_id for the graph (required by GraphRAG functions)
        graph.graph["source_id"] = ["api_call"]
        
        # Set node degrees for community detection
        for node_degree in graph.degree:
            graph.nodes[str(node_degree[0])]["rank"] = int(node_degree[1])
        
        # Progress tracking variables
        progress_data = {
            "total_communities": 0,
            "processed_communities": 0,
            "tokens_used": 0,
            "current_status": "initializing"
        }
        
        # Initialize progress data in global storage
        community_detection_progress[kb_id] = progress_data
        
        # Run community detection using the existing GraphRAG functions
        def progress_callback(msg=""):
            import re
            logging.info(f"Community detection progress: {msg}")
            
            # Parse progress messages to extract metrics
            # Actual format: "Communities: 3/4, used tokens: 12750"
            if "communities:" in msg.lower():
                # Extract community progress (e.g., "Communities: 3/4")
                match = re.search(r'Communities:\s*(\d+)/(\d+)', msg, re.IGNORECASE)
                if match:
                    progress_data["processed_communities"] = int(match.group(1))
                    progress_data["total_communities"] = int(match.group(2))
                    progress_data["current_status"] = "processing"
            
            # Update status based on message content
            if "done" in msg.lower():
                progress_data["current_status"] = "completed"
            elif "extracting" in msg.lower() or "extracted" in msg.lower():
                progress_data["current_status"] = "processing"
            
            # Update the global progress storage
            community_detection_progress[kb_id] = progress_data.copy()
        
        async def run_community_detection():
            from rag.utils.redis_conn import RedisDistributedLock
            
            # Acquire the same lock used by document parsing
            graphrag_task_lock = RedisDistributedLock(
                f"graphrag_task_{kb_id}", 
                lock_value="api_community_detection", 
                timeout=1200
            )
            
            try:
                await graphrag_task_lock.spin_acquire()
                
                chat_model = LLMBundle(kb.tenant_id, LLMType.CHAT, llm_name=None, lang=kb.language)
                embedding_model = LLMBundle(kb.tenant_id, LLMType.EMBEDDING, llm_name=None, lang=kb.language)
                
                # Call the existing extract_community function
                await extract_community(
                    graph=graph,
                    tenant_id=kb.tenant_id,
                    kb_id=kb_id,
                    doc_id="api_call",  # Use placeholder since this is a manual API call
                    llm_bdl=chat_model,
                    embed_bdl=embedding_model,
                    callback=progress_callback
                )
                
                # Convert updated graph back to JSON format
                updated_nodes = []
                for node_id, node_data in graph.nodes(data=True):
                    node_dict = {"id": node_id, **node_data}
                    updated_nodes.append(node_dict)
                
                updated_edges = []
                for source, target, edge_data in graph.edges(data=True):
                    edge_dict = {"source": source, "target": target, **edge_data}
                    updated_edges.append(edge_dict)
                
                updated_graph_data = {
                    "nodes": updated_nodes,
                    "edges": updated_edges
                }
                
                # Update stored knowledge graph
                settings.docStoreConn.update(
                    {"knowledge_graph_kwd": ["graph"], "kb_id": [kb_id]},
                    {"content_with_weight": json.dumps(updated_graph_data)},
                    search.index_name(kb.tenant_id),
                    kb_id
                )
                
                # Count communities in the updated graph
                communities = set()
                for node_data in updated_nodes:
                    if "community" in node_data:
                        communities.add(node_data["community"])
                
                progress_data["current_status"] = "completed"
                community_detection_progress[kb_id] = progress_data.copy()
                
            except Exception as e:
                logging.exception(f"Community detection failed for kb {kb_id}: {str(e)}")
                community_detection_progress[kb_id]["current_status"] = "failed"
                raise
            finally:
                graphrag_task_lock.release()
        
        # Run community detection directly with trio
        trio.run(run_community_detection)
        
        return get_json_result(
            data=True,
            message='Community detection started successfully.',
            code=settings.RetCode.SUCCESS
        )
        
    except Exception as e:
        logging.error(f"Community detection failed: {str(e)}")
        if kb_id in community_detection_progress:
            del community_detection_progress[kb_id]
        return get_json_result(
            data=False,
            message=f'Community detection failed: {str(e)}',
            code=settings.RetCode.SERVER_ERROR
        )


@manager.route('/<kb_id>/knowledge_graph/extract_entities', methods=['POST'])  # noqa: F821
@login_required
def extract_entities(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    
    try:
        _, kb = KnowledgebaseService.get_by_id(kb_id)
        
        # Check if documents are currently being parsed
        if DocumentService.has_documents_parsing(kb_id):
            return get_json_result(
                data=False,
                message='Cannot extract entities while documents are being parsed. Please wait for parsing to complete.',
                code=423  # HTTP 423 Locked
            )
        
        # Check if index exists
        if not settings.docStoreConn.indexExist(search.index_name(kb.tenant_id), kb_id):
            return get_json_result(
                data=False,
                message='Knowledge base index not found.',
                code=settings.RetCode.DATA_ERROR
            )
        
        # Check if there are chunks to process
        req = {"kb_id": [kb_id]}
        sres = settings.retrievaler.search(req, search.index_name(kb.tenant_id), [kb_id])
        if not len(sres.ids):
            return get_json_result(
                data=False,
                message='No documents found to extract entities from.',
                code=settings.RetCode.DATA_ERROR
            )
        
        # Initialize progress tracking
        import threading
        progress_lock = threading.Lock()
        
        with progress_lock:
            entity_extraction_progress[kb_id] = {
                "total_documents": 0,
                "processed_documents": 0,
                "entities_found": 0,
                "current_status": "starting"
            }
        
        # Set up callback for progress tracking
        def progress_callback(progress=None, msg=""):
            import re
            # Handle both callback styles: callback(msg) and callback(progress, msg="text")
            if progress is None:
                # Old style: callback(msg)
                pass
            elif isinstance(progress, str):
                # Old style: callback(msg) where first arg is actually the message
                msg = progress
            # else: New style: callback(progress_float, msg="text") - use msg parameter
            
            # Parse progress messages to update tracking
            if "Starting entity extraction for" in msg:
                match = re.search(r"(\d+) documents", msg)
                if match:
                    with progress_lock:
                        entity_extraction_progress[kb_id]["total_documents"] = int(match.group(1))
                        entity_extraction_progress[kb_id]["current_status"] = "processing"
            elif "Document" in msg and "extracted" in msg:
                # Parse: "Document doc_id: extracted X entities, Y relations"
                match = re.search(r"extracted (\d+) entities", msg)
                if match:
                    with progress_lock:
                        current_entities = entity_extraction_progress[kb_id].get("entities_found", 0)
                        entity_extraction_progress[kb_id]["entities_found"] = current_entities + int(match.group(1))
                        entity_extraction_progress[kb_id]["processed_documents"] += 1
            elif "Entity extraction completed" in msg:
                with progress_lock:
                    entity_extraction_progress[kb_id]["current_status"] = "completed"
        
        # Create LLM bundle
        llm_bdl = LLMBundle(kb.tenant_id, LLMType.CHAT, llm_name=None, lang=kb.language)
        embed_bdl = LLMBundle(kb.tenant_id, LLMType.EMBEDDING, llm_name=kb.embd_id, lang=kb.language)
        
        # Run entity extraction in background
        async def run_extraction():
            logging.info(f"Starting entity extraction for kb {kb_id}")
            try:
                from graphrag.general.index import generate_subgraph, merge_subgraph
                from graphrag.light.graph_extractor import GraphExtractor as LightKGExt
                from graphrag.general.graph_extractor import GraphExtractor as GeneralKGExt
                
                # Get GraphRAG configuration
                graphrag_config = kb.parser_config.get("graphrag", {})
                entity_types = graphrag_config.get("entity_types", ["organization", "person", "location", "event", "time"])
                method = graphrag_config.get("method", "light")
                logging.info(f"GraphRAG config: method={method}, entity_types={entity_types}")
                
                # Lock to prevent concurrent operations
                from rag.utils.redis_conn import RedisDistributedLock
                extract_lock = RedisDistributedLock(f"graph_extract_{kb_id}", lock_value="api_extract_entities", timeout=1200)
                logging.info(f"Acquiring lock for kb {kb_id}")
                await extract_lock.spin_acquire()
                
                try:
                    # Select extractor based on method (same as parsing workflow)
                    extractor_class = LightKGExt if method == "light" else GeneralKGExt
                    logging.info(f"Using extractor class: {extractor_class.__name__}")
                    
                    # Get documents that have chunks using DocumentService
                    from api.db.services.document_service import DocumentService
                    docs, count = DocumentService.get_by_kb_id(kb_id, 1, 1000, 'create_time', False, '', [], [], [])
                    docs_with_chunks = [doc for doc in docs if doc['chunk_num'] > 0]
                    logging.info(f"Found {len(docs_with_chunks)} documents with chunks (out of {count} total documents)")
                    
                    # Group chunks by document
                    doc_chunks = {}
                    total_docs = 0
                    for doc in docs_with_chunks:
                        doc_id = doc['id']
                        chunks = []
                        for d in settings.retrievaler.chunk_list(
                            doc_id, kb.tenant_id, [kb_id], fields=["content_with_weight"]
                        ):
                            chunks.append(d["content_with_weight"])
                        if chunks:
                            doc_chunks[doc_id] = chunks
                            total_docs += 1
                        else:
                            logging.warning(f"Document {doc_id} ({doc['name']}) reports {doc['chunk_num']} chunks but chunk_list returned none")
                    
                    logging.info(f"Processing {total_docs} documents with chunks")
                    
                    if total_docs == 0:
                        logging.warning("No documents with chunks found to process")
                        entity_extraction_progress[kb_id]["current_status"] = "completed"
                        return
                    
                    # Use parallel entity extraction function
                    from graphrag.general.index import extract_entities_only
                    
                    # Convert doc_chunks to doc_ids list for extract_entities_only function
                    doc_ids = list(doc_chunks.keys())
                    
                    # Call parallel entity extraction function
                    result = await extract_entities_only(
                        tenant_id=kb.tenant_id,
                        kb_id=kb_id,
                        doc_ids=doc_ids,
                        language=kb.language,
                        entity_types=entity_types,
                        method=method,
                        llm_bdl=llm_bdl,
                        embed_bdl=embed_bdl,
                        callback=progress_callback
                    )
                    
                    # Update progress from result
                    with progress_lock:
                        entity_extraction_progress[kb_id].update({
                            "total_documents": result["total_documents"],
                            "processed_documents": result["processed_documents"],
                            "entities_found": result["entities_found"],
                            "relations_found": result["relations_found"],
                            "current_status": result["status"]
                        })
                    
                    logging.info(f"Entity extraction completed for kb {kb_id}: {result['entities_found']} entities, {result['relations_found']} relations")
                    
                finally:
                    extract_lock.release()
                    logging.info(f"Released lock for kb {kb_id}")
                
            except Exception as e:
                logging.exception(f"Entity extraction failed for kb {kb_id}: {str(e)}")
                with progress_lock:
                    entity_extraction_progress[kb_id]["current_status"] = "failed"
                raise
        
        # Run entity extraction directly with trio
        trio.run(run_extraction)
        
        return get_json_result(
            data=True,
            message='Entity extraction started successfully.'
        )
        
    except Exception as e:
        logging.exception(f"Extract entities failed for kb {kb_id}: {str(e)}")
        if kb_id in entity_extraction_progress:
            del entity_extraction_progress[kb_id]
        
        return get_json_result(
            data=False,
            message=f'Entity extraction failed: {str(e)}',
            code=settings.RetCode.SERVER_ERROR
        )


@manager.route('/<kb_id>/knowledge_graph/build_graph', methods=['POST'])  # noqa: F821
@login_required
def build_graph(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    
    try:
        _, kb = KnowledgebaseService.get_by_id(kb_id)
        
        # Check if documents are currently being parsed
        if DocumentService.has_documents_parsing(kb_id):
            return get_json_result(
                data=False,
                message='Cannot build graph while documents are being parsed. Please wait for parsing to complete.',
                code=423  # HTTP 423 Locked
            )
        
        # Check if index exists
        if not settings.docStoreConn.indexExist(search.index_name(kb.tenant_id), kb_id):
            return get_json_result(
                data=False,
                message='Knowledge base index not found.',
                code=settings.RetCode.DATA_ERROR
            )
        
        # Check if entities exist
        req = {
            "kb_id": [kb_id],
            "knowledge_graph_kwd": ["entity"]
        }
        sres = settings.retrievaler.search(req, search.index_name(kb.tenant_id), [kb_id])
        if not len(sres.ids):
            return get_json_result(
                data=False,
                message='No entities found. Please extract entities first.',
                code=settings.RetCode.DATA_ERROR
            )
        
        # Initialize progress tracking
        graph_building_progress[kb_id] = {
            "total_entities": 0,
            "processed_entities": 0,
            "relationships_created": 0,
            "current_status": "starting"
        }
        
        # Set up callback for progress tracking
        def progress_callback(msg=""):
            import re
            # Parse progress messages to update tracking
            if "Found" in msg and "entities and" in msg:
                # Parse: "Found X entities and Y relations"
                entity_match = re.search(r"Found (\d+) entities", msg)
                if entity_match:
                    graph_building_progress[kb_id]["total_entities"] = int(entity_match.group(1))
                    graph_building_progress[kb_id]["current_status"] = "processing"
            elif "Built graph with" in msg:
                # Parse: "Built graph with X nodes and Y edges"
                edge_match = re.search(r"(\d+) edges", msg)
                if edge_match:
                    graph_building_progress[kb_id]["relationships_created"] = int(edge_match.group(1))
            elif "Graph building completed" in msg:
                graph_building_progress[kb_id]["current_status"] = "completed"
        
        # Create LLM bundle
        llm_bdl = LLMBundle(kb.tenant_id, LLMType.CHAT, llm_name=None, lang=kb.language)
        embed_bdl = LLMBundle(kb.tenant_id, LLMType.EMBEDDING, llm_name=kb.embd_id, lang=kb.language)
        
        # Run graph building in background
        async def run_build():
            try:
                from graphrag.general.index import build_graph_from_entities
                
                # Lock to prevent concurrent operations
                from rag.utils.redis_conn import RedisDistributedLock
                build_lock = RedisDistributedLock(f"graph_build_{kb_id}", lock_value="api_build_graph", timeout=1200)
                await build_lock.spin_acquire()
                
                try:
                    result = await build_graph_from_entities(
                        tenant_id=kb.tenant_id,
                        kb_id=kb_id,
                        embed_bdl=embed_bdl,
                        callback=progress_callback
                    )
                finally:
                    build_lock.release()
                    
                # Update final progress from result
                graph_building_progress[kb_id].update({
                    "total_entities": result["total_entities"],
                    "processed_entities": result["processed_entities"],
                    "relationships_created": result["relationships_created"],
                    "current_status": result["status"]
                })
                
            except Exception as e:
                logging.exception(f"Graph building failed for kb {kb_id}: {str(e)}")
                graph_building_progress[kb_id]["current_status"] = "failed"
                raise
        
        # Run graph building directly with trio
        trio.run(run_build)
        
        return get_json_result(
            data=True,
            message='Graph building started successfully.'
        )
        
    except Exception as e:
        logging.exception(f"Build graph failed for kb {kb_id}: {str(e)}")
        # Clean up progress on failure
        if kb_id in graph_building_progress:
            del graph_building_progress[kb_id]
        
        return get_json_result(
            data=False,
            message=f'Graph building failed: {str(e)}',
            code=settings.RetCode.SERVER_ERROR
        )


@manager.route('/<kb_id>/knowledge_graph/progress', methods=['GET'])  # noqa: F821
@login_required
def get_progress(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    
    # Check for operation type parameter
    operation = request.args.get('operation', 'community_detection')
    
    # Get progress data for this kb_id based on operation type
    if operation == 'entity_resolution':
        progress_data = entity_resolution_progress.get(kb_id, None)
        operation_name = 'entity resolution'
    elif operation == 'entity_extraction':
        progress_data = entity_extraction_progress.get(kb_id, None)
        operation_name = 'entity extraction'
    elif operation == 'graph_building':
        progress_data = graph_building_progress.get(kb_id, None)
        operation_name = 'graph building'
    else:
        progress_data = community_detection_progress.get(kb_id, None)
        operation_name = 'community detection'
    
    if progress_data is None:
        return get_json_result(
            data=None,
            message=f'No active {operation_name} operation.',
            code=settings.RetCode.SUCCESS
        )
    
    return get_json_result(
        data=progress_data,
        message='Progress retrieved successfully.',
        code=settings.RetCode.SUCCESS
    )


@manager.route('/<kb_id>/knowledge_graph/extract_entities/progress', methods=['GET'])  # noqa: F821
@login_required
def get_extraction_progress(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    
    progress_data = entity_extraction_progress.get(kb_id, None)
    
    if progress_data is None:
        return get_json_result(
            data=None,
            message='No active entity extraction operation.',
            code=settings.RetCode.SUCCESS
        )
    
    return get_json_result(
        data=progress_data,
        message='Entity extraction progress retrieved successfully.',
        code=settings.RetCode.SUCCESS
    )


@manager.route('/<kb_id>/knowledge_graph/build_graph/progress', methods=['GET'])  # noqa: F821
@login_required
def get_build_progress(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    
    progress_data = graph_building_progress.get(kb_id, None)
    
    if progress_data is None:
        return get_json_result(
            data=None,
            message='No active graph building operation.',
            code=settings.RetCode.SUCCESS
        )
    
    return get_json_result(
        data=progress_data,
        message='Graph building progress retrieved successfully.',
        code=settings.RetCode.SUCCESS
    )


@manager.route('/<kb_id>/document_parsing_status', methods=['GET'])  # noqa: F821
@login_required
def check_document_parsing_status(kb_id):
    if not KnowledgebaseService.accessible(kb_id, current_user.id):
        return get_json_result(
            data=False,
            message='No authorization.',
            code=settings.RetCode.AUTHENTICATION_ERROR
        )
    
    try:
        # Check if any documents are currently being parsed
        is_parsing = DocumentService.has_documents_parsing(kb_id)
        
        return get_json_result(
            data={"is_parsing": is_parsing},
            message='Document parsing status retrieved successfully.',
            code=settings.RetCode.SUCCESS
        )
        
    except Exception as e:
        logging.error(f"Failed to check document parsing status: {str(e)}")
        return get_json_result(
            data=False,
            message=f'Failed to check document parsing status: {str(e)}',
            code=settings.RetCode.SERVER_ERROR
        )
