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
from __future__ import annotations

import faulthandler
import runpy
import sys
import types
from pathlib import Path

import pytest


_ROOT = Path(__file__).resolve().parents[4]
_EXPECTED_EVENTS = [
    "faulthandler",
    "root_logger",
    "otel",
    "flask.create",
    "flask.instrument",
    "flask.blueprint",
    "session",
    "show_configs",
    "login_manager.create",
    "login_manager.init_app",
    "settings.init",
    "auth.setup",
    "auth.default",
    "config.load",
    "run_simple",
]


def _install_package(monkeypatch, name: str):
    package = types.ModuleType(name)
    package.__path__ = []
    monkeypatch.setitem(sys.modules, name, package)
    return package


def _install_module(monkeypatch, name: str, **attributes):
    module = types.ModuleType(name)
    module.__dict__.update(attributes)
    monkeypatch.setitem(sys.modules, name, module)
    parent_name, _, child_name = name.rpartition(".")
    if parent_name:
        setattr(sys.modules[parent_name], child_name, module)
    return module


def _assert_admin_bootstrap_events(events):
    assert events == _EXPECTED_EVENTS


def _run_admin_server(monkeypatch):
    events = []
    observed = {}
    _install_package(monkeypatch, "common")

    class FakeFlask:
        def __init__(self, module_name):
            events.append("flask.create")
            observed["module_name"] = module_name
            self.config = {}

        def register_blueprint(self, blueprint):
            events.append("flask.blueprint")
            observed["blueprint"] = blueprint

    class FakeLoginManager:
        def __init__(self):
            events.append("login_manager.create")

        def init_app(self, app):
            events.append("login_manager.init_app")
            observed["login_app"] = app

    def run_simple(**kwargs):
        events.append("run_simple")
        observed["run_simple"] = kwargs

    admin_blueprint = object()
    service_configs = types.SimpleNamespace(configs=None)
    _install_module(monkeypatch, "flask", Flask=FakeFlask)
    _install_module(monkeypatch, "flask_login", LoginManager=FakeLoginManager)
    _install_package(monkeypatch, "werkzeug")
    _install_module(monkeypatch, "werkzeug.serving", run_simple=run_simple)
    _install_module(monkeypatch, "routes", admin_bp=admin_blueprint)
    _install_module(monkeypatch, "common.log_utils", init_root_logger=lambda _name: events.append("root_logger"))
    _install_module(monkeypatch, "common.constants", SERVICE_CONF="service-conf.yaml")
    _install_module(monkeypatch, "common.config_utils", show_configs=lambda: events.append("show_configs"))
    _install_module(monkeypatch, "common.settings", init_settings=lambda: events.append("settings.init"))

    def load_configurations(path):
        events.append("config.load")
        observed["config_path"] = path
        return {"loaded": True}

    _install_module(
        monkeypatch,
        "config",
        load_configurations=load_configurations,
        SERVICE_CONFIGS=service_configs,
    )
    _install_module(
        monkeypatch,
        "auth",
        init_default_admin=lambda: events.append("auth.default"),
        setup_auth=lambda _manager: events.append("auth.setup"),
    )
    _install_module(monkeypatch, "flask_session", Session=lambda _app: events.append("session"))
    _install_module(monkeypatch, "common.versions", get_ragflow_version=lambda: "test-version")
    _install_module(
        monkeypatch,
        "common.observability",
        configure_otel=lambda _service, _version: events.append("otel"),
        install_flask_instrumentation=lambda _app, _service: events.append("flask.instrument"),
    )
    monkeypatch.setattr(faulthandler, "enable", lambda: events.append("faulthandler"))
    monkeypatch.setenv("MAX_CONTENT_LENGTH", "2048")

    namespace = runpy.run_path(str(_ROOT / "admin" / "server" / "admin_server.py"), run_name="__main__")
    observed["service_configs"] = service_configs
    observed["admin_blueprint"] = admin_blueprint
    return events, observed, namespace


def test_admin_server_bootstrap_contract_is_exact(monkeypatch):
    events, observed, namespace = _run_admin_server(monkeypatch)

    _assert_admin_bootstrap_events(events)
    app = namespace["app"]
    assert observed["module_name"] == "__main__"
    assert observed["blueprint"] is observed["admin_blueprint"]
    assert observed["login_app"] is app
    assert app.config == {
        "SESSION_PERMANENT": False,
        "SESSION_TYPE": "filesystem",
        "MAX_CONTENT_LENGTH": 2048,
    }
    assert observed["config_path"] == "service-conf.yaml"
    assert observed["service_configs"].configs == {"loaded": True}
    assert observed["run_simple"] == {
        "hostname": "0.0.0.0",
        "port": 9381,
        "application": app,
        "threaded": True,
        "use_reloader": False,
        "use_debugger": False,
    }


def test_admin_server_bootstrap_contract_rejects_missing_http_start():
    mutated = [event for event in _EXPECTED_EVENTS if event != "run_simple"]

    with pytest.raises(AssertionError):
        _assert_admin_bootstrap_events(mutated)
