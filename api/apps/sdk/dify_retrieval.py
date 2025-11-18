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

from quart import request, jsonify

from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.utils.api_utils import validate_request, build_error_result, apikey_required
from rag.app.tag import label_question
from api.db.services.dialog_service import meta_filter, convert_conditions
from common.constants import RetCode, LLMType
from common import settings

@manager.route('/dify/retrieval', methods=['POST'])  # noqa: F821
@apikey_required
@validate_request("knowledge_id", "query")
async def retrieval(tenant_id):
    """
    Dify-compatible retrieval API
    ---
    tags:
      - SDK
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        required: true
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
    req = await request.json
    question = req["query"]
    kb_id = req["knowledge_id"]
    use_kg = req.get("use_kg", False)
    retrieval_setting = req.get("retrieval_setting", {})
    similarity_threshold = float(retrieval_setting.get("score_threshold", 0.0))
    top = int(retrieval_setting.get("top_k", 1024))
    metadata_condition = req.get("metadata_condition", {})
    metas = DocumentService.get_meta_by_kbs([kb_id])

    doc_ids = []
    try:

        e, kb = KnowledgebaseService.get_by_id(kb_id)
        if not e:
            return build_error_result(message="Knowledgebase not found!", code=RetCode.NOT_FOUND)

        embd_mdl = LLMBundle(kb.tenant_id, LLMType.EMBEDDING.value, llm_name=kb.embd_id)
        if metadata_condition:
            doc_ids.extend(meta_filter(metas, convert_conditions(metadata_condition)))
        if not doc_ids and metadata_condition:
            doc_ids = ["-999"]
        ranks = settings.retriever.retrieval(
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

        if use_kg:
            ck = settings.kg_retriever.retrieval(question,
                                                 [tenant_id],
                                                 [kb_id],
                                                 embd_mdl,
                                                 LLMBundle(kb.tenant_id, LLMType.CHAT))
            if ck["content_with_weight"]:
                ranks["chunks"].insert(0, ck)

        records = []
        for c in ranks["chunks"]:
            e, doc = DocumentService.get_by_id(c["doc_id"])
            c.pop("vector", None)
            meta = getattr(doc, 'meta_fields', {})
            meta["doc_id"] = c["doc_id"]
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
