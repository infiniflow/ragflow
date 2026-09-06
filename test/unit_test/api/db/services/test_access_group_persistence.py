"""Group persistence with real queries and disposable SQLite transactions."""

import inspect

import pytest
from peewee import IntegrityError, SqliteDatabase

from api.db.db_models import AccessGroup, AccessGroupDataset, AccessGroupSection, AccessGroupUser, Knowledgebase, User
from api.db.services import access_group_service as service


@pytest.fixture
def group_database(monkeypatch):
    database = SqliteDatabase(":memory:")
    tables = [AccessGroup, AccessGroupDataset, AccessGroupSection, AccessGroupUser, Knowledgebase, User]
    # Retain service bodies; replace only decorators bound to the configured pool.
    # This lane validates persistence/transactions, not connection-pool lifecycle.
    for name in ("create", "update"):
        monkeypatch.setattr(service.AccessGroupService, name, classmethod(inspect.unwrap(getattr(service.AccessGroupService, name).__func__)))
    for name in ("list_groups", "delete", "effective_policy"):
        monkeypatch.setattr(service.AccessGroupService, name, staticmethod(inspect.unwrap(getattr(service.AccessGroupService, name))))
    monkeypatch.setattr(service, "DB", database)
    with database.bind_ctx(tables, bind_refs=False, bind_backrefs=False):
        database.connect()
        database.create_tables(tables)
        User.create(id="member", nickname="Member", email="member@example.test")
        Knowledgebase.create(id="dataset", tenant_id="tenant", name="Dataset", embd_id="synthetic", created_by="member")
        yield database
        database.close()


def _group(name, sections):
    return service.AccessGroupService.create({"name": name, "user_ids": ["member"], "dataset_ids": ["dataset"], "sections": sections})


def test_persisted_group_union_repeated_update_and_delete(group_database):
    first = _group("First", ["dataset"])
    second = _group("Second", ["chat"])
    assert service.AccessGroupService.effective_policy("member") == {"sections": {"dataset", "chat"}, "dataset_ids": {"dataset"}}
    for _ in range(2):
        service.AccessGroupService.update(first["id"], {"sections": ["agent"], "dataset_ids": []})
    assert AccessGroupSection.select().where(AccessGroupSection.group_id == first["id"]).count() == 1
    assert service.AccessGroupService.effective_policy("member") == {"sections": {"agent", "chat"}, "dataset_ids": {"dataset"}}
    service.AccessGroupService.delete(second["id"])
    assert service.AccessGroupService.effective_policy("member") == {"sections": {"agent"}, "dataset_ids": set()}
    for table in (AccessGroupDataset, AccessGroupUser, AccessGroupSection):
        assert table.select().where(table.group_id == second["id"]).count() == 0


def test_group_link_failure_rolls_back_name_and_all_grants(group_database, monkeypatch):
    group = _group("Original", ["dataset"])

    def fail_section_write(*_args, **_kwargs):
        raise IntegrityError("synthetic link write failure")

    monkeypatch.setattr(AccessGroupSection, "insert_many", fail_section_write)
    with pytest.raises(IntegrityError, match="synthetic link write failure"):
        service.AccessGroupService.update(group["id"], {"name": "Changed", "dataset_ids": [], "sections": ["chat"]})
    assert AccessGroup.get_by_id(group["id"]).name == "Original"
    assert service.AccessGroupService.effective_policy("member") == {"sections": {"dataset"}, "dataset_ids": {"dataset"}}


@pytest.mark.parametrize("field", ["user_ids", "dataset_ids"])
def test_group_unknown_reference_does_not_replace_existing_grants(group_database, field):
    group = _group("Original", ["dataset"])
    with pytest.raises(service.AccessGroupValidationError, match="Unknown"):
        service.AccessGroupService.update(group["id"], {field: ["missing"], "sections": ["chat"]})
    assert service.AccessGroupService.effective_policy("member") == {"sections": {"dataset"}, "dataset_ids": {"dataset"}}
