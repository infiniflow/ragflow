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

from quart import jsonify, request
from werkzeug.exceptions import BadRequest as WerkzeugBadRequest

try:
    from quart.exceptions import BadRequest as QuartBadRequest
except ImportError:  # pragma: no cover - optional dependency
    QuartBadRequest = None

from api.db.services.document_service import DocumentService
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.db.joint_services.tenant_model_service import get_model_config_by_id, get_model_config_by_type_and_name, get_tenant_default_model_by_type
from common.metadata_utils import meta_filter, convert_conditions
from api.utils.api_utils import apikey_required, build_error_result, get_request_json
from rag.app.tag import label_question
from common.constants import RetCode, LLMType
from common import settings

logger = logging.getLogger(__name__)


async def _read_retrieval_request():
    try:
        method = request.method
    except RuntimeError:
        # Unit tests may call the handler directly without a request context.
        method = "POST"
    if method == "GET":
        query_args = request.args
        retrieval_setting = {}
        knowledge_id = query_args.get("knowledge_id")
        query = query_args.get("query")
        use_kg = str(query_args.get("use_kg", "")).lower() in {"1", "true", "yes", "on"}
        top_k = query_args.get("top_k")
        score_threshold = query_args.get("score_threshold")
        try:
            if top_k not in (None, ""):
                retrieval_setting["top_k"] = int(top_k)
            if score_threshold not in (None, ""):
                retrieval_setting["score_threshold"] = float(score_threshold)
        except (TypeError, ValueError):
            raise ValueError("top_k must be integer and score_threshold must be numeric")
        safe_query = f"len={len(query)}" if isinstance(query, str) else "len=0"
        logger.debug(
            "Dify retrieval GET normalization: knowledge_id=%s query=%s use_kg=%s top_k=%s score_threshold=%s",
            knowledge_id,
            safe_query,
            use_kg,
            retrieval_setting.get("top_k"),
            retrieval_setting.get("score_threshold"),
        )

        req = {
            "knowledge_id": knowledge_id,
            "query": query,
            "use_kg": use_kg,
            "retrieval_setting": retrieval_setting,
        }
        return req
    return await get_request_json()


def _parse_retrieval_options(retrieval_setting):
    if retrieval_setting is None:
        retrieval_setting = {}
    if not isinstance(retrieval_setting, dict):
        raise ValueError("retrieval_setting must be an object")
    try:
        similarity_threshold = float(retrieval_setting.get("score_threshold", 0.0))
        top = int(retrieval_setting.get("top_k", 1024))
    except (TypeError, ValueError):
        raise ValueError("top_k must be integer and score_threshold must be numeric")
    return retrieval_setting, similarity_threshold, top


