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

import asyncio
import base64
import hashlib
import hmac
import ipaddress
import json
import logging
import time
from typing import Any, cast

import jwt

from agent.canvas import Canvas
from api.apps.services.canvas_replica_service import CanvasReplicaService
from api.db import CanvasCategory
from api.db.services.canvas_service import UserCanvasService
from api.db.services.file_service import FileService
from api.db.services.user_service import UserService
from api.db.services.user_canvas_version import UserCanvasVersionService
from common.constants import RetCode
from common.misc_utils import get_uuid
from api.utils.api_utils import get_data_error_result, get_error_data_result, get_json_result, get_request_json, token_required
from api.utils.api_utils import get_result
from quart import request, Response
from rag.utils.redis_conn import REDIS_CONN


def _get_user_nickname(user_id: str) -> str:
    exists, user = UserService.get_by_id(user_id)
    if not exists:
        return user_id
    return str(getattr(user, "nickname", "") or user_id)


@manager.route('/agents', methods=['GET'])  # noqa: F821
@token_required
def list_agents(tenant_id):
    id = request.args.get("id")
    title = request.args.get("title")
    if id or title:
        canvas = UserCanvasService.query(id=id, title=title, user_id=tenant_id)
        if not canvas:
            return get_error_data_result("The agent doesn't exist.")
    page_number = int(request.args.get("page", 1))
    items_per_page = int(request.args.get("page_size", 30))
    order_by = request.args.get("orderby", "update_time")
    if str(request.args.get("desc","false")).lower() == "false":
        desc = False
    else:
        desc = True
    canvas = UserCanvasService.get_list(tenant_id, page_number, items_per_page, order_by, desc, id, title)
    return get_result(data=canvas)


@manager.route("/agents", methods=["POST"])  # noqa: F821
@token_required
async def create_agent(tenant_id: str):
    req: dict[str, Any] = cast(dict[str, Any], await get_request_json())
    req["user_id"] = tenant_id

    if req.get("dsl") is not None:
        try:
            req["dsl"] = CanvasReplicaService.normalize_dsl(req["dsl"])
        except ValueError as e:
            return get_json_result(data=False, message=str(e), code=RetCode.ARGUMENT_ERROR)
    else:
        return get_json_result(data=False, message="No DSL data in request.", code=RetCode.ARGUMENT_ERROR)

    if req.get("title") is not None:
        req["title"] = req["title"].strip()
    else:
        return get_json_result(data=False, message="No title in request.", code=RetCode.ARGUMENT_ERROR)

    if UserCanvasService.query(user_id=tenant_id, title=req["title"]):
        return get_data_error_result(message=f"Agent with title {req['title']} already exists.")

    agent_id = get_uuid()
    req["id"] = agent_id

    if not UserCanvasService.save(**req):
        return get_data_error_result(message="Fail to create agent.")

    owner_nickname = _get_user_nickname(tenant_id)
    UserCanvasVersionService.save_or_replace_latest(
        user_canvas_id=agent_id,
        title=UserCanvasVersionService.build_version_title(owner_nickname, req.get("title")),
        dsl=req["dsl"]
    )

    return get_json_result(data=True)


@manager.route("/agents/<agent_id>", methods=["PUT"])  # noqa: F821
@token_required
async def update_agent(tenant_id: str, agent_id: str):
    req: dict[str, Any] = {k: v for k, v in cast(dict[str, Any], (await get_request_json())).items() if v is not None}
    req["user_id"] = tenant_id

    if req.get("dsl") is not None:
        try:
            req["dsl"] = CanvasReplicaService.normalize_dsl(req["dsl"])
        except ValueError as e:
            return get_json_result(data=False, message=str(e), code=RetCode.ARGUMENT_ERROR)

    if req.get("title") is not None:
        req["title"] = req["title"].strip()

    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        return get_json_result(
            data=False, message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR)

    _, current_agent = UserCanvasService.get_by_id(agent_id)
    agent_title_for_version = req.get("title") or (current_agent.title if current_agent else "")
    owner_nickname = _get_user_nickname(tenant_id)

    UserCanvasService.update_by_id(agent_id, req)

    if req.get("dsl") is not None:
        UserCanvasVersionService.save_or_replace_latest(
            user_canvas_id=agent_id,
            title=UserCanvasVersionService.build_version_title(owner_nickname, agent_title_for_version),
            dsl=req["dsl"]
        )

    return get_json_result(data=True)


@manager.route("/agents/<agent_id>", methods=["DELETE"])  # noqa: F821
@token_required
def delete_agent(tenant_id: str, agent_id: str):
    if not UserCanvasService.query(user_id=tenant_id, id=agent_id):
        return get_json_result(
            data=False, message="Only owner of canvas authorized for this operation.",
            code=RetCode.OPERATING_ERROR)

    UserCanvasService.delete_by_id(agent_id)
    return get_json_result(data=True)

