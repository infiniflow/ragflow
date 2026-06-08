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
"""Cross-tenant Knowledge Federation service.

Responsibilities
----------------
* CRUD for ``FederationGrant`` rows (create / revoke / list).
* ``resolve_federated_kbs()``: given a requesting tenant and a list of KB ids,
  returns the subset that are owned by a *different* tenant and have an active
  grant for the requester.
* ``build_policy_filter()``: translates ``policy_json`` rules into doc-store
  filter dicts, validated against a server-side field allowlist.
* ``write_audit_log()``: fire-and-forget audit record for every cross-tenant
  retrieval event.

Security notes
--------------
* ``POLICY_FIELD_ALLOWLIST`` is enforced server-side regardless of what is
  stored in ``policy_json``.  A grant owner cannot craft a filter that
  enumerates or leaks arbitrary index fields.
* Query text is hashed (SHA-256) before storage; the plaintext never leaves
  the grantee's tenant context.
"""
import asyncio
import hashlib
import logging
from typing import Any

from api.db.db_models import DB, FederationAuditLog, FederationGrant, Knowledgebase
from api.db.services.common_service import CommonService
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp

logger = logging.getLogger(__name__)

# Only fields in this set may appear as filter keys in policy_json.
# Prevents a malicious grant owner from constructing arbitrary field probes.
POLICY_FIELD_ALLOWLIST = frozenset({"doc_tags", "available_int", "document_id", "create_time"})

_OP_MAP = {
    "eq": lambda k, v: {k: v},
    "in": lambda k, v: {k: v if isinstance(v, list) else [v]},
    # range ops are encoded as nested dicts and passed straight through to the
    # connector's condition dict; the connector translates them into ES range
    # queries or Infinity filter expressions.
    "gte": lambda k, v: {k: {"gte": v}},
    "lte": lambda k, v: {k: {"lte": v}},
    "gt":  lambda k, v: {k: {"gt": v}},
    "lt":  lambda k, v: {k: {"lt": v}},
}


