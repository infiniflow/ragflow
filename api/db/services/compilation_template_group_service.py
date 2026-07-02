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

from peewee import fn

from api.db.db_models import DB, CompilationTemplate, CompilationTemplateGroup
from api.db.services.common_service import CommonService
from common.constants import StatusEnum
from common.misc_utils import get_uuid


SCOPE_FILE = "file"
SCOPE_DATASET = "dataset"


class GroupValidationError(ValueError):
    pass


def _derive_scope(templates: list[dict]) -> str:
    """Derive the group's scope from its child templates.

    One artifacts child = dataset scope (and must be the only child).
    Otherwise file scope, with no artifacts allowed.
    """
    if not templates:
        raise GroupValidationError("A template group must contain at least one template.")
    kinds = [str((t or {}).get("kind") or "").strip() for t in templates]
    artifact_count = sum(1 for k in kinds if k == "artifacts")
    if artifact_count > 0:
        if artifact_count != 1 or len(templates) != 1:
            raise GroupValidationError("An artifacts template cannot be combined with other templates in the same group.")
        return SCOPE_DATASET

    _enforce_single_rechunk_tree(templates)
    return SCOPE_FILE


def _enforce_single_rechunk_tree(templates: list[dict]) -> None:
    """At most one tree-kind child in the group may enable re-chunking.

    Re-chunking soft-deletes the doc's original chunks via
    ``available_int=0`` and inserts merged replacements; running two
    such templates would race on the same source chunks and produce
    non-deterministic output. Per-tenant invariant is enforced
    server-side here and mirrored client-side in
    ``group-interface.ts``.
    """
    rechunk_trees = 0
    for t in templates:
        if str((t or {}).get("kind") or "").strip() != "tree":
            continue
        cfg = (t or {}).get("config") or {}
        raptor = (cfg or {}).get("raptor") or {}
        if bool(raptor.get("rechunk")):
            rechunk_trees += 1
    if rechunk_trees > 1:
        raise GroupValidationError("Only one tree template in a group may enable re-chunking.")


