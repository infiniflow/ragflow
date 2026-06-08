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
"""REST endpoints for the Prompt Optimisation Pipeline.

Routes:
  POST   /chats/<chat_id>/optimise                         — start a new run
  GET    /chats/<chat_id>/optimise/<run_id>                — poll run status + variants
  POST   /chats/<chat_id>/optimise/<run_id>/promote        — promote a variant
  GET    /prompts/variants                                 — list variants for current user
  GET    /prompts/variants/<variant_id>/diff               — unified diff vs current prompt
"""

from quart import request

from api.apps import current_user, login_required
from api.db.services.dialog_service import DialogService
from api.db.services.prompt_optimisation_service import PromptOptimisationService
from api.db.services.user_service import UserTenantService
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    server_error_response,
)


# ── POST /chats/<chat_id>/optimise ──────────────────────────────────────────

@manager.route("/chats/<chat_id>/optimise", methods=["POST"])  # noqa: F821
@login_required
async def start_optimise(chat_id):
    """Start a prompt optimisation run for ``chat_id``."""
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")
        tenant_id = tenants[0].tenant_id

        ok, dialog = DialogService.get_by_id(chat_id)
        if not ok:
            return get_data_error_result(message="Chat not found!")
        if dialog.tenant_id != tenant_id:
            return get_data_error_result(message="No permission for this chat.")

        req = await request.get_json(force=True) or {}
        eval_dataset_id = req.get("eval_dataset_id")
        n_variants = int(req.get("n_variants", 5))
        if n_variants < 1 or n_variants > 10:
            return get_data_error_result(message="`n_variants` must be between 1 and 10.")

        run_id = PromptOptimisationService.start_run(
            tenant_id=tenant_id,
            source_type="dialog",
            source_id=chat_id,
            eval_dataset_id=eval_dataset_id,
            triggered_by="manual",
            n_variants=n_variants,
        )
        return get_json_result(data={"run_id": run_id})
    except Exception as ex:
        return server_error_response(ex)


# ── GET /chats/<chat_id>/optimise/<run_id> ──────────────────────────────────

@manager.route("/chats/<chat_id>/optimise/<run_id>", methods=["GET"])  # noqa: F821
@login_required
async def get_optimise_run(chat_id, run_id):
    """Return run status and associated variant rows."""
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")
        tenant_id = tenants[0].tenant_id

        ok, dialog = DialogService.get_by_id(chat_id)
        if not ok:
            return get_data_error_result(message="Chat not found!")
        if dialog.tenant_id != tenant_id:
            return get_data_error_result(message="No permission for this chat.")

        run = PromptOptimisationService.get_run(run_id)
        if run is None:
            return get_data_error_result(message="Run not found!")
        if run.get("source_id") != chat_id:
            return get_data_error_result(message="Run does not belong to this chat.")

        return get_json_result(data=run)
    except Exception as ex:
        return server_error_response(ex)


# ── POST /chats/<chat_id>/optimise/<run_id>/promote ─────────────────────────

@manager.route("/chats/<chat_id>/optimise/<run_id>/promote", methods=["POST"])  # noqa: F821
@login_required
async def promote_variant(chat_id, run_id):
    """Promote a specific variant to active for ``chat_id``."""
    try:
        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")
        tenant_id = tenants[0].tenant_id

        ok, dialog = DialogService.get_by_id(chat_id)
        if not ok:
            return get_data_error_result(message="Chat not found!")
        if dialog.tenant_id != tenant_id:
            return get_data_error_result(message="No permission for this chat.")

        req = await request.get_json(force=True) or {}
        variant_id = req.get("variant_id", "").strip()
        if not variant_id:
            return get_data_error_result(message="`variant_id` is required.")

        success, message = PromptOptimisationService.promote_variant(run_id, variant_id)
        if not success:
            return get_data_error_result(message=message)
        return get_json_result(data={"promoted": True})
    except Exception as ex:
        return server_error_response(ex)


# ── GET /prompts/variants ───────────────────────────────────────────────────

@manager.route("/prompts/variants", methods=["GET"])  # noqa: F821
@login_required
async def list_variants():
    """List all prompt variants for the caller's dialogs.

    Query params: ``source_id`` (required), ``source_type`` (default: "dialog").
    """
    try:
        source_id = request.args.get("source_id", "").strip()
        source_type = request.args.get("source_type", "dialog").strip()
        if not source_id:
            return get_data_error_result(message="`source_id` is required.")

        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")
        tenant_id = tenants[0].tenant_id

        if source_type == "dialog":
            ok, dialog = DialogService.get_by_id(source_id)
            if not ok or dialog.tenant_id != tenant_id:
                return get_data_error_result(message="Source not found or no permission.")

        variants = PromptOptimisationService.list_variants(source_type, source_id)
        return get_json_result(data=variants)
    except Exception as ex:
        return server_error_response(ex)


# ── GET /prompts/variants/<variant_id>/diff ─────────────────────────────────

@manager.route("/prompts/variants/<variant_id>/diff", methods=["GET"])  # noqa: F821
@login_required
async def variant_diff(variant_id):
    """Return a unified diff between the variant and the source's current prompt.

    Query param: ``source_id`` (required).
    """
    try:
        source_id = request.args.get("source_id", "").strip()
        if not source_id:
            return get_data_error_result(message="`source_id` is required.")

        tenants = UserTenantService.query(user_id=current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")
        tenant_id = tenants[0].tenant_id

        ok, dialog = DialogService.get_by_id(source_id)
        if not ok or dialog.tenant_id != tenant_id:
            return get_data_error_result(message="Source not found or no permission.")

        current_prompt = (dialog.prompt_config or {}).get("system", "")
        diff = PromptOptimisationService.get_variant_diff(variant_id, current_prompt)
        return get_json_result(data={"diff": diff})
    except Exception as ex:
        return server_error_response(ex)
