"""Isolated exact registration contract for the owned user/access HTTP surface."""

import importlib.util
from pathlib import Path

import pytest


ROOT = Path(__file__).resolve().parents[4]


def _load_fixture(name, relative_path):
    spec = importlib.util.spec_from_file_location(name, ROOT / relative_path)
    fixture = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(fixture)
    return fixture


USER_FIXTURE = _load_fixture(
    "access_policy_user_route_contract_fixture",
    "test/testcases/restful_api/test_user_tenant_routes_unit.py",
)
SYSTEM_FIXTURE = _load_fixture(
    "access_policy_system_route_contract_fixture",
    "test/testcases/test_web_api/test_system_app/test_system_routes_unit.py",
)
DATASET_FIXTURE = _load_fixture(
    "access_policy_dataset_route_contract_fixture",
    "test/testcases/test_web_api/test_dataset_management/test_dataset_sdk_routes_unit.py",
)
FILE_COMMIT_FIXTURE = _load_fixture(
    "access_policy_file_commit_route_contract_fixture",
    "test/testcases/restful_api/test_file_commit_routes_unit.py",
)

EXPECTED_SYSTEM_ROUTES = {
    ("/system/ping", ("GET",), "ping"),
    ("/system/version", ("GET",), "version"),
    ("/system/status", ("GET",), "status"),
    ("/system/oceanbase/status", ("GET",), "oceanbase_status"),
    ("/system/config", ("GET",), "get_config"),
    ("/system/healthz", ("GET",), "healthz"),
    ("/system/tokens", ("GET",), "token_list"),
    ("/system/tokens", ("POST",), "new_token"),
    ("/system/tokens/<token>", ("DELETE",), "rm"),
    ("/system/client-errors", ("POST",), "report_client_error"),
    ("/system/config/log", ("GET",), "get_logger_levels"),
    ("/system/config/log", ("PUT",), "set_logger_level"),
}

EXPECTED_DATASET_ROUTES = {
    ("/datasets/tags/aggregation", ("GET",), "aggregate_tags"),
    ("/datasets/metadata/flattened", ("GET",), "get_flattened_metadata"),
    ("/datasets", ("POST",), "create"),
    ("/datasets", ("DELETE",), "delete"),
    ("/datasets/<dataset_id>", ("PUT",), "update"),
    ("/datasets", ("GET",), "list_datasets"),
    ("/datasets/<dataset_id>", ("GET",), "get_dataset"),
    ("/datasets/<dataset_id>/ingestions/summary", ("GET",), "get_ingestion_summary"),
    ("/datasets/<dataset_id>/tags", ("GET",), "list_tags"),
    ("/datasets/<dataset_id>/tags", ("DELETE",), "delete_tags"),
    ("/datasets/<dataset_id>/tags", ("PUT",), "rename_tag"),
    ("/datasets/search", ("POST",), "search_datasets"),
    ("/datasets/<dataset_id>/search", ("POST",), "search"),
    ("/datasets/<dataset_id>/graph", ("GET",), "get_knowledge_graph"),
    ("/datasets/<dataset_id>/any_artifact", ("GET",), "has_any_wiki"),
    ("/datasets/<dataset_id>/artifacts", ("GET",), "list_wiki_pages"),
    ("/datasets/<dataset_id>/artifacts/graph", ("GET",), "get_wiki_graph"),
    ("/datasets/<dataset_id>/artifacts", ("DELETE",), "clear_wiki"),
    ("/datasets/<dataset_id>/artifacts/<page_type>/<path:slug>", ("GET",), "get_wiki_page"),
    ("/datasets/<dataset_id>/any_skill", ("GET",), "has_any_skill"),
    ("/datasets/<dataset_id>/skills", ("GET",), "get_skill_tree"),
    ("/datasets/<dataset_id>/skills/<path:skill_kwd>", ("GET",), "get_skill_page"),
    ("/datasets/<dataset_id>/artifacts/<page_type>/<path:slug>", ("PUT",), "update_wiki_page"),
    ("/datasets/<dataset_id>/index", ("POST",), "run_index"),
    ("/datasets/<dataset_id>/index", ("GET",), "trace_index"),
    ("/datasets/<dataset_id>/<index_type>", ("DELETE",), "delete_index"),
    ("/datasets/<dataset_id>/index", ("DELETE",), "delete_index"),
    ("/datasets/<dataset_id>/embedding", ("POST",), "run_embedding"),
    ("/datasets/<dataset_id>/embedding/check", ("POST",), "check_embedding"),
    ("/datasets/<dataset_id>/ingestions", ("GET",), "list_ingestion_logs"),
    ("/datasets/<dataset_id>/ingestions/<log_id>", ("GET",), "get_ingestion_log"),
    ("/datasets/<dataset_id>/metadata/config", ("GET",), "get_auto_metadata"),
    ("/datasets/<dataset_id>/metadata/config", ("PUT",), "update_auto_metadata"),
}

