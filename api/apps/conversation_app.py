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
import re
import logging
from copy import deepcopy
import tempfile
from typing import Annotated
from quart import Response, request
from pydantic import BaseModel, ConfigDict, Field
from quart_schema import validate_request as qs_validate_request, validate_response, tag

from api.apps import current_user, login_required
from api.db.db_models import APIToken
from api.db.services.conversation_service import ConversationService, structure_answer
from api.db.services.dialog_service import DialogService, async_ask, async_chat, gen_mindmap
from api.db.services.llm_service import LLMBundle
from api.db.services.search_service import SearchService
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.user_service import TenantService, UserTenantService
from api.utils.api_utils import get_data_error_result, get_json_result, get_request_json, server_error_response, validate_request
from rag.prompts.template import load_prompt
from rag.prompts.generator import chunks_format
from common.constants import RetCode, LLMType


# Pydantic Schemas for OpenAPI Documentation


class BaseSchema(BaseModel):
    """Base schema with common configuration."""
    model_config = ConfigDict(extra="forbid", strict=True)


class ConversationResponse(BaseModel):
    """Response schema for conversation operations."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Conversation data")]
    message: Annotated[str, Field("Success", description="Response message")]


class SetConversationRequest(BaseSchema):
    """Request schema for setting conversation."""
    conversation_id: Annotated[str, Field(..., description="Conversation ID")]
    is_new: Annotated[bool, Field(..., description="Whether this is a new conversation")]
    name: Annotated[str, Field("New conversation", description="Conversation name", max_length=255)]
    dialog_id: Annotated[str, Field(..., description="Associated dialog ID")]


class GetConversationResponse(BaseModel):
    """Response schema for getting conversation details."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Conversation details including messages and references")]
    message: Annotated[str, Field("Success", description="Response message")]


class GetSSEConversationResponse(BaseModel):
    """Response schema for getting SSE conversation."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Dialog data with avatar")]
    message: Annotated[str, Field("Success", description="Response message")]


class DeleteConversationRequest(BaseSchema):
    """Request schema for deleting conversations."""
    conversation_ids: Annotated[list[str], Field(..., description="List of conversation IDs to delete", min_length=1)]


class DeleteConversationResponse(BaseModel):
    """Response schema for deleting conversations."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[bool, Field(..., description="Deletion status")]
    message: Annotated[str, Field("Success", description="Response message")]


class ListConversationResponse(BaseModel):
    """Response schema for listing conversations."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[list[dict], Field(..., description="List of conversations")]
    message: Annotated[str, Field("Success", description="Response message")]


class CompletionRequest(BaseSchema):
    """Request schema for conversation completion."""
    conversation_id: Annotated[str, Field(..., description="Conversation ID")]
    messages: Annotated[list[dict], Field(..., description="List of messages in the conversation")]
    llm_id: Annotated[str | None, Field(None, description="Optional LLM model ID to use")]
    temperature: Annotated[float | None, Field(None, description="Temperature for LLM generation", ge=0, le=2)]
    top_p: Annotated[float | None, Field(None, description="Top p sampling parameter", ge=0, le=1)]
    frequency_penalty: Annotated[float | None, Field(None, description="Frequency penalty", ge=-2, le=2)]
    presence_penalty: Annotated[float | None, Field(None, description="Presence penalty", ge=-2, le=2)]
    max_tokens: Annotated[int | None, Field(None, description="Maximum tokens to generate", ge=1)]
    stream: Annotated[bool, Field(True, description="Whether to stream the response")]


class CompletionResponse(BaseModel):
    """Response schema for conversation completion (non-streaming)."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Completion result with answer and reference")]
    message: Annotated[str, Field("Success", description="Response message")]


