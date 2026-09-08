"""Exact registration contract for the locally changed agent HTTP surface."""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType

import pytest


ROOT = Path(__file__).resolve().parents[4]
FIXTURE_PATH = ROOT / "test/testcases/test_web_api/test_agent_app/test_agents_webhook_unit.py"
SPEC = importlib.util.spec_from_file_location("agent_runtime_route_contract_fixture", FIXTURE_PATH)
ROUTE_FIXTURE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(ROUTE_FIXTURE)

EXPECTED_AGENT_ROUTES = {
    ("/agents/<agent_id>/sessions", ("GET",), "list_agent_sessions"),
    ("/agents/<agent_id>/sessions", ("POST",), "create_agent_session"),
    ("/agents/<agent_id>/sessions/<session_id>", ("GET",), "get_agent_session"),
    ("/agents/<agent_id>/sessions/<session_id>", ("DELETE",), "delete_agent_session_item"),
    ("/agents/<agent_id>/sessions", ("DELETE",), "delete_agent_session"),
    ("/agents/download", ("GET",), "download_agent_file"),
    ("/agents/templates", ("GET",), "list_agent_template"),
    ("/agents/prompts", ("GET",), "prompts"),
    ("/agents", ("GET",), "list_agents"),
    ("/agents/tags", ("GET",), "list_agent_tags"),
    ("/agents/<canvas_id>/tags", ("PUT",), "update_agent_tags"),
    ("/agents", ("POST",), "create_agent"),
    ("/agents/<agent_id>/upload", ("POST",), "upload_agent_file"),
    ("/agents/<agent_id>/components/<component_id>/input-form", ("GET",), "get_agent_component_input_form"),
    ("/agents/<agent_id>/components/<component_id>/debug", ("POST",), "debug_agent_component"),
    ("/agents/<agent_id>", ("GET",), "get_agent"),
    ("/agents/<agent_id>/versions", ("GET",), "list_agent_versions"),
    ("/agents/<agent_id>/versions/<version_id>", ("GET",), "get_agent_version"),
    ("/agents/<agent_id>/logs/<message_id>", ("GET",), "get_agent_logs"),
    ("/agents/<agent_id>", ("DELETE",), "delete_agent"),
    ("/agents/<agent_id>", ("PUT",), "update_agent"),
    ("/agents/<agent_id>/reset", ("POST",), "reset_agent"),
    ("/agents/rerun", ("POST",), "rerun_agent"),
    ("/agents/test_db_connection", ("POST",), "test_db_connection"),
    ("/agents/chat/completions", ("POST",), "agent_chat_completion"),
    (
        "/agents/<agent_id>/webhook",
        ("DELETE", "GET", "HEAD", "PATCH", "POST", "PUT"),
        "webhook",
    ),
    (
        "/agents/<agent_id>/webhook/test",
        ("DELETE", "GET", "HEAD", "PATCH", "POST", "PUT"),
        "webhook_test",
    ),
    ("/agents/<agent_id>/webhook/logs", ("GET",), "webhook_trace"),
    ("/agents/attachments/<attachment_id>/preview", ("GET",), "preview_attachment"),
    ("/agents/attachments/<attachment_id>/download", ("GET",), "download_attachment"),
}


class _RecordingManager:
    def __init__(self):
        self.routes = []

    def route(self, path, *, methods, endpoint=None, **_kwargs):
        def decorator(func):
            self.routes.append((path, tuple(sorted(methods)), endpoint or func.__name__))
            return func

        return decorator


def _load_agent(monkeypatch):
    agent_file_service = ModuleType("api.apps.services.agent_file_service")
    agent_file_service.upload_agent_files = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, agent_file_service.__name__, agent_file_service)
    monkeypatch.setattr(ROUTE_FIXTURE, "_DummyManager", _RecordingManager)
    return ROUTE_FIXTURE._load_agents_app(monkeypatch)


def _assert_exact_inventory(module):
    assert len(module.manager.routes) == len(EXPECTED_AGENT_ROUTES)
    assert set(module.manager.routes) == EXPECTED_AGENT_ROUTES


def test_agent_runtime_route_inventory_is_exact(monkeypatch):
    _assert_exact_inventory(_load_agent(monkeypatch))


def test_agent_runtime_route_inventory_rejects_changed_registration(monkeypatch):
    module = _load_agent(monkeypatch)
    module.manager.routes[-1] = ("/agents/unexpected", ("POST",), "unexpected")

    with pytest.raises(AssertionError):
        _assert_exact_inventory(module)