_FILE_COMMIT_OPERATIONS = (
    ("/commits", ("POST",), "create_commit"),
    ("/commits", ("GET",), "list_commits"),
    ("/commits/<commit_id>", ("GET",), "get_commit"),
    ("/commits/<commit_id>/files", ("GET",), "list_commit_files"),
    ("/commits/diff", ("GET",), "diff_commits"),
    ("/changes", ("GET",), "get_uncommitted_changes"),
    ("/commits/<commit_id>/tree", ("GET",), "get_commit_tree"),
    ("/commits/<commit_id>/files/<file_id>/content", ("GET",), "get_commit_file_content"),
)
EXPECTED_FILE_COMMIT_ROUTES = {
    (f"{prefix}{suffix}", methods, f"{endpoint}_{index}")
    for index, prefix in enumerate(
        ("/datasets/<entity_id>", "/workspace/<entity_id>", "/folders/<entity_id>"),
        start=1,
    )
    for suffix, methods, endpoint in _FILE_COMMIT_OPERATIONS
} | {("/files/<file_id>/versions", ("GET",), "get_file_version_history")}


class _RecordingManager:
    def __init__(self):
        self.routes = []

    def route(self, path, *, methods, endpoint=None, **_kwargs):
        def decorator(func):
            self.routes.append((path, tuple(sorted(methods)), endpoint or func.__name__))
            return func

        return decorator


def _assert_exact_inventory(module, expected):
    assert len(module.manager.routes) == len(expected)
    assert set(module.manager.routes) == expected


def _load_with_recording_manager(monkeypatch, fixture, loader_name):
    monkeypatch.setattr(fixture, "_DummyManager", _RecordingManager)
    if fixture is DATASET_FIXTURE:
        return getattr(fixture, loader_name)(monkeypatch, registration_only=True)
    return getattr(fixture, loader_name)(monkeypatch)


def test_user_route_inventory_is_exact(monkeypatch):
    _assert_exact_inventory(
        USER_FIXTURE._load_user_app(monkeypatch),
        USER_FIXTURE.EXPECTED_USER_ROUTES,
    )


def test_user_route_inventory_rejects_changed_registration(monkeypatch):
    module = USER_FIXTURE._load_user_app(monkeypatch)
    module.manager.routes[-1] = ("/users/unexpected", ("POST",), "unexpected")

    with pytest.raises(AssertionError):
        _assert_exact_inventory(module, USER_FIXTURE.EXPECTED_USER_ROUTES)


@pytest.mark.parametrize(
    ("fixture", "loader_name", "expected"),
    (
        (SYSTEM_FIXTURE, "_load_system_module", EXPECTED_SYSTEM_ROUTES),
        (DATASET_FIXTURE, "_load_dataset_module", EXPECTED_DATASET_ROUTES),
        (FILE_COMMIT_FIXTURE, "_load_module", EXPECTED_FILE_COMMIT_ROUTES),
    ),
    ids=("system", "dataset", "file-commit"),
)
def test_access_policy_route_inventory_is_exact(monkeypatch, fixture, loader_name, expected):
    module = _load_with_recording_manager(monkeypatch, fixture, loader_name)
    _assert_exact_inventory(module, expected)


@pytest.mark.parametrize(
    ("fixture", "loader_name", "expected"),
    (
        (SYSTEM_FIXTURE, "_load_system_module", EXPECTED_SYSTEM_ROUTES),
        (DATASET_FIXTURE, "_load_dataset_module", EXPECTED_DATASET_ROUTES),
        (FILE_COMMIT_FIXTURE, "_load_module", EXPECTED_FILE_COMMIT_ROUTES),
    ),
    ids=("system", "dataset", "file-commit"),
)
def test_access_policy_route_inventory_rejects_changed_registration(monkeypatch, fixture, loader_name, expected):
    module = _load_with_recording_manager(monkeypatch, fixture, loader_name)
    module.manager.routes[-1] = ("/access-policy/unexpected", ("POST",), "unexpected")

    with pytest.raises(AssertionError):
        _assert_exact_inventory(module, expected)