@manager.route('/dify/retrieval', methods=['POST', 'GET'])  # noqa: F821
@apikey_required
async def retrieval(tenant_id):
    """
    Dify-compatible retrieval API
    ---
    tags:
      - SDK
    security:
      - ApiKeyAuth: []
    parameters:
      - in: query
        name: knowledge_id
        required: false
        type: string
        description: Knowledge base ID (for GET requests)
      - in: query
        name: query
        required: false
        type: string
        description: Query text (for GET requests)
      - in: query
        name: use_kg
        required: false
        type: boolean
        description: Whether to use knowledge graph (for GET requests)
      - in: query
        name: top_k
        required: false
        type: integer
        description: Number of results to return (for GET requests)
      - in: query
        name: score_threshold
        required: false
        type: number
        description: Similarity threshold (for GET requests)
      - in: body
        name: body
        required: false
        schema:
          type: object
          required:
            - knowledge_id
            - query
          properties:
            knowledge_id:
              type: string
              description: Knowledge base ID
            query:
              type: string
              description: Query text
            use_kg:
              type: boolean
              description: Whether to use knowledge graph
              default: false
            retrieval_setting:
              type: object
              description: Retrieval configuration
              properties:
                score_threshold:
                  type: number
                  description: Similarity threshold
                  default: 0.0
                top_k:
                  type: integer
                  description: Number of results to return
                  default: 1024
            metadata_condition:
              type: object
              description: Metadata filter condition
              properties:
                conditions:
                  type: array
                  items:
                    type: object
                    properties:
                      name:
                        type: string
                        description: Field name
                      comparison_operator:
                        type: string
                        description: Comparison operator
                      value:
                        type: string
                        description: Field value
    responses:
      200:
        description: Retrieval succeeded
        schema:
          type: object
          properties:
            records:
              type: array
              items:
                type: object
                properties:
                  content:
                    type: string
                    description: Content text
                  score:
                    type: number
                    description: Similarity score
                  title:
                    type: string
                    description: Document title
                  metadata:
                    type: object
                    description: Metadata info
      404:
        description: Knowledge base or document not found
    """
    parse_exception_types = (AttributeError, TypeError, ValueError, WerkzeugBadRequest)
    if QuartBadRequest is not None:
        parse_exception_types = parse_exception_types + (QuartBadRequest,)
    try:
        req = await _read_retrieval_request()
    except parse_exception_types as e:
        return build_error_result(
            message=f"invalid or malformed arguments: {str(e)}; ",
            code=RetCode.ARGUMENT_ERROR,
        )
    missing = [field for field in ("knowledge_id", "query") if not req.get(field)]
    if missing:
        return build_error_result(
            message=f"required arguments are missing: {','.join(missing)}; ",
            code=RetCode.ARGUMENT_ERROR,
        )
    question = req["query"]
    kb_id = req["knowledge_id"]
    use_kg = req.get("use_kg", False)
    try:
        retrieval_setting, similarity_threshold, top = _parse_retrieval_options(req.get("retrieval_setting", {}))
    except ValueError as e:
        return build_error_result(
            message=f"invalid or malformed arguments: {str(e)}; ",
            code=RetCode.ARGUMENT_ERROR,
        )
    metadata_condition = req.get("metadata_condition", {}) or {}
    metas = DocMetadataService.get_flatted_meta_by_kbs([kb_id])

    doc_ids = []
    try:

        e, kb = KnowledgebaseService.get_by_id(kb_id)
        if not e:
            return build_error_result(message="Knowledgebase not found!", code=RetCode.NOT_FOUND)
        if kb.tenant_embd_id:
            model_config = get_model_config_by_id(kb.tenant_embd_id)
        else:
            model_config = get_model_config_by_type_and_name(kb.tenant_id, LLMType.EMBEDDING, kb.embd_id)
        embd_mdl = LLMBundle(kb.tenant_id, model_config)
        if metadata_condition:
            doc_ids.extend(meta_filter(metas, convert_conditions(metadata_condition), metadata_condition.get("logic", "and")))
        if not doc_ids and metadata_condition:
            doc_ids = ["-999"]
        ranks = await settings.retriever.retrieval(
            question,
            embd_mdl,
            kb.tenant_id,
            [kb_id],
            page=1,
            page_size=top,
            similarity_threshold=similarity_threshold,
            vector_similarity_weight=0.3,
            top=top,
            doc_ids=doc_ids,
            rank_feature=label_question(question, [kb])
        )
        ranks["chunks"] = settings.retriever.retrieval_by_children(ranks["chunks"], [tenant_id])

        if use_kg:
            model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.CHAT)
            ck = await settings.kg_retriever.retrieval(question,
                                                 [tenant_id],
                                                 [kb_id],
                                                 embd_mdl,
                                                 LLMBundle(kb.tenant_id, model_config))
            if ck["content_with_weight"]:
                ranks["chunks"].insert(0, ck)

        records = []
        for c in ranks["chunks"]:
            e, doc = DocumentService.get_by_id(c["doc_id"])
            c.pop("vector", None)
            meta = getattr(doc, 'meta_fields', {})
            meta["doc_id"] = c["doc_id"]
            # Dify expects metadata.document_id for external retrieval sources.
            meta["document_id"] = c["doc_id"]
            records.append({
                "content": c["content_with_weight"],
                "score": c["similarity"],
                "title": c["docnm_kwd"],
                "metadata": meta
            })

        return jsonify({"records": records})
    except Exception as e:
        if str(e).find("not_found") > 0:
            return build_error_result(
                message='No chunk found! Check the chunk status please!',
                code=RetCode.NOT_FOUND
            )
        logging.exception(e)
        return build_error_result(message=str(e), code=RetCode.SERVER_ERROR)
