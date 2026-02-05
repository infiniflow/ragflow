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
import copy
import re
import time

import os
import tempfile
import logging

from quart import Response, jsonify, request

from common.token_utils import num_tokens_from_string

from agent.canvas import Canvas
from api.db.db_models import APIToken
from api.db.services.api_service import API4ConversationService
from api.db.services.canvas_service import UserCanvasService, completion_openai
from api.db.services.canvas_service import completion as agent_completion
from api.db.services.conversation_service import ConversationService
from api.db.services.conversation_service import async_iframe_completion as iframe_completion
from api.db.services.conversation_service import async_completion as rag_completion
from api.db.services.dialog_service import DialogService, async_ask, async_chat, gen_mindmap
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from common.metadata_utils import apply_meta_data_filter, convert_conditions, meta_filter
from api.db.services.search_service import SearchService
from api.db.services.user_service import TenantService,UserTenantService
from common.misc_utils import get_uuid
from api.utils.api_utils import check_duplicate_ids, get_data_openai, get_error_data_result, get_json_result, \
    get_result, get_request_json, server_error_response, token_required, validate_request
from rag.app.tag import label_question
from rag.prompts.template import load_prompt
from rag.prompts.generator import cross_languages, keyword_extraction, chunks_format
from common.constants import RetCode, LLMType, StatusEnum
from common import settings


@manager.route("/chats/<chat_id>/sessions", methods=["POST"])  # noqa: F821
@token_required
async def create(tenant_id, chat_id):
    req = await get_request_json()
    req["dialog_id"] = chat_id
    dia = DialogService.query(tenant_id=tenant_id, id=req["dialog_id"], status=StatusEnum.VALID.value)
    if not dia:
        return get_error_data_result(message="You do not own the assistant.")
    conv = {
        "id": get_uuid(),
        "dialog_id": req["dialog_id"],
        "name": req.get("name", "New session"),
        "message": [{"role": "assistant", "content": dia[0].prompt_config.get("prologue")}],
        "user_id": req.get("user_id", ""),
        "reference": [],
    }
    if not conv.get("name"):
        return get_error_data_result(message="`name` can not be empty.")
    ConversationService.save(**conv)
    e, conv = ConversationService.get_by_id(conv["id"])
    if not e:
        return get_error_data_result(message="Fail to create a session!")
    conv = conv.to_dict()
    conv["messages"] = conv.pop("message")
    conv["chat_id"] = conv.pop("dialog_id")
    del conv["reference"]
    return get_result(data=conv)


@manager.route("/agents/<agent_id>/sessions", methods=["POST"])  # noqa: F821
@token_required
async def create_agent_session(tenant_id, agent_id):
    user_id = request.args.get("user_id", tenant_id)
    e, cvs = UserCanvasService.get_by_id(agent_id)
    if not e:
        return get_error_data_result("Agent not found.")
    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        return get_error_data_result("You cannot access the agent.")
    if not isinstance(cvs.dsl, str):
        cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)

    session_id = get_uuid()
    canvas = Canvas(cvs.dsl, tenant_id, agent_id, canvas_id=cvs.id)
    canvas.reset()

    cvs.dsl = json.loads(str(canvas))
    conv = {"id": session_id, "dialog_id": cvs.id, "user_id": user_id,
            "message": [{"role": "assistant", "content": canvas.get_prologue()}], "source": "agent", "dsl": cvs.dsl}
    API4ConversationService.save(**conv)
    conv["agent_id"] = conv.pop("dialog_id")
    return get_result(data=conv)


@manager.route("/chats/<chat_id>/sessions/<session_id>", methods=["PUT"])  # noqa: F821
@token_required
async def update(tenant_id, chat_id, session_id):
    req = await get_request_json()
    req["dialog_id"] = chat_id
    conv_id = session_id
    conv = ConversationService.query(id=conv_id, dialog_id=chat_id)
    if not conv:
        return get_error_data_result(message="Session does not exist")
    if not DialogService.query(id=chat_id, tenant_id=tenant_id, status=StatusEnum.VALID.value):
        return get_error_data_result(message="You do not own the session")
    if "message" in req or "messages" in req:
        return get_error_data_result(message="`message` can not be change")
    if "reference" in req:
        return get_error_data_result(message="`reference` can not be change")
    if "name" in req and not req.get("name"):
        return get_error_data_result(message="`name` can not be empty.")
    if not ConversationService.update_by_id(conv_id, req):
        return get_error_data_result(message="Session updates error")
    return get_result()


@manager.route("/chats/<chat_id>/completions", methods=["POST"])  # noqa: F821
@token_required
async def chat_completion(tenant_id, chat_id):
    req = await get_request_json()
    if not req:
        req = {"question": ""}
    if not req.get("session_id"):
        req["question"] = ""
    dia = DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value)
    if not dia:
        return get_error_data_result(f"You don't own the chat {chat_id}")
    dia = dia[0]
    if req.get("session_id"):
        if not ConversationService.query(id=req["session_id"], dialog_id=chat_id):
            return get_error_data_result(f"You don't own the session {req['session_id']}")

    metadata_condition = req.get("metadata_condition") or {}
    if metadata_condition and not isinstance(metadata_condition, dict):
        return get_error_data_result(message="metadata_condition must be an object.")

    if metadata_condition and req.get("question"):
        metas = DocMetadataService.get_flatted_meta_by_kbs(dia.kb_ids or [])
        filtered_doc_ids = meta_filter(
            metas,
            convert_conditions(metadata_condition),
            metadata_condition.get("logic", "and"),
        )
        if metadata_condition.get("conditions") and not filtered_doc_ids:
            filtered_doc_ids = ["-999"]

        if filtered_doc_ids:
            req["doc_ids"] = ",".join(filtered_doc_ids)
        else:
            req.pop("doc_ids", None)

    if req.get("stream", True):
        resp = Response(rag_completion(tenant_id, chat_id, **req), mimetype="text/event-stream")
        resp.headers.add_header("Cache-control", "no-cache")
        resp.headers.add_header("Connection", "keep-alive")
        resp.headers.add_header("X-Accel-Buffering", "no")
        resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")

        return resp
    else:
        answer = None
        async for ans in rag_completion(tenant_id, chat_id, **req):
            answer = ans
            break
        return get_result(data=answer)


