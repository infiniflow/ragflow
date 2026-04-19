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

import logging

from quart import Response, request

from agent.canvas import Canvas
from api.db.db_models import APIToken
from api.db.services.api_service import API4ConversationService
from api.db.services.canvas_service import UserCanvasService
from api.db.services.canvas_service import completion as agent_completion
from api.db.services.user_canvas_version import UserCanvasVersionService
from api.db.services.conversation_service import async_iframe_completion as iframe_completion
from api.db.services.dialog_service import DialogService, async_ask, gen_mindmap
from api.db.services.doc_metadata_service import DocMetadataService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.llm_service import LLMBundle
from common.metadata_utils import apply_meta_data_filter
from api.db.services.search_service import SearchService
from api.db.services.user_service import UserTenantService
from api.db.joint_services.tenant_model_service import get_tenant_default_model_by_type, get_model_config_by_id, \
    get_model_config_by_type_and_name
from common.misc_utils import get_uuid
from api.utils.api_utils import check_duplicate_ids, get_error_data_result, get_json_result, \
    get_result, get_request_json, server_error_response, token_required, validate_request
from rag.app.tag import label_question
from rag.prompts.template import load_prompt
from rag.prompts.generator import cross_languages, keyword_extraction
from common.constants import RetCode, LLMType
from common import settings


@token_required
async def create_agent_session(tenant_id, agent_id):
    req = await get_request_json()
    user_id = req.get("user_id") or request.args.get("user_id", tenant_id)
    release_mode = bool(req.get("release", request.args.get("release", False)))

    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        return get_error_data_result("You cannot access the agent.")

    try:
        cvs, dsl = UserCanvasService.get_agent_dsl_with_release(agent_id, release_mode, tenant_id)
    except LookupError:
        return get_error_data_result("Agent not found.")
    except PermissionError as e:
        return get_error_data_result(str(e))

    session_id = get_uuid()
    canvas = Canvas(dsl, tenant_id, agent_id, canvas_id=cvs.id)
    canvas.reset()

    cvs.dsl = json.loads(str(canvas))
    # Get the version title based on release_mode
    version_title = UserCanvasVersionService.get_latest_version_title(cvs.id, release_mode=release_mode)
    conv = {
        "id": session_id,
        "dialog_id": cvs.id,
        "user_id": user_id,
        "message": [{"role": "assistant", "content": canvas.get_prologue()}],
        "source": "agent",
        "dsl": cvs.dsl,
        "version_title": version_title
    }
    API4ConversationService.save(**conv)
    conv["agent_id"] = conv.pop("dialog_id")
    return get_result(data=conv)


@manager.route("/agents/<agent_id>/sessions", methods=["DELETE"])  # noqa: F821
@token_required
async def delete_agent_session(tenant_id, agent_id):
    errors = []
    success_count = 0
    req = await get_request_json()
    cvs = UserCanvasService.query(user_id=tenant_id, id=agent_id)
    if not cvs:
        return get_error_data_result(f"You don't own the agent {agent_id}")

    if not req:
        return get_result()

    ids = req.get("ids")
    if not ids:
        if req.get("delete_all") is True:
            ids = [conv.id for conv in API4ConversationService.query(dialog_id=agent_id)]
            if not ids:
                return get_result()
        else:
            return get_result()

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



@manager.route("/chatbots/<dialog_id>/completions", methods=["POST"])  # noqa: F821
async def chatbot_completions(dialog_id):
    req = await get_request_json()

    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!')
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
        return get_error_data_result(message='Authorization is not valid!')
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
        return get_error_data_result(message='Authorization is not valid!')
    token = token[1]
    objs = APIToken.query(beta=token)
    if not objs:
        return get_error_data_result(message='Authentication error: API key is invalid!"')

    if req.get("stream", True):
        async def stream():
            try:
                async for answer in agent_completion(objs[0].tenant_id, agent_id, **req):
                    yield answer
            except Exception as e:
                logging.exception(e)
                error_result = get_error_data_result(message=str(e) or "Unknown error")
                yield "data:" + json.dumps(
                    {
                        "event": "message",
                        "data": {"content": f"Error {error_result['code']}: {error_result['message']}\n\n"},
                        **error_result,
                    },
                    ensure_ascii=False,
                ) + "\n\n"

        resp = Response(stream(), mimetype="text/event-stream")
        resp.headers.add_header("Cache-control", "no-cache")
        resp.headers.add_header("Connection", "keep-alive")
        resp.headers.add_header("X-Accel-Buffering", "no")
        resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
        return resp

    try:
        async for answer in agent_completion(objs[0].tenant_id, agent_id, **req):
            return get_result(data=answer)
    except Exception as e:
        logging.exception(e)
        return get_error_data_result(message=str(e) or "Unknown error")

    return None

