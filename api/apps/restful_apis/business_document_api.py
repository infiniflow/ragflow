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

"""HTTP boundary for governed business requirements documents."""

from urllib.parse import quote

from quart import Response, jsonify, request
from werkzeug.exceptions import BadRequest

from api.apps import current_user, login_required
from api.apps.business_documents.eva_changes import EvaDocumentChangeService
from api.apps.business_documents.errors import BusinessDocumentError
from api.apps.business_documents.exports import BusinessDocumentExportService
from api.apps.business_documents.service import BusinessDocumentService
from api.apps.business_documents.worker import wake_business_document_worker
from api.utils.api_utils import get_request_json
from common.misc_utils import thread_pool_exec


def _success(data, status=200):
    return jsonify({"code": 0, "data": data}), status


def _error(error: BusinessDocumentError):
    payload = {
        "code": error.status,
        "message": error.message,
        "data": {"error_code": error.code, "details": error.details},
    }
    return jsonify(payload), error.status


def _is_admin() -> bool:
    return bool(getattr(current_user, "is_superuser", False))


def _access_role() -> str:
    return str(getattr(current_user, "business_document_role", "AUTHOR_CREATOR") or "AUTHOR_CREATOR")


@manager.route("/business-documents/eva/sources", methods=["GET"])  # noqa: F821
@login_required
async def search_eva_business_document_sources():
    try:
        actor_id = current_user.id
        query = request.args.get("query", "")
        limit = int(request.args.get("limit", 20))
        result = await thread_pool_exec(EvaDocumentChangeService.search_sources, actor_id, query, limit)
        return _success(result)
    except (TypeError, ValueError):
        return _error(BusinessDocumentError("INVALID_EVA_SEARCH", "limit must be an integer", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/eva/changes", methods=["POST"])  # noqa: F821
@login_required
async def create_eva_business_document_change():
    try:
        data = await get_request_json()
        if not data:
            raise BusinessDocumentError("INVALID_EVA_CHANGE", "Request body must be a valid JSON object", 422)
        if "draft_markdown" in data:
            raise BusinessDocumentError(
                "MANUAL_EVA_DRAFT_DISABLED",
                "Нельзя передать готовый текст доработки EVA: его формирует агент по задаче автора.",
                422,
            )
        actor_id = current_user.id
        result = await thread_pool_exec(EvaDocumentChangeService.create_change, actor_id, actor_id, data)
        try:
            result = await thread_pool_exec(
                EvaDocumentChangeService.generate_draft,
                actor_id,
                actor_id,
                result["change_id"],
                {"expected_state_version": result["state_version"]},
            )
        except BusinessDocumentError:
            # The pinned request remains resumable and exposes the generation
            # error plus an explicit retry action in its projection.
            result = await thread_pool_exec(
                EvaDocumentChangeService.get_change,
                actor_id,
                actor_id,
                result["change_id"],
            )
        return _success(result, 201)
    except (AttributeError, TypeError, BadRequest):
        return _error(BusinessDocumentError("INVALID_EVA_CHANGE", "Request body must be a valid JSON object", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/eva/changes", methods=["GET"])  # noqa: F821
@login_required
async def list_eva_business_document_changes():
    try:
        actor_id = current_user.id
        page = int(request.args.get("page", 1))
        page_size = int(request.args.get("page_size", 20))
        result = await thread_pool_exec(EvaDocumentChangeService.list_changes, actor_id, actor_id, page, page_size)
        return _success(result)
    except (TypeError, ValueError):
        return _error(BusinessDocumentError("INVALID_PAGINATION", "page and page_size must be integers", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/eva/changes/<change_id>", methods=["GET"])  # noqa: F821
@login_required
async def get_eva_business_document_change(change_id):
    try:
        actor_id = current_user.id
        result = await thread_pool_exec(EvaDocumentChangeService.get_change, actor_id, actor_id, change_id)
        return _success(result)
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/eva/changes/<change_id>/generate", methods=["POST"])  # noqa: F821
@login_required
async def generate_eva_business_document_change_draft(change_id):
    try:
        data = await get_request_json()
        if not data:
            raise BusinessDocumentError("INVALID_EVA_GENERATION", "Передайте параметры подготовки доработки в непустом JSON-объекте.", 422)
        actor_id = current_user.id
        result = await thread_pool_exec(EvaDocumentChangeService.generate_draft, actor_id, actor_id, change_id, data)
        return _success(result)
    except (AttributeError, TypeError, BadRequest):
        return _error(BusinessDocumentError("INVALID_EVA_GENERATION", "Передайте параметры подготовки доработки в непустом JSON-объекте.", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/eva/changes/<change_id>/approve", methods=["POST"])  # noqa: F821
@login_required
async def approve_eva_business_document_change(change_id):
    try:
        data = await get_request_json()
        actor_id = current_user.id
        result = await thread_pool_exec(EvaDocumentChangeService.approve, actor_id, actor_id, change_id, data)
        return _success(result)
    except (AttributeError, TypeError, BadRequest):
        return _error(BusinessDocumentError("INVALID_EVA_CHANGE", "Request body must be a valid JSON object", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/eva/changes/<change_id>/prepare", methods=["POST"])  # noqa: F821
@login_required
async def prepare_eva_business_document_change(change_id):
    try:
        data = await get_request_json()
        actor_id = current_user.id
        result = await thread_pool_exec(EvaDocumentChangeService.prepare_eva_draft, actor_id, actor_id, change_id, data)
        return _success(result)
    except (AttributeError, TypeError, BadRequest):
        return _error(BusinessDocumentError("INVALID_EVA_CHANGE", "Request body must be a valid JSON object", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/eva/changes/<change_id>/publish", methods=["POST"])  # noqa: F821
@login_required
async def publish_eva_business_document_change(change_id):
    try:
        data = await get_request_json()
        actor_id = current_user.id
        result = await thread_pool_exec(EvaDocumentChangeService.publish, actor_id, actor_id, change_id, data)
        return _success(result)
    except (AttributeError, TypeError, BadRequest):
        return _error(BusinessDocumentError("INVALID_EVA_CHANGE", "Request body must be a valid JSON object", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents", methods=["POST"])  # noqa: F821
@login_required
async def create_business_document():
    try:
        data = await get_request_json()
        if not data:
            raise BusinessDocumentError("INVALID_DOCUMENT", "Request body must be a valid JSON object", 422)
        actor_id = current_user.id
        result = await thread_pool_exec(BusinessDocumentService.create_document, actor_id, actor_id, data, _is_admin(), _access_role())
        return _success(result, 201)
    except (AttributeError, TypeError, BadRequest):
        return _error(BusinessDocumentError("INVALID_DOCUMENT", "Request body must be a valid JSON object", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents", methods=["GET"])  # noqa: F821
@login_required
async def list_business_documents():
    try:
        actor_id = current_user.id
        page = int(request.args.get("page", 1))
        page_size = int(request.args.get("page_size", 20))
        scope = request.args.get("scope", "all")
        return _success(
            await thread_pool_exec(
                BusinessDocumentService.list_documents,
                actor_id,
                actor_id,
                page,
                page_size,
                _is_admin(),
                _access_role(),
                scope,
            )
        )
    except (TypeError, ValueError):
        return _error(BusinessDocumentError("INVALID_PAGINATION", "page and page_size must be integers", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/access/users", methods=["GET"])  # noqa: F821
@login_required
async def list_business_document_access_users():
    try:
        actor_id = current_user.id
        result = await thread_pool_exec(BusinessDocumentService.list_access_users, actor_id, _is_admin(), _access_role())
        return _success(result)
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/access/users/<user_id>", methods=["PATCH"])  # noqa: F821
@login_required
async def update_business_document_access_user(user_id):
    try:
        data = await get_request_json()
        if not data:
            raise BusinessDocumentError("INVALID_ACCESS_ROLE", "Request body must be a valid JSON object", 422)
        result = await thread_pool_exec(BusinessDocumentService.update_user_access_role, current_user.id, user_id, data, _is_admin())
        return _success(result)
    except (AttributeError, TypeError, BadRequest):
        return _error(BusinessDocumentError("INVALID_ACCESS_ROLE", "Request body must be a valid JSON object", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/<document_id>", methods=["GET"])  # noqa: F821
@login_required
async def get_business_document(document_id):
    try:
        tenant_id = current_user.id
        return _success(await thread_pool_exec(BusinessDocumentService.get_document, tenant_id, document_id, tenant_id, _is_admin(), _access_role()))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/<document_id>", methods=["DELETE"])  # noqa: F821
@login_required
async def delete_business_document(document_id):
    try:
        actor_id = current_user.id
        result = await thread_pool_exec(BusinessDocumentService.delete_document, actor_id, document_id, _is_admin(), _access_role())
        return _success(result)
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/<document_id>/owner", methods=["PUT"])  # noqa: F821
@login_required
async def assign_business_document_owner(document_id):
    try:
        data = await get_request_json()
        if not data:
            raise BusinessDocumentError("INVALID_DOCUMENT_ASSIGNMENT", "Request body must be a valid JSON object", 422)
        result = await thread_pool_exec(
            BusinessDocumentService.assign_document,
            current_user.id,
            document_id,
            data,
            _is_admin(),
            _access_role(),
        )
        return _success(result)
    except (AttributeError, TypeError, BadRequest):
        return _error(BusinessDocumentError("INVALID_DOCUMENT_ASSIGNMENT", "Request body must be a valid JSON object", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/<document_id>/eva/pull", methods=["POST"])  # noqa: F821
@login_required
async def pull_business_document_from_eva(document_id):
    try:
        data = await get_request_json()
        actor_id = current_user.id
        result = await thread_pool_exec(BusinessDocumentService.pull_from_eva, actor_id, actor_id, document_id, data, _is_admin(), _access_role())
        return _success(result)
    except (AttributeError, TypeError, BadRequest):
        return _error(BusinessDocumentError("INVALID_EVA_SYNC", "Request body must be a valid JSON object", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/<document_id>/eva/rebind", methods=["POST"])  # noqa: F821
@login_required
async def rebind_business_document_to_eva(document_id):
    try:
        data = await get_request_json()
        actor_id = current_user.id
        result = await thread_pool_exec(BusinessDocumentService.rebind_eva, actor_id, actor_id, document_id, data, _is_admin(), _access_role())
        return _success(result)
    except (AttributeError, TypeError, BadRequest):
        return _error(BusinessDocumentError("INVALID_EVA_BINDING", "Request body must be a valid JSON object", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/<document_id>/eva/changes", methods=["POST"])  # noqa: F821
@login_required
async def create_business_document_eva_change(document_id):
    try:
        data = await get_request_json()
        actor_id = current_user.id
        result = await thread_pool_exec(
            BusinessDocumentService.create_eva_change_from_revision,
            actor_id,
            actor_id,
            document_id,
            data,
            _is_admin(),
            _access_role(),
        )
        return _success(result, 201)
    except (AttributeError, TypeError, BadRequest):
        return _error(BusinessDocumentError("INVALID_EVA_SYNC", "Request body must be a valid JSON object", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/<document_id>/commands", methods=["POST"])  # noqa: F821
@login_required
async def execute_business_document_command(document_id):
    try:
        data = await get_request_json()
        if not data:
            raise BusinessDocumentError("INVALID_COMMAND_REQUEST", "Request body must be a valid JSON object", 422)
        actor_id = current_user.id
        result = await thread_pool_exec(
            BusinessDocumentService.execute_command,
            actor_id,
            actor_id,
            document_id,
            data,
            _is_admin(),
            _access_role(),
        )
        if result.get("job_id"):
            wake_business_document_worker()
        return _success(result, 202 if result.get("job_id") else 200)
    except (AttributeError, TypeError, BadRequest):
        return _error(BusinessDocumentError("INVALID_COMMAND_REQUEST", "Request body must be a valid JSON object", 422))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/<document_id>/revisions", methods=["GET"])  # noqa: F821
@login_required
async def list_business_document_revisions(document_id):
    try:
        tenant_id = current_user.id
        return _success(await thread_pool_exec(BusinessDocumentService.list_revisions, tenant_id, document_id, tenant_id, _is_admin()))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/<document_id>/revisions/<revision_id>", methods=["GET"])  # noqa: F821
@login_required
async def get_business_document_revision(document_id, revision_id):
    try:
        tenant_id = current_user.id
        return _success(await thread_pool_exec(BusinessDocumentService.get_revision, tenant_id, document_id, revision_id, tenant_id, _is_admin()))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/<document_id>/jobs", methods=["GET"])  # noqa: F821
@login_required
async def list_business_document_jobs(document_id):
    try:
        actor_id = current_user.id
        return _success(await thread_pool_exec(BusinessDocumentService.list_jobs, actor_id, actor_id, document_id, _is_admin()))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/<document_id>/exports", methods=["GET"])  # noqa: F821
@login_required
async def list_business_document_exports(document_id):
    try:
        actor_id = current_user.id
        return _success(await thread_pool_exec(BusinessDocumentExportService.list_artifacts, actor_id, actor_id, document_id, _is_admin()))
    except BusinessDocumentError as error:
        return _error(error)


@manager.route("/business-documents/<document_id>/exports/<artifact_id>/download", methods=["GET"])  # noqa: F821
@login_required
async def download_business_document_export(document_id, artifact_id):
    try:
        actor_id = current_user.id
        artifact, content = await thread_pool_exec(
            BusinessDocumentExportService.download,
            actor_id,
            actor_id,
            document_id,
            artifact_id,
            is_admin=_is_admin(),
        )
        response = Response(content, content_type=artifact["mime_type"])
        response.headers["Content-Disposition"] = f"attachment; filename*=UTF-8''{quote(artifact['filename'])}"
        response.headers["Content-Length"] = str(artifact["size"])
        response.headers["ETag"] = f'"{artifact["content_hash"].removeprefix("sha256:")}"'
        return response
    except BusinessDocumentError as error:
        return _error(error)