@manager.route("/chats_openai/<chat_id>/chat/completions", methods=["POST"])  # noqa: F821
@validate_request("model", "messages")  # noqa: F821
@token_required
async def chat_completion_openai_like(tenant_id, chat_id):
    """
    OpenAI-like chat completion API that simulates the behavior of OpenAI's completions endpoint.

    This function allows users to interact with a model and receive responses based on a series of historical messages.
    If `stream` is set to True (by default), the response will be streamed in chunks, mimicking the OpenAI-style API.
    Set `stream` to False explicitly, the response will be returned in a single complete answer.

    Reference:

    - If `stream` is True, the final answer and reference information will appear in the **last chunk** of the stream.
    - If `stream` is False, the reference will be included in `choices[0].message.reference`.
    - If `extra_body.reference_metadata.include` is True, each reference chunk may include `document_metadata` in both streaming and non-streaming responses.

    Example usage:

    curl -X POST https://ragflow_address.com/api/v1/chats_openai/<chat_id>/chat/completions \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer $RAGFLOW_API_KEY" \
        -d '{
            "model": "model",
            "messages": [{"role": "user", "content": "Say this is a test!"}],
            "stream": true
        }'

    Alternatively, you can use Python's `OpenAI` client:

    NOTE: Streaming via `client.chat.completions.create(stream=True, ...)` does
    not return `reference` currently. The only way to return `reference` is
    non-stream mode with `with_raw_response`.

    from openai import OpenAI
    import json

    model = "model"
    client = OpenAI(api_key="ragflow-api-key", base_url=f"http://ragflow_address/api/v1/chats_openai/<chat_id>")

    stream = True
    reference = True

    request_kwargs = dict(
        model="model",
        messages=[
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Who are you?"},
            {"role": "assistant", "content": "I am an AI assistant named..."},
            {"role": "user", "content": "Can you tell me how to install neovim"},
        ],
        extra_body={
            "reference": reference,
            "reference_metadata": {
                "include": True,
                "fields": ["author", "year", "source"],
            },
            "metadata_condition": {
                "logic": "and",
                "conditions": [
                    {
                        "name": "author",
                        "comparison_operator": "is",
                        "value": "bob"
                    }
                ]
            }
        },
    )

    if stream:
        completion = client.chat.completions.create(stream=True, **request_kwargs)
        for chunk in completion:
            print(chunk)
    else:
        resp = client.chat.completions.with_raw_response.create(
            stream=False, **request_kwargs
        )
        print("status:", resp.http_response.status_code)
        raw_text = resp.http_response.text
        print("raw:", raw_text)

        data = json.loads(raw_text)
        print("assistant:", data["choices"][0]["message"].get("content"))
        print("reference:", data["choices"][0]["message"].get("reference"))

    """
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
    # To prevent empty [] input
    if len(messages) < 1:
        return get_error_data_result("You have to provide messages.")
    if messages[-1]["role"] != "user":
        return get_error_data_result("The last content of this conversation is not from user.")

    prompt = messages[-1]["content"]
    # Treat context tokens as reasoning tokens
    context_token_used = sum(num_tokens_from_string(message["content"]) for message in messages)

    dia = DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value)
    if not dia:
        return get_error_data_result(f"You don't own the chat {chat_id}")
    dia = dia[0]

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

    # Filter system and non-sense assistant messages
    msg = []
    for m in messages:
        if m["role"] == "system":
            continue
        if m["role"] == "assistant" and not msg:
            continue
        msg.append(m)

    # tools = get_tools()
    # toolcall_session = SimpleFunctionCallServer()
    tools = None
    toolcall_session = None

    if req.get("stream", True):
        # The value for the usage field on all chunks except for the last one will be null.
        # The usage field on the last chunk contains token usage statistics for the entire request.
        # The choices field on the last chunk will always be an empty array [].
        async def streamed_response_generator(chat_id, dia, msg):
            token_used = 0
            last_ans = {}
            full_content = ""
            full_reasoning = ""
            final_answer = None
            final_reference = None
            in_think = False
            response = {
                "id": f"chatcmpl-{chat_id}",
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
                "model": "model",
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
                        final_answer = ans.get("answer") or full_content
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
                        full_reasoning += delta
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

            # The last chunk
            response["choices"][0]["delta"]["content"] = None
            response["choices"][0]["delta"]["reasoning_content"] = None
            response["choices"][0]["finish_reason"] = "stop"
            prompt_tokens = num_tokens_from_string(prompt)
            response["usage"] = {"prompt_tokens": prompt_tokens, "completion_tokens": token_used, "total_tokens": prompt_tokens + token_used}
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

        resp = Response(streamed_response_generator(chat_id, dia, msg), mimetype="text/event-stream")
        resp.headers.add_header("Cache-control", "no-cache")
        resp.headers.add_header("Connection", "keep-alive")
        resp.headers.add_header("X-Accel-Buffering", "no")
        resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
        return resp
    else:
        answer = None
        chat_kwargs = {"toolcall_session": toolcall_session, "tools": tools, "quote": need_reference}
        if doc_ids_str:
            chat_kwargs["doc_ids"] = doc_ids_str
        async for ans in async_chat(dia, msg, False, **chat_kwargs):
            # focus answer content only
            answer = ans
            break
        content = answer["answer"]

        response = {
            "id": f"chatcmpl-{chat_id}",
            "object": "chat.completion",
            "created": int(time.time()),
            "model": req.get("model", ""),
            "usage": {
                "prompt_tokens": num_tokens_from_string(prompt),
                "completion_tokens": num_tokens_from_string(content),
                "total_tokens": num_tokens_from_string(prompt) + num_tokens_from_string(content),
                "completion_tokens_details": {
                    "reasoning_tokens": context_token_used,
                    "accepted_prediction_tokens": num_tokens_from_string(content),
                    "rejected_prediction_tokens": 0,  # 0 for simplicity
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


@manager.route("/agents_openai/<agent_id>/chat/completions", methods=["POST"])  # noqa: F821
@validate_request("model", "messages")  # noqa: F821
@token_required
async def agents_completion_openai_compatibility(tenant_id, agent_id):
    req = await get_request_json()
    messages = req.get("messages", [])
    if not messages:
        return get_error_data_result("You must provide at least one message.")
    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        return get_error_data_result(f"You don't own the agent {agent_id}")

    filtered_messages = [m for m in messages if m["role"] in ["user", "assistant"]]
    prompt_tokens = sum(num_tokens_from_string(m["content"]) for m in filtered_messages)
    if not filtered_messages:
        return jsonify(
            get_data_openai(
                id=agent_id,
                content="No valid messages found (user or assistant).",
                finish_reason="stop",
                model=req.get("model", ""),
                completion_tokens=num_tokens_from_string("No valid messages found (user or assistant)."),
                prompt_tokens=prompt_tokens,
            )
        )

    question = next((m["content"] for m in reversed(messages) if m["role"] == "user"), "")

    stream = req.pop("stream", False)
    if stream:
        resp = Response(
            completion_openai(
                tenant_id,
                agent_id,
                question,
                session_id=req.pop("session_id", req.get("id", "")) or req.get("metadata", {}).get("id", ""),
                stream=True,
                **req,
            ),
            mimetype="text/event-stream",
        )
        resp.headers.add_header("Cache-control", "no-cache")
        resp.headers.add_header("Connection", "keep-alive")
        resp.headers.add_header("X-Accel-Buffering", "no")
        resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
        return resp
    else:
        # For non-streaming, just return the response directly
        async for response in completion_openai(
                tenant_id,
                agent_id,
                question,
                session_id=req.pop("session_id", req.get("id", "")) or req.get("metadata", {}).get("id", ""),
                stream=False,
                **req,
            ):
            return jsonify(response)

        return None


@manager.route("/agents/<agent_id>/completions", methods=["POST"])  # noqa: F821
@token_required
async def agent_completions(tenant_id, agent_id):
    req = await get_request_json()
    return_trace = bool(req.get("return_trace", False))

    if req.get("stream", True):

        async def generate():
            trace_items = []
            async for answer in agent_completion(tenant_id=tenant_id, agent_id=agent_id, **req):
                if isinstance(answer, str):
                    try:
                        ans = json.loads(answer[5:])  # remove "data:"
                    except Exception:
                        continue

                event = ans.get("event")
                if event == "node_finished":
                    if return_trace:
                        data = ans.get("data", {})
                        trace_items.append(
                            {
                                "component_id": data.get("component_id"),
                                "trace": [copy.deepcopy(data)],
                            }
                        )
                        ans.setdefault("data", {})["trace"] = trace_items
                        answer = "data:" + json.dumps(ans, ensure_ascii=False) + "\n\n"
                    yield answer

                if event not in ["message", "message_end"]:
                    continue

                yield answer

            yield "data:[DONE]\n\n"

        resp = Response(generate(), mimetype="text/event-stream")
        resp.headers.add_header("Cache-control", "no-cache")
        resp.headers.add_header("Connection", "keep-alive")
        resp.headers.add_header("X-Accel-Buffering", "no")
        resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
        return resp

    full_content = ""
    reference = {}
    final_ans = ""
    trace_items = []
    async for answer in agent_completion(tenant_id=tenant_id, agent_id=agent_id, **req):
        try:
            ans = json.loads(answer[5:])

            if ans["event"] == "message":
                full_content += ans["data"]["content"]

            if ans.get("data", {}).get("reference", None):
                reference.update(ans["data"]["reference"])

            if return_trace and ans.get("event") == "node_finished":
                data = ans.get("data", {})
                trace_items.append(
                    {
                        "component_id": data.get("component_id"),
                        "trace": [copy.deepcopy(data)],
                    }
                )

            final_ans = ans
        except Exception as e:
            return get_result(data=f"**ERROR**: {str(e)}")
    final_ans["data"]["content"] = full_content
    final_ans["data"]["reference"] = reference
    if return_trace and final_ans:
        final_ans["data"]["trace"] = trace_items
    return get_result(data=final_ans)


@manager.route("/chats/<chat_id>/sessions", methods=["GET"])  # noqa: F821
@token_required
async def list_session(tenant_id, chat_id):
    if not DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value):
        return get_error_data_result(message=f"You don't own the assistant {chat_id}.")
    id = request.args.get("id")
    name = request.args.get("name")
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 30))
    orderby = request.args.get("orderby", "create_time")
    user_id = request.args.get("user_id")
    if request.args.get("desc") == "False" or request.args.get("desc") == "false":
        desc = False
    else:
        desc = True
    convs = ConversationService.get_list(chat_id, page_number, items_per_page, orderby, desc, id, name, user_id)
    if not convs:
        return get_result(data=[])
    for conv in convs:
        conv["messages"] = conv.pop("message")
        infos = conv["messages"]
        for info in infos:
            if "prompt" in info:
                info.pop("prompt")
        conv["chat_id"] = conv.pop("dialog_id")
        ref_messages = conv["reference"]
        if ref_messages:
            messages = conv["messages"]
            message_num = 0
            ref_num = 0
            while message_num < len(messages) and ref_num < len(ref_messages):
                if messages[message_num]["role"] != "user":
                    chunk_list = []
                    if "chunks" in ref_messages[ref_num]:
                        chunks = ref_messages[ref_num]["chunks"]
                        for chunk in chunks:
                            new_chunk = {
                                "id": chunk.get("chunk_id", chunk.get("id")),
                                "content": chunk.get("content_with_weight", chunk.get("content")),
                                "document_id": chunk.get("doc_id", chunk.get("document_id")),
                                "document_name": chunk.get("docnm_kwd", chunk.get("document_name")),
                                "dataset_id": chunk.get("kb_id", chunk.get("dataset_id")),
                                "image_id": chunk.get("image_id", chunk.get("img_id")),
                                "positions": chunk.get("positions", chunk.get("position_int")),
                            }

                            chunk_list.append(new_chunk)
                    messages[message_num]["reference"] = chunk_list
                    ref_num += 1
                message_num += 1
        del conv["reference"]
    return get_result(data=convs)


@manager.route("/agents/<agent_id>/sessions", methods=["GET"])  # noqa: F821
@token_required
async def list_agent_session(tenant_id, agent_id):
    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        return get_error_data_result(message=f"You don't own the agent {agent_id}.")
    id = request.args.get("id")
    user_id = request.args.get("user_id")
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 30))
    orderby = request.args.get("orderby", "update_time")
    if request.args.get("desc") == "False" or request.args.get("desc") == "false":
        desc = False
    else:
        desc = True
    # dsl defaults to True in all cases except for False and false
    include_dsl = request.args.get("dsl") != "False" and request.args.get("dsl") != "false"
    total, convs = API4ConversationService.get_list(agent_id, tenant_id, page_number, items_per_page, orderby, desc, id,
                                                    user_id, include_dsl)
    if not convs:
        return get_result(data=[])
    for conv in convs:
        conv["messages"] = conv.pop("message")
        infos = conv["messages"]
        for info in infos:
            if "prompt" in info:
                info.pop("prompt")
        conv["agent_id"] = conv.pop("dialog_id")
        # Fix for session listing endpoint
        if conv["reference"]:
            messages = conv["messages"]
            message_num = 0
            chunk_num = 0
            # Ensure reference is a list type to prevent KeyError
            if not isinstance(conv["reference"], list):
                conv["reference"] = []
            while message_num < len(messages):
                if message_num != 0 and messages[message_num]["role"] != "user":
                    chunk_list = []
                    # Add boundary and type checks to prevent KeyError
                    if chunk_num < len(conv["reference"]) and conv["reference"][chunk_num] is not None and isinstance(
                            conv["reference"][chunk_num], dict) and "chunks" in conv["reference"][chunk_num]:
                        chunks = conv["reference"][chunk_num]["chunks"]
                        for chunk in chunks:
                            # Ensure chunk is a dictionary before calling get method
                            if not isinstance(chunk, dict):
                                continue
                            new_chunk = {
                                "id": chunk.get("chunk_id", chunk.get("id")),
                                "content": chunk.get("content_with_weight", chunk.get("content")),
                                "document_id": chunk.get("doc_id", chunk.get("document_id")),
                                "document_name": chunk.get("docnm_kwd", chunk.get("document_name")),
                                "dataset_id": chunk.get("kb_id", chunk.get("dataset_id")),
                                "image_id": chunk.get("image_id", chunk.get("img_id")),
                                "positions": chunk.get("positions", chunk.get("position_int")),
                            }
                            chunk_list.append(new_chunk)
                    chunk_num += 1
                    messages[message_num]["reference"] = chunk_list
                message_num += 1
        del conv["reference"]
    return get_result(data=convs)