class Sequence2TxtResponse(BaseModel):
    """Response schema for audio to text conversion."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Converted text from audio")]
    message: Annotated[str, Field("Success", description="Response message")]


class TTSRequest(BaseSchema):
    """Request schema for text to speech conversion."""
    text: Annotated[str, Field(..., description="Text to convert to speech", min_length=1)]


class DeleteMessageRequest(BaseSchema):
    """Request schema for deleting a message."""
    conversation_id: Annotated[str, Field(..., description="Conversation ID")]
    message_id: Annotated[str, Field(..., description="Message ID to delete")]


class ThumbUpRequest(BaseSchema):
    """Request schema for thumb up/down feedback."""
    conversation_id: Annotated[str, Field(..., description="Conversation ID")]
    message_id: Annotated[str, Field(..., description="Message ID to give feedback on")]
    thumbup: Annotated[bool | None, Field(None, description="True for thumb up, False for thumb down")]
    feedback: Annotated[str, Field("", description="Optional feedback text", max_length=65535)]


class AskRequest(BaseSchema):
    """Request schema for asking a question."""
    question: Annotated[str, Field(..., description="Question to ask", min_length=1)]
    kb_ids: Annotated[list[str], Field(..., description="Knowledge base IDs to search", min_length=1)]
    search_id: Annotated[str | None, Field(None, description="Optional search app ID")]


class MindMapRequest(BaseSchema):
    """Request schema for generating a mind map."""
    question: Annotated[str, Field(..., description="Question to generate mind map for", min_length=1)]
    kb_ids: Annotated[list[str], Field(..., description="Knowledge base IDs", min_length=1)]
    search_id: Annotated[str | None, Field(None, description="Optional search app ID")]


class MindMapResponse(BaseModel):
    """Response schema for mind map generation."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[dict, Field(..., description="Generated mind map data")]
    message: Annotated[str, Field("Success", description="Response message")]


class RelatedQuestionsRequest(BaseSchema):
    """Request schema for getting related questions."""
    question: Annotated[str, Field(..., description="Question to get related questions for", min_length=1)]
    search_id: Annotated[str | None, Field(None, description="Optional search app ID")]


class RelatedQuestionsResponse(BaseModel):
    """Response schema for related questions."""
    code: Annotated[int, Field(0, description="Response code, 0 for success")]
    data: Annotated[list[str], Field(..., description="List of related questions")]
    message: Annotated[str, Field("Success", description="Response message")]


# Conversation API Endpoints

conversation_tag = tag(["conversation"])


@manager.route("/set", methods=["POST"])  # noqa: F821
@login_required
@qs_validate_request(SetConversationRequest)
@validate_response(200, ConversationResponse)
@conversation_tag
async def set_conversation():
    """
    Create or update a conversation.

    Creates a new conversation or updates an existing one.
    For new conversations, initializes with the dialog's prologue message.
    For existing conversations, updates the conversation settings.
    """
    req = await get_request_json()
    conv_id = req.get("conversation_id")
    is_new = req.get("is_new")
    name = req.get("name", "New conversation")
    req["user_id"] = current_user.id

    if len(name) > 255:
        name = name[0:255]

    del req["is_new"]
    if not is_new:
        del req["conversation_id"]
        try:
            if not ConversationService.update_by_id(conv_id, req):
                return get_data_error_result(message="Conversation not found!")
            e, conv = ConversationService.get_by_id(conv_id)
            if not e:
                return get_data_error_result(message="Fail to update a conversation!")
            conv = conv.to_dict()
            return get_json_result(data=conv)
        except Exception as e:
            return server_error_response(e)

    try:
        e, dia = DialogService.get_by_id(req["dialog_id"])
        if not e:
            return get_data_error_result(message="Dialog not found")
        conv = {
            "id": conv_id,
            "dialog_id": req["dialog_id"],
            "name": name,
            "message": [{"role": "assistant", "content": dia.prompt_config["prologue"]}],
            "user_id": current_user.id,
            "reference": [],
        }
        ConversationService.save(**conv)
        return get_json_result(data=conv)
    except Exception as e:
        return server_error_response(e)


@manager.route("/get", methods=["GET"])  # noqa: F821
@login_required
@validate_response(200, GetConversationResponse)
@conversation_tag
async def get():
    """
    Get conversation details.

    Retrieves detailed information about a specific conversation including
    all messages, references, and associated dialog avatar.
    Only authorized users (conversation owners) can access this endpoint.
    """
    conv_id = request.args["conversation_id"]
    try:
        e, conv = ConversationService.get_by_id(conv_id)
        if not e:
            return get_data_error_result(message="Conversation not found!")
        tenants = UserTenantService.query(user_id=current_user.id)
        for tenant in tenants:
            dialog = DialogService.query(tenant_id=tenant.tenant_id, id=conv.dialog_id)
            if dialog and len(dialog) > 0:
                avatar = dialog[0].icon
                break
        else:
            return get_json_result(data=False, message="Only owner of conversation authorized for this operation.", code=RetCode.OPERATING_ERROR)

        for ref in conv.reference:
            if isinstance(ref, list):
                continue
            ref["chunks"] = chunks_format(ref)

        conv = conv.to_dict()
        conv["avatar"] = avatar
        return get_json_result(data=conv)
    except Exception as e:
        return server_error_response(e)


