#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#

from peewee import IntegrityError

from api.db.db_models import (
    DB,
    AccessGroup,
    AccessGroupDataset,
    AccessGroupSection,
    AccessGroupUser,
    Knowledgebase,
    User,
)
from api.db.services.navigation_visibility_service import ACCESS_CONTROLLED_NAVIGATION_SECTIONS
from common.misc_utils import get_uuid
from common.constants import StatusEnum
from common.time_utils import current_timestamp, datetime_format
from datetime import datetime


BLUEPRINT_SECTIONS = {
    "dataset_api": "dataset",
    "document_api": "dataset",
    "chunk_api": "dataset",
    "chat_api": "chat",
    "search_api": "search",
    "agent_api": "agent",
    "bot_api": "agent",
    "memory_api": "memory",
    "openmetadata_api": "catalog",
    "business_document_api": "business_documents",
    "file_api": "file_manager",
    "file_commit_api": "file_manager",
    "file2document_api": "file_manager",
}


def section_for_blueprint(blueprint):
    return BLUEPRINT_SECTIONS.get(blueprint)


class AccessGroupValidationError(ValueError):
    pass


def _normalize_ids(value, field):
    if not isinstance(value, list) or not all(isinstance(item, str) and item for item in value):
        raise AccessGroupValidationError(f"{field} must be an array of non-empty strings")
    if len(value) != len(set(value)):
        raise AccessGroupValidationError(f"{field} must not contain duplicates")
    return value


def validate_group_payload(payload, *, partial=False):
    if not isinstance(payload, dict):
        raise AccessGroupValidationError("JSON object is required")
    result = {}
    if not partial or "name" in payload:
        name = payload.get("name")
        if not isinstance(name, str) or not name.strip() or len(name.strip()) > 128:
            raise AccessGroupValidationError("name must be a non-empty string up to 128 characters")
        result["name"] = name.strip()
    if not partial or "description" in payload:
        description = payload.get("description", "")
        if not isinstance(description, str) or len(description) > 2000:
            raise AccessGroupValidationError("description must be a string up to 2000 characters")
        result["description"] = description.strip()
    for field in ("user_ids", "dataset_ids", "sections"):
        if not partial or field in payload:
            result[field] = _normalize_ids(payload.get(field, []), field)
    if "sections" in result:
        unknown = sorted(set(result["sections"]) - set(ACCESS_CONTROLLED_NAVIGATION_SECTIONS))
        if unknown:
            raise AccessGroupValidationError(f"Unknown sections: {', '.join(unknown)}")
        selected = set(result["sections"])
        result["sections"] = [item for item in ACCESS_CONTROLLED_NAVIGATION_SECTIONS if item in selected]
    return result