@manager.route("/chats/<chat_id>/sessions", methods=["DELETE"])  # noqa: F821
@token_required
async def delete(tenant_id, chat_id):
    if not DialogService.query(id=chat_id, tenant_id=tenant_id, status=StatusEnum.VALID.value):
        return get_error_data_result(message="You don't own the chat")

    errors = []
    success_count = 0
    req = await get_request_json()
    convs = ConversationService.query(dialog_id=chat_id)
    if not req:
        ids = None
    else:
        ids = req.get("ids")

    if not ids:
        conv_list = []
        for conv in convs:
            conv_list.append(conv.id)
    else:
        conv_list = ids

    unique_conv_ids, duplicate_messages = check_duplicate_ids(conv_list, "session")
    conv_list = unique_conv_ids

    for id in conv_list:
        conv = ConversationService.query(id=id, dialog_id=chat_id)
        if not conv:
            errors.append(f"The chat doesn't own the session {id}")
            continue
        ConversationService.delete_by_id(id)
        success_count += 1

    if errors:
        if success_count > 0:
            return get_result(data={"success_count": success_count, "errors": errors},
                              message=f"Partially deleted {success_count} sessions with {len(errors)} errors")
        else:
            return get_error_data_result(message="; ".join(errors))

    if duplicate_messages:
        if success_count > 0:
            return get_result(
                message=f"Partially deleted {success_count} sessions with {len(duplicate_messages)} errors",
                data={"success_count": success_count, "errors": duplicate_messages})
        else:
            return get_error_data_result(message=";".join(duplicate_messages))

    return get_result()