class CompilationTemplateGroupService(CommonService):
    model = CompilationTemplateGroup

    @classmethod
    def ensure_table(cls) -> None:
        if not cls.model.table_exists():
            cls.model.create_table(safe=True)

    # ------------------------------------------------------------------
    # Read paths
    # ------------------------------------------------------------------

    @classmethod
    def _group_to_dict(cls, group: CompilationTemplateGroup, templates: list[CompilationTemplate]) -> dict:
        from api.db.services.compilation_template_service import CompilationTemplateService

        return {
            "id": group.id,
            "name": group.name,
            "description": group.description or "",
            "scope": group.scope,
            "create_time": group.create_time,
            "update_time": group.update_time,
            "templates": [CompilationTemplateService._to_saved_dict(t) for t in templates],
        }

    @classmethod
    @DB.connection_context()
    def list_saved(
        cls,
        tenant_id: str,
        keywords: str = "",
        scope: str = "",
        orderby: str = "create_time",
        desc: bool = True,
    ) -> list[dict]:
        cls.ensure_table()
        query = cls.model.select().where(
            cls.model.tenant_id == tenant_id,
            cls.model.status == StatusEnum.VALID.value,
        )
        if keywords:
            query = query.where(cls.model.name.contains(keywords))
        if scope:
            query = query.where(cls.model.scope == scope)
        if not hasattr(cls.model, orderby):
            orderby = "create_time"
        order_field = getattr(cls.model, orderby)
        query = query.order_by(order_field.desc() if desc else order_field.asc())

        groups = list(query)
        if not groups:
            return []

        group_ids = [g.id for g in groups]
        children = list(
            CompilationTemplate.select()
            .where(
                CompilationTemplate.group_id.in_(group_ids),
                CompilationTemplate.status == StatusEnum.VALID.value,
            )
            .order_by(CompilationTemplate.create_time.asc())
        )
        children_by_group: dict[str, list[CompilationTemplate]] = {gid: [] for gid in group_ids}
        for child in children:
            children_by_group.setdefault(child.group_id, []).append(child)

        return [cls._group_to_dict(g, children_by_group.get(g.id, [])) for g in groups]

    @classmethod
    @DB.connection_context()
    def get_saved(cls, group_id: str, tenant_id: str) -> dict | None:
        group = cls.model.get_or_none(
            cls.model.id == group_id,
            cls.model.tenant_id == tenant_id,
            cls.model.status == StatusEnum.VALID.value,
        )
        if not group:
            return None
        children = list(
            CompilationTemplate.select()
            .where(
                CompilationTemplate.group_id == group_id,
                CompilationTemplate.status == StatusEnum.VALID.value,
            )
            .order_by(CompilationTemplate.create_time.asc())
        )
        return cls._group_to_dict(group, children)

    @classmethod
    @DB.connection_context()
    def list_for_resolution(cls, tenant_id: str) -> list[dict]:
        """Light list used by frontend pickers (dataset parse-config dropdown).

        Returns one row per group with just the fields the picker needs +
        the child template ids so the orchestrator can resolve them later.
        """
        cls.ensure_table()
        groups = list(
            cls.model.select().where(
                cls.model.tenant_id == tenant_id,
                cls.model.status == StatusEnum.VALID.value,
            )
        )
        if not groups:
            return []
        group_ids = [g.id for g in groups]
        kid_pairs = list(
            CompilationTemplate.select(
                CompilationTemplate.group_id,
                CompilationTemplate.id,
                CompilationTemplate.kind,
                CompilationTemplate.name,
            ).where(
                CompilationTemplate.group_id.in_(group_ids),
                CompilationTemplate.status == StatusEnum.VALID.value,
            )
        )
        by_group: dict[str, list[dict]] = {}
        for child in kid_pairs:
            by_group.setdefault(child.group_id, []).append({"id": child.id, "kind": child.kind, "name": child.name})
        return [
            {
                "id": g.id,
                "name": g.name,
                "description": g.description or "",
                "scope": g.scope,
                "templates": by_group.get(g.id, []),
            }
            for g in groups
        ]

    @classmethod
    @DB.connection_context()
    def name_exists(cls, tenant_id: str, name: str, exclude_id: str = "") -> bool:
        cls.ensure_table()
        query = cls.model.select(fn.COUNT(cls.model.id)).where(
            cls.model.tenant_id == tenant_id,
            cls.model.name == name,
            cls.model.status == StatusEnum.VALID.value,
        )
        if exclude_id:
            query = query.where(cls.model.id != exclude_id)
        return query.scalar() > 0

    @classmethod
    @DB.connection_context()
    def resolve_template_ids(cls, group_id: str, tenant_id: str) -> list[str]:
        """Resolve a group id to its child template ids. Used by the orchestrator
        when reading ``parser_config.compilation_template_group_id``.
        """
        cls.ensure_table()
        group = cls.model.get_or_none(
            cls.model.id == group_id,
            cls.model.tenant_id == tenant_id,
            cls.model.status == StatusEnum.VALID.value,
        )
        if not group:
            return []
        rows = list(
            CompilationTemplate.select(CompilationTemplate.id)
            .where(
                CompilationTemplate.group_id == group_id,
                CompilationTemplate.status == StatusEnum.VALID.value,
            )
            .order_by(CompilationTemplate.create_time.asc())
        )
        return [r.id for r in rows]

    # ------------------------------------------------------------------
    # Write paths
    # ------------------------------------------------------------------

    @classmethod
    @DB.connection_context()
    def create_group(cls, tenant_id: str, name: str, description: str, templates: list[dict]) -> dict:
        cls.ensure_table()
        scope = _derive_scope(templates)
        group_id = get_uuid()
        with DB.atomic():
            CompilationTemplateGroup.create(
                id=group_id,
                tenant_id=tenant_id,
                name=name,
                description=description or "",
                scope=scope,
                status=StatusEnum.VALID.value,
            )
            for i, child in enumerate(templates):
                cls._insert_child(group_id, tenant_id, child, index=i)
        saved = cls.get_saved(group_id, tenant_id)
        assert saved is not None
        return saved

    @classmethod
    @DB.connection_context()
    def update_group(
        cls,
        group_id: str,
        tenant_id: str,
        name: str | None,
        description: str | None,
        templates: list[dict] | None,
    ) -> dict | None:
        cls.ensure_table()
        existing = cls.model.get_or_none(
            cls.model.id == group_id,
            cls.model.tenant_id == tenant_id,
            cls.model.status == StatusEnum.VALID.value,
        )
        if not existing:
            return None

        with DB.atomic():
            updates: dict = {}
            if name is not None:
                updates["name"] = name
            if description is not None:
                updates["description"] = description
            if templates is not None:
                updates["scope"] = _derive_scope(templates)
            if updates:
                cls.model.update(**updates).where(cls.model.id == group_id).execute()

            if templates is not None:
                # Soft-delete previous children (en-bloc replace). Simpler than
                # diffing and acceptable given small N — child IDs are not
                # referenced externally (parser_config keys the group, not its
                # children).
                CompilationTemplate.update(status=StatusEnum.INVALID.value).where(
                    CompilationTemplate.group_id == group_id,
                    CompilationTemplate.status == StatusEnum.VALID.value,
                ).execute()
                for i, child in enumerate(templates):
                    cls._insert_child(group_id, tenant_id, child, index=i)

        return cls.get_saved(group_id, tenant_id)

    @classmethod
    @DB.connection_context()
    def delete_group(cls, group_id: str, tenant_id: str) -> bool:
        cls.ensure_table()
        existing = cls.model.get_or_none(
            cls.model.id == group_id,
            cls.model.tenant_id == tenant_id,
            cls.model.status == StatusEnum.VALID.value,
        )
        if not existing:
            return False
        with DB.atomic():
            cls.model.update(status=StatusEnum.INVALID.value).where(cls.model.id == group_id).execute()
            CompilationTemplate.update(status=StatusEnum.INVALID.value).where(
                CompilationTemplate.group_id == group_id,
                CompilationTemplate.status == StatusEnum.VALID.value,
            ).execute()
        return True

    @classmethod
    def _insert_child(
        cls,
        group_id: str,
        tenant_id: str,
        child: dict,
        *,
        index: int,
    ) -> None:
        kind = str((child or {}).get("kind") or "").strip()
        name = str((child or {}).get("name") or "").strip()
        config = (child or {}).get("config") or {}
        if not kind or not name or not isinstance(config, dict):
            raise GroupValidationError("Each template must include a name, kind, and config object.")
        from api.db.services.compilation_template_service import CompilationTemplateService

        config = CompilationTemplateService.fill_config_default_llm(config, tenant_id)
        template_id = get_uuid()
        CompilationTemplate.create(
            id=template_id,
            tenant_id=tenant_id,
            group_id=group_id,
            name=name,
            description=str((child or {}).get("description") or ""),
            kind=kind,
            config=config,
            is_builtin=False,
            status=StatusEnum.VALID.value,
        )

    # ------------------------------------------------------------------
    # Lookup helpers used by the orchestrator
    # ------------------------------------------------------------------

    @classmethod
    @DB.connection_context()
    def get_for_kb(cls, group_id: str, tenant_id: str) -> dict | None:
        """Like ``get_saved`` but returns ``None`` quietly and avoids the
        ``_to_saved_dict`` LLM-lookup branch — for orchestrator use where
        we only need the scope + child rows.
        """
        cls.ensure_table()
        group = cls.model.get_or_none(
            cls.model.id == group_id,
            cls.model.tenant_id == tenant_id,
            cls.model.status == StatusEnum.VALID.value,
        )
        if not group:
            return None
        children = list(
            CompilationTemplate.select()
            .where(
                CompilationTemplate.group_id == group_id,
                CompilationTemplate.status == StatusEnum.VALID.value,
            )
            .order_by(CompilationTemplate.create_time.asc())
        )
        return {
            "id": group.id,
            "name": group.name,
            "scope": group.scope,
            "template_ids": [c.id for c in children],
            "templates_by_kind": {c.kind: c.id for c in children},
        }
