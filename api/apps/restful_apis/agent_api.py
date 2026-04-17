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

from quart import request

from agent.canvas import Canvas
from agent.dsl_migration import normalize_chunker_dsl
from api.apps import login_required
from api.apps.services.canvas_replica_service import CanvasReplicaService
from api.db import CanvasCategory
from api.db.services.canvas_service import UserCanvasService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import TenantService, UserService
from api.db.services.user_canvas_version import UserCanvasVersionService
from api.utils.api_utils import (
    add_tenant_id_to_kwargs,
    get_data_error_result,
    get_json_result,
    get_request_json,
    server_error_response,
)
from common.constants import RetCode
from common.misc_utils import get_uuid


def _get_user_nickname(user_id: str) -> str:
    exists, user = UserService.get_by_id(user_id)
    if not exists:
        return user_id
    return str(getattr(user, "nickname", "") or user_id)


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

    if not owner_ids:
        tenants = TenantService.get_joined_tenants_by_user_id(tenant_id)
        tenants = [member["tenant_id"] for member in tenants]
        tenants.append(tenant_id)
        canvas, total = UserCanvasService.get_by_tenant_ids(
            tenants,
            tenant_id,
            page_number,
            items_per_page,
            order_by,
            desc,
            keywords,
            canvas_category,
        )
    else:
        canvas, total = UserCanvasService.get_by_tenant_ids(
            owner_ids,
            tenant_id,
            0,
            0,
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


@manager.route("/agents/<agent_id>", methods=["PATCH"])  # noqa: F821
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
    owner_nickname = _get_user_nickname(tenant_id)
    UserCanvasService.update_by_id(agent_id, req)

    if req.get("dsl") is not None:
        UserCanvasVersionService.save_or_replace_latest(
            user_canvas_id=agent_id,
            title=UserCanvasVersionService.build_version_title(owner_nickname, agent_title_for_version),
            dsl=req["dsl"],
        )

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
        return get_json_result(data=dsl)
    except Exception as exc:
        return server_error_response(exc)