@manager.route("/agents/<agent_id>/sessions", methods=["DELETE"])  # noqa: F821
@token_required
async def delete_agent_session(tenant_id, agent_id):
    errors = []
    success_count = 0
    req = await get_request_json()
    cvs = UserCanvasService.query(user_id=tenant_id, id=agent_id)
    if not cvs:
        return get_error_data_result(f"You don't own the agent {agent_id}")

    convs = API4ConversationService.query(dialog_id=agent_id)
    if not convs:
        return get_error_data_result(f"Agent {agent_id} has no sessions")

    if not req:
        ids = None
    else:
        ids = req.get("ids")

    if not ids:
        conv_list = []
        for conv in convs:
            conv_list.append(conv.id)
    else:
        conv_list = ids

    unique_conv_ids, duplicate_messages = check_duplicate_ids(conv_list, "session")
    conv_list = unique_conv_ids

    for session_id in conv_list:
        conv = API4ConversationService.query(id=session_id, dialog_id=agent_id)
        if not conv:
            errors.append(f"The agent doesn't own the session {session_id}")
            continue
        API4ConversationService.delete_by_id(session_id)
        success_count += 1

    if errors:
        if success_count > 0:
            return get_result(data={"success_count": success_count, "errors": errors},
                              message=f"Partially deleted {success_count} sessions with {len(errors)} errors")
        else:
            return get_error_data_result(message="; ".join(errors))

    if duplicate_messages:
        if success_count > 0:
            return get_result(
                message=f"Partially deleted {success_count} sessions with {len(duplicate_messages)} errors",
                data={"success_count": success_count, "errors": duplicate_messages})
        else:
            return get_error_data_result(message=";".join(duplicate_messages))

    return get_result()


