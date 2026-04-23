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

from quart import request

from api.apps import current_user, login_required
from api.db.joint_services.tenant_model_service import (
    get_model_config_by_id,
    get_model_config_by_type_and_name,
    get_tenant_default_model_by_type,
)
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.db.services.search_service import SearchService
from api.db.services.user_service import UserTenantService
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    get_request_json,
    server_error_response,
    validate_request,
)
from common import settings
from common.constants import LLMType, RetCode
from common.metadata_utils import apply_meta_data_filter
from rag.app.tag import label_question
from rag.nlp import search
from rag.prompts.generator import cross_languages, keyword_extraction


@manager.route('/retrieval_test', methods=['POST'])  # noqa: F821
@login_required
@validate_request("kb_id", "question")
async def retrieval_test():
    req = await get_request_json()
    page = int(req.get("page", 1))
    size = int(req.get("size", 30))
    question = req["question"]
    kb_ids = req["kb_id"]
    if isinstance(kb_ids, str):
        kb_ids = [kb_ids]
    if not kb_ids:
        return get_json_result(data=False, message='Please specify dataset firstly.',
                               code=RetCode.DATA_ERROR)

    doc_ids = req.get("doc_ids", [])
    use_kg = req.get("use_kg", False)
    top = int(req.get("top_k", 1024))
    langs = req.get("cross_languages", [])
    user_id = current_user.id

    async def _retrieval():
        local_doc_ids = list(doc_ids) if doc_ids else []
        tenant_ids = []

        meta_data_filter = {}
        chat_mdl = None
        if req.get("search_id", ""):
            search_config = SearchService.get_detail(req.get("search_id", "")).get("search_config", {})
            meta_data_filter = search_config.get("meta_data_filter", {})
            if meta_data_filter.get("method") in ["auto", "semi_auto"]:
                chat_id = search_config.get("chat_id", "")
                if chat_id:
                    chat_model_config = get_model_config_by_type_and_name(user_id, LLMType.CHAT, search_config["chat_id"])
                else:
                    chat_model_config = get_tenant_default_model_by_type(user_id, LLMType.CHAT)
                chat_mdl = LLMBundle(user_id, chat_model_config)
        else:
            meta_data_filter = req.get("meta_data_filter") or {}
            if meta_data_filter.get("method") in ["auto", "semi_auto"]:
                chat_model_config = get_tenant_default_model_by_type(user_id, LLMType.CHAT)
                chat_mdl = LLMBundle(user_id, chat_model_config)

        if meta_data_filter:
            metas = DocMetadataService.get_flatted_meta_by_kbs(kb_ids)
            local_doc_ids = await apply_meta_data_filter(meta_data_filter, metas, question, chat_mdl, local_doc_ids)

        tenants = UserTenantService.query(user_id=user_id)
        for kb_id in kb_ids:
            for tenant in tenants:
                if KnowledgebaseService.query(
                        tenant_id=tenant.tenant_id, id=kb_id):
                    tenant_ids.append(tenant.tenant_id)
                    break
            else:
                return get_json_result(
                    data=False, message='Only owner of dataset authorized for this operation.',
                    code=RetCode.OPERATING_ERROR)

        e, kb = KnowledgebaseService.get_by_id(kb_ids[0])
        if not e:
            return get_data_error_result(message="Knowledgebase not found!")

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
                        kb_ids,
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
            default_chat_model_config = get_tenant_default_model_by_type(user_id, LLMType.CHAT)
            ck = await settings.kg_retriever.retrieval(_question,
                                                   tenant_ids,
                                                   kb_ids,
                                                   embd_mdl,
                                                   LLMBundle(kb.tenant_id, default_chat_model_config))
            if ck["content_with_weight"]:
                ranks["chunks"].insert(0, ck)
        ranks["chunks"] = settings.retriever.retrieval_by_children(ranks["chunks"], tenant_ids)

        for c in ranks["chunks"]:
            c.pop("vector", None)
        ranks["labels"] = labels

        return get_json_result(data=ranks)

    try:
        return await _retrieval()
    except Exception as e:
        if str(e).find("not_found") > 0:
            return get_json_result(data=False, message='No chunk found! Check the chunk status please!',
                                   code=RetCode.DATA_ERROR)
        return server_error_response(e)


@manager.route('/knowledge_graph', methods=['GET'])  # noqa: F821
@login_required
async def knowledge_graph():
    doc_id = request.args["doc_id"]
    tenant_id = DocumentService.get_tenant_id(doc_id)
    kb_ids = KnowledgebaseService.get_kb_ids(tenant_id)
    req = {
        "doc_ids": [doc_id],
        "knowledge_graph_kwd": ["graph", "mind_map"]
    }
    sres = await settings.retriever.search(req, search.index_name(tenant_id), kb_ids)
    obj = {"graph": {}, "mind_map": {}}
    for id in sres.ids[:2]:
        ty = sres.field[id]["knowledge_graph_kwd"]
        try:
            content_json = json.loads(sres.field[id]["content_with_weight"])
        except Exception:
            continue

        if ty == 'mind_map':
            node_dict = {}

            def repeat_deal(content_json, node_dict):
                if 'id' in content_json:
                    if content_json['id'] in node_dict:
                        node_name = content_json['id']
                        content_json['id'] += f"({node_dict[content_json['id']]})"
                        node_dict[node_name] += 1
                    else:
                        node_dict[content_json['id']] = 1
                if 'children' in content_json and content_json['children']:
                    for item in content_json['children']:
                        repeat_deal(item, node_dict)

            repeat_deal(content_json, node_dict)

        obj[ty] = content_json

    return get_json_result(data=obj)
