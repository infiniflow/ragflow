"""Exact registration contracts for the locally changed model HTTP surfaces."""

import importlib.util
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace

import pytest


ROOT = Path(__file__).resolve().parents[4]

EXPECTED_MODELS_ROUTES = {
    ("/models", ("GET",), "get_added_models"),
    ("/models/default", ("GET",), "get_default_models"),
    ("/models/default", ("PATCH",), "set_default_models"),
}

EXPECTED_PROVIDER_ROUTES = {
    ("/providers", ("GET",), "list_providers"),
    ("/providers", ("PUT",), "add_provider"),
    ("/providers/<provider_id_or_name>", ("GET",), "show_provider"),
    ("/providers/<provider_id_or_name>", ("DELETE",), "delete_provider"),
    ("/providers/<provider_id_or_name>/models", ("GET",), "list_provider_models"),
    ("/providers/<provider_id_or_name>/models/<path:model_name>", ("GET",), "show_provider_model"),
    ("/providers/<provider_id_or_name>/instances", ("POST",), "create_provider_instance"),
    ("/providers/<provider_id_or_name>/connection", ("POST",), "verify_provider_api_key"),
    ("/providers/<provider_id_or_name>/instances", ("GET",), "list_provider_instances"),
    (
        "/providers/<provider_id_or_name>/instances/<instance_id_or_name>",
        ("GET",),
        "show_provider_instance",
    ),
    ("/providers/<provider_id_or_name>/instances", ("DELETE",), "drop_provider_instances"),
    (
        "/providers/<provider_id_or_name>/instances/<instance_id_or_name>/models",
        ("GET",),
        "list_instance_models",
    ),
    (
        "/providers/<provider_id_or_name>/instances/<instance_id_or_name>/models",
        ("PUT",),
        "update_instance_models",
    ),
    (
        "/providers/<provider_id_or_name>/instances/<instance_id_or_name>/models",
        ("POST",),
        "add_model_to_instance",
    ),
    (
        "/providers/<provider_id_or_name>/instances/<instance_id_or_name>/models/<path:model_name>",
        ("PATCH",),
        "enable_or_disable_model",
    ),
    (
        "/providers/<provider_id_or_name>/instances/<instance_id_or_name>/models/<path:model_name>",
        ("POST",),
        "chat_to_model",
    ),
}


class _RecordingManager:
    def __init__(self):
        self.routes = []

    def route(self, path, *, methods, endpoint=None, **_kwargs):
        def decorator(func):
            self.routes.append((path, tuple(sorted(methods)), endpoint or func.__name__))
            return func

        return decorator


def _stub_module(monkeypatch, name, **attrs):
    module = ModuleType(name)
    for key, value in attrs.items():
        setattr(module, key, value)
    monkeypatch.setitem(sys.modules, name, module)
    return module


def _passthrough_decorator(func=None, **_kwargs):
    if func is None:
        return lambda decorated: decorated
    return func


def _install_registration_stubs(monkeypatch):
    api_mod = _stub_module(monkeypatch, "api")
    api_mod.__path__ = [str(ROOT / "api")]
    apps_mod = _stub_module(
        monkeypatch,
        "api.apps",
        current_user=SimpleNamespace(id="user-1", is_superuser=False),
        login_required=_passthrough_decorator,
    )
    apps_mod.__path__ = [str(ROOT / "api/apps")]

    services_mod = _stub_module(monkeypatch, "api.apps.services")
    services_mod.__path__ = []
    services_mod.models_api_service = _stub_module(monkeypatch, "api.apps.services.models_api_service")
    services_mod.provider_api_service = _stub_module(monkeypatch, "api.apps.services.provider_api_service")

    db_mod = _stub_module(monkeypatch, "api.db")
    db_mod.__path__ = []
    db_services_mod = _stub_module(monkeypatch, "api.db.services")
    db_services_mod.__path__ = []
    _stub_module(
        monkeypatch,
        "api.db.services.managed_resource_service",
        ManagedResourceService=type("ManagedResourceService", (), {}),
    )

    utils_mod = _stub_module(monkeypatch, "api.utils")
    utils_mod.__path__ = []
    _stub_module(
        monkeypatch,
        "api.utils.api_utils",
        add_managed_resource_owner_id_to_kwargs=_passthrough_decorator,
        add_tenant_id_to_kwargs=_passthrough_decorator,
        get_error_argument_result=lambda **_kwargs: None,
        get_error_data_result=lambda **_kwargs: None,
        get_result=lambda **_kwargs: None,
    )
    _stub_module(monkeypatch, "quart", request=SimpleNamespace())


def _load_api(monkeypatch, file_name):
    _install_registration_stubs(monkeypatch)
    module_name = f"models_runtime_{file_name.removesuffix('.py')}_route_contract_module"
    spec = importlib.util.spec_from_file_location(module_name, ROOT / "api/apps/restful_apis" / file_name)
    module = importlib.util.module_from_spec(spec)
    module.manager = _RecordingManager()
    monkeypatch.setitem(sys.modules, module_name, module)
    spec.loader.exec_module(module)
    return module


def _assert_exact_inventory(module, expected):
    assert len(module.manager.routes) == len(expected)
    assert set(module.manager.routes) == expected


@pytest.mark.parametrize(
    ("file_name", "expected"),
    (("models_api.py", EXPECTED_MODELS_ROUTES), ("provider_api.py", EXPECTED_PROVIDER_ROUTES)),
    ids=("models", "provider"),
)
def test_models_runtime_route_inventory_is_exact(monkeypatch, file_name, expected):
    _assert_exact_inventory(_load_api(monkeypatch, file_name), expected)


@pytest.mark.parametrize(
    ("file_name", "expected"),
    (("models_api.py", EXPECTED_MODELS_ROUTES), ("provider_api.py", EXPECTED_PROVIDER_ROUTES)),
    ids=("models", "provider"),
)
def test_models_runtime_route_inventory_rejects_changed_registration(monkeypatch, file_name, expected):
    module = _load_api(monkeypatch, file_name)
    module.manager.routes[-1] = ("/models-runtime/unexpected", ("POST",), "unexpected")

    with pytest.raises(AssertionError):
        _assert_exact_inventory(module, expected)