@manager.route("/sessions/ask", methods=["POST"])  # noqa: F821
@token_required
async def ask_about(tenant_id):
    req = await get_request_json()
    if not req.get("question"):
        return get_error_data_result("`question` is required.")
    if not req.get("dataset_ids"):
        return get_error_data_result("`dataset_ids` is required.")
    if not isinstance(req.get("dataset_ids"), list):
        return get_error_data_result("`dataset_ids` should be a list.")
    req["kb_ids"] = req.pop("dataset_ids")
    for kb_id in req["kb_ids"]:
        if not KnowledgebaseService.accessible(kb_id, tenant_id):
            return get_error_data_result(f"You don't own the dataset {kb_id}.")
        kbs = KnowledgebaseService.query(id=kb_id)
        kb = kbs[0]
        if kb.chunk_num == 0:
            return get_error_data_result(f"The dataset {kb_id} doesn't own parsed file")
    uid = tenant_id

    async def stream():
        nonlocal req, uid
        try:
            async for ans in async_ask(req["question"], req["kb_ids"], uid):
                yield "data:" + json.dumps({"code": 0, "message": "", "data": ans}, ensure_ascii=False) + "\n\n"
        except Exception as e:
            yield "data:" + json.dumps(
                {"code": 500, "message": str(e), "data": {"answer": "**ERROR**: " + str(e), "reference": []}},
                ensure_ascii=False) + "\n\n"
        yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"

    resp = Response(stream(), mimetype="text/event-stream")
    resp.headers.add_header("Cache-control", "no-cache")
    resp.headers.add_header("Connection", "keep-alive")
    resp.headers.add_header("X-Accel-Buffering", "no")
    resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
    return resp


