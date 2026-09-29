#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
"""`OAUTH_AUTO_REGISTER` as `init_settings()` actually resolves it.

`test_env_flag` covers the parser in isolation. This covers the one line that
binds the parser to the environment variable, so a renamed variable, a flipped
default or a helper that stops being called is caught at the boundary the
deployment documentation describes.

Importing `common.settings` pulls in every document store, message store and
blob storage client, so those modules are replaced by inert stubs first, the
same way `test_gaussdb_settings` does it.
"""

import importlib
import sys
import types

import pytest


class _Stub:
    """Stands in for any client `init_settings()` constructs.

    Calling it or reading an attribute yields the stub again, so none of the
    storage or index clients need a purpose-built fake here.
    """

    def __call__(self, *args, **kwargs):
        return self

    def __getattr__(self, name):
        return self


_STUB = _Stub()


class _StubModule(types.ModuleType):
    def __getattr__(self, name):
        return _STUB


_STUBBED_MODULES = (
    "rag.utils.es_conn",
    "rag.utils.infinity_conn",
    "rag.utils.ob_conn",
    "rag.utils.opensearch_conn",
    "rag.utils.gaussdb_conn",
    "rag.utils.azure_sas_conn",
    "rag.utils.azure_spn_conn",
    "rag.utils.gcs_conn",
    "rag.utils.minio_conn",
    "rag.utils.opendal_conn",
    "rag.utils.redis_conn",
    "rag.utils.s3_conn",
    "rag.utils.oss_conn",
    "rag.nlp",
    "rag.graphrag",
    "rag.graphrag.search",
    "memory.utils.es_conn",
    "memory.utils.infinity_conn",
    "memory.utils.ob_conn",
    "memory.utils.gaussdb_conn",
)


@pytest.fixture
def settings(monkeypatch):
    importlib.import_module("rag.utils")
    importlib.import_module("memory.utils")

    for name in _STUBBED_MODULES:
        stub = _StubModule(name)
        monkeypatch.setitem(sys.modules, name, stub)
        parent_name, _, child_name = name.rpartition(".")
        parent = sys.modules.get(parent_name)
        if parent is not None:
            monkeypatch.setattr(parent, child_name, stub, raising=False)

    import common

    # Import machinery assigns a freshly imported submodule to
    # ``common.settings`` on the parent package. Restoring only
    # ``sys.modules["common.settings"]`` leaves that parent attribute pointing
    # at this test's stub-backed module and contaminates later tests.
    monkeypatch.setattr(common, "settings", getattr(common, "settings", None), raising=False)
    # ``delitem`` records nothing when the key is absent, so the stub-backed
    # module imported below would outlive the test. ``setitem`` always records
    # the previous state, and teardown then removes or restores the entry.
    monkeypatch.setitem(sys.modules, "common.settings", None)
    del sys.modules["common.settings"]
    return importlib.import_module("common.settings")


def test_unset_provisions_oauth_arrivals(settings, monkeypatch):
    monkeypatch.delenv("OAUTH_AUTO_REGISTER", raising=False)

    settings.init_settings()

    assert settings.OAUTH_AUTO_REGISTER is True


@pytest.mark.parametrize("raw", ["1", "true", "TRUE", "yes", "on", " true "])
def test_an_enabled_value_keeps_auto_registration_on(settings, monkeypatch, raw):
    monkeypatch.setenv("OAUTH_AUTO_REGISTER", raw)

    settings.init_settings()

    assert settings.OAUTH_AUTO_REGISTER is True


@pytest.mark.parametrize("raw", ["0", "false", "FALSE", "no", "off", "disabled", ""])
def test_anything_else_turns_auto_registration_off(settings, monkeypatch, raw):
    """`off` and `disabled` are the values this pull request changes.

    The previous deny-list parse recognized only `0`, `false` and `no`, so an
    administrator who wrote `off` kept provisioning accounts for every stranger
    who arrived over OAuth.
    """
    monkeypatch.setenv("OAUTH_AUTO_REGISTER", raw)

    settings.init_settings()

    assert settings.OAUTH_AUTO_REGISTER is False