class FederationService(CommonService):
    model = FederationGrant

    # ─────────────────────────── grant lifecycle ───────────────────────────

    @classmethod
    @DB.connection_context()
    def create_grant(
        cls,
        owner_tenant_id: str,
        grantee_tenant_id: str,
        kb_id: str,
        policy_json: list[dict] | None = None,
        valid_until: int | None = None,
        valid_from: int | None = None,
    ) -> dict:
        """Create a new active grant. Returns the serialised grant row."""
        if owner_tenant_id == grantee_tenant_id:
            raise ValueError("Cannot grant federation access to the owning tenant.")

        kb = Knowledgebase.get_or_none(
            (Knowledgebase.id == kb_id) &
            (Knowledgebase.tenant_id == owner_tenant_id)
        )
        if kb is None:
            raise LookupError(f"KB {kb_id} not found or not owned by tenant {owner_tenant_id}.")
        if not kb.federation_enabled:
            raise ValueError(f"KB {kb_id} does not have federation enabled.")

        _validate_policy(policy_json or [])

        now = current_timestamp()
        row = FederationGrant.create(
            id=get_uuid(),
            owner_tenant_id=owner_tenant_id,
            grantee_tenant_id=grantee_tenant_id,
            kb_id=kb_id,
            policy_json=policy_json or [],
            status="active",
            valid_from=valid_from or now,
            valid_until=valid_until,
            created_by=owner_tenant_id,
            create_time=now,
            update_time=now,
        )
        return row.to_dict()

    @classmethod
    @DB.connection_context()
    def revoke_grant(cls, grant_id: str, requesting_tenant_id: str) -> bool:
        """Revoke a grant. Only the owning tenant may revoke."""
        row = FederationGrant.get_or_none(FederationGrant.id == grant_id)
        if row is None:
            return False
        if row.owner_tenant_id != requesting_tenant_id:
            raise PermissionError("Only the grant owner may revoke a grant.")
        FederationGrant.update(
            status="revoked",
            update_time=current_timestamp(),
        ).where(FederationGrant.id == grant_id).execute()
        return True

    @classmethod
    @DB.connection_context()
    def list_grants(
        cls,
        tenant_id: str,
        role: str = "owner",
        page: int = 1,
        page_size: int = 20,
    ) -> list[dict]:
        """List grants where this tenant is owner or grantee."""
        if role == "owner":
            q = FederationGrant.select().where(
                FederationGrant.owner_tenant_id == tenant_id
            )
        else:
            q = FederationGrant.select().where(
                FederationGrant.grantee_tenant_id == tenant_id
            )
        q = q.order_by(FederationGrant.create_time.desc()).paginate(page, page_size)
        return list(q.dicts())

    @classmethod
    @DB.connection_context()
    def list_grants_for_grantee(cls, tenant_id: str) -> list["FederationGrant"]:
        """Return all active, non-expired grants for a grantee tenant."""
        now = current_timestamp()
        return list(
            FederationGrant.select().where(
                (FederationGrant.grantee_tenant_id == tenant_id) &
                (FederationGrant.status == "active") &
                (
                    (FederationGrant.valid_from.is_null(True)) |
                    (FederationGrant.valid_from <= now)
                ) &
                (
                    (FederationGrant.valid_until.is_null(True)) |
                    (FederationGrant.valid_until > now)
                )
            )
        )

    # ─────────────────────────── resolution ────────────────────────────────

    @classmethod
    @DB.connection_context()
    def resolve_federated_kbs(
        cls, requesting_tenant_id: str, kb_ids: list[str]
    ) -> dict[str, "FederationGrant"]:
        """For each KB id not owned by the requester, return its active grant
        (if one exists).  KB ids owned by the requester are skipped.

        Returns ``{kb_id: FederationGrant}``.
        """
        if not kb_ids:
            return {}

        # Filter to KBs NOT owned by the requester
        foreign_kbs = list(
            Knowledgebase.select(Knowledgebase.id).where(
                (Knowledgebase.id << kb_ids) &
                (Knowledgebase.tenant_id != requesting_tenant_id) &
                (Knowledgebase.federation_enabled == True)
            ).tuples()
        )
        if not foreign_kbs:
            return {}

        foreign_kb_ids = [row[0] for row in foreign_kbs]
        now = current_timestamp()
        grants = list(
            FederationGrant.select().where(
                (FederationGrant.grantee_tenant_id == requesting_tenant_id) &
                (FederationGrant.kb_id << foreign_kb_ids) &
                (FederationGrant.status == "active") &
                (
                    (FederationGrant.valid_from.is_null(True)) |
                    (FederationGrant.valid_from <= now)
                ) &
                (
                    (FederationGrant.valid_until.is_null(True)) |
                    (FederationGrant.valid_until > now)
                )
            )
        )
        return {g.kb_id: g for g in grants}

    # ─────────────────────────── policy filters ────────────────────────────

    @classmethod
    def build_policy_filter(cls, grant: "FederationGrant") -> dict:
        """Translate a grant's ``policy_json`` rules into a doc-store condition
        dict.  Only allowlisted fields are emitted.

        Multiple rules on the same field are collected as a list so all
        constraints are preserved (e.g. gte + lte for a date range).
        The returned dict is merged into the ``condition`` passed to
        ``Dealer.search()`` via the ``extra_filters`` parameter.
        """
        # Collect constraints per field as a list to avoid overwriting.
        per_field: dict[str, list[dict]] = {}
        for rule in (grant.policy_json or []):
            if not isinstance(rule, dict):
                logger.warning("FederationService: skipping non-dict policy rule %r", rule)
                continue
            field = rule.get("field", "")
            op = rule.get("op", "eq")
            value = rule.get("value")
            if field not in POLICY_FIELD_ALLOWLIST:
                logger.warning(
                    "FederationService: skipping policy rule with non-allowlisted field %r", field
                )
                continue
            if op not in _OP_MAP:
                logger.warning(
                    "FederationService: skipping policy rule with unknown op %r", op
                )
                continue
            per_field.setdefault(field, []).append(_OP_MAP[op](field, value))

        # Merge per-field constraints: a single constraint is used directly;
        # multiple constraints on the same field are merged into one dict so
        # that e.g. {"create_time": {"gte": X}} + {"create_time": {"lte": Y}}
        # becomes {"create_time": {"gte": X, "lte": Y}}.
        result: dict[str, Any] = {}
        for field, constraints in per_field.items():
            if len(constraints) == 1:
                result.update(constraints[0])
            else:
                merged_value: Any = {}
                for c in constraints:
                    v = c[field]
                    if isinstance(v, dict) and isinstance(merged_value, dict):
                        merged_value.update(v)
                    else:
                        # Non-dict constraints (eq/in): last one wins but this
                        # combination is semantically invalid; keep first only.
                        if not merged_value:
                            merged_value = v
                result[field] = merged_value

        # Always enforce published_doc_tags if the KB has them; intersect with
        # any grant-specific doc_tags rule rather than replacing it.
        kb = Knowledgebase.get_or_none(Knowledgebase.id == grant.kb_id)
        if kb and kb.published_doc_tags:
            kb_tags = set(kb.published_doc_tags)
            if "doc_tags" in result:
                existing = result["doc_tags"]
                if isinstance(existing, list):
                    # Keep only tags that satisfy both the grant rule and the KB gate.
                    result["doc_tags"] = [t for t in existing if t in kb_tags]
                else:
                    result["doc_tags"] = kb.published_doc_tags
            else:
                result["doc_tags"] = kb.published_doc_tags

        return result

    # ─────────────────────────── audit log ─────────────────────────────────

    @classmethod
    def write_audit_log(
        cls,
        grant_id: str,
        querying_tenant_id: str,
        querying_user_id: str | None,
        query_text: str,
        chunk_ids: list[str],
        latency_ms: int,
    ) -> None:
        """Persist an audit log entry.  Call via ``asyncio.create_task()`` so
        it does not block the search response path.
        """
        try:
            with DB.connection_context():
                now = current_timestamp()
                FederationAuditLog.create(
                    id=get_uuid(),
                    grant_id=grant_id,
                    querying_tenant_id=querying_tenant_id,
                    querying_user_id=querying_user_id or "",
                    query_text_hash=hashlib.sha256(
                        (query_text or "").encode()
                    ).hexdigest(),
                    chunk_ids_returned=chunk_ids,
                    retrieved_at=now,
                    search_latency_ms=latency_ms,
                    create_time=now,
                    update_time=now,
                )
        except Exception as e:
            logger.error("FederationService.write_audit_log failed: %s", e)

    @classmethod
    @DB.connection_context()
    def get_audit_log(
        cls,
        grant_id: str,
        requesting_tenant_id: str,
        page: int = 1,
        page_size: int = 50,
    ) -> list[dict]:
        """Return paginated audit log entries for a grant.

        Only the grant owner may call this; raises ``PermissionError`` otherwise.
        """
        grant = FederationGrant.get_or_none(FederationGrant.id == grant_id)
        if grant is None:
            raise LookupError(f"Grant {grant_id} not found.")
        if grant.owner_tenant_id != requesting_tenant_id:
            raise PermissionError("Only the grant owner may read the audit log.")

        rows = (
            FederationAuditLog
            .select()
            .where(FederationAuditLog.grant_id == grant_id)
            .order_by(FederationAuditLog.retrieved_at.desc())
            .paginate(page, page_size)
        )
        return list(rows.dicts())

    @classmethod
    @DB.connection_context()
    def list_accessible_kbs(cls, requesting_tenant_id: str) -> list[dict]:
        """Return all federated KBs accessible to ``requesting_tenant_id``
        with their active policy summary and grant metadata.
        """
        grants = cls.list_grants_for_grantee(requesting_tenant_id)
        result = []
        for grant in grants:
            kb = Knowledgebase.get_or_none(
                (Knowledgebase.id == grant.kb_id) &
                (Knowledgebase.federation_enabled == True)
            )
            if kb is None:
                continue
            result.append({
                "kb_id": grant.kb_id,
                "kb_name": kb.name,
                "owner_tenant_id": grant.owner_tenant_id,
                "grant_id": grant.id,
                "policy_summary": [
                    {"field": r.get("field"), "op": r.get("op"), "value": r.get("value")}
                    for r in (grant.policy_json or [])
                    if r.get("field") in POLICY_FIELD_ALLOWLIST
                ],
                "valid_until": grant.valid_until,
                "status": grant.status,
            })
        return result


# ─────────────────────────── helpers ───────────────────────────────────────

def _validate_policy(policy_json: list[dict]) -> None:
    """Raise ``ValueError`` if any rule is not a dict, references a
    non-allowlisted field, or uses an unknown operator.
    """
    for i, rule in enumerate(policy_json):
        if not isinstance(rule, dict):
            raise ValueError(
                f"Policy rule at index {i} must be a JSON object, got {type(rule).__name__!r}."
            )
        field = rule.get("field", "")
        op = rule.get("op", "eq")
        if field not in POLICY_FIELD_ALLOWLIST:
            raise ValueError(
                f"Policy field {field!r} is not in the allowlist "
                f"{sorted(POLICY_FIELD_ALLOWLIST)}."
            )
        if op not in _OP_MAP:
            raise ValueError(
                f"Policy operator {op!r} is not supported. "
                f"Allowed: {sorted(_OP_MAP)}."
            )
