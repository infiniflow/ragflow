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
from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type, resolve_model_config
from common.metadata_utils import convert_conditions
from common.temporal_retrieval import resolve_temporal_retrieval_context
from api.apps import login_required
from api.utils.api_utils import add_tenant_id_to_kwargs, build_error_result, get_request_json, get_json_result
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
    req = await get_request_json()
    knowledge_id = req.get("knowledge_id") if isinstance(req, dict) else None
    query = req.get("query") if isinstance(req, dict) else None
    use_kg = req.get("use_kg", False) if isinstance(req, dict) else False
    retrieval_setting = req.get("retrieval_setting", {}) if isinstance(req, dict) else {}
    if not isinstance(retrieval_setting, dict):
        retrieval_setting = {}
    safe_query = f"len={len(query)}" if isinstance(query, str) else "len=0"
    logger.debug(
        "Dify retrieval GET normalization: knowledge_id=%s query=%s use_kg=%s top_k=%s score_threshold=%s",
        knowledge_id,
        safe_query,
        use_kg,
        retrieval_setting.get("top_k"),
        retrieval_setting.get("score_threshold"),
    )
    return req


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


@manager.route("/dify/retrieval", methods=["POST", "GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
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
        _, similarity_threshold, top = _parse_retrieval_options(req.get("retrieval_setting", {}))
    except ValueError as e:
        return build_error_result(
            message=f"invalid or malformed arguments: {str(e)}; ",
            code=RetCode.ARGUMENT_ERROR,
        )
    metadata_condition = req.get("metadata_condition")
    temporal_retrieval = req.get("temporal_retrieval")
    if metadata_condition is not None and not isinstance(metadata_condition, dict):
        return build_error_result(
            message="metadata_condition must be an object.; ",
            code=RetCode.ARGUMENT_ERROR,
        )
    if temporal_retrieval is not None and not isinstance(temporal_retrieval, dict):
        return build_error_result(
            message="temporal_retrieval must be an object.; ",
            code=RetCode.ARGUMENT_ERROR,
        )
    from common.temporal_validation import validate_temporal_retrieval_config

    temporal_err = validate_temporal_retrieval_config(temporal_retrieval)
    if temporal_err:
        return build_error_result(message=f"{temporal_err}; ", code=RetCode.ARGUMENT_ERROR)
    metadata_condition = metadata_condition or {}
    temporal_retrieval = temporal_retrieval or {}
    meta_data_filter = {}
    if metadata_condition:
        meta_data_filter = {
            "method": "manual",
            "manual": convert_conditions(metadata_condition),
            "logic": metadata_condition.get("logic", "and"),
        }

    doc_ids = []
    try:
        e, kb = KnowledgebaseService.get_by_id(kb_id)
        if not e:
            return build_error_result(message="Knowledgebase not found!", code=RetCode.NOT_FOUND)
        if not KnowledgebaseService.accessible(kb_id, tenant_id):
            logger.warning(
                "Rejected /dify/retrieval cross-tenant access: caller_tenant=%s knowledge_id=%s",
                tenant_id,
                kb_id,
            )
            return build_error_result(message="No authorization.", code=RetCode.AUTHENTICATION_ERROR)
        model_config = resolve_model_config(kb.tenant_id, LLMType.EMBEDDING, kb.embd_id)
        embd_mdl = LLMBundle(kb.tenant_id, model_config)
        temporal_ctx = await resolve_temporal_retrieval_context(
            raw_query=question,
            refined_query=question,
            retrieval_query=question,
            meta_data_filter=meta_data_filter,
            temporal_retrieval=temporal_retrieval,
            kb_ids=[kb_id],
            base_doc_ids=doc_ids,
            metas_loader=lambda: DocMetadataService.get_flatted_meta_by_kbs([kb_id]),
        )
        doc_ids = temporal_ctx.doc_ids
        if metadata_condition.get("conditions") and doc_ids == ["-999"]:
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
            rank_feature=label_question(question, [kb]),
            temporal_rank_policy=temporal_ctx.temporal_rank_policy,
        )
        ranks["chunks"] = settings.retriever.retrieval_by_children(ranks["chunks"], [tenant_id])

        if use_kg:
            model_config = get_tenant_default_model_by_type(kb.tenant_id, LLMType.CHAT)
            ck = await settings.kg_retriever.retrieval(question, [tenant_id], [kb_id], embd_mdl, LLMBundle(kb.tenant_id, model_config))
            if ck["content_with_weight"]:
                ranks["chunks"].insert(0, ck)

        doc_ids = list(set([c["doc_id"] for c in ranks["chunks"]]))
        docs = DocumentService.get_by_ids(doc_ids)
        doc_map = {doc.id: doc for doc in docs}

        records = []
        for c in ranks["chunks"]:
            doc = doc_map.get(c["doc_id"])
            if not doc:
                continue
            c.pop("vector", None)
            meta = getattr(doc, "meta_fields", {})
            meta["doc_id"] = c["doc_id"]
            # Dify expects metadata.document_id for external retrieval sources.
            meta["document_id"] = c["doc_id"]
            records.append({"content": c["content_with_weight"], "score": c["similarity"], "title": c["docnm_kwd"], "metadata": meta})

        return jsonify({"records": records})
    except Exception as e:
        if "not_found" in str(e):
            return build_error_result(message="No chunk found! Check the chunk status please!", code=RetCode.NOT_FOUND)
        logger.exception(e)
        return build_error_result(message=str(e), code=RetCode.SERVER_ERROR)


@manager.route("/dify/retrieval/health", methods=["GET"])  # noqa: F821
async def retrieval_health_check():
    """Health check endpoint for Dify external knowledge base connectivity verification."""
    return get_json_result(data=True)
