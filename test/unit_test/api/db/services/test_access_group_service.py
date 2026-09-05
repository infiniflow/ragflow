import pytest

from types import SimpleNamespace

from api.db.services.access_group_service import (
    AccessGroupService,
    AccessGroupValidationError,
    section_for_blueprint,
    validate_group_payload,
)


def test_group_payload_normalizes_sections_in_navigation_order():
    payload = validate_group_payload({"name": " Legal ", "description": " Docs ", "user_ids": [], "dataset_ids": [], "sections": ["file_manager", "dataset"]})
    assert payload["name"] == "Legal"
    assert payload["description"] == "Docs"
    assert payload["sections"] == ["dataset", "file_manager"]


@pytest.mark.parametrize(
    "payload",
    [
        {},
        {"name": "x", "description": "", "user_ids": ["u", "u"], "dataset_ids": [], "sections": []},
        {"name": "x", "description": "", "user_ids": [], "dataset_ids": [], "sections": ["unknown"]},
    ],
)
def test_group_payload_rejects_invalid_contract(payload):
    with pytest.raises(AccessGroupValidationError):
        validate_group_payload(payload)


def test_group_payload_does_not_treat_global_home_visibility_as_an_acl_grant():
    with pytest.raises(AccessGroupValidationError, match="Unknown sections: home"):
        validate_group_payload(
            {
                "name": "x",
                "description": "",
                "user_ids": [],
                "dataset_ids": [],
                "sections": ["home"],
            }
        )


def test_section_mapping_covers_protected_application_entrypoints():
    assert section_for_blueprint("dataset_api") == "dataset"
    assert section_for_blueprint("business_document_api") == "business_documents"
    assert section_for_blueprint("system_api") is None


def test_group_sections_are_unioned_and_superuser_bypasses(monkeypatch):
    user = SimpleNamespace(id="u1", is_superuser=False)
    monkeypatch.setattr(AccessGroupService, "effective_policy", classmethod(lambda cls, user_id: {"sections": {"dataset"}, "dataset_ids": set()}))
    assert AccessGroupService.has_section_access(user, "dataset") is True
    assert AccessGroupService.has_section_access(user, "chat") is False
    assert AccessGroupService.has_section_access(SimpleNamespace(id="admin", is_superuser=True), "chat") is True
