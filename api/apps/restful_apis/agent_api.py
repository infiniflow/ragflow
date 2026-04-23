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

import inspect
import copy
import json
import logging
from functools import partial

from quart import Response, jsonify, request

from agent.component import LLM
from agent.canvas import Canvas
from agent.dsl_migration import normalize_chunker_dsl
from api.apps import login_required
from api.apps.services.canvas_replica_service import CanvasReplicaService
from api.db import CanvasCategory
from api.db.db_models import Task
from api.db.services.api_service import API4ConversationService
from api.db.services.canvas_service import (
    CanvasTemplateService,
    UserCanvasService,
    completion as agent_completion,
    completion_openai,
)
from api.db.services.document_service import DocumentService
from api.db.services.file_service import FileService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.pipeline_operation_log_service import PipelineOperationLogService
from api.db.services.task_service import CANVAS_DEBUG_DOC_ID, TaskService, queue_dataflow
from api.db.services.user_service import TenantService, UserService
from api.db.services.user_canvas_version import UserCanvasVersionService
from api.utils.api_utils import (
    add_tenant_id_to_kwargs,
    get_data_error_result,
    get_json_result,
    get_result,
    get_request_json,
    server_error_response,
    validate_request,
)
from common.constants import RetCode
from common.misc_utils import get_uuid, thread_pool_exec
from common import settings
from peewee import MySQLDatabase, PostgresqlDatabase
from rag.flow.pipeline import Pipeline
from rag.nlp import search
from rag.utils.redis_conn import REDIS_CONN


def _get_user_nickname(user_id: str) -> str:
    exists, user = UserService.get_by_id(user_id)
    if not exists:
        return user_id
    return str(getattr(user, "nickname", "") or user_id)


def _build_sse_response(body):
    resp = Response(body, mimetype="text/event-stream")
    resp.headers.add_header("Cache-control", "no-cache")
    resp.headers.add_header("Connection", "keep-alive")
    resp.headers.add_header("X-Accel-Buffering", "no")
    resp.headers.add_header("Content-Type", "text/event-stream; charset=utf-8")
    return resp


def _normalize_agent_session(conv):
    conv["messages"] = conv.pop("message")
    for info in conv["messages"]:
        if "prompt" in info:
            info.pop("prompt")
    conv["agent_id"] = conv.pop("dialog_id")
    if conv["reference"]:
        chunk_num = 0
        for message_num, message in enumerate(conv["messages"]):
            if message_num != 0 and message["role"] != "user":
                chunks = conv["reference"][chunk_num]["chunks"]
                message["reference"] = [
                    {
                        "id": chunk.get("chunk_id", chunk.get("id")),
                        "content": chunk.get("content_with_weight", chunk.get("content")),
                        "document_id": chunk.get("doc_id", chunk.get("document_id")),
                        "document_name": chunk.get("docnm_kwd", chunk.get("document_name")),
                        "dataset_id": chunk.get("kb_id", chunk.get("dataset_id")),
                        "image_id": chunk.get("image_id", chunk.get("img_id")),
                        "positions": chunk.get("positions", chunk.get("position_int")),
                    }
                    for chunk in chunks
                ]
                chunk_num += 1
    del conv["reference"]
    return conv


@manager.route("/agents/<agent_id>/sessions", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def list_agent_sessions(agent_id, tenant_id):
    if not UserCanvasService.accessible(agent_id, tenant_id):
        return get_json_result(
            data=False,
            message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR,
        )

    session_id = request.args.get("id")
    user_id = request.args.get("user_id")
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 30))
    keywords = request.args.get("keywords")
    from_date = request.args.get("from_date")
    to_date = request.args.get("to_date")
    orderby = request.args.get("orderby", "update_time")
    exp_user_id = request.args.get("exp_user_id")
    desc = request.args.get("desc") not in {"False", "false"}

    if exp_user_id:
        sessions = API4ConversationService.get_names(agent_id, exp_user_id)
        result = get_json_result(data=sessions)
        result["total"] = len(sessions)
        return result

    include_dsl = request.args.get("dsl") not in {"False", "false"}
    total, sessions = API4ConversationService.get_list(
        agent_id,
        tenant_id,
        page_number,
        items_per_page,
        orderby,
        desc,
        session_id,
        user_id,
        include_dsl,
        keywords,
        from_date,
        to_date,
        exp_user_id=exp_user_id,
    )
    sessions = [_normalize_agent_session(session) for session in sessions]
    result = get_json_result(data=sessions)
    result["total"] = total
    return result


