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
import re
import time

import tiktoken
from flask import Response, jsonify, request

from agent.canvas import Canvas
from api import settings
from api.db import LLMType, StatusEnum
from api.db.db_models import APIToken
from api.db.services.api_service import API4ConversationService
from api.db.services.canvas_service import UserCanvasService, completionOpenAI
from api.db.services.canvas_service import completion as agent_completion
from api.db.services.conversation_service import ConversationService, iframe_completion
from api.db.services.conversation_service import completion as rag_completion
from api.db.services.dialog_service import DialogService, ask, chat, gen_mindmap, meta_filter
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from api.db.services.search_service import SearchService
from api.db.services.user_service import UserTenantService
from api.utils import get_uuid
from api.utils.api_utils import check_duplicate_ids, get_data_openai, get_error_data_result, get_json_result, get_result, server_error_response, token_required, validate_request
from rag.app.tag import label_question
from rag.prompts import chunks_format
from rag.prompts.prompt_template import load_prompt
from rag.prompts.prompts import cross_languages, gen_meta_filter, keyword_extraction


@manager.route("/chats/<chat_id>/sessions", methods=["POST"])  # noqa: F821
@token_required
def create(tenant_id, chat_id):
    req = request.json
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
        "reference": [{}],
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
def create_agent_session(tenant_id, agent_id):
    user_id = request.args.get("user_id", tenant_id)
    e, cvs = UserCanvasService.get_by_id(agent_id)
    if not e:
        return get_error_data_result("Agent not found.")
    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        return get_error_data_result("You cannot access the agent.")
    if not isinstance(cvs.dsl, str):
        cvs.dsl = json.dumps(cvs.dsl, ensure_ascii=False)

    session_id = get_uuid()
    canvas = Canvas(cvs.dsl, tenant_id, agent_id)
    canvas.reset()

    cvs.dsl = json.loads(str(canvas))
    conv = {"id": session_id, "dialog_id": cvs.id, "user_id": user_id, "message": [{"role": "assistant", "content": canvas.get_prologue()}], "source": "agent", "dsl": cvs.dsl}
    API4ConversationService.save(**conv)
    conv["agent_id"] = conv.pop("dialog_id")
    return get_result(data=conv)


@manager.route("/chats/<chat_id>/sessions/<session_id>", methods=["PUT"])  # noqa: F821
@token_required
def update(tenant_id, chat_id, session_id):
    req = request.json
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
def chat_completion(tenant_id, chat_id):
    req = request.json
    if not req:
        req = {"question": ""}
    if not req.get("session_id"):
        req["question"] = ""
    if not DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value):
        return get_error_data_result(f"You don't own the chat {chat_id}")
    if req.get("session_id"):
        if not ConversationService.query(id=req["session_id"], dialog_id=chat_id):
            return get_error_data_result(f"You don't own the session {req['session_id']}")
    if req.get("stream", True):
        resp = Response(rag_completion(tenant_id, chat_id, **req), mimetype="text/event-stream")
        resp.headers.add_header("Cache-control", "no-cache")
        resp.headers.add_header("Connection", "keep-alive")
        resp.headers.add_header("X-Accel-Buffering", "no")
        resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")

        return resp
    else:
        answer = None
        for ans in rag_completion(tenant_id, chat_id, **req):
            answer = ans
            break
        return get_result(data=answer)