@manager.route("/sessions/related_questions", methods=["POST"])  # noqa: F821
@token_required
async def related_questions(tenant_id):
    req = await get_request_json()
    if not req.get("question"):
        return get_error_data_result("`question` is required.")
    question = req["question"]
    industry = req.get("industry", "")
    chat_mdl = LLMBundle(tenant_id, LLMType.CHAT)
    prompt = """
Objective: To generate search terms related to the user's search keywords, helping users find more valuable information.
Instructions:
 - Based on the keywords provided by the user, generate 5-10 related search terms.
 - Each search term should be directly or indirectly related to the keyword, guiding the user to find more valuable information.
 - Use common, general terms as much as possible, avoiding obscure words or technical jargon.
 - Keep the term length between 2-4 words, concise and clear.
 - DO NOT translate, use the language of the original keywords.
"""
    if industry:
        prompt += f" - Ensure all search terms are relevant to the industry: {industry}.\n"
    prompt += """
### Example:
Keywords: Chinese football
Related search terms:
1. Current status of Chinese football
2. Reform of Chinese football
3. Youth training of Chinese football
4. Chinese football in the Asian Cup
5. Chinese football in the World Cup

Reason:
 - When searching, users often only use one or two keywords, making it difficult to fully express their information needs.
 - Generating related search terms can help users dig deeper into relevant information and improve search efficiency.
 - At the same time, related terms can also help search engines better understand user needs and return more accurate search results.

"""
    ans = await chat_mdl.async_chat(
        prompt,
        [
            {
                "role": "user",
                "content": f"""
Keywords: {question}
Related search terms:
    """,
            }
        ],
        {"temperature": 0.9},
    )
    return get_result(data=[re.sub(r"^[0-9]\. ", "", a) for a in ans.split("\n") if re.match(r"^[0-9]\. ", a)])


@manager.route("/chatbots/<dialog_id>/completions", methods=["POST"])  # noqa: F821
async def chatbot_completions(dialog_id):
    req = await get_request_json()

    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    if "quote" not in req:
        req["quote"] = False

    if req.get("stream", True):
        resp = Response(iframe_completion(dialog_id, **req), mimetype="text/event-stream")
        resp.headers.add_header("Cache-control", "no-cache")
        resp.headers.add_header("Connection", "keep-alive")
        resp.headers.add_header("X-Accel-Buffering", "no")
        resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
        return resp

    async for answer in iframe_completion(dialog_id, **req):
        return get_result(data=answer)

    return None

@manager.route("/chatbots/<dialog_id>/info", methods=["GET"])  # noqa: F821
async def chatbots_inputs(dialog_id):
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    e, dialog = DialogService.get_by_id(dialog_id)
    if not e:
        return get_error_data_result(f"Can't find dialog by ID: {dialog_id}")

    return get_result(
        data={
            "title": dialog.name,
            "avatar": dialog.icon,
            "prologue": dialog.prompt_config.get("prologue", ""),
            "has_tavily_key": bool(dialog.prompt_config.get("tavily_api_key", "").strip()),
        }
    )


@manager.route("/agentbots/<agent_id>/completions", methods=["POST"])  # noqa: F821
async def agent_bot_completions(agent_id):
    req = await get_request_json()

    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    if req.get("stream", True):
        resp = Response(agent_completion(objs[0].tenant_id, agent_id, **req), mimetype="text/event-stream")
        resp.headers.add_header("Cache-control", "no-cache")
        resp.headers.add_header("Connection", "keep-alive")
        resp.headers.add_header("X-Accel-Buffering", "no")
        resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
        return resp

    async for answer in agent_completion(objs[0].tenant_id, agent_id, **req):
        return get_result(data=answer)

    return None

@manager.route("/agentbots/<agent_id>/inputs", methods=["GET"])  # noqa: F821
async def begin_inputs(agent_id):
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    e, cvs = UserCanvasService.get_by_id(agent_id)
    if not e:
        return get_error_data_result(f"Can't find agent by ID: {agent_id}")

    canvas = Canvas(json.dumps(cvs.dsl), objs[0].tenant_id, canvas_id=cvs.id)
    return get_result(
        data={"title": cvs.title, "avatar": cvs.avatar, "inputs": canvas.get_component_input_form("begin"),
              "prologue": canvas.get_prologue(), "mode": canvas.get_mode()})


@manager.route("/searchbots/ask", methods=["POST"])  # noqa: F821
@validate_request("question", "kb_ids")
async def ask_about_embedded():
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    req = await get_request_json()
    uid = objs[0].tenant_id

    search_id = req.get("search_id", "")
    search_config = {}
    if search_id:
        if search_app := SearchService.get_detail(search_id):
            search_config = search_app.get("search_config", {})

    async def stream():
        nonlocal req, uid
        try:
            async for ans in async_ask(req["question"], req["kb_ids"], uid, search_config=search_config):
                yield "data:" + json.dumps({"code": 0, "message": "", "data": ans}, ensure_ascii=False) + "\n\n"
        except Exception as e:
            yield "data:" + json.dumps(
                {"code": 500, "message": str(e), "data": {"answer": "**ERROR**: " + str(e), "reference": []}},
                ensure_ascii=False) + "\n\n"
        yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"

    resp = Response(stream(), mimetype="text/event-stream")
    resp.headers.add_header("Cache-control", "no-cache")
    resp.headers.add_header("Connection", "keep-alive")
    resp.headers.add_header("X-Accel-Buffering", "no")
    resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
    return resp


