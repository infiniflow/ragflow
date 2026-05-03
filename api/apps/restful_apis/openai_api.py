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
import time

from quart import Response, jsonify

from api.apps import current_user, login_required
from api.db.services.dialog_service import DialogService, async_chat
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.tenant_llm_service import TenantLLMService
from api.utils.api_utils import get_error_data_result, get_request_json, validate_request
from common.constants import RetCode, StatusEnum
from common.metadata_utils import convert_conditions, meta_filter
from common.token_utils import num_tokens_from_string
from rag.prompts.generator import chunks_format

def _validate_llm_id(llm_id, tenant_id, llm_setting=None):
    if not llm_id:
        return None

    llm_name, llm_factory = TenantLLMService.split_model_name_and_factory(llm_id)
    model_type = (llm_setting or {}).get("model_type")
    if model_type not in {"chat", "image2text"}:
        model_type = "chat"

    if not TenantLLMService.query(
        tenant_id=tenant_id,
        llm_name=llm_name,
        llm_factory=llm_factory,
        model_type=model_type,
    ):
        return f"`llm_id` {llm_id} doesn't exist"
    return None


import logging
from api.utils.reference_metadata_utils import enrich_chunks_with_document_metadata

def _build_reference_chunks(reference, include_metadata=False, metadata_fields=None):
    chunks = chunks_format(reference)
    if not include_metadata:
        logging.debug("Skipping document metadata enrichment (include_metadata=False)")
        return chunks

    normalized_fields = None
    if metadata_fields is not None:
        if not isinstance(metadata_fields, list):
            return chunks
        normalized_fields = {f for f in metadata_fields if isinstance(f, str)}
        if not normalized_fields:
            return chunks

    logging.debug(
        "Enriching %d chunks with document metadata (fields: %s)",
        len(chunks),
        "ALL" if normalized_fields is None else list(normalized_fields),
    )

    enrich_chunks_with_document_metadata(
        chunks,
        normalized_fields,
        kb_field="dataset_id",
        doc_field="document_id",
    )

    return chunks


def _build_sse_response(body):
    resp = Response(body, mimetype="text/event-stream")
    resp.headers.add_header("Cache-control", "no-cache")
    resp.headers.add_header("Connection", "keep-alive")
    resp.headers.add_header("X-Accel-Buffering", "no")
    resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
    return resp