@manager.route("/getsse/<dialog_id>", methods=["GET"])  # type: ignore # noqa: F821
@validate_response(200, GetSSEConversationResponse)
@conversation_tag
def getsse(dialog_id):
    """
    Get dialog for SSE connection.

    Retrieves dialog information for Server-Sent Events (SSE) connections.
    Uses API token authentication instead of session-based authentication.
    This endpoint is designed for external API integrations.
    """
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_data_error_result(message='Authorization is not valid!"')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_data_error_result(message='Authentication error: API key is invalid!"')
    try:
        e, conv = DialogService.get_by_id(dialog_id)
        if not e:
            return get_data_error_result(message="Dialog not found!")
        conv = conv.to_dict()
        conv["avatar"] = conv["icon"]
        del conv["icon"]
        return get_json_result(data=conv)
    except Exception as e:
        return server_error_response(e)


@manager.route("/rm", methods=["POST"])  # noqa: F821
@login_required
@validate_request("conversation_ids")
@qs_validate_request(DeleteConversationRequest)
@validate_response(200, DeleteConversationResponse)
@conversation_tag
async def rm():
    """
    Delete conversations.

    Permanently deletes one or more conversations by their IDs.
    Only the owner of the conversations can delete them.
    """
    req = await get_request_json()
    conv_ids = req["conversation_ids"]
    try:
        for cid in conv_ids:
            exist, conv = ConversationService.get_by_id(cid)
            if not exist:
                return get_data_error_result(message="Conversation not found!")
            tenants = UserTenantService.query(user_id=current_user.id)
            for tenant in tenants:
                if DialogService.query(tenant_id=tenant.tenant_id, id=conv.dialog_id):
                    break
            else:
                return get_json_result(data=False, message="Only owner of conversation authorized for this operation.", code=RetCode.OPERATING_ERROR)
            ConversationService.delete_by_id(cid)
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route("/list", methods=["GET"])  # noqa: F821
@login_required
@validate_response(200, ListConversationResponse)
@conversation_tag
async def list_conversation():
    """
    List conversations for a dialog.

    Retrieves all conversations associated with a specific dialog,
    ordered by creation time (most recent first).
    Only the dialog owner can access this endpoint.
    """
    dialog_id = request.args["dialog_id"]
    try:
        if not DialogService.query(tenant_id=current_user.id, id=dialog_id):
            return get_json_result(data=False, message="Only owner of dialog authorized for this operation.", code=RetCode.OPERATING_ERROR)
        convs = ConversationService.query(dialog_id=dialog_id, order_by=ConversationService.model.create_time, reverse=True)

        convs = [d.to_dict() for d in convs]
        return get_json_result(data=convs)
    except Exception as e:
        return server_error_response(e)


@manager.route("/completion", methods=["POST"])  # noqa: F821
@login_required
@validate_request("conversation_id", "messages")
@qs_validate_request(CompletionRequest)
@validate_response(200, CompletionResponse)
@conversation_tag
async def completion():
    """
    Complete a conversation with an AI response.

    Processes a conversation and generates an AI response using the configured LLM.
    Supports both streaming and non-streaming modes.
    Can override the default LLM settings with custom parameters.

    Streaming mode returns Server-Sent Events (SSE) with incremental responses.
    Non-streaming mode returns a complete response in a single message.
    """
    req = await get_request_json()
    msg = []
    for m in req["messages"]:
        if m["role"] == "system":
            continue
        if m["role"] == "assistant" and not msg:
            continue
        msg.append(m)
    message_id = msg[-1].get("id")
    chat_model_id = req.get("llm_id", "")
    req.pop("llm_id", None)

    chat_model_config = {}
    for model_config in [
        "temperature",
        "top_p",
        "frequency_penalty",
        "presence_penalty",
        "max_tokens",
    ]:
        config = req.get(model_config)
        if config:
            chat_model_config[model_config] = config

    try:
        e, conv = ConversationService.get_by_id(req["conversation_id"])
        if not e:
            return get_data_error_result(message="Conversation not found!")
        conv.message = deepcopy(req["messages"])
        e, dia = DialogService.get_by_id(conv.dialog_id)
        if not e:
            return get_data_error_result(message="Dialog not found!")
        del req["conversation_id"]
        del req["messages"]

        if not conv.reference:
            conv.reference = []
        conv.reference = [r for r in conv.reference if r]
        conv.reference.append({"chunks": [], "doc_aggs": []})

        if chat_model_id:
            if not TenantLLMService.get_api_key(tenant_id=dia.tenant_id, model_name=chat_model_id):
                req.pop("chat_model_id", None)
                req.pop("chat_model_config", None)
                return get_data_error_result(message=f"Cannot use specified model {chat_model_id}.")
            dia.llm_id = chat_model_id
            dia.llm_setting = chat_model_config

        is_embedded = bool(chat_model_id)
        async def stream():
            nonlocal dia, msg, req, conv
            try:
                async for ans in async_chat(dia, msg, True, **req):
                    ans = structure_answer(conv, ans, message_id, conv.id)
                    yield "data:" + json.dumps({"code": 0, "message": "", "data": ans}, ensure_ascii=False) + "\n\n"
                if not is_embedded:
                    ConversationService.update_by_id(conv.id, conv.to_dict())
            except Exception as e:
                logging.exception(e)
                yield "data:" + json.dumps({"code": 500, "message": str(e), "data": {"answer": "**ERROR**: " + str(e), "reference": []}}, ensure_ascii=False) + "\n\n"
            yield "data:" + json.dumps({"code": 0, "message": "", "data": True}, ensure_ascii=False) + "\n\n"

        if req.get("stream", True):
            resp = Response(stream(), mimetype="text/event-stream")
            resp.headers.add_header("Cache-control", "no-cache")
            resp.headers.add_header("Connection", "keep-alive")
            resp.headers.add_header("X-Accel-Buffering", "no")
            resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
            return resp

        else:
            answer = None
            async for ans in async_chat(dia, msg, **req):
                answer = structure_answer(conv, ans, message_id, conv.id)
                if not is_embedded:
                    ConversationService.update_by_id(conv.id, conv.to_dict())
                break
            return get_json_result(data=answer)
    except Exception as e:
        return server_error_response(e)