@manager.route("/agentbots/<agent_id>/inputs", methods=["GET"])  # noqa: F821
async def begin_inputs(agent_id):
    token = request.headers.get("Authorization").split()
    if len(token) != 2:
        return get_error_data_result(message='Authorization is not valid!')
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
        return get_error_data_result(message='Authorization is not valid!')
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
        return get_error_data_result(message='Authorization is not valid!')
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
    tenant_rerank_id = req.get("tenant_rerank_id", "")
    tenant_id = objs[0].tenant_id
    if not tenant_id:
        return get_error_data_result(message="permission denined.")
    search_config = {}

    async def _retrieval():
        nonlocal similarity_threshold, vector_similarity_weight, top, rerank_id
        local_doc_ids = list(doc_ids) if doc_ids else []
        tenant_ids = []
        _question = question

        meta_data_filter = {}
        chat_mdl = None
        if req.get("search_id", ""):
            nonlocal search_config
            detail = SearchService.get_detail(req.get("search_id", ""))
            if detail:
                search_config = detail.get("search_config", {})
                meta_data_filter = search_config.get("meta_data_filter", {})
            if meta_data_filter.get("method") in ["auto", "semi_auto"]:
                chat_id = search_config.get("chat_id", "")
                if chat_id:
                    chat_model_config = get_model_config_by_type_and_name(tenant_id, LLMType.CHAT, chat_id)
                else:
                    chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
                chat_mdl = LLMBundle(tenant_id, chat_model_config)
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
                chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
                chat_mdl = LLMBundle(tenant_id, chat_model_config)

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
        if kb.tenant_embd_id:
            embd_model_config = get_model_config_by_id(kb.tenant_embd_id)
        else:
            embd_model_config = get_model_config_by_type_and_name(kb.tenant_id, LLMType.EMBEDDING, kb.embd_id)
        embd_mdl = LLMBundle(kb.tenant_id, embd_model_config)

        rerank_mdl = None
        if tenant_rerank_id:
            rerank_model_config = get_model_config_by_id(tenant_rerank_id)
            rerank_mdl = LLMBundle(kb.tenant_id, rerank_model_config)
        elif rerank_id:
            rerank_model_config = get_model_config_by_type_and_name(tenant_id, LLMType.RERANK, rerank_id)
            rerank_mdl = LLMBundle(kb.tenant_id, rerank_model_config)

        if req.get("keyword", False):
            default_chat_model = get_tenant_default_model_by_type(kb.tenant_id, LLMType.CHAT)
            chat_mdl = LLMBundle(kb.tenant_id, default_chat_model)
            _question += await keyword_extraction(chat_mdl, _question)

        labels = label_question(_question, [kb])
        ranks = await settings.retriever.retrieval(
            _question, embd_mdl, tenant_ids, kb_ids, page, size, similarity_threshold, vector_similarity_weight, top,
            local_doc_ids, rerank_mdl=rerank_mdl, highlight=req.get("highlight"), rank_feature=labels
        )
        if use_kg:
            default_chat_model = get_tenant_default_model_by_type(kb.tenant_id, LLMType.CHAT)
            ck = await settings.kg_retriever.retrieval(_question, tenant_ids, kb_ids, embd_mdl,
                                                 LLMBundle(kb.tenant_id, default_chat_model))
            if ck["content_with_weight"]:
                ranks["chunks"].insert(0, ck)

        for c in ranks["chunks"]:
            c.pop("vector", None)

        include_metadata, metadata_fields = _resolve_reference_metadata(req, search_config)
        if include_metadata:
            _enrich_retrieval_chunks_with_metadata(ranks["chunks"], metadata_fields)

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
        return get_error_data_result(message='Authorization is not valid!')
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
    if chat_id:
        chat_model_config = get_model_config_by_type_and_name(tenant_id, LLMType.CHAT, chat_id)
    else:
        chat_model_config = get_tenant_default_model_by_type(tenant_id, LLMType.CHAT)
    chat_mdl = LLMBundle(tenant_id, chat_model_config)

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
        return get_error_data_result(message='Authorization is not valid!')
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
        return get_error_data_result(message='Authorization is not valid!')
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

def _resolve_reference_metadata(req, search_config=None):
    """
    Resolve metadata include/fields from request and optional search config.
    Request values take precedence over search config values.
    Also supports legacy flat keys: include_metadata / metadata_fields.
    """
    config_ref = (search_config or {}).get("reference_metadata", {})
    request_ref = req.get("reference_metadata", {})

    resolved = {}
    if isinstance(config_ref, dict):
        resolved.update(config_ref)
    if isinstance(request_ref, dict):
        resolved.update(request_ref)

    if "include_metadata" in req and "include" not in resolved:
        resolved["include"] = bool(req.get("include_metadata"))
    if "metadata_fields" in req and "fields" not in resolved:
        resolved["fields"] = req.get("metadata_fields")

    include_metadata = bool(resolved.get("include", False))
    fields = resolved.get("fields")
    if fields is None:
        return include_metadata, None
    if not isinstance(fields, list):
        return include_metadata, set()
    return include_metadata, {f for f in fields if isinstance(f, str)}


def _enrich_retrieval_chunks_with_metadata(chunks, metadata_fields=None):
    """
    Mutates retrieval_test chunk payloads in-place by attaching `document_metadata`.
    """
    if metadata_fields is not None and not metadata_fields:
        return

    doc_ids_by_kb = {}
    for chunk in chunks:
        kb_id = chunk.get("kb_id")
        doc_id = chunk.get("doc_id")
        if not kb_id or not doc_id:
            continue
        doc_ids_by_kb.setdefault(kb_id, set()).add(doc_id)

    if not doc_ids_by_kb:
        return

    meta_by_doc = {}
    for kb_id, doc_ids in doc_ids_by_kb.items():
        meta_map = DocMetadataService.get_metadata_for_documents(list(doc_ids), kb_id)
        if meta_map:
            meta_by_doc.update(meta_map)

    for chunk in chunks:
        doc_id = chunk.get("doc_id")
        if not doc_id:
            continue
        meta = meta_by_doc.get(doc_id)
        if not meta:
            continue
        if metadata_fields is not None:
            meta = {k: v for k, v in meta.items() if k in metadata_fields}
        if meta:
            chunk["document_metadata"] = meta