@manager.route("/agents/<agent_id>/sessions", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def create_agent_session(agent_id, tenant_id):
    req = await get_request_json()
    user_id = req.get("user_id") or request.args.get("user_id", tenant_id)
    release_mode = bool(req.get("release", request.args.get("release", False)))

    try:
        cvs, dsl = UserCanvasService.get_agent_dsl_with_release(agent_id, release_mode, tenant_id)
    except LookupError:
        return get_data_error_result(message="Agent not found.")
    except PermissionError as e:
        return get_data_error_result(message=str(e))

    session_id = get_uuid()
    canvas = Canvas(dsl, tenant_id, agent_id, canvas_id=cvs.id)
    canvas.reset()

    cvs.dsl = json.loads(str(canvas))
    version_title = UserCanvasVersionService.get_latest_version_title(cvs.id, release_mode=release_mode)
    conv = {
        "id": session_id,
        "name": req.get("name", ""),
        "dialog_id": cvs.id,
        "user_id": user_id,
        "exp_user_id": user_id,
        "message": [{"role": "assistant", "content": canvas.get_prologue()}],
        "source": "agent",
        "dsl": cvs.dsl,
        "reference": [],
        "version_title": version_title,
    }
    API4ConversationService.save(**conv)
    return get_result(data=_normalize_agent_session(conv))


@manager.route("/agents/<agent_id>/sessions/<session_id>", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def get_agent_session(agent_id, session_id, tenant_id):
    if not UserCanvasService.accessible(agent_id, tenant_id):
        return get_json_result(
            data=False,
            message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR,
        )
    _, conv = API4ConversationService.get_by_id(session_id)
    return get_json_result(data=conv.to_dict())


@manager.route("/agents/<agent_id>/sessions/<session_id>", methods=["DELETE"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def delete_agent_session_item(agent_id, session_id, tenant_id):
    if not UserCanvasService.accessible(agent_id, tenant_id):
        return get_json_result(
            data=False,
            message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR,
        )
    return get_json_result(data=API4ConversationService.delete_by_id(session_id))


async def _iter_session_completion_events(tenant_id, agent_id, req, return_trace):
    # Stream and non-stream session completions share the same event parsing and trace injection.
    trace_items = []
    async for answer in agent_completion(tenant_id=tenant_id, agent_id=agent_id, **req):
        if isinstance(answer, str):
            try:
                ans = json.loads(answer[5:])
            except Exception:
                continue
        else:
            ans = answer

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
            yield ans
            continue

        if event in ["message", "message_end"]:
            yield ans


@manager.route("/agents/templates", methods=["GET"])  # noqa: F821
@login_required
def list_agent_template():
    return get_json_result(data=[item.to_dict() for item in CanvasTemplateService.get_all()])


@manager.route("/agents/prompts", methods=["GET"])  # noqa: F821
@login_required
def prompts():
    from rag.prompts.generator import (
        ANALYZE_TASK_SYSTEM,
        ANALYZE_TASK_USER,
        CITATION_PROMPT_TEMPLATE,
        NEXT_STEP,
        REFLECT,
    )

    return get_json_result(
        data={
            "task_analysis": f"{ANALYZE_TASK_SYSTEM}\n\n{ANALYZE_TASK_USER}",
            "plan_generation": NEXT_STEP,
            "reflection": REFLECT,
            "citation_guidelines": CITATION_PROMPT_TEMPLATE,
        }
    )


@manager.route("/agents", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def list_agents(tenant_id):
    keywords = request.args.get("keywords", "")
    canvas_category = request.args.get("canvas_category")
    owner_ids = [item for item in request.args.get("owner_ids", "").strip().split(",") if item]

    page_number = int(request.args.get("page", 0))
    items_per_page = int(request.args.get("page_size", 0))
    order_by = request.args.get("orderby", "create_time")
    desc = str(request.args.get("desc", "true")).lower() != "false"
    tenants = TenantService.get_joined_tenants_by_user_id(tenant_id)
    authorized_owner_ids = {member["tenant_id"] for member in tenants}
    authorized_owner_ids.add(tenant_id)

    if owner_ids:
        requested_owner_ids = set(owner_ids)
        unauthorized_owner_ids = requested_owner_ids - authorized_owner_ids
        if unauthorized_owner_ids:
            return get_json_result(
                data=False,
                message="Only authorized owner_ids can be queried.",
                code=RetCode.OPERATING_ERROR,
            )
        effective_owner_ids = list(requested_owner_ids)
    else:
        effective_owner_ids = list(authorized_owner_ids)

    canvas, total = UserCanvasService.get_by_tenant_ids(
        effective_owner_ids,
        tenant_id,
        page_number,
        items_per_page,
        order_by,
        desc,
        keywords,
        canvas_category,
    )

    return get_json_result(data={"canvas": canvas, "total": total})


@manager.route("/agents", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def create_agent(tenant_id):
    req = {k: v for k, v in (await get_request_json()).items() if v is not None}
    req["user_id"] = tenant_id
    req["canvas_category"] = req.get("canvas_category") or CanvasCategory.Agent
    req["release"] = bool(req.get("release", ""))

    if req.get("dsl") is None:
        return get_json_result(
            data=False,
            message="No DSL data in request.",
            code=RetCode.ARGUMENT_ERROR,
        )

    try:
        req["dsl"] = CanvasReplicaService.normalize_dsl(req["dsl"])
    except ValueError as exc:
        return get_json_result(
            data=False,
            message=str(exc),
            code=RetCode.ARGUMENT_ERROR,
        )

    if req.get("title") is None:
        return get_json_result(
            data=False,
            message="No title in request.",
            code=RetCode.ARGUMENT_ERROR,
        )

    req["title"] = req["title"].strip()
    if UserCanvasService.query(
        user_id=tenant_id,
        title=req["title"],
        canvas_category=req["canvas_category"],
    ):
        return get_data_error_result(message=f"{req['title']} already exists.")

    req["id"] = get_uuid()
    if not UserCanvasService.save(**req):
        return get_data_error_result(message="Fail to create agent.")

    owner_nickname = _get_user_nickname(tenant_id)
    UserCanvasVersionService.save_or_replace_latest(
        user_canvas_id=req["id"],
        title=UserCanvasVersionService.build_version_title(owner_nickname, req.get("title")),
        dsl=req["dsl"],
        release=req.get("release"),
    )
    replica_ok = CanvasReplicaService.replace_for_set(
        canvas_id=req["id"],
        tenant_id=str(tenant_id),
        runtime_user_id=str(tenant_id),
        dsl=req["dsl"],
        canvas_category=req["canvas_category"],
        title=req.get("title", ""),
    )
    if not replica_ok:
        return get_data_error_result(message="canvas saved, but replica sync failed.")

    exists, created_agent = UserCanvasService.get_by_canvas_id(req["id"])
    if not exists:
        return get_data_error_result(message="Fail to create agent.")
    return get_json_result(data=created_agent)


@manager.route("/agents/<agent_id>/upload", methods=["POST"])  # noqa: F821
async def upload_agent_file(agent_id):
    exists, canvas = UserCanvasService.get_by_canvas_id(agent_id)
    if not exists:
        return get_data_error_result(message="canvas not found.")

    user_id = canvas["user_id"]
    files = await request.files
    file_objs = files.getlist("file") if files and files.get("file") else []
    try:
        if len(file_objs) == 1:
            return get_json_result(
                data=FileService.upload_info(user_id, file_objs[0], request.args.get("url"))
            )
        results = [FileService.upload_info(user_id, file_obj) for file_obj in file_objs]
        return get_json_result(data=results)
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/agents/<agent_id>/components/<component_id>/input-form", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def get_agent_component_input_form(agent_id, component_id, tenant_id):
    try:
        exists, user_canvas = UserCanvasService.get_by_id(agent_id)
        if not exists:
            return get_data_error_result(message="canvas not found.")
        if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
            return get_json_result(
                data=False,
                message="Only owner of canvas authorized for this operation.",
                code=RetCode.OPERATING_ERROR,
            )

        canvas = Canvas(json.dumps(user_canvas.dsl), tenant_id, canvas_id=user_canvas.id)
        return get_json_result(data=canvas.get_component_input_form(component_id))
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/agents/<agent_id>/components/<component_id>/debug", methods=["POST"])  # noqa: F821
@validate_request("params")
@login_required
@add_tenant_id_to_kwargs
async def debug_agent_component(agent_id, component_id, tenant_id):
    req = await get_request_json()
    if not UserCanvasService.accessible(agent_id, tenant_id):
        return get_json_result(
            data=False,
            message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR,
        )
    try:
        _, user_canvas = UserCanvasService.get_by_id(agent_id)
        canvas = Canvas(json.dumps(user_canvas.dsl), tenant_id, canvas_id=user_canvas.id)
        canvas.reset()
        canvas.message_id = get_uuid()
        component = canvas.get_component(component_id)["obj"]
        component.reset()

        if isinstance(component, LLM):
            component.set_debug_inputs(req["params"])
        component.invoke(**{k: o["value"] for k, o in req["params"].items()})
        outputs = component.output()
        for k in outputs.keys():
            if isinstance(outputs[k], partial):
                txt = ""
                iter_obj = outputs[k]()
                if inspect.isasyncgen(iter_obj):
                    async for c in iter_obj:
                        txt += c
                else:
                    for c in iter_obj:
                        txt += c
                outputs[k] = txt
        return get_json_result(data=outputs)
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/agents/<agent_id>", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def get_agent(agent_id, tenant_id):
    if not UserCanvasService.accessible(agent_id, tenant_id):
        return get_data_error_result(message="canvas not found.")

    exists, canvas = UserCanvasService.get_by_canvas_id(agent_id)
    if not exists:
        return get_data_error_result(message="canvas not found.")

    try:
        CanvasReplicaService.bootstrap(
            canvas_id=agent_id,
            tenant_id=str(tenant_id),
            runtime_user_id=str(tenant_id),
            dsl=canvas.get("dsl"),
            canvas_category=canvas.get("canvas_category", CanvasCategory.Agent),
            title=canvas.get("title", ""),
        )
    except ValueError as exc:
        return get_data_error_result(message=str(exc))

    last_publish_time = None
    versions = UserCanvasVersionService.list_by_canvas_id(agent_id)
    if versions:
        released_versions = [version for version in versions if version.release]
        if released_versions:
            released_versions.sort(key=lambda version: version.update_time, reverse=True)
            last_publish_time = released_versions[0].update_time

    canvas["dsl"] = normalize_chunker_dsl(canvas.get("dsl", {}))
    canvas["last_publish_time"] = last_publish_time

    if canvas.get("canvas_category") == CanvasCategory.DataFlow:
        datasets = list(KnowledgebaseService.query(pipeline_id=agent_id))
        canvas["datasets"] = [{"id": item.id, "name": item.name, "avatar": item.avatar} for item in datasets]

    return get_json_result(data=canvas)


@manager.route("/agents/<agent_id>/versions", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def list_agent_versions(agent_id, tenant_id):
    if not UserCanvasService.accessible(agent_id, tenant_id):
        return get_json_result(
            data=False,
            message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR,
        )

    try:
        versions = sorted(
            [item.to_dict() for item in UserCanvasVersionService.list_by_canvas_id(agent_id)],
            key=lambda item: item["update_time"] * -1,
        )
        return get_json_result(data=versions)
    except Exception as exc:
        return get_data_error_result(message=f"Error getting history files: {exc}")


@manager.route("/agents/<agent_id>/versions/<version_id>", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def get_agent_version(agent_id, version_id, tenant_id):
    if not UserCanvasService.accessible(agent_id, tenant_id):
        return get_json_result(
            data=False,
            message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR,
        )

    try:
        exists, version = UserCanvasVersionService.get_by_id(version_id)
        if not exists or not version or str(version.user_canvas_id) != str(agent_id):
            return get_data_error_result(message="Version not found.")
        return get_json_result(data=version.to_dict())
    except Exception as exc:
        return get_data_error_result(message=f"Error getting history file: {exc}")


@manager.route("/agents/<agent_id>/logs/<message_id>", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def get_agent_logs(agent_id, message_id, tenant_id):
    if not UserCanvasService.accessible(agent_id, tenant_id):
        return get_json_result(
            data=False,
            message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR,
        )

    try:
        binary = REDIS_CONN.get(f"{agent_id}-{message_id}-logs")
        if not binary:
            return get_json_result(data={})

        return get_json_result(data=json.loads(binary.encode("utf-8")))
    except Exception as exc:
        logging.exception(exc)
        return server_error_response(exc)


@manager.route("/agents/<agent_id>", methods=["DELETE"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def delete_agent(agent_id, tenant_id):
    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        return get_json_result(
            data=False,
            message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR,
        )

    UserCanvasService.delete_by_id(agent_id)
    return get_json_result(data=True)


@manager.route("/agents/<agent_id>", methods=["PUT"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def update_agent(agent_id, tenant_id):
    req = {k: v for k, v in (await get_request_json()).items() if v is not None}
    req["user_id"] = tenant_id

    if req.get("dsl") is not None:
        try:
            req["dsl"] = CanvasReplicaService.normalize_dsl(req["dsl"])
        except ValueError as exc:
            return get_json_result(
                data=False,
                message=str(exc),
                code=RetCode.ARGUMENT_ERROR,
            )

    if req.get("title") is not None:
        req["title"] = req["title"].strip()

    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        return get_json_result(
            data=False,
            message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR,
        )

    _, current_agent = UserCanvasService.get_by_id(agent_id)
    agent_title_for_version = req.get("title") or (current_agent.title if current_agent else "")
    canvas_category = (
        req.get("canvas_category")
        or (current_agent.canvas_category if current_agent else CanvasCategory.Agent)
    )
    owner_nickname = _get_user_nickname(tenant_id)
    UserCanvasService.update_by_id(agent_id, req)

    if req.get("dsl") is not None:
        UserCanvasVersionService.save_or_replace_latest(
            user_canvas_id=agent_id,
            title=UserCanvasVersionService.build_version_title(owner_nickname, agent_title_for_version),
            dsl=req["dsl"],
        )
        replica_ok = CanvasReplicaService.replace_for_set(
            canvas_id=agent_id,
            tenant_id=str(tenant_id),
            runtime_user_id=str(tenant_id),
            dsl=req["dsl"],
            canvas_category=canvas_category,
            title=agent_title_for_version,
        )
        if not replica_ok:
            return get_data_error_result(message="agent saved, but replica sync failed.")

    return get_json_result(data=True)


@manager.route("/agents/<agent_id>/reset", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def reset_agent(agent_id, tenant_id):
    if not UserCanvasService.accessible(agent_id, tenant_id):
        return get_json_result(
            data=False,
            message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR,
        )

    try:
        exists, user_canvas = UserCanvasService.get_by_id(agent_id)
        if not exists:
            return get_data_error_result(message="canvas not found.")

        canvas = Canvas(json.dumps(user_canvas.dsl), tenant_id, canvas_id=user_canvas.id)
        canvas.reset()
        dsl = json.loads(str(canvas))
        UserCanvasService.update_by_id(agent_id, {"dsl": dsl})
        replica_ok = CanvasReplicaService.replace_for_set(
            canvas_id=agent_id,
            tenant_id=str(tenant_id),
            runtime_user_id=str(tenant_id),
            dsl=dsl,
            canvas_category=user_canvas.canvas_category,
            title=user_canvas.title,
        )
        if not replica_ok:
            return get_data_error_result(message="agent reset, but replica sync failed.")
        return get_json_result(data=dsl)
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/agents/rerun", methods=["POST"])  # noqa: F821
@validate_request("id", "dsl", "component_id")
@login_required
@add_tenant_id_to_kwargs
async def rerun_agent(tenant_id):
    req = await get_request_json()
    doc = PipelineOperationLogService.get_documents_info(req["id"])
    if not doc:
        return get_data_error_result(message="Document not found.")
    doc = doc[0]
    if 0 < doc["progress"] < 1:
        return get_data_error_result(message=f"`{doc['name']}` is processing...")

    if settings.docStoreConn.index_exist(search.index_name(tenant_id), doc["kb_id"]):
        settings.docStoreConn.delete({"doc_id": doc["id"]}, search.index_name(tenant_id), doc["kb_id"])
    doc["progress_msg"] = ""
    doc["chunk_num"] = 0
    doc["token_num"] = 0
    DocumentService.clear_chunk_num_when_rerun(doc["id"])
    DocumentService.update_by_id(doc["id"], doc)
    TaskService.filter_delete([Task.doc_id == doc["id"]])

    dsl = req["dsl"]
    dsl["path"] = [req["component_id"]]
    PipelineOperationLogService.update_by_id(req["id"], {"dsl": dsl})
    queue_dataflow(
        tenant_id=tenant_id,
        flow_id=req["id"],
        task_id=get_uuid(),
        doc_id=doc["id"],
        priority=0,
        rerun=True,
    )
    return get_json_result(data=True)


@manager.route("/agents/test_db_connection", methods=["POST"])  # noqa: F821
@validate_request("db_type", "database", "username", "host", "port", "password")
@login_required
async def test_db_connection():
    req = await get_request_json()
    try:
        if req["db_type"] in ["mysql", "mariadb"]:
            db = MySQLDatabase(
                req["database"],
                user=req["username"],
                host=req["host"],
                port=req["port"],
                password=req["password"],
            )
        elif req["db_type"] == "oceanbase":
            db = MySQLDatabase(
                req["database"],
                user=req["username"],
                host=req["host"],
                port=req["port"],
                password=req["password"],
                charset="utf8mb4",
            )
        elif req["db_type"] == "postgres":
            db = PostgresqlDatabase(
                req["database"],
                user=req["username"],
                host=req["host"],
                port=req["port"],
                password=req["password"],
            )
        elif req["db_type"] == "mssql":
            import pyodbc

            connection_string = (
                f"DRIVER={{ODBC Driver 17 for SQL Server}};"
                f"SERVER={req['host']},{req['port']};"
                f"DATABASE={req['database']};"
                f"UID={req['username']};"
                f"PWD={req['password']};"
            )
            db = pyodbc.connect(connection_string)
            cursor = db.cursor()
            cursor.execute("SELECT 1")
            cursor.close()
        elif req["db_type"] == "IBM DB2":
            import ibm_db

            conn_str = (
                f"DATABASE={req['database']};"
                f"HOSTNAME={req['host']};"
                f"PORT={req['port']};"
                f"PROTOCOL=TCPIP;"
                f"UID={req['username']};"
                f"PWD={req['password']};"
            )
            logging.info(
                "DATABASE=%s;HOSTNAME=%s;PORT=%s;PROTOCOL=TCPIP;UID=%s;PWD=****;",
                req["database"],
                req["host"],
                req["port"],
                req["username"],
            )
            conn = ibm_db.connect(conn_str, "", "")
            stmt = ibm_db.exec_immediate(conn, "SELECT 1 FROM sysibm.sysdummy1")
            ibm_db.fetch_assoc(stmt)
            ibm_db.close(conn)
            return get_json_result(data="Database Connection Successful!")
        elif req["db_type"] == "trino":
            import os
            import trino

            db_name = req["database"]
            if "." in db_name:
                catalog, schema = db_name.split(".", 1)
            elif "/" in db_name:
                catalog, schema = db_name.split("/", 1)
            else:
                catalog, schema = db_name, "default"

            http_scheme = "https" if os.environ.get("TRINO_USE_TLS", "0") == "1" else "http"
            auth = None
            if http_scheme == "https" and req.get("password"):
                auth = trino.BasicAuthentication(req.get("username") or "ragflow", req["password"])

            conn = trino.dbapi.connect(
                host=req["host"],
                port=int(req["port"] or 8080),
                user=req["username"] or "ragflow",
                catalog=catalog,
                schema=schema or "default",
                http_scheme=http_scheme,
                auth=auth,
            )
            cur = conn.cursor()
            cur.execute("SELECT 1")
            cur.fetchall()
            cur.close()
            conn.close()
            return get_json_result(data="Database Connection Successful!")
        else:
            return server_error_response("Unsupported database type.")

        if req["db_type"] != "mssql":
            db.connect()
        db.close()
        return get_json_result(data="Database Connection Successful!")
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/agents/chat/completion", methods=["POST"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
async def agent_chat_completion(tenant_id):
    # This endpoint serves two execution modes:
    # 1. Draft/runtime execution without session state. The request runs against the caller's
    #    runtime replica, which is populated from the editable canvas state.
    # 2. Session continuation with an existing session_id. The request resumes from the stored
    #    API4Conversation state and must stay bound to the same agent and an accessible canvas.
    #
    # Security constraints:
    # - agent_id is always supplied at the route layer and is not forwarded downstream as a free-form kwarg.
    # - New runs without session_id must pass UserCanvasService.accessible(...) before the runtime replica is loaded.
    # - Existing sessions are validated here at the route layer before handing control to the lower-level
    #   completion functions, so canvas_service only executes a pre-authorized session payload.
    #
    # Response modes:
    # - Regular mode emits internal agent events.
    # - openai-compatible mode reshapes the same execution into an OpenAI-like wire format.
    req = await get_request_json()
    agent_id = req.get("agent_id")
    openai_compatible = bool(req.get("openai-compatible", False))
    if not agent_id:
        return get_json_result(
            data=False,
            message="`agent_id` is required.",
            code=RetCode.ARGUMENT_ERROR,
        )
    # Route-level selectors should not be forwarded into the lower-level completion functions.
    req = dict(req)
    req.pop("agent_id", None)
    req.pop("openai-compatible", None)
    session_id = req.get("session_id")
    if session_id:
        exists, conv = API4ConversationService.get_by_id(session_id)
        if not exists:
            return get_data_error_result(message="Session not found!")
        if conv.dialog_id != agent_id:
            return get_json_result(
                data=False,
                message="Session does not belong to the requested agent.",
                code=RetCode.OPERATING_ERROR,
            )
        if not UserCanvasService.accessible(agent_id, tenant_id):
            return get_json_result(
                data=False,
                message="Only authorized users can access this agent session.",
                code=RetCode.OPERATING_ERROR,
            )

    if openai_compatible:
        # OpenAI-compatible mode uses a different wire format, keep it separate from regular agent events.
        messages = req.get("messages", [])
        if not messages:
            return get_data_error_result(message="You must provide at least one message.")
        question = next((m.get("content", "") for m in reversed(messages) if m.get("role") == "user"), "")
        stream = req.pop("stream", False)
        session_id = req.pop("session_id", req.get("id", "")) or req.get("metadata", {}).get("id", "")
        if stream:
            return _build_sse_response(
                completion_openai(
                    tenant_id,
                    agent_id,
                    question,
                    session_id=session_id,
                    stream=True,
                    **req,
                )
            )

        async for response in completion_openai(
            tenant_id,
            agent_id,
            question,
            session_id=session_id,
            stream=False,
            **req,
        ):
            return jsonify(response)
        return None

    if not session_id:
        # Without session state, run against the runtime replica that tracks draft edits.
        query = req.get("query", "")
        files = req.get("files", [])
        inputs = req.get("inputs", {})
        runtime_user_id = req.get("user_id") or tenant_id
        user_id = str(runtime_user_id)
        if not await thread_pool_exec(UserCanvasService.accessible, agent_id, tenant_id):
            return get_json_result(
                data=False,
                message="Only owner of canvas authorized for this operation.",
                code=RetCode.OPERATING_ERROR,
            )

        replica_payload = CanvasReplicaService.load_for_run(
            canvas_id=agent_id,
            tenant_id=str(tenant_id),
            runtime_user_id=user_id,
        )
        if not replica_payload:
            return get_data_error_result(message="canvas replica not found, please fetch the agent first.")

        replica_dsl = replica_payload.get("dsl", {})
        canvas_title = replica_payload.get("title", "")
        canvas_category = replica_payload.get("canvas_category", CanvasCategory.Agent)
        dsl_str = json.dumps(replica_dsl, ensure_ascii=False)

        _, cvs = await thread_pool_exec(UserCanvasService.get_by_id, agent_id)
        if cvs.canvas_category == CanvasCategory.DataFlow:
            task_id = get_uuid()
            Pipeline(
                dsl_str,
                tenant_id=str(tenant_id),
                doc_id=CANVAS_DEBUG_DOC_ID,
                task_id=task_id,
                flow_id=agent_id,
            )
            ok, error_message = await thread_pool_exec(
                queue_dataflow,
                user_id,
                agent_id,
                task_id,
                CANVAS_DEBUG_DOC_ID,
                files[0],
                0,
            )
            if not ok:
                return get_data_error_result(message=error_message)
            return get_json_result(data={"message_id": task_id})

        try:
            canvas = Canvas(dsl_str, str(tenant_id), canvas_id=agent_id)
        except Exception as exc:
            return server_error_response(exc)

        async def sse():
            nonlocal canvas
            try:
                async for ans in canvas.run(query=query, files=files, user_id=user_id, inputs=inputs):
                    yield "data:" + json.dumps(ans, ensure_ascii=False) + "\n\n"

                commit_ok = CanvasReplicaService.commit_after_run(
                    canvas_id=agent_id,
                    tenant_id=str(tenant_id),
                    runtime_user_id=user_id,
                    dsl=json.loads(str(canvas)),
                    canvas_category=canvas_category,
                    title=canvas_title,
                )
                if not commit_ok:
                    logging.error(
                        "Canvas runtime replica commit failed: canvas_id=%s tenant_id=%s runtime_user_id=%s",
                        agent_id,
                        tenant_id,
                        user_id,
                    )
            except Exception as exc:
                logging.exception(exc)
                canvas.cancel_task()
                yield (
                    "data:"
                    + json.dumps({"code": 500, "message": str(exc), "data": False}, ensure_ascii=False)
                    + "\n\n"
                )

        return _build_sse_response(sse())

    return_trace = bool(req.get("return_trace", False))
    if req.get("stream", True):

        async def generate():
            async for ans in _iter_session_completion_events(tenant_id, agent_id, req, return_trace):
                yield "data:" + json.dumps(ans, ensure_ascii=False) + "\n\n"
            yield "data:[DONE]\n\n"

        return _build_sse_response(generate())

    full_content = ""
    reference = {}
    final_ans = ""
    trace_items = []
    structured_output = {}
    async for ans in _iter_session_completion_events(tenant_id, agent_id, req, return_trace):
        try:
            if ans["event"] == "message":
                full_content += ans["data"]["content"]
            if ans.get("data", {}).get("reference", None):
                reference.update(ans["data"]["reference"])
            if ans.get("event") == "node_finished":
                data = ans.get("data", {})
                node_out = data.get("outputs", {})
                component_id = data.get("component_id")
                if component_id is not None and "structured" in node_out:
                    structured_output[component_id] = copy.deepcopy(node_out["structured"])
                if return_trace:
                    trace_items = ans.get("data", {}).get("trace", trace_items)
            final_ans = ans
        except Exception as exc:
            return get_result(data=f"**ERROR**: {str(exc)}")

    final_ans["data"]["content"] = full_content
    final_ans["data"]["reference"] = reference
    if structured_output:
        final_ans["data"]["structured"] = structured_output
    if return_trace and final_ans:
        final_ans["data"]["trace"] = trace_items
    return get_result(data=final_ans)