@manager.route("/sequence2txt", methods=["POST"])  # noqa: F821
@login_required
@validate_response(200, Sequence2TxtResponse)
@conversation_tag
async def sequence2txt():
    """
    Convert audio to text (Speech-to-Text).

    Uploads an audio file and converts it to text using the configured ASR model.
    Supports both streaming and non-streaming modes.
    Streaming mode returns incremental transcription results via SSE.

    Accepted audio formats: .wav, .mp3, .m4a, .aac, .flac, .ogg, .webm, .opus, .wma

    Requires the tenant to have a default ASR model configured.
    """
    req = await request.form
    stream_mode = req.get("stream", "false").lower() == "true"
    files = await request.files
    if "file" not in files:
        return get_data_error_result(message="Missing 'file' in multipart form-data")

    uploaded = files["file"]

    ALLOWED_EXTS = {
        ".wav", ".mp3", ".m4a", ".aac",
        ".flac", ".ogg", ".webm",
        ".opus", ".wma"
    }

    filename = uploaded.filename or ""
    suffix = os.path.splitext(filename)[-1].lower()
    if suffix not in ALLOWED_EXTS:
        return get_data_error_result(message=
            f"Unsupported audio format: {suffix}. "
            f"Allowed: {', '.join(sorted(ALLOWED_EXTS))}"
        )
    fd, temp_audio_path = tempfile.mkstemp(suffix=suffix)
    os.close(fd)
    await uploaded.save(temp_audio_path)

    tenants = TenantService.get_info_by(current_user.id)
    if not tenants:
        return get_data_error_result(message="Tenant not found!")

    asr_id = tenants[0]["asr_id"]
    if not asr_id:
        return get_data_error_result(message="No default ASR model is set")

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
@login_required
@qs_validate_request(TTSRequest)
@conversation_tag
async def tts():
    """
    Convert text to speech (Text-to-Speech).

    Converts the provided text to audio using the configured TTS model.
    Returns audio data in MP3 format via streaming response.

    The text is automatically split at punctuation marks for better speech synthesis.
    Requires the tenant to have a default TTS model configured.
    """
    req = await get_request_json()
    text = req["text"]

    tenants = TenantService.get_info_by(current_user.id)
    if not tenants:
        return get_data_error_result(message="Tenant not found!")

    tts_id = tenants[0]["tts_id"]
    if not tts_id:
        return get_data_error_result(message="No default TTS model is set")

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