@manager.route("/searchbots/retrieval_test", methods=["POST"])  # noqa: F821
@validate_request("kb_id", "question")
async def retrieval_test_embedded():
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

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
    similarity_threshold = float(req.get("similarity_threshold", 0.0))
    vector_similarity_weight = float(req.get("vector_similarity_weight", 0.3))
    use_kg = req.get("use_kg", False)
    top = int(req.get("top_k", 1024))
    langs = req.get("cross_languages", [])
    rerank_id = req.get("rerank_id", "")
    tenant_id = objs[0].tenant_id
    if not tenant_id:
        return get_error_data_result(message="permission denined.")

    async def _retrieval():
        nonlocal similarity_threshold, vector_similarity_weight, top, rerank_id
        local_doc_ids = list(doc_ids) if doc_ids else []
        tenant_ids = []
        _question = question

        meta_data_filter = {}
        chat_mdl = None
        if req.get("search_id", ""):
            search_config = SearchService.get_detail(req.get("search_id", "")).get("search_config", {})
            meta_data_filter = search_config.get("meta_data_filter", {})
            if meta_data_filter.get("method") in ["auto", "semi_auto"]:
                chat_mdl = LLMBundle(tenant_id, LLMType.CHAT, llm_name=search_config.get("chat_id", ""))
            # Apply search_config settings if not explicitly provided in request
            if not req.get("similarity_threshold"):
                similarity_threshold = float(search_config.get("similarity_threshold", similarity_threshold))
            if not req.get("vector_similarity_weight"):
                vector_similarity_weight = float(search_config.get("vector_similarity_weight", vector_similarity_weight))
            if not req.get("top_k"):
                top = int(search_config.get("top_k", top))
            if not req.get("rerank_id"):
                rerank_id = search_config.get("rerank_id", "")
        else:
            meta_data_filter = req.get("meta_data_filter") or {}
            if meta_data_filter.get("method") in ["auto", "semi_auto"]:
                chat_mdl = LLMBundle(tenant_id, LLMType.CHAT)

        if meta_data_filter:
            metas = DocMetadataService.get_flatted_meta_by_kbs(kb_ids)
            local_doc_ids = await apply_meta_data_filter(meta_data_filter, metas, _question, chat_mdl, local_doc_ids)

        tenants = UserTenantService.query(user_id=tenant_id)
        for kb_id in kb_ids:
            for tenant in tenants:
                if KnowledgebaseService.query(tenant_id=tenant.tenant_id, id=kb_id):
                    tenant_ids.append(tenant.tenant_id)
                    break
            else:
                return get_json_result(data=False, message="Only owner of dataset authorized for this operation.",
                                       code=RetCode.OPERATING_ERROR)

        e, kb = KnowledgebaseService.get_by_id(kb_ids[0])
        if not e:
            return get_error_data_result(message="Knowledgebase not found!")

        if langs:
            _question = await cross_languages(kb.tenant_id, None, _question, langs)

        embd_mdl = LLMBundle(kb.tenant_id, LLMType.EMBEDDING.value, llm_name=kb.embd_id)

        rerank_mdl = None
        if rerank_id:
            rerank_mdl = LLMBundle(kb.tenant_id, LLMType.RERANK.value, llm_name=rerank_id)

        if req.get("keyword", False):
            chat_mdl = LLMBundle(kb.tenant_id, LLMType.CHAT)
            _question += await keyword_extraction(chat_mdl, _question)

        labels = label_question(_question, [kb])
        ranks = await settings.retriever.retrieval(
            _question, embd_mdl, tenant_ids, kb_ids, page, size, similarity_threshold, vector_similarity_weight, top,
            local_doc_ids, rerank_mdl=rerank_mdl, highlight=req.get("highlight"), rank_feature=labels
        )
        if use_kg:
            ck = await settings.kg_retriever.retrieval(_question, tenant_ids, kb_ids, embd_mdl,
                                                 LLMBundle(kb.tenant_id, LLMType.CHAT))
            if ck["content_with_weight"]:
                ranks["chunks"].insert(0, ck)

        for c in ranks["chunks"]:
            c.pop("vector", None)
        ranks["labels"] = labels

        return get_json_result(data=ranks)

    try:
        return await _retrieval()
    except Exception as e:
        if str(e).find("not_found") > 0:
            return get_json_result(data=False, message="No chunk found! Check the chunk status please!",
                                   code=RetCode.DATA_ERROR)
        return server_error_response(e)


@manager.route("/searchbots/related_questions", methods=["POST"])  # noqa: F821
@validate_request("question")
async def related_questions_embedded():
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    req = await get_request_json()
    tenant_id = objs[0].tenant_id
    if not tenant_id:
        return get_error_data_result(message="permission denined.")

    search_id = req.get("search_id", "")
    search_config = {}
    if search_id:
        if search_app := SearchService.get_detail(search_id):
            search_config = search_app.get("search_config", {})

    question = req["question"]

    chat_id = search_config.get("chat_id", "")
    chat_mdl = LLMBundle(tenant_id, LLMType.CHAT, chat_id)

    gen_conf = search_config.get("llm_setting", {"temperature": 0.9})
    prompt = load_prompt("related_question")
    ans = await chat_mdl.async_chat(
        prompt,
        [
            {
                "role": "user",
                "content": f"""
Keywords: {question}
Related search terms:
    """,
            }
        ],
        gen_conf,
    )
    return get_json_result(data=[re.sub(r"^[0-9]\. ", "", a) for a in ans.split("\n") if re.match(r"^[0-9]\. ", a)])


@manager.route("/searchbots/detail", methods=["GET"])  # noqa: F821
async def detail_share_embedded():
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    search_id = request.args["search_id"]
    tenant_id = objs[0].tenant_id
    if not tenant_id:
        return get_error_data_result(message="permission denined.")
    try:
        tenants = UserTenantService.query(user_id=tenant_id)
        for tenant in tenants:
            if SearchService.query(tenant_id=tenant.tenant_id, id=search_id):
                break
        else:
            return get_json_result(data=False, message="Has no permission for this operation.",
                                   code=RetCode.OPERATING_ERROR)

        search = SearchService.get_detail(search_id)
        if not search:
            return get_error_data_result(message="Can't find this Search App!")
        return get_json_result(data=search)
    except Exception as e:
        return server_error_response(e)