@manager.route("/chats_openai/<chat_id>/chat/completions", methods=["POST"])  # noqa: F821
@validate_request("model", "messages")  # noqa: F821
@token_required
def chat_completion_openai_like(tenant_id, chat_id):
    """
    OpenAI-like chat completion API that simulates the behavior of OpenAI's completions endpoint.

    This function allows users to interact with a model and receive responses based on a series of historical messages.
    If `stream` is set to True (by default), the response will be streamed in chunks, mimicking the OpenAI-style API.
    Set `stream` to False explicitly, the response will be returned in a single complete answer.

    Reference:

    - If `stream` is True, the final answer and reference information will appear in the **last chunk** of the stream.
    - If `stream` is False, the reference will be included in `choices[0].message.reference`.

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

    from openai import OpenAI

    model = "model"
    client = OpenAI(api_key="ragflow-api-key", base_url=f"http://ragflow_address/api/v1/chats_openai/<chat_id>")

    stream = True
    reference = True

    completion = client.chat.completions.create(
        model=model,
        messages=[
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Who are you?"},
            {"role": "assistant", "content": "I am an AI assistant named..."},
            {"role": "user", "content": "Can you tell me how to install neovim"},
        ],
        stream=stream,
        extra_body={"reference": reference}
    )

    if stream:
    for chunk in completion:
        print(chunk)
        if reference and chunk.choices[0].finish_reason == "stop":
            print(f"Reference:\n{chunk.choices[0].delta.reference}")
            print(f"Final content:\n{chunk.choices[0].delta.final_content}")
    else:
        print(completion.choices[0].message.content)
        if reference:
            print(completion.choices[0].message.reference)
    """
    req = request.get_json()

    need_reference = bool(req.get("reference", False))

    messages = req.get("messages", [])
    # To prevent empty [] input
    if len(messages) < 1:
        return get_error_data_result("You have to provide messages.")
    if messages[-1]["role"] != "user":
        return get_error_data_result("The last content of this conversation is not from user.")

    prompt = messages[-1]["content"]
    # Treat context tokens as reasoning tokens
    context_token_used = sum(len(message["content"]) for message in messages)

    dia = DialogService.query(tenant_id=tenant_id, id=chat_id, status=StatusEnum.VALID.value)
    if not dia:
        return get_error_data_result(f"You don't own the chat {chat_id}")
    dia = dia[0]

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
        def streamed_response_generator(chat_id, dia, msg):
            token_used = 0
            answer_cache = ""
            reasoning_cache = ""
            last_ans = {}
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
                for ans in chat(dia, msg, True, toolcall_session=toolcall_session, tools=tools, quote=need_reference):
                    last_ans = ans
                    answer = ans["answer"]

                    reasoning_match = re.search(r"<think>(.*?)</think>", answer, flags=re.DOTALL)
                    if reasoning_match:
                        reasoning_part = reasoning_match.group(1)
                        content_part = answer[reasoning_match.end() :]
                    else:
                        reasoning_part = ""
                        content_part = answer

                    reasoning_incremental = ""
                    if reasoning_part:
                        if reasoning_part.startswith(reasoning_cache):
                            reasoning_incremental = reasoning_part.replace(reasoning_cache, "", 1)
                        else:
                            reasoning_incremental = reasoning_part
                        reasoning_cache = reasoning_part

                    content_incremental = ""
                    if content_part:
                        if content_part.startswith(answer_cache):
                            content_incremental = content_part.replace(answer_cache, "", 1)
                    else:
                        content_incremental = content_part
                    answer_cache = content_part

                    token_used += len(reasoning_incremental) + len(content_incremental)

                    if not any([reasoning_incremental, content_incremental]):
                        continue

                    if reasoning_incremental:
                        response["choices"][0]["delta"]["reasoning_content"] = reasoning_incremental
                    else:
                        response["choices"][0]["delta"]["reasoning_content"] = None

                    if content_incremental:
                        response["choices"][0]["delta"]["content"] = content_incremental
                    else:
                        response["choices"][0]["delta"]["content"] = None

                    yield f"data:{json.dumps(response, ensure_ascii=False)}\n\n"
            except Exception as e:
                response["choices"][0]["delta"]["content"] = "**ERROR**: " + str(e)
                yield f"data:{json.dumps(response, ensure_ascii=False)}\n\n"

            # The last chunk
            response["choices"][0]["delta"]["content"] = None
            response["choices"][0]["delta"]["reasoning_content"] = None
            response["choices"][0]["finish_reason"] = "stop"
            response["usage"] = {"prompt_tokens": len(prompt), "completion_tokens": token_used, "total_tokens": len(prompt) + token_used}
            if need_reference:
                response["choices"][0]["delta"]["reference"] = chunks_format(last_ans.get("reference", []))
                response["choices"][0]["delta"]["final_content"] = last_ans.get("answer", "")
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
        for ans in chat(dia, msg, False, toolcall_session=toolcall_session, tools=tools, quote=need_reference):
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
                "prompt_tokens": len(prompt),
                "completion_tokens": len(content),
                "total_tokens": len(prompt) + len(content),
                "completion_tokens_details": {
                    "reasoning_tokens": context_token_used,
                    "accepted_prediction_tokens": len(content),
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
            response["choices"][0]["message"]["reference"] = chunks_format(answer.get("reference", []))

        return jsonify(response)


@manager.route("/agents_openai/<agent_id>/chat/completions", methods=["POST"])  # noqa: F821
@validate_request("model", "messages")  # noqa: F821
@token_required
def agents_completion_openai_compatibility(tenant_id, agent_id):
    req = request.json
    tiktokenenc = tiktoken.get_encoding("cl100k_base")
    messages = req.get("messages", [])
    if not messages:
        return get_error_data_result("You must provide at least one message.")
    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        return get_error_data_result(f"You don't own the agent {agent_id}")

    filtered_messages = [m for m in messages if m["role"] in ["user", "assistant"]]
    prompt_tokens = sum(len(tiktokenenc.encode(m["content"])) for m in filtered_messages)
    if not filtered_messages:
        return jsonify(
            get_data_openai(
                id=agent_id,
                content="No valid messages found (user or assistant).",
                finish_reason="stop",
                model=req.get("model", ""),
                completion_tokens=len(tiktokenenc.encode("No valid messages found (user or assistant).")),
                prompt_tokens=prompt_tokens,
            )
        )

    question = next((m["content"] for m in reversed(messages) if m["role"] == "user"), "")

    stream = req.pop("stream", False)
    if stream:
        resp = Response(
            completionOpenAI(
                tenant_id,
                agent_id,
                question,
                session_id=req.get("id", req.get("metadata", {}).get("id", "")),
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
        response = next(
            completionOpenAI(
                tenant_id,
                agent_id,
                question,
                session_id=req.get("id", req.get("metadata", {}).get("id", "")),
                stream=False,
                **req,
            )
        )
        return jsonify(response)


@manager.route("/agents/<agent_id>/completions", methods=["POST"])  # noqa: F821
@token_required
def agent_completions(tenant_id, agent_id):
    req = request.json

    ans = {}
    if req.get("stream", True):

        def generate():
            for answer in agent_completion(tenant_id=tenant_id, agent_id=agent_id, **req):
                if isinstance(answer, str):
                    try:
                        ans = json.loads(answer[5:])  # remove "data:"
                    except Exception:
                        continue

                if ans.get("event") != "message" or not ans.get("data", {}).get("reference", None):
                    continue

                yield answer

            yield "data:[DONE]\n\n"

    if req.get("stream", True):
        resp = Response(generate(), mimetype="text/event-stream")
        resp.headers.add_header("Cache-control", "no-cache")
        resp.headers.add_header("Connection", "keep-alive")
        resp.headers.add_header("X-Accel-Buffering", "no")
        resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
        return resp

    full_content = ""
    for answer in agent_completion(tenant_id=tenant_id, agent_id=agent_id, **req):
        try:
            ans = json.loads(answer[5:])

            if ans["event"] == "message":
                full_content += ans["data"]["content"]

            if ans.get("data", {}).get("reference", None):
                ans["data"]["content"] = full_content
                return get_result(data=ans)
        except Exception as e:
            return get_result(data=f"**ERROR**: {str(e)}")
    return get_result(data=ans)


@manager.route("/chats/<chat_id>/sessions", methods=["GET"])  # noqa: F821
@token_required
def list_session(tenant_id, chat_id):
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
def list_agent_session(tenant_id, agent_id):
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
    total, convs = API4ConversationService.get_list(agent_id, tenant_id, page_number, items_per_page, orderby, desc, id, user_id, include_dsl)
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
                    if chunk_num < len(conv["reference"]) and conv["reference"][chunk_num] is not None and isinstance(conv["reference"][chunk_num], dict) and "chunks" in conv["reference"][chunk_num]:
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
def delete(tenant_id, chat_id):
    if not DialogService.query(id=chat_id, tenant_id=tenant_id, status=StatusEnum.VALID.value):
        return get_error_data_result(message="You don't own the chat")

    errors = []
    success_count = 0
    req = request.json
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
            return get_result(data={"success_count": success_count, "errors": errors}, message=f"Partially deleted {success_count} sessions with {len(errors)} errors")
        else:
            return get_error_data_result(message="; ".join(errors))

    if duplicate_messages:
        if success_count > 0:
            return get_result(message=f"Partially deleted {success_count} sessions with {len(duplicate_messages)} errors", data={"success_count": success_count, "errors": duplicate_messages})
        else:
            return get_error_data_result(message=";".join(duplicate_messages))

    return get_result()


@manager.route("/agents/<agent_id>/sessions", methods=["DELETE"])  # noqa: F821
@token_required
def delete_agent_session(tenant_id, agent_id):
    errors = []
    success_count = 0
    req = request.json
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
            return get_result(data={"success_count": success_count, "errors": errors}, message=f"Partially deleted {success_count} sessions with {len(errors)} errors")
        else:
            return get_error_data_result(message="; ".join(errors))

    if duplicate_messages:
        if success_count > 0:
            return get_result(message=f"Partially deleted {success_count} sessions with {len(duplicate_messages)} errors", data={"success_count": success_count, "errors": duplicate_messages})
        else:
            return get_error_data_result(message=";".join(duplicate_messages))

    return get_result()


@manager.route("/sessions/ask", methods=["POST"])  # noqa: F821
@token_required
def ask_about(tenant_id):
    req = request.json
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

    def stream():
        nonlocal req, uid
        try:
            for ans in ask(req["question"], req["kb_ids"], uid):
                yield "data:" + json.dumps({"code": 0, "message": "", "data": ans}, ensure_ascii=False) + "\n\n"
        except Exception as e:
            yield "data:" + json.dumps({"code": 500, "message": str(e), "data": {"answer": "**ERROR**: " + str(e), "reference": []}}, ensure_ascii=False) + "\n\n"
        yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"

    resp = Response(stream(), mimetype="text/event-stream")
    resp.headers.add_header("Cache-control", "no-cache")
    resp.headers.add_header("Connection", "keep-alive")
    resp.headers.add_header("X-Accel-Buffering", "no")
    resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
    return resp


@manager.route("/sessions/related_questions", methods=["POST"])  # noqa: F821
@token_required
def related_questions(tenant_id):
    req = request.json
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
    ans = chat_mdl.chat(
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
def chatbot_completions(dialog_id):
    req = request.json

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

    for answer in iframe_completion(dialog_id, **req):
        return get_result(data=answer)


@manager.route("/chatbots/<dialog_id>/info", methods=["GET"])  # noqa: F821
def chatbots_inputs(dialog_id):
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
        }
    )


@manager.route("/agentbots/<agent_id>/completions", methods=["POST"])  # noqa: F821
def agent_bot_completions(agent_id):
    req = request.json

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

    for answer in agent_completion(objs[0].tenant_id, agent_id, **req):
        return get_result(data=answer)


@manager.route("/agentbots/<agent_id>/inputs", methods=["GET"])  # noqa: F821
def begin_inputs(agent_id):
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

    canvas = Canvas(json.dumps(cvs.dsl), objs[0].tenant_id)
    return get_result(data={"title": cvs.title, "avatar": cvs.avatar, "inputs": canvas.get_component_input_form("begin"), "prologue": canvas.get_prologue(), "mode": canvas.get_mode()})


@manager.route("/searchbots/ask", methods=["POST"])  # noqa: F821
@validate_request("question", "kb_ids")
def ask_about_embedded():
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    req = request.json
    uid = objs[0].tenant_id

    search_id = req.get("search_id", "")
    search_config = {}
    if search_id:
        if search_app := SearchService.get_detail(search_id):
            search_config = search_app.get("search_config", {})

    def stream():
        nonlocal req, uid
        try:
            for ans in ask(req["question"], req["kb_ids"], uid, search_config=search_config):
                yield "data:" + json.dumps({"code": 0, "message": "", "data": ans}, ensure_ascii=False) + "\n\n"
        except Exception as e:
            yield "data:" + json.dumps({"code": 500, "message": str(e), "data": {"answer": "**ERROR**: " + str(e), "reference": []}}, ensure_ascii=False) + "\n\n"
        yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"

    resp = Response(stream(), mimetype="text/event-stream")
    resp.headers.add_header("Cache-control", "no-cache")
    resp.headers.add_header("Connection", "keep-alive")
    resp.headers.add_header("X-Accel-Buffering", "no")
    resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
    return resp


@manager.route("/searchbots/retrieval_test", methods=["POST"])  # noqa: F821
@validate_request("kb_id", "question")
def retrieval_test_embedded():
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    req = request.json
    page = int(req.get("page", 1))
    size = int(req.get("size", 30))
    question = req["question"]
    kb_ids = req["kb_id"]
    if isinstance(kb_ids, str):
        kb_ids = [kb_ids]
    doc_ids = req.get("doc_ids", [])
    similarity_threshold = float(req.get("similarity_threshold", 0.0))
    vector_similarity_weight = float(req.get("vector_similarity_weight", 0.3))
    use_kg = req.get("use_kg", False)
    top = int(req.get("top_k", 1024))
    langs = req.get("cross_languages", [])
    tenant_ids = []

    tenant_id = objs[0].tenant_id
    if not tenant_id:
        return get_error_data_result(message="permission denined.")

    if req.get("search_id", ""):
        search_config = SearchService.get_detail(req.get("search_id", "")).get("search_config", {})
        meta_data_filter = search_config.get("meta_data_filter", {})
        metas = DocumentService.get_meta_by_kbs(kb_ids)
        if meta_data_filter.get("method") == "auto":
            chat_mdl = LLMBundle(tenant_id, LLMType.CHAT, llm_name=search_config.get("chat_id", ""))
            filters = gen_meta_filter(chat_mdl, metas, question)
            doc_ids.extend(meta_filter(metas, filters))
            if not doc_ids:
                doc_ids = None
        elif meta_data_filter.get("method") == "manual":
            doc_ids.extend(meta_filter(metas, meta_data_filter["manual"]))
            if not doc_ids:
                doc_ids = None

    try:
        tenants = UserTenantService.query(user_id=tenant_id)
        for kb_id in kb_ids:
            for tenant in tenants:
                if KnowledgebaseService.query(tenant_id=tenant.tenant_id, id=kb_id):
                    tenant_ids.append(tenant.tenant_id)
                    break
            else:
                return get_json_result(data=False, message="Only owner of knowledgebase authorized for this operation.", code=settings.RetCode.OPERATING_ERROR)

        e, kb = KnowledgebaseService.get_by_id(kb_ids[0])
        if not e:
            return get_error_data_result(message="Knowledgebase not found!")

        if langs:
            question = cross_languages(kb.tenant_id, None, question, langs)

        embd_mdl = LLMBundle(kb.tenant_id, LLMType.EMBEDDING.value, llm_name=kb.embd_id)

        rerank_mdl = None
        if req.get("rerank_id"):
            rerank_mdl = LLMBundle(kb.tenant_id, LLMType.RERANK.value, llm_name=req["rerank_id"])

        if req.get("keyword", False):
            chat_mdl = LLMBundle(kb.tenant_id, LLMType.CHAT)
            question += keyword_extraction(chat_mdl, question)

        labels = label_question(question, [kb])
        ranks = settings.retrievaler.retrieval(
            question, embd_mdl, tenant_ids, kb_ids, page, size, similarity_threshold, vector_similarity_weight, top, doc_ids, rerank_mdl=rerank_mdl, highlight=req.get("highlight"), rank_feature=labels
        )
        if use_kg:
            ck = settings.kg_retrievaler.retrieval(question, tenant_ids, kb_ids, embd_mdl, LLMBundle(kb.tenant_id, LLMType.CHAT))
            if ck["content_with_weight"]:
                ranks["chunks"].insert(0, ck)

        for c in ranks["chunks"]:
            c.pop("vector", None)
        ranks["labels"] = labels

        return get_json_result(data=ranks)
    except Exception as e:
        if str(e).find("not_found") > 0:
            return get_json_result(data=False, message="No chunk found! Check the chunk status please!", code=settings.RetCode.DATA_ERROR)
        return server_error_response(e)


@manager.route("/searchbots/related_questions", methods=["POST"])  # noqa: F821
@validate_request("question")
def related_questions_embedded():
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    req = request.json
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
    ans = chat_mdl.chat(
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
def detail_share_embedded():
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
            return get_json_result(data=False, message="Has no permission for this operation.", code=settings.RetCode.OPERATING_ERROR)

        search = SearchService.get_detail(search_id)
        if not search:
            return get_error_data_result(message="Can't find this Search App!")
        return get_json_result(data=search)
    except Exception as e:
        return server_error_response(e)


@manager.route("/searchbots/mindmap", methods=["POST"])  # noqa: F821
@validate_request("question", "kb_ids")
def mindmap():
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    tenant_id = objs[0].tenant_id
    req = request.json

    search_id = req.get("search_id", "")
    search_app = SearchService.get_detail(search_id) if search_id else {}

    mind_map = gen_mindmap(req["question"], req["kb_ids"], tenant_id, search_app.get("search_config", {}))
    if "error" in mind_map:
        return server_error_response(Exception(mind_map["error"]))
    return get_json_result(data=mind_map)