@manager.route("/openai/<chat_id>/chat/completions", methods=["POST"])  # noqa: F821
@login_required
@validate_request("model", "messages")
async def openai_chat_completions(chat_id):
    req = await get_request_json()

    extra_body = req.get("extra_body") or {}
    if extra_body and not isinstance(extra_body, dict):
        return get_error_data_result("extra_body must be an object.")

    need_reference = bool(extra_body.get("reference", False))
    reference_metadata = extra_body.get("reference_metadata") or {}
    if reference_metadata and not isinstance(reference_metadata, dict):
        return get_error_data_result("reference_metadata must be an object.")
    include_reference_metadata = bool(reference_metadata.get("include", False))
    metadata_fields = reference_metadata.get("fields")
    if metadata_fields is not None and not isinstance(metadata_fields, list):
        return get_error_data_result("reference_metadata.fields must be an array.")

    messages = req.get("messages", [])
    if len(messages) < 1:
        return get_error_data_result("You have to provide messages.")
    if messages[-1]["role"] != "user":
        return get_error_data_result("The last content of this conversation is not from user.")

    prompt = messages[-1]["content"]
    context_token_used = sum(num_tokens_from_string(message["content"]) for message in messages)
    requested_model = req.get("model", "") or ""
    completion_id = f"chatcmpl-{chat_id}"

    dia = DialogService.query(tenant_id=current_user.id, id=chat_id, status=StatusEnum.VALID.value)
    if not dia:
        return get_error_data_result(f"You don't own the chat {chat_id}")
    dia = dia[0]

    using_placeholder_model = requested_model == "model"
    if using_placeholder_model:
        requested_model = dia.llm_id or requested_model
    else:
        llm_id_error = _validate_llm_id(requested_model, current_user.id, {"model_type": "chat"})
        if llm_id_error:
            return get_error_data_result(message=llm_id_error, code=RetCode.ARGUMENT_ERROR)
        dia.llm_id = requested_model
        if not TenantLLMService.get_api_key(tenant_id=dia.tenant_id, model_name=requested_model):
            return get_error_data_result(message=f"Cannot use specified model {requested_model}.")

    metadata_condition = extra_body.get("metadata_condition") or {}
    if metadata_condition and not isinstance(metadata_condition, dict):
        return get_error_data_result(message="metadata_condition must be an object.")

    doc_ids_str = None
    if metadata_condition:
        metas = DocMetadataService.get_flatted_meta_by_kbs(dia.kb_ids or [])
        filtered_doc_ids = meta_filter(
            metas,
            convert_conditions(metadata_condition),
            metadata_condition.get("logic", "and"),
        )
        if metadata_condition.get("conditions") and not filtered_doc_ids:
            filtered_doc_ids = ["-999"]
        doc_ids_str = ",".join(filtered_doc_ids) if filtered_doc_ids else None

    msg = []
    for message in messages:
        if message["role"] == "system":
            continue
        if message["role"] == "assistant" and not msg:
            continue
        msg.append(message)

    tools = None
    toolcall_session = None
    stream_mode = req.get("stream", True)

    if stream_mode:
        async def streamed_response_generator():
            token_used = 0
            last_ans = {}
            full_content = ""
            final_answer = None
            final_reference = None
            in_think = False
            response = {
                "id": completion_id,
                "choices": [
                    {
                        "delta": {
                            "content": "",
                            "role": "assistant",
                            "function_call": None,
                            "tool_calls": None,
                            "reasoning_content": "",
                        },
                        "finish_reason": None,
                        "index": 0,
                        "logprobs": None,
                    }
                ],
                "created": int(time.time()),
                "model": requested_model,
                "object": "chat.completion.chunk",
                "system_fingerprint": "",
                "usage": None,
            }

            try:
                chat_kwargs = {"toolcall_session": toolcall_session, "tools": tools, "quote": need_reference}
                if doc_ids_str:
                    chat_kwargs["doc_ids"] = doc_ids_str
                async for ans in async_chat(dia, msg, True, **chat_kwargs):
                    last_ans = ans
                    if ans.get("final"):
                        if ans.get("answer"):
                            full_content = ans["answer"]
                            response["choices"][0]["delta"]["content"] = full_content
                            response["choices"][0]["delta"]["reasoning_content"] = None
                            yield f"data:{json.dumps(response, ensure_ascii=False)}\n\n"
                        final_answer = full_content
                        final_reference = ans.get("reference", {})
                        continue
                    if ans.get("start_to_think"):
                        in_think = True
                        continue
                    if ans.get("end_to_think"):
                        in_think = False
                        continue
                    delta = ans.get("answer") or ""
                    if not delta:
                        continue
                    token_used += num_tokens_from_string(delta)
                    if in_think:
                        response["choices"][0]["delta"]["reasoning_content"] = delta
                        response["choices"][0]["delta"]["content"] = None
                    else:
                        full_content += delta
                        response["choices"][0]["delta"]["content"] = delta
                        response["choices"][0]["delta"]["reasoning_content"] = None
                    yield f"data:{json.dumps(response, ensure_ascii=False)}\n\n"
            except Exception as e:
                response["choices"][0]["delta"]["content"] = "**ERROR**: " + str(e)
                yield f"data:{json.dumps(response, ensure_ascii=False)}\n\n"

            response["choices"][0]["delta"]["content"] = None
            response["choices"][0]["delta"]["reasoning_content"] = None
            response["choices"][0]["finish_reason"] = "stop"
            prompt_tokens = num_tokens_from_string(prompt)
            response["usage"] = {
                "prompt_tokens": prompt_tokens,
                "completion_tokens": token_used,
                "total_tokens": prompt_tokens + token_used,
            }
            if need_reference:
                reference_payload = final_reference if final_reference is not None else last_ans.get("reference", [])
                response["choices"][0]["delta"]["reference"] = _build_reference_chunks(
                    reference_payload,
                    include_metadata=include_reference_metadata,
                    metadata_fields=metadata_fields,
                )
                response["choices"][0]["delta"]["final_content"] = final_answer if final_answer is not None else full_content
            yield f"data:{json.dumps(response, ensure_ascii=False)}\n\n"
            yield "data:[DONE]\n\n"

        return _build_sse_response(streamed_response_generator())

    answer = None
    chat_kwargs = {"toolcall_session": toolcall_session, "tools": tools, "quote": need_reference}
    if doc_ids_str:
        chat_kwargs["doc_ids"] = doc_ids_str
    async for ans in async_chat(dia, msg, False, **chat_kwargs):
        answer = ans
        break

    content = answer["answer"]
    response = {
        "id": completion_id,
        "object": "chat.completion",
        "created": int(time.time()),
        "model": requested_model,
        "usage": {
            "prompt_tokens": num_tokens_from_string(prompt),
            "completion_tokens": num_tokens_from_string(content),
            "total_tokens": num_tokens_from_string(prompt) + num_tokens_from_string(content),
            "completion_tokens_details": {
                "reasoning_tokens": context_token_used,
                "accepted_prediction_tokens": num_tokens_from_string(content),
                "rejected_prediction_tokens": 0,
            },
        },
        "choices": [
            {
                "message": {
                    "role": "assistant",
                    "content": content,
                },
                "logprobs": None,
                "finish_reason": "stop",
                "index": 0,
            }
        ],
    }
    if need_reference:
        response["choices"][0]["message"]["reference"] = _build_reference_chunks(
            answer.get("reference", {}),
            include_metadata=include_reference_metadata,
            metadata_fields=metadata_fields,
        )

    return jsonify(response)
