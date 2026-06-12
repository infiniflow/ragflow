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
from api.db.db_models import CompilationTemplate
from api.db.services.compilation_template_service import CompilationTemplateService
from api.utils.api_utils import get_data_error_result, get_json_result, get_request_json, server_error_response, validate_request
from api.utils.pagination_utils import validate_rest_api_page_size
from common.constants import StatusEnum
from common.misc_utils import get_uuid


def _validate_template_payload(req: dict, require_all: bool = True) -> str:
    required = ["name", "kind", "config"] if require_all else []
    for key in required:
        if key not in req:
            return f"Missing required field: {key}."

    name = req.get("name")
    if name is not None and (not isinstance(name, str) or not name.strip() or len(name.encode("utf-8")) > 128):
        return "Invalid template name."

    description = req.get("description")
    if description is not None and (not isinstance(description, str) or len(description) > 1024):
        return "Invalid template description."

    kind = req.get("kind")
    if kind is not None and (not isinstance(kind, str) or not kind):
        return "Invalid template kind."

    config = req.get("config")
    if config is not None and not isinstance(config, dict):
        return "Invalid template config."
    if isinstance(config, dict):
        if len(str(config.get("global_rules") or "")) > 4096:
            return "Global compilation rules is too long."
        for section in ["entity", "relation"]:
            fields = ((config.get(section) or {}).get("fields") or [])
            seen_types = set()
            for field in fields:
                field_type = str((field or {}).get("type") or "").strip()
                if not field_type:
                    return f"{section.capitalize()} type is required."
                if field_type in seen_types:
                    return f"{section.capitalize()} type can not be duplicated."
                seen_types.add(field_type)
                if not str((field or {}).get("description") or "").strip():
                    return f"{section.capitalize()} field description is required."
                if len(str((field or {}).get("description") or "")) > 1024:
                    return f"{section.capitalize()} field description is too long."
                if len(str((field or {}).get("rule") or "")) > 1024:
                    return f"{section.capitalize()} field rule is too long."
        if config.get("kind") == "artifacts" or req.get("kind") == "artifacts":
            for field in ((config.get("claim") or {}).get("fields") or []):
                if not str((field or {}).get("statement") or "").strip():
                    return "Claim statement is required."
                if not str((field or {}).get("subject") or "").strip():
                    return "Claim subject is required."
                if len(str((field or {}).get("statement") or "")) > 1024:
                    return "Claim statement is too long."
                if len(str((field or {}).get("subject") or "")) > 1024:
                    return "Claim subject is too long."
            for field in ((config.get("concept") or {}).get("fields") or []):
                if not str((field or {}).get("term") or "").strip():
                    return "Concept term is required."
                if not str((field or {}).get("definition_excerpt") or "").strip():
                    return "Concept definition excerpt is required."
                if len(str((field or {}).get("term") or "")) > 1024:
                    return "Concept term is too long."
                if len(str((field or {}).get("definition_excerpt") or "")) > 1024:
                    return "Concept definition excerpt is too long."

    return ""


@manager.route("/compilation_templates", methods=["GET"])  # noqa: F821
@login_required
def list_templates() -> Response:
    keywords = request.args.get("keywords", "")
    kind = request.args.get("kind", "")
    page_number = int(request.args.get("page", 0))
    items_per_page = validate_rest_api_page_size(int(request.args.get("page_size", 0)))
    orderby = request.args.get("orderby", "create_time")
    desc = request.args.get("desc", "true").lower() != "false"

    try:
        templates = CompilationTemplateService.list_saved(current_user.id, keywords, kind, orderby, desc)
        total = len(templates)
        if page_number and items_per_page:
            templates = templates[(page_number - 1) * items_per_page : page_number * items_per_page]
        return get_json_result(data={"templates": templates, "total": total})
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/compilation_templates/builtins", methods=["GET"])  # noqa: F821
@login_required
def list_builtin_templates() -> Response:
    try:
        templates = CompilationTemplateService.list_builtins()
        if not templates:
            CompilationTemplateService.seed_builtins_from_files()
            templates = CompilationTemplateService.list_builtins()
        if not templates:
            templates = [
                {
                    "id": template["id"],
                    "kind": template["kind"],
                    "display_name": template["name"],
                    "description": template.get("description", ""),
                    "config": template["config"],
                }
                for template in CompilationTemplateService.load_builtins_from_files()
            ]
        return get_json_result(data=templates)
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/compilation_templates/<template_id>", methods=["GET"])  # noqa: F821
@login_required
def detail(template_id: str) -> Response:
    try:
        template = CompilationTemplateService.get_saved(template_id, current_user.id)
        if template is None:
            return get_data_error_result(message=f"Cannot find compilation template {template_id}.")
        return get_json_result(data=template)
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/compilation_templates", methods=["POST"])  # noqa: F821
@login_required
@validate_request("name", "kind", "config")
async def create() -> Response:
    req = await get_request_json()
    error = _validate_template_payload(req)
    if error:
        return get_data_error_result(message=error)

    name = req["name"].strip()
    if CompilationTemplateService.name_exists(current_user.id, name):
        return get_data_error_result(message="Duplicated compilation template name.")

    data = {
        "id": get_uuid(),
        "tenant_id": current_user.id,
        "name": name,
        "description": req.get("description", ""),
        "kind": req["kind"],
        "config": req["config"],
        "is_builtin": False,
        "status": StatusEnum.VALID.value,
    }
    try:
        CompilationTemplateService.insert(**data)
        return get_json_result(data=data)
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/compilation_templates/<template_id>", methods=["PUT"])  # noqa: F821
@login_required
async def update(template_id: str) -> Response:
    req = await get_request_json()
    error = _validate_template_payload(req, require_all=False)
    if error:
        return get_data_error_result(message=error)

    existing = CompilationTemplateService.get_saved(template_id, current_user.id)
    if existing is None:
        return get_data_error_result(message=f"Cannot find compilation template {template_id}.")

    data = {key: req[key] for key in ["name", "description", "kind", "config"] if key in req}
    if "name" in data:
        data["name"] = data["name"].strip()
        if CompilationTemplateService.name_exists(current_user.id, data["name"], template_id):
            return get_data_error_result(message="Duplicated compilation template name.")

    try:
        CompilationTemplateService.filter_update(
            [
                CompilationTemplate.id == template_id,
                CompilationTemplate.tenant_id == current_user.id,
                CompilationTemplate.is_builtin == False,
            ],
            data,
        )
        updated = CompilationTemplateService.get_saved(template_id, current_user.id)
        return get_json_result(data=updated)
    except Exception as exc:
        return server_error_response(exc)


@manager.route("/compilation_templates/<template_id>", methods=["DELETE"])  # noqa: F821
@login_required
def delete(template_id: str) -> Response:
    existing = CompilationTemplateService.get_saved(template_id, current_user.id)
    if existing is None:
        return get_data_error_result(message=f"Cannot find compilation template {template_id}.")

    try:
        CompilationTemplateService.filter_update(
            [
                CompilationTemplate.id == template_id,
                CompilationTemplate.tenant_id == current_user.id,
                CompilationTemplate.is_builtin == False,
            ],
            {"status": StatusEnum.INVALID.value},
        )
        return get_json_result(data=True)
    except Exception as exc:
        return server_error_response(exc)
