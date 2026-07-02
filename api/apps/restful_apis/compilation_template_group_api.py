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

from quart import Response, request

from api.apps import current_user, login_required
from api.apps.restful_apis.utils.compilation_template_validation import validate_template_payload
from api.db.services.compilation_template_group_service import (
    CompilationTemplateGroupService,
    GroupValidationError,
)
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    get_request_json,
    server_error_response,
    validate_request,
)
from api.utils.pagination_utils import validate_rest_api_page_size


_GROUP_NAME_MAX = 128
_GROUP_DESCRIPTION_MAX = 1024


def _validate_group_payload(req: dict, require_all: bool = True) -> str:
    if require_all:
        for key in ("name", "templates"):
            if key not in req:
                return f"Missing required field: {key}."

    name = req.get("name")
    if name is not None:
        if not isinstance(name, str) or not name.strip():
            return "Invalid template group name."
        if len(name.encode("utf-8")) > _GROUP_NAME_MAX:
            return "Template group name is too long."

    description = req.get("description")
    if description is not None and (not isinstance(description, str) or len(description) > _GROUP_DESCRIPTION_MAX):
        return "Invalid template group description."

    templates = req.get("templates")
    if templates is not None:
        if not isinstance(templates, list) or not templates:
            return "A template group must contain at least one template."
        for child in templates:
            if not isinstance(child, dict):
                return "Invalid template entry in group."
            err = validate_template_payload(child, require_all=True)
            if err:
                return err
    return ""


@manager.route("/compilation_template_groups", methods=["GET"])  # noqa: F821
@login_required
def list_groups() -> Response:
    keywords = request.args.get("keywords", "")
    scope = request.args.get("scope", "")
    page_number = int(request.args.get("page", 0))
    items_per_page = validate_rest_api_page_size(int(request.args.get("page_size", 0)))
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", "true").lower() != "false"

    try:
        groups = CompilationTemplateGroupService.list_saved(current_user.id, keywords, scope, orderby, desc)
        total = len(groups)
        if page_number and items_per_page:
            groups = groups[(page_number - 1) * items_per_page : page_number * items_per_page]
        return get_json_result(data={"groups": groups, "total": total})
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/compilation_template_groups/<group_id>", methods=["GET"])  # noqa: F821
@login_required
def detail(group_id: str) -> Response:
    try:
        group = CompilationTemplateGroupService.get_saved(group_id, current_user.id)
        if group is None:
            return get_data_error_result(message=f"Cannot find compilation template group {group_id}.")
        return get_json_result(data=group)
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/compilation_template_groups", methods=["POST"])  # noqa: F821
@login_required
@validate_request("name", "templates")
async def create() -> Response:
    req = await get_request_json()
    error = _validate_group_payload(req)
    if error:
        return get_data_error_result(message=error)

    name = req["name"].strip()
    if CompilationTemplateGroupService.name_exists(current_user.id, name):
        return get_data_error_result(message="Duplicated compilation template group name.")

    try:
        saved = CompilationTemplateGroupService.create_group(
            tenant_id=current_user.id,
            name=name,
            description=req.get("description", ""),
            templates=req["templates"],
        )
        return get_json_result(data=saved)
    except GroupValidationError as exc:
        return get_data_error_result(message=str(exc))
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/compilation_template_groups/<group_id>", methods=["PUT"])  # noqa: F821
@login_required
async def update(group_id: str) -> Response:
    req = await get_request_json()
    error = _validate_group_payload(req, require_all=False)
    if error:
        return get_data_error_result(message=error)

    existing = CompilationTemplateGroupService.get_saved(group_id, current_user.id)
    if existing is None:
        return get_data_error_result(message=f"Cannot find compilation template group {group_id}.")

    name = req.get("name")
    if isinstance(name, str):
        name = name.strip()
        if CompilationTemplateGroupService.name_exists(current_user.id, name, group_id):
            return get_data_error_result(message="Duplicated compilation template group name.")

    try:
        updated = CompilationTemplateGroupService.update_group(
            group_id=group_id,
            tenant_id=current_user.id,
            name=name if isinstance(name, str) else None,
            description=req.get("description") if "description" in req else None,
            templates=req.get("templates") if "templates" in req else None,
        )
        if updated is None:
            return get_data_error_result(message=f"Cannot find compilation template group {group_id}.")
        return get_json_result(data=updated)
    except GroupValidationError as exc:
        return get_data_error_result(message=str(exc))
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/compilation_template_groups/<group_id>", methods=["DELETE"])  # noqa: F821
@login_required
def delete(group_id: str) -> Response:
    try:
        ok = CompilationTemplateGroupService.delete_group(group_id, current_user.id)
        if not ok:
            return get_data_error_result(message=f"Cannot find compilation template group {group_id}.")
        return get_json_result(data=True)
    except Exception as exc:
        return server_error_response(exc)
