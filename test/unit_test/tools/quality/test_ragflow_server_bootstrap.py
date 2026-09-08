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
import signal
import sys
import threading
import types
from pathlib import Path

import pytest


_ROOT = Path(__file__).resolve().parents[4]
_EXPECTED_BOOTSTRAP_EVENTS = [
    "root_logger",
    "otel",
    "show_configs",
    "settings.init",
    "settings.print",
    "db.init_tables",
    "db.init_data",
    "audit.purge",
    "runtime.init_env",
    "runtime.init_config",
    "plugins.load",
    "signal.SIGINT",
    "signal.SIGTERM",
    "timer.start",
    "thread.start:chat-channels",
    "channel.start",
    "business_documents.start",
    "app.run",
]


def _install_module(monkeypatch, name: str, **attributes):
    module = types.ModuleType(name)
    module.__dict__.update(attributes)
    monkeypatch.setitem(sys.modules, name, module)
    parent_name, _, child_name = name.rpartition(".")
    if parent_name:
        setattr(sys.modules[parent_name], child_name, module)
    return module


def _install_package(monkeypatch, name: str):
    package = types.ModuleType(name)
    package.__path__ = []
    monkeypatch.setitem(sys.modules, name, package)
    parent_name, _, child_name = name.rpartition(".")
    if parent_name:
        setattr(sys.modules[parent_name], child_name, package)
    return package


def _assert_bootstrap_events(events):
    assert events == _EXPECTED_BOOTSTRAP_EVENTS


def _run_server(monkeypatch):
    events = []
    observed = {}

    for package_name in (
        "api",
        "api.apps",
        "api.apps.business_documents",
        "api.channels",
        "api.db",
        "api.db.services",
        "common",
        "agent",
        "rag",
        "rag.utils",
    ):
        _install_package(monkeypatch, package_name)

    def app_run(**kwargs):
        events.append("app.run")
        observed["app_run"] = kwargs

    sys.modules["api.apps"].app = types.SimpleNamespace(run=app_run)

    class RuntimeConfig:
        DEBUG = False

        @classmethod
        def init_env(cls):
            events.append("runtime.init_env")

        @classmethod
        def init_config(cls, **kwargs):
            events.append("runtime.init_config")
            observed["runtime_config"] = kwargs

    class GlobalPluginManager:
        @staticmethod
        def load_plugins():
            events.append("plugins.load")

    class RedisDistributedLock:
        def __init__(self, *_args, **_kwargs):
            pass

    class DocumentService:
        @staticmethod
        def update_progress():
            raise AssertionError("the delayed progress loop must not run in this contract")

    settings = _install_module(
        monkeypatch,
        "common.settings",
        HOST_IP="127.0.0.2",
        HOST_PORT=9380,
        init_settings=lambda: events.append("settings.init"),
        print_rag_settings=lambda: events.append("settings.print"),
    )
    _install_module(monkeypatch, "api.db.runtime_config", RuntimeConfig=RuntimeConfig)
    _install_module(monkeypatch, "api.db.services.document_service", DocumentService=DocumentService)
    _install_module(monkeypatch, "common.file_utils", get_project_base_directory=lambda: str(_ROOT))
    _install_module(monkeypatch, "api.db.db_models", init_database_tables=lambda: events.append("db.init_tables"))
    _install_module(
        monkeypatch,
        "api.db.init_data",
        init_web_data=lambda: events.append("db.init_data"),
        init_superuser=lambda: events.append("db.init_superuser"),
    )
    _install_module(monkeypatch, "common.versions", get_ragflow_version=lambda: "test-version")
    _install_module(monkeypatch, "common.config_utils", show_configs=lambda: events.append("show_configs"))
    _install_module(monkeypatch, "common.mcp_tool_call_conn", shutdown_all_mcp_sessions=lambda: events.append("mcp.shutdown"))
    _install_module(monkeypatch, "common.log_utils", init_root_logger=lambda _name: events.append("root_logger"))
    _install_module(
        monkeypatch,
        "common.observability",
        configure_otel=lambda _service, _version: events.append("otel"),
    )
    _install_module(monkeypatch, "agent.plugin", GlobalPluginManager=GlobalPluginManager)
    _install_module(monkeypatch, "rag.utils.redis_conn", RedisDistributedLock=RedisDistributedLock)
    _install_module(
        monkeypatch,
        "api.db.services.audit_service",
        purge_expired_audit_events=lambda: events.append("audit.purge") or 0,
    )

    def start_channel_server(stop_event):
        events.append("channel.start")
        observed["channel_stop_event"] = stop_event

    def start_business_document_worker(stop_event):
        events.append("business_documents.start")
        observed["business_documents_stop_event"] = stop_event

    _install_module(monkeypatch, "api.channels.bootstrap", start_channel_server=start_channel_server)
    _install_module(
        monkeypatch,
        "api.apps.business_documents.worker",
        start_business_document_worker=start_business_document_worker,
    )

    class FakeTimer:
        def __init__(self, interval, target):
            observed["timer"] = (interval, target)

        def start(self):
            events.append("timer.start")

    class FakeThread:
        def __init__(self, *, target, args=(), daemon=None, name=None):
            self.target = target
            self.args = args
            self.daemon = daemon
            self.name = name

        def start(self):
            events.append(f"thread.start:{self.name}")
            observed["thread"] = self
            self.target(*self.args)

    def register_signal(sig, handler):
        label = "SIGINT" if sig == signal.SIGINT else "SIGTERM" if sig == signal.SIGTERM else str(sig)
        events.append(f"signal.{label}")
        observed.setdefault("signals", []).append((sig, handler))

    monkeypatch.setattr(threading, "Timer", FakeTimer)
    monkeypatch.setattr(threading, "Thread", FakeThread)
    monkeypatch.setattr(signal, "signal", register_signal)
    monkeypatch.setattr(faulthandler, "enable", lambda: None)
    monkeypatch.setattr(sys, "argv", ["ragflow_server.py"])
    monkeypatch.setenv("RAGFLOW_DEBUGPY_LISTEN", "0")
    monkeypatch.setenv("LITELLM_LOCAL_MODEL_COST_MAP", "True")

    namespace = runpy.run_path(str(_ROOT / "api" / "ragflow_server.py"), run_name="__main__")
    observed["settings"] = settings
    return events, observed, namespace


def test_ragflow_server_bootstrap_contract_is_exact(monkeypatch):
    events, observed, namespace = _run_server(monkeypatch)

    _assert_bootstrap_events(events)
    assert observed["runtime_config"] == {"JOB_SERVER_HOST": "127.0.0.2", "HTTP_PORT": 9380}
    assert observed["app_run"] == {
        "host": "127.0.0.2",
        "port": 9380,
        "use_reloader": False,
        "debug": False,
    }
    assert observed["timer"][0] == 1.0
    assert observed["timer"][1].__name__ == "delayed_start_update_progress"
    assert observed["thread"].daemon is True
    assert observed["thread"].name == "chat-channels"
    assert observed["channel_stop_event"] is namespace["stop_event"]
    assert observed["business_documents_stop_event"] is namespace["stop_event"]
    assert [sig for sig, _handler in observed["signals"]] == [signal.SIGINT, signal.SIGTERM]
    assert all(handler is namespace["signal_handler"] for _sig, handler in observed["signals"])


def test_ragflow_server_bootstrap_contract_rejects_missing_worker():
    mutated = [event for event in _EXPECTED_BOOTSTRAP_EVENTS if event != "business_documents.start"]

    with pytest.raises(AssertionError):
        _assert_bootstrap_events(mutated)
