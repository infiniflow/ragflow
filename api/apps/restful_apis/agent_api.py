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

from quart import request

from agent.dsl_migration import normalize_chunker_dsl
from api.apps import current_user, login_required
from api.apps.services.canvas_replica_service import CanvasReplicaService
from api.db import CanvasCategory
from api.db.services.canvas_service import UserCanvasService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import TenantService
from api.db.services.user_canvas_version import UserCanvasVersionService
from api.utils.api_utils import add_tenant_id_to_kwargs, get_data_error_result, get_json_result


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
        tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
        tenants = [member["tenant_id"] for member in tenants]
        tenants.append(current_user.id)
        canvas, total = UserCanvasService.get_by_tenant_ids(
            tenants,
            current_user.id,
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
            current_user.id,
            0,
            0,
            order_by,
            desc,
            keywords,
            canvas_category,
        )

    return get_json_result(data={"canvas": canvas, "total": total})


@manager.route("/agents/<agent_id>", methods=["GET"])  # noqa: F821
@login_required
@add_tenant_id_to_kwargs
def get_agent(agent_id, tenant_id):
    if not UserCanvasService.accessible(agent_id, current_user.id):
        return get_data_error_result(message="canvas not found.")

    exists, canvas = UserCanvasService.get_by_canvas_id(agent_id)
    if not exists:
        return get_data_error_result(message="canvas not found.")

    try:
        CanvasReplicaService.bootstrap(
            canvas_id=agent_id,
            tenant_id=str(tenant_id),
            runtime_user_id=str(current_user.id),
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