@manager.route("/delete_msg", methods=["POST"])  # noqa: F821
@login_required
@validate_request("conversation_id", "message_id")
@qs_validate_request(DeleteMessageRequest)
@validate_response(200, ConversationResponse)
@conversation_tag
async def delete_msg():
    """
    Delete a message from a conversation.

    Removes a user message and its corresponding assistant response from the conversation.
    Also removes the associated reference documents.
    The message is identified by its unique ID.
    """
    req = await get_request_json()
    e, conv = ConversationService.get_by_id(req["conversation_id"])
    if not e:
        return get_data_error_result(message="Conversation not found!")

    conv = conv.to_dict()
    for i, msg in enumerate(conv["message"]):
        if req["message_id"] != msg.get("id", ""):
            continue
        assert conv["message"][i + 1]["id"] == req["message_id"]
        conv["message"].pop(i)
        conv["message"].pop(i)
        conv["reference"].pop(max(0, i // 2 - 1))
        break

    ConversationService.update_by_id(conv["id"], conv)
    return get_json_result(data=conv)


@manager.route("/thumbup", methods=["POST"])  # noqa: F821
@login_required
@validate_request("conversation_id", "message_id")
@qs_validate_request(ThumbUpRequest)
@validate_response(200, ConversationResponse)
@conversation_tag
async def thumbup():
    """
    Provide feedback on an assistant message.

    Allows users to give thumbs up/down feedback on assistant responses.
    Can optionally include text feedback for thumbs down responses.
    This feedback is used to improve the system's performance.
    """
    req = await get_request_json()
    e, conv = ConversationService.get_by_id(req["conversation_id"])
    if not e:
        return get_data_error_result(message="Conversation not found!")
    up_down = req.get("thumbup")
    feedback = req.get("feedback", "")
    conv = conv.to_dict()
    for i, msg in enumerate(conv["message"]):
        if req["message_id"] == msg.get("id", "") and msg.get("role", "") == "assistant":
            if up_down:
                msg["thumbup"] = True
                if "feedback" in msg:
                    del msg["feedback"]
            else:
                msg["thumbup"] = False
                if feedback:
                    msg["feedback"] = feedback
            break

    ConversationService.update_by_id(conv["id"], conv)
    return get_json_result(data=conv)


@manager.route("/ask", methods=["POST"])  # noqa: F821
@login_required
@validate_request("question", "kb_ids")
@qs_validate_request(AskRequest)
@conversation_tag
async def ask_about():
    """
    Ask a question against knowledge bases.

    Performs semantic search across specified knowledge bases and generates
    an AI response based on the retrieved context. Returns results via SSE streaming.

    This is a standalone question-answering endpoint that doesn't require
    a conversation or dialog. It searches the specified knowledge bases
    and generates an answer using RAG (Retrieval-Augmented Generation).
    """
    req = await get_request_json()
    uid = current_user.id

    search_id = req.get("search_id", "")
    search_app = None
    search_config = {}
    if search_id:
        search_app = SearchService.get_detail(search_id)
    if search_app:
        search_config = search_app.get("search_config", {})

    async def stream():
        nonlocal req, uid
        try:
            async for ans in async_ask(req["question"], req["kb_ids"], uid, search_config=search_config):
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


@manager.route("/mindmap", methods=["POST"])  # noqa: F821
@login_required
@validate_request("question", "kb_ids")
@qs_validate_request(MindMapRequest)
@validate_response(200, MindMapResponse)
@conversation_tag
async def mindmap():
    """
    Generate a mind map for a question.

    Creates a hierarchical mind map representation of a question by analyzing
    relevant content from the specified knowledge bases.

    The mind map structure helps visualize relationships and subtopics
    related to the main question. Useful for exploring complex topics
    and understanding the conceptual structure.
    """
    req = await get_request_json()
    search_id = req.get("search_id", "")
    search_app = SearchService.get_detail(search_id) if search_id else {}
    search_config = search_app.get("search_config", {}) if search_app else {}
    kb_ids = search_config.get("kb_ids", [])
    kb_ids.extend(req["kb_ids"])
    kb_ids = list(set(kb_ids))

    mind_map = await gen_mindmap(req["question"], kb_ids, search_app.get("tenant_id", current_user.id), search_config)
    if "error" in mind_map:
        return server_error_response(Exception(mind_map["error"]))
    return get_json_result(data=mind_map)


@manager.route("/related_questions", methods=["POST"])  # noqa: F821
@login_required
@validate_request("question")
@qs_validate_request(RelatedQuestionsRequest)
@validate_response(200, RelatedQuestionsResponse)
@conversation_tag
async def related_questions():
    """
    Generate related questions for a given question.

    Uses AI to generate a list of related follow-up questions based on
    the input question. These questions can help users explore a topic
    more deeply or discover related aspects they hadn't considered.

    Useful for suggesting next steps in a research or inquiry process.
    """
    req = await get_request_json()

    search_id = req.get("search_id", "")
    search_config = {}
    if search_id:
        if search_app := SearchService.get_detail(search_id):
            search_config = search_app.get("search_config", {})

    question = req["question"]

    chat_id = search_config.get("chat_id", "")
    chat_mdl = LLMBundle(current_user.id, LLMType.CHAT, chat_id)

    gen_conf = search_config.get("llm_setting", {"temperature": 0.9})
    if "parameter" in gen_conf:
        del gen_conf["parameter"]
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