class AccessGroupService:
    @staticmethod
    def _validate_references(payload):
        if "user_ids" in payload:
            found = {row.id for row in User.select(User.id).where(User.id.in_(payload["user_ids"]))}
            missing = sorted(set(payload["user_ids"]) - found)
            if missing:
                raise AccessGroupValidationError(f"Unknown user IDs: {', '.join(missing)}")
        if "dataset_ids" in payload:
            found = {row.id for row in Knowledgebase.select(Knowledgebase.id).where(Knowledgebase.id.in_(payload["dataset_ids"]))}
            missing = sorted(set(payload["dataset_ids"]) - found)
            if missing:
                raise AccessGroupValidationError(f"Unknown dataset IDs: {', '.join(missing)}")

    @staticmethod
    @DB.connection_context()
    def list_groups():
        groups = list(AccessGroup.select().order_by(AccessGroup.name.asc()).dicts())
        group_ids = [group["id"] for group in groups]
        users = {group_id: [] for group_id in group_ids}
        datasets = {group_id: [] for group_id in group_ids}
        sections = {group_id: [] for group_id in group_ids}
        if group_ids:
            for row in AccessGroupUser.select().where(AccessGroupUser.group_id.in_(group_ids)).dicts():
                users[row["group_id"]].append(row["user_id"])
            for row in AccessGroupDataset.select().where(AccessGroupDataset.group_id.in_(group_ids)).dicts():
                datasets[row["group_id"]].append(row["dataset_id"])
            for row in AccessGroupSection.select().where(AccessGroupSection.group_id.in_(group_ids)).dicts():
                sections[row["group_id"]].append(row["section"])
        for group in groups:
            group["user_ids"] = sorted(users[group["id"]])
            group["dataset_ids"] = sorted(datasets[group["id"]])
            selected = set(sections[group["id"]])
            group["sections"] = [item for item in ACCESS_CONTROLLED_NAVIGATION_SECTIONS if item in selected]
        return groups

    @staticmethod
    @DB.connection_context()
    def options():
        users = list(User.select(User.id, User.email, User.nickname, User.is_superuser).order_by(User.email.asc()).dicts())
        datasets = list(
            Knowledgebase.select(Knowledgebase.id, Knowledgebase.name, Knowledgebase.tenant_id).where(Knowledgebase.status == StatusEnum.VALID.value).order_by(Knowledgebase.name.asc()).dicts()
        )
        return {"users": users, "datasets": datasets, "sections": list(ACCESS_CONTROLLED_NAVIGATION_SECTIONS)}

    @staticmethod
    def _replace_links(group_id, payload):
        mappings = (
            ("user_ids", AccessGroupUser, "user_id"),
            ("dataset_ids", AccessGroupDataset, "dataset_id"),
            ("sections", AccessGroupSection, "section"),
        )
        for field, model, target in mappings:
            if field not in payload:
                continue
            model.delete().where(model.group_id == group_id).execute()
            values = payload[field]
            if values:
                model.insert_many([{"group_id": group_id, target: value} for value in values]).execute()

    @classmethod
    @DB.connection_context()
    def create(cls, payload):
        data = validate_group_payload(payload)
        cls._validate_references(data)
        group_id = get_uuid()
        now = current_timestamp()
        with DB.atomic():
            try:
                AccessGroup.create(
                    id=group_id,
                    name=data["name"],
                    description=data["description"],
                    create_time=now,
                    update_time=now,
                    create_date=datetime_format(datetime.now()),
                    update_date=datetime_format(datetime.now()),
                )
            except IntegrityError as error:
                raise AccessGroupValidationError("A group with this name already exists") from error
            cls._replace_links(group_id, data)
        return next(group for group in cls.list_groups() if group["id"] == group_id)

    @classmethod
    @DB.connection_context()
    def update(cls, group_id, payload):
        data = validate_group_payload(payload, partial=True)
        cls._validate_references(data)
        if AccessGroup.get_or_none(AccessGroup.id == group_id) is None:
            raise LookupError("Access group not found")
        values = {key: value for key, value in data.items() if key in {"name", "description"}}
        values["update_time"] = current_timestamp()
        values["update_date"] = datetime_format(datetime.now())
        with DB.atomic():
            try:
                AccessGroup.update(values).where(AccessGroup.id == group_id).execute()
            except IntegrityError as error:
                raise AccessGroupValidationError("A group with this name already exists") from error
            cls._replace_links(group_id, data)
        return next(group for group in cls.list_groups() if group["id"] == group_id)

    @staticmethod
    @DB.connection_context()
    def delete(group_id):
        with DB.atomic():
            for model in (AccessGroupUser, AccessGroupDataset, AccessGroupSection):
                model.delete().where(model.group_id == group_id).execute()
            deleted = AccessGroup.delete().where(AccessGroup.id == group_id).execute()
        if not deleted:
            raise LookupError("Access group not found")

    @staticmethod
    @DB.connection_context()
    def effective_policy(user_id):
        group_ids = [row.group_id for row in AccessGroupUser.select(AccessGroupUser.group_id).where(AccessGroupUser.user_id == user_id)]
        if not group_ids:
            return None
        dataset_ids = {row.dataset_id for row in AccessGroupDataset.select(AccessGroupDataset.dataset_id).where(AccessGroupDataset.group_id.in_(group_ids))}
        sections = {row.section for row in AccessGroupSection.select(AccessGroupSection.section).where(AccessGroupSection.group_id.in_(group_ids))}
        return {"dataset_ids": dataset_ids, "sections": sections}

    @classmethod
    def has_section_access(cls, user, section):
        if getattr(user, "is_superuser", False):
            return True
        policy = cls.effective_policy(user.id)
        return policy is None or section in policy["sections"]

    @classmethod
    def allowed_dataset_ids(cls, user_id):
        policy = cls.effective_policy(user_id)
        return None if policy is None else policy["dataset_ids"]
