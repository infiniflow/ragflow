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

from flask import request
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
from graphrag.general.index import resolve_entities, extract_community
from api.db.services.llm_service import LLMBundle
from api.db import LLMType


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
        obj["graph"]["nodes"] = sorted(obj["graph"]["nodes"], key=lambda x: x.get("pagerank", 0), reverse=True)[:256]
        if "edges" in obj["graph"]:
            total_edges = len(obj["graph"]["edges"])
            node_id_set = { o["id"] for o in obj["graph"]["nodes"] }
            filtered_edges = [o for o in obj["graph"]["edges"] if o["source"] != o["target"] and o["source"] in node_id_set and o["target"] in node_id_set]
            obj["graph"]["edges"] = sorted(filtered_edges, key=lambda x: x.get("weight", 0), reverse=True)[:128]
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
        
        # Run entity resolution using the existing GraphRAG functions
        def progress_callback(msg=""):
            logging.info(f"Entity resolution progress: {msg}")
        
        async def run_entity_resolution():
            chat_model = LLMBundle(kb.tenant_id, LLMType.CHAT, llm_name=None, lang=kb.language)
            embedding_model = LLMBundle(kb.tenant_id, LLMType.EMBEDDING, llm_name=None, lang=kb.language)
            
            # Get all nodes as subgraph nodes for entity resolution
            subgraph_nodes = set(graph.nodes())
            
            # Call the existing resolve_entities function
            await resolve_entities(
                graph=graph,
                subgraph_nodes=subgraph_nodes,
                tenant_id=kb.tenant_id,
                kb_id=kb_id,
                doc_id="api_call",  # Use placeholder since this is a manual API call
                llm_bdl=chat_model,
                embed_bdl=embedding_model,
                callback=progress_callback
            )
            
            return graph
        
        # Execute the async function using trio
        updated_graph = trio.run(run_entity_resolution)
        
        # Convert updated graph back to JSON format
        updated_nodes = []
        for node_id, node_data in updated_graph.nodes(data=True):
            node_dict = {"id": node_id, **node_data}
            updated_nodes.append(node_dict)
        
        updated_edges = []
        for source, target, edge_data in updated_graph.edges(data=True):
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
        
        return get_json_result(
            data=True,
            message=f'Entity resolution completed successfully. Graph now has {len(updated_nodes)} nodes and {len(updated_edges)} edges.',
            code=settings.RetCode.SUCCESS
        )
        
    except Exception as e:
        logging.error(f"Entity resolution failed: {str(e)}")
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
        
        # Run community detection using the existing GraphRAG functions
        def progress_callback(msg=""):
            import re
            logging.info(f"Community detection progress: {msg}")
            
            # Parse progress messages to extract metrics
            if "communities:" in msg.lower():
                # Extract community progress (e.g., "Communities: 3/4")
                match = re.search(r'communities?:\s*(\d+)/(\d+)', msg, re.IGNORECASE)
                if match:
                    progress_data["processed_communities"] = int(match.group(1))
                    progress_data["total_communities"] = int(match.group(2))
            
            if "tokens:" in msg.lower() or "token" in msg.lower():
                # Extract token usage (e.g., "used tokens: 12750")
                match = re.search(r'tokens?[^\d]*(\d+)', msg, re.IGNORECASE)
                if match:
                    progress_data["tokens_used"] = int(match.group(1))
            
            # Update status based on message content
            if "completed" in msg.lower():
                progress_data["current_status"] = "completed"
            elif "processing" in msg.lower() or "generating" in msg.lower():
                progress_data["current_status"] = "processing"
            elif "detecting" in msg.lower():
                progress_data["current_status"] = "detecting"
        
        async def run_community_detection():
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
            
            return graph
        
        # Execute the async function using trio
        updated_graph = trio.run(run_community_detection)
        
        # Convert updated graph back to JSON format
        updated_nodes = []
        for node_id, node_data in updated_graph.nodes(data=True):
            node_dict = {"id": node_id, **node_data}
            updated_nodes.append(node_dict)
        
        updated_edges = []
        for source, target, edge_data in updated_graph.edges(data=True):
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
        
        return get_json_result(
            data={
                "success": True,
                "nodes_count": len(updated_nodes),
                "edges_count": len(updated_edges),
                "communities_count": len(communities),
                "progress": progress_data
            },
            message=f'Community detection completed successfully. Graph now has {len(updated_nodes)} nodes, {len(updated_edges)} edges, and {len(communities)} communities. Tokens used: {progress_data["tokens_used"]}',
            code=settings.RetCode.SUCCESS
        )
        
    except Exception as e:
        logging.error(f"Community detection failed: {str(e)}")
        return get_json_result(
            data=False,
            message=f'Community detection failed: {str(e)}',
            code=settings.RetCode.SERVER_ERROR
        )
