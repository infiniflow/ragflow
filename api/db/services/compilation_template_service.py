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

import logging
import os

from peewee import fn
from ruamel.yaml import YAML

from api.db.db_models import CompilationTemplate, DB
from api.db.services.common_service import CommonService
from common.constants import StatusEnum
from common.file_utils import get_project_base_directory


class CompilationTemplateService(CommonService):
    model = CompilationTemplate

    @classmethod
    def fill_config_default_llm(cls, config: dict, tenant_id: str | None) -> dict:
        if not isinstance(config, dict) or config.get("llm_id") or not tenant_id:
            return config
        try:
            from api.db.services.user_service import TenantService

            ok, tenant = TenantService.get_by_id(tenant_id)
            if ok and getattr(tenant, "llm_id", None):
                config = dict(config)
                config["llm_id"] = tenant.llm_id
        except Exception:
            logging.exception(
                "compilation_template: llm_id default-fill lookup failed for tenant=%s",
                tenant_id,
            )
        return config

    @classmethod
    def fill_default_llm_for_templates(cls, templates: list[dict], tenant_id: str | None) -> list[dict]:
        if not tenant_id:
            return templates
        filled = []
        for template in templates:
            item = dict(template)
            item["config"] = cls.fill_config_default_llm(item.get("config") or {}, tenant_id)
            filled.append(item)
        return filled

    @classmethod
    def _sort_builtins(cls, templates: list[dict]) -> list[dict]:
        return sorted(
            templates,
            key=lambda template: (
                template.get("kind") == "empty" or template.get("id") == "empty",
                template.get("display_name") or template.get("name") or "",
            ),
        )

    @classmethod
    @DB.connection_context()
    def ensure_table(cls) -> None:
        if not cls.model.table_exists():
            cls.model.create_table(safe=True)

    @classmethod
    def _to_saved_dict(cls, template: CompilationTemplate) -> dict:
        data = template.to_dict()
        config = data.get("config") or {}
        # Lazy-fill llm_id with the tenant's default chat model so the
        # frontend always sees a value (legacy templates predate the
        # field). The DB row is left untouched — this is a read-side
        # default. If the tenant has no default chat model set,
        # silently leave llm_id absent and let the caller fall back
        # however it likes.
        if isinstance(config, dict) and not config.get("llm_id"):
            tenant_id = data.get("tenant_id")
            if tenant_id:
                try:
                    from api.db.services.user_service import TenantService

                    ok, tenant = TenantService.get_by_id(tenant_id)
                    if ok and getattr(tenant, "llm_id", None):
                        config = dict(config)
                        config["llm_id"] = tenant.llm_id
                except Exception:
                    logging.exception(
                        "compilation_template: llm_id lazy-fill lookup failed for tenant=%s",
                        tenant_id,
                    )
        return {
            "id": data["id"],
            "name": data["name"],
            "description": data.get("description") or "",
            "kind": data["kind"],
            "config": cls.fill_config_default_llm(config, data.get("tenant_id")),
            "create_time": data.get("create_time"),
            "update_time": data.get("update_time"),
        }

    @classmethod
    def _to_builtin_dict(cls, template: CompilationTemplate) -> dict:
        data = template.to_dict()
        return {
            "id": data["id"],
            "kind": data["kind"],
            "display_name": data["name"],
            "description": data.get("description") or "",
            "config": data.get("config") or {},
        }

    @classmethod
    @DB.connection_context()
    def list_saved(cls, tenant_id: str, keywords: str = "", kind: str = "", orderby: str = "create_time", desc: bool = True) -> list[dict]:
        query = cls.model.select().where(
            cls.model.tenant_id == tenant_id,
            cls.model.status == StatusEnum.VALID.value,
        )
        if keywords:
            query = query.where(cls.model.name.contains(keywords))
        if kind:
            query = query.where(cls.model.kind == kind)
        if not hasattr(cls.model, orderby):
            orderby = "create_time"
        order_field = getattr(cls.model, orderby)
        query = query.order_by(order_field.desc() if desc else order_field.asc())
        return [cls._to_saved_dict(template) for template in query]

    @classmethod
    @DB.connection_context()
    def list_builtins(cls) -> list[dict]:
        cls.ensure_table()
        query = cls.model.select().where(cls.model.is_builtin, cls.model.status == StatusEnum.VALID.value).order_by(cls.model.create_time.asc(), cls.model.name.asc())
        return cls._sort_builtins([cls._to_builtin_dict(template) for template in query])

    @classmethod
    @DB.connection_context()
    def get_saved(cls, template_id: str, tenant_id: str) -> dict | None:
        template = cls.model.get_or_none(
            cls.model.id == template_id,
            cls.model.tenant_id == tenant_id,
            cls.model.status == StatusEnum.VALID.value,
        )
        return cls._to_saved_dict(template) if template else None

    @classmethod
    @DB.connection_context()
    def name_exists(cls, tenant_id: str, name: str, exclude_id: str = "") -> bool:
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
    def upsert_builtin(cls, template: dict) -> None:
        template_id = template["id"]
        existing = cls.model.get_or_none(cls.model.id == template_id)
        data = {
            "id": template_id,
            "tenant_id": None,
            "name": template["name"],
            "description": template.get("description", ""),
            "kind": template["kind"],
            "config": template["config"],
            "is_builtin": True,
            "status": StatusEnum.VALID.value,
        }
        if existing:
            cls.update_by_id(template_id, data)
        else:
            cls.insert(**data)

    @classmethod
    def seed_builtins_from_files(cls) -> None:
        cls.ensure_table()
        for template in cls.load_builtins_from_files():
            cls.upsert_builtin(template)

    @classmethod
    def load_builtins_from_files(cls) -> list[dict]:
        template_dir = os.path.join(get_project_base_directory(), "api", "db", "init_data", "compilation_templates")
        if not os.path.exists(template_dir):
            logging.warning("Missing compilation templates: %s", template_dir)
            return []

        templates = []
        yaml = YAML(typ="safe", pure=True)
        for filename in sorted(os.listdir(template_dir)):
            if not filename.endswith((".yaml", ".yml")):
                continue
            template_path = os.path.join(template_dir, filename)
            try:
                with open(template_path, "r", encoding="utf-8") as f:
                    template = yaml.load(f) or {}
                kind = template.get("kind")
                display_name = template.get("display_name")
                config = template.get("config")
                if not kind or not display_name or not isinstance(config, dict):
                    logging.warning("Skipping invalid compilation template file: %s", template_path)
                    continue
                templates.append(
                    {
                        "id": os.path.splitext(filename)[0],
                        "name": display_name,
                        "description": template.get("description", ""),
                        "kind": kind,
                        "config": config,
                    }
                )
            except Exception as e:
                logging.exception("Add compilation template error for %s: %s", template_path, e)
        return cls._sort_builtins(templates)

    @classmethod
    def load_wiki_presets_from_files(cls) -> list[dict]:
        """Load wiki page-structure presets from
        ``api/db/init_data/compilation_templates/wiki/*.yaml``.

        Each file contributes one preset dict with ``topic`` /
        ``instruction`` / ``page_example`` fields (plus ``id`` derived
        from the filename stem so the frontend can key list items
        even when several presets share the same ``topic`` — which is
        by design; the UI groups presets by topic).

        Filesystem-fresh on every call: these are read-only reference
        data with low request volume, so no DB seed / no ES cache.
        Same failure-isolation policy as :meth:`load_builtins_from_files`
        — a single malformed file logs and is skipped; the rest still
        load. Ordered by filename for stability.
        """
        wiki_dir = os.path.join(
            get_project_base_directory(),
            "api", "db", "init_data", "compilation_templates", "wiki",
        )
        if not os.path.exists(wiki_dir):
            logging.warning("Missing wiki presets directory: %s", wiki_dir)
            return []

        presets: list[dict] = []
        yaml = YAML(typ="safe", pure=True)
        for filename in sorted(os.listdir(wiki_dir)):
            if not filename.endswith((".yaml", ".yml")):
                continue
            path = os.path.join(wiki_dir, filename)
            try:
                with open(path, "r", encoding="utf-8") as f:
                    doc = yaml.load(f) or {}
            except Exception:
                logging.exception("wiki preset load failed for %s", path)
                continue
            if not isinstance(doc, dict):
                logging.warning("wiki preset skipped (not a mapping): %s", path)
                continue
            # Missing fields degrade to empty strings so the frontend
            # doesn't have to null-check every row.
            presets.append({
                "id": os.path.splitext(filename)[0],
                "topic": str(doc.get("topic") or "").strip(),
                "instruction": str(doc.get("instruction") or ""),
                "page_example": str(doc.get("page_example") or ""),
            })
        return presets
