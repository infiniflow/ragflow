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
"""REST endpoints for Cross-Tenant Knowledge Federation.

Routes:
  POST   /federation/grants                          — create grant (owner)
  DELETE /federation/grants/<grant_id>               — revoke grant (owner)
  GET    /federation/grants?role=owner|grantee       — list grants
  GET    /federation/grants/<grant_id>/audit         — paginated audit log (owner only)
  GET    /federation/kbs                             — federated KBs accessible to caller
  PATCH  /datasets/<kb_id>/federation                — toggle federation + published_doc_tags
"""

import logging

from quart import request

from api.apps import current_user, login_required
from api.db.services.federation_service import FederationService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import UserTenantService
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    server_error_response,
)
from common.constants import StatusEnum

logger = logging.getLogger(__name__)


def _tenant_id():
    tenants = UserTenantService.query(user_id=current_user.id, status=StatusEnum.VALID.value)
    if not tenants:
        return None
    return tenants[0].tenant_id


# ── POST /federation/grants ─────────────────────────────────────────────────

@manager.route("/federation/grants", methods=["POST"])  # noqa: F821
@login_required
async def create_grant():
    """Create a federation grant (KB owner only)."""
    try:
        tenant_id = _tenant_id()
        if not tenant_id:
            return get_data_error_result(message="Tenant not found!")

        req = await request.get_json(force=True) or {}
        kb_id = req.get("kb_id", "").strip()
        grantee_tenant_id = req.get("grantee_tenant_id", "").strip()
        policy_json = req.get("policy_json") or []
        valid_until_raw = req.get("valid_until")
        valid_from_raw = req.get("valid_from")

        if not kb_id:
            return get_data_error_result(message="`kb_id` is required.")
        if not grantee_tenant_id:
            return get_data_error_result(message="`grantee_tenant_id` is required.")
        if not isinstance(policy_json, list):
            return get_data_error_result(message="`policy_json` must be a list.")

        try:
            valid_until = int(valid_until_raw) if valid_until_raw is not None else None
            valid_from = int(valid_from_raw) if valid_from_raw is not None else None
        except (TypeError, ValueError):
            return get_data_error_result(message="`valid_until` and `valid_from` must be integers (epoch ms).")

        logger.info(
            "federation create_grant: tenant=%s kb=%s grantee=%s",
            tenant_id, kb_id, grantee_tenant_id,
        )
        grant = FederationService.create_grant(
            owner_tenant_id=tenant_id,
            grantee_tenant_id=grantee_tenant_id,
            kb_id=kb_id,
            policy_json=policy_json,
            valid_until=valid_until,
            valid_from=valid_from,
        )
        return get_json_result(data=grant)
    except (LookupError, ValueError) as ex:
        return get_data_error_result(message=str(ex))
    except Exception as ex:
        return server_error_response(ex)


# ── DELETE /federation/grants/<grant_id> ────────────────────────────────────

@manager.route("/federation/grants/<grant_id>", methods=["DELETE"])  # noqa: F821
@login_required
async def revoke_grant(grant_id):
    """Revoke a grant (owner only)."""
    try:
        tenant_id = _tenant_id()
        if not tenant_id:
            return get_data_error_result(message="Tenant not found!")

        logger.info("federation revoke_grant: tenant=%s grant=%s", tenant_id, grant_id)
        ok = FederationService.revoke_grant(grant_id, tenant_id)
        if not ok:
            return get_data_error_result(message="Grant not found.")
        return get_json_result(data={"revoked": True})
    except PermissionError as ex:
        return get_data_error_result(message=str(ex))
    except Exception as ex:
        return server_error_response(ex)


# ── GET /federation/grants ──────────────────────────────────────────────────

@manager.route("/federation/grants", methods=["GET"])  # noqa: F821
@login_required
async def list_grants():
    """List grants where the caller is owner or grantee.

    Query params: ``role`` (owner|grantee, default owner), ``page``, ``page_size``.
    """
    try:
        tenant_id = _tenant_id()
        if not tenant_id:
            return get_data_error_result(message="Tenant not found!")

        role = request.args.get("role", "owner").strip()
        if role not in ("owner", "grantee"):
            return get_data_error_result(message="`role` must be 'owner' or 'grantee'.")
        page = int(request.args.get("page", 1))
        page_size = int(request.args.get("page_size", 20))

        logger.debug("federation list_grants: tenant=%s role=%s", tenant_id, role)
        grants = FederationService.list_grants(tenant_id, role, page, page_size)
        return get_json_result(data=grants)
    except Exception as ex:
        return server_error_response(ex)


# ── GET /federation/grants/<grant_id>/audit ─────────────────────────────────

@manager.route("/federation/grants/<grant_id>/audit", methods=["GET"])  # noqa: F821
@login_required
async def get_audit_log(grant_id):
    """Return paginated audit log for a grant (owner only)."""
    try:
        tenant_id = _tenant_id()
        if not tenant_id:
            return get_data_error_result(message="Tenant not found!")

        page = int(request.args.get("page", 1))
        page_size = int(request.args.get("page_size", 50))

        logger.debug("federation get_audit_log: tenant=%s grant=%s", tenant_id, grant_id)
        logs = FederationService.get_audit_log(grant_id, tenant_id, page, page_size)
        return get_json_result(data=logs)
    except PermissionError as ex:
        return get_data_error_result(message=str(ex))
    except LookupError as ex:
        return get_data_error_result(message=str(ex))
    except Exception as ex:
        return server_error_response(ex)


# ── GET /federation/kbs ─────────────────────────────────────────────────────

@manager.route("/federation/kbs", methods=["GET"])  # noqa: F821
@login_required
async def list_accessible_kbs():
    """List all federated KBs accessible to the calling tenant."""
    try:
        tenant_id = _tenant_id()
        if not tenant_id:
            return get_data_error_result(message="Tenant not found!")

        logger.debug("federation list_accessible_kbs: tenant=%s", tenant_id)
        kbs = FederationService.list_accessible_kbs(tenant_id)
        return get_json_result(data=kbs)
    except Exception as ex:
        return server_error_response(ex)


# ── PATCH /datasets/<kb_id>/federation ──────────────────────────────────────

@manager.route("/datasets/<kb_id>/federation", methods=["PATCH"])  # noqa: F821
@login_required
async def update_federation(kb_id):
    """Toggle federation and/or update ``published_doc_tags`` for a KB.

    Body: ``{federation_enabled: bool, published_doc_tags: [str, ...]}``.
    Both fields are optional; omit to leave unchanged.
    """
    try:
        tenant_id = _tenant_id()
        if not tenant_id:
            return get_data_error_result(message="Tenant not found!")

        req = await request.get_json(force=True) or {}
        federation_enabled = req.get("federation_enabled")
        published_doc_tags = req.get("published_doc_tags")

        if federation_enabled is None and published_doc_tags is None:
            return get_data_error_result(
                message="Provide at least one of `federation_enabled` or `published_doc_tags`."
            )

        logger.info(
            "federation update_federation: tenant=%s kb=%s enabled=%s tags=%s",
            tenant_id, kb_id, federation_enabled, published_doc_tags,
        )
        KnowledgebaseService.update_federation_settings(
            kb_id=kb_id,
            tenant_id=tenant_id,
            federation_enabled=federation_enabled,
            published_doc_tags=published_doc_tags,
        )
        return get_json_result(data={"updated": True})
    except (LookupError, ValueError) as ex:
        return get_data_error_result(message=str(ex))
    except Exception as ex:
        return server_error_response(ex)