@manager.route("/searchbots/mindmap", methods=["POST"])  # noqa: F821
@validate_request("question", "kb_ids")
async def mindmap():
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    tenant_id = objs[0].tenant_id
    req = await get_request_json()

    search_id = req.get("search_id", "")
    search_app = SearchService.get_detail(search_id) if search_id else {}

    mind_map =await gen_mindmap(req["question"], req["kb_ids"], tenant_id, search_app.get("search_config", {}))
    if "error" in mind_map:
        return server_error_response(Exception(mind_map["error"]))
    return get_json_result(data=mind_map)

@manager.route("/sequence2txt", methods=["POST"])  # noqa: F821
@token_required
async def sequence2txt(tenant_id):
    req = await request.form
    stream_mode = req.get("stream", "false").lower() == "true"
    files = await request.files
    if "file" not in files:
        return get_error_data_result(message="Missing 'file' in multipart form-data")

    uploaded = files["file"]

    ALLOWED_EXTS = {
        ".wav", ".mp3", ".m4a", ".aac",
        ".flac", ".ogg", ".webm",
        ".opus", ".wma"
    }

    filename = uploaded.filename or ""
    suffix = os.path.splitext(filename)[-1].lower()
    if suffix not in ALLOWED_EXTS:
        return get_error_data_result(message=
            f"Unsupported audio format: {suffix}. "
            f"Allowed: {', '.join(sorted(ALLOWED_EXTS))}"
        )
    fd, temp_audio_path = tempfile.mkstemp(suffix=suffix)
    os.close(fd)
    await uploaded.save(temp_audio_path)

    tenants = TenantService.get_info_by(tenant_id)
    if not tenants:
        return get_error_data_result(message="Tenant not found!")

    asr_id = tenants[0]["asr_id"]
    if not asr_id:
        return get_error_data_result(message="No default ASR model is set")

    asr_mdl=LLMBundle(tenants[0]["tenant_id"], LLMType.SPEECH2TEXT, asr_id)
    if not stream_mode:
        text = asr_mdl.transcription(temp_audio_path)
        try:
            os.remove(temp_audio_path)
        except Exception as e:
            logging.error(f"Failed to remove temp audio file: {str(e)}")
        return get_json_result(data={"text": text})
    async def event_stream():
        try:
            for evt in asr_mdl.stream_transcription(temp_audio_path):
                yield f"data: {json.dumps(evt, ensure_ascii=False)}\n\n"
        except Exception as e:
            err = {"event": "error", "text": str(e)}
            yield f"data: {json.dumps(err, ensure_ascii=False)}\n\n"
        finally:
            try:
                os.remove(temp_audio_path)
            except Exception as e:
                logging.error(f"Failed to remove temp audio file: {str(e)}")

    return Response(event_stream(), content_type="text/event-stream")

@manager.route("/tts", methods=["POST"])  # noqa: F821
@token_required
async def tts(tenant_id):
    req = await get_request_json()
    text = req["text"]

    tenants = TenantService.get_info_by(tenant_id)
    if not tenants:
        return get_error_data_result(message="Tenant not found!")

    tts_id = tenants[0]["tts_id"]
    if not tts_id:
        return get_error_data_result(message="No default TTS model is set")

    tts_mdl = LLMBundle(tenants[0]["tenant_id"], LLMType.TTS, tts_id)

    def stream_audio():
        try:
            for txt in re.split(r"[，。/《》？；：！\n\r:;]+", text):
                for chunk in tts_mdl.tts(txt):
                    yield chunk
        except Exception as e:
            yield ("data:" + json.dumps({"code": 500, "message": str(e), "data": {"answer": "**ERROR**: " + str(e)}}, ensure_ascii=False)).encode("utf-8")

    resp = Response(stream_audio(), mimetype="audio/mpeg")
    resp.headers.add_header("Cache-Control", "no-cache")
    resp.headers.add_header("Connection", "keep-alive")
    resp.headers.add_header("X-Accel-Buffering", "no")

    return resp


def _build_reference_chunks(reference, include_metadata=False, metadata_fields=None):
    chunks = chunks_format(reference)
    if not include_metadata:
        return chunks

    doc_ids_by_kb = {}
    for chunk in chunks:
        kb_id = chunk.get("dataset_id")
        doc_id = chunk.get("document_id")
        if not kb_id or not doc_id:
            continue
        doc_ids_by_kb.setdefault(kb_id, set()).add(doc_id)

    if not doc_ids_by_kb:
        return chunks

    meta_by_doc = {}
    for kb_id, doc_ids in doc_ids_by_kb.items():
        meta_map = DocMetadataService.get_metadata_for_documents(list(doc_ids), kb_id)
        if meta_map:
            meta_by_doc.update(meta_map)

    if metadata_fields is not None:
        metadata_fields = {f for f in metadata_fields if isinstance(f, str)}
        if not metadata_fields:
            return chunks

    for chunk in chunks:
        doc_id = chunk.get("document_id")
        if not doc_id:
            continue
        meta = meta_by_doc.get(doc_id)
        if not meta:
            continue
        if metadata_fields is not None:
            meta = {k: v for k, v in meta.items() if k in metadata_fields}
        if meta:
            chunk["document_metadata"] = meta

    return chunks
