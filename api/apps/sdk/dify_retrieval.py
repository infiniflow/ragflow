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
from typing import Any

from pydantic import BaseModel, ConfigDict, Field
from quart import jsonify
from quart_schema import DataSource, document_request, document_response, tag

from api.db.services.document_service import DocumentService
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.db.joint_services.tenant_model_service import get_model_config_by_id, get_model_config_by_type_and_name, get_tenant_default_model_by_type
from common.metadata_utils import meta_filter, convert_conditions
from api.utils.api_utils import apikey_required, build_error_result, get_request_json, validate_request
from rag.app.tag import label_question
from common.constants import RetCode, LLMType
from common import settings


class ErrorResponse(BaseModel):
    code: int = Field(description="Response code.")
    message: str = Field(description="Response message.")
    data: Any | None = Field(default=None, description="Response payload.")


class DifyRetrievalSetting(BaseModel):
    model_config = ConfigDict(extra="allow")

    score_threshold: float | None = Field(default=0.0, description="Similarity threshold.")
    top_k: int | None = Field(default=1024, description="Maximum number of records to return.")


class DifyMetadataCondition(BaseModel):
    model_config = ConfigDict(extra="allow")

    logic: str | None = Field(default=None, description="Condition join logic, for example `and` or `or`.")
    conditions: list[dict[str, Any]] | None = Field(default=None, description="Metadata condition list.")


class DifyRetrievalBody(BaseModel):
    model_config = ConfigDict(
        extra="allow",
        json_schema_extra={
            "example": {
                "knowledge_id": "dataset_id_123",
                "query": "What does this dataset contain?",
                "use_kg": False,
                "retrieval_setting": {"score_threshold": 0.0, "top_k": 10},
            }
        },
    )

    knowledge_id: str = Field(description="Knowledge base (dataset) ID.")
    query: str = Field(description="User query text.")
    use_kg: bool | None = Field(default=False, description="Whether to include knowledge graph retrieval.")
    retrieval_setting: DifyRetrievalSetting | None = Field(default=None, description="Retrieval configuration.")
    metadata_condition: DifyMetadataCondition | dict[str, Any] | None = Field(default=None, description="Optional metadata filter condition.")


class DifyRecord(BaseModel):
    model_config = ConfigDict(extra="allow")

    content: str = Field(description="Retrieved content text.")
    score: float | None = Field(default=None, description="Similarity score.")
    title: str | None = Field(default=None, description="Document title.")
    metadata: dict[str, Any] = Field(description="Record metadata.")


class DifyRetrievalResponse(BaseModel):
    records: list[DifyRecord] = Field(description="Retrieved records.")


@manager.route('/dify/retrieval', methods=['POST'])  # noqa: F821
@apikey_required
@validate_request("knowledge_id", "query")
@tag(["SDK Dify Retrieval"])
@document_request(DifyRetrievalBody, source=DataSource.JSON)
@document_response(DifyRetrievalResponse)
@document_response(ErrorResponse, 404)
async def retrieval(tenant_id):
    """Run a Dify-compatible retrieval request against a dataset."""
    req = await get_request_json()
    question = req["query"]
    kb_id = req["knowledge_id"]
    use_kg = req.get("use_kg", False)
    retrieval_setting = req.get("retrieval_setting", {})
    similarity_threshold = float(retrieval_setting.get("score_threshold", 0.0))
    top = int(retrieval_setting.get("top_k", 1024))
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
