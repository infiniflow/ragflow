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
"""Regression tests for list_tenant_added_models() in models_api_service.

Covers the ValueError that occurred when a manual model record's model_name
contained an `@` character (e.g. LM Studio embedding model IDs that include
the quantization tag, such as `text-embedding-nomic-embed-text-v1.5@q8_0`).
"""

import importlib.util
import logging
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace
from unittest.mock import MagicMock

import pytest

pytestmark = pytest.mark.p2


def _stub(monkeypatch, name, **attrs):
    """Register a stub module in `sys.modules` with the given attributes.

    For dotted names (e.g. `api.db.services.user_service`) the stub is also
    attached as an attribute of the parent module so that code which already
    imported the parent and references `parent.child` sees the stub.

    Args:
        monkeypatch: pytest monkeypatch fixture.
        name: Fully qualified module name to stub.
        **attrs: Attributes to set on the stub module.

    Returns:
        The created `types.ModuleType` instance.
    """
    mod = ModuleType(name)
    for key, value in attrs.items():
        setattr(mod, key, value)
    monkeypatch.setitem(sys.modules, name, mod)
    if "." in name:
        parent_name, _, child_name = name.rpartition(".")
        parent_mod = sys.modules.get(parent_name)
        if parent_mod is not None:
            monkeypatch.setattr(parent_mod, child_name, mod, raising=False)
    return mod


def _load_module(monkeypatch, *, tenant_model_records, factory_llm_infos=None, tenant_llm_rows=None):
    """Load models_api_service with stubbed dependencies.

    `tenant_model_records` is the list returned by
    TenantModelService.get_models_by_provider_ids_and_instance_ids. Pass an
    empty list to test the no-models path.

    `tenant_llm_rows` controls the rows returned by
    ``TenantLLMService.get_my_llms``; pass ``None`` (default) to have the
    stub raise on access, or pass a list of dicts to have the stub return
    that list. This exercises the bare-model resolution path in
    ``_get_model_info``.

    Returns a tuple `(module, stubs)` where `stubs` is a dict mapping the
    stubbed module names to the stub modules, so callers can monkeypatch
    additional behaviour at runtime.
    """

    tenant = SimpleNamespace(id="tenant-1")
    provider = SimpleNamespace(
        id="provider-1",
        provider_name="LM-Studio",
    )
    instance = SimpleNamespace(
        id="instance-1",
        provider_id="provider-1",
        instance_name="default",
    )

    _stub(
        monkeypatch,
        "api.db.services.user_service",
        TenantService=SimpleNamespace(get_by_id=lambda tenant_id: (True, tenant)),
    )
    _stub(
        monkeypatch,
        "api.db.services.tenant_model_provider_service",
        TenantModelProviderService=SimpleNamespace(
            get_by_tenant_id=lambda tenant_id: [provider],
            # Default no-op; tests that need to observe the resolved provider
            # name passed by `_get_model_info` override this with
            # monkeypatch.setattr on the loaded stub.
            get_by_tenant_id_and_provider_name=lambda tenant_id, provider_name: SimpleNamespace(
                id="provider-1", provider_name=provider_name
            ),
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.tenant_model_instance_service",
        TenantModelInstanceService=SimpleNamespace(
            get_by_provider_ids=lambda provider_ids: [instance],
            get_by_provider_id_and_instance_name=lambda provider_id, instance_name: SimpleNamespace(
                id="instance-1", provider_id=provider_id, instance_name=instance_name
            ),
        ),
    )
    _stub(
        monkeypatch,
        "api.db.services.tenant_model_service",
        TenantModelService=SimpleNamespace(
            get_models_by_provider_ids_and_instance_ids=lambda p_ids, i_ids: list(tenant_model_records),
            # Default returns an "active" model so `_get_model_info` treats
            # the row as enabled when exercising the bare-model branch.
            get_by_provider_id_and_instance_id_and_model_type_and_model_name=lambda *args: SimpleNamespace(
                status=1
            ),
        ),
    )

    if tenant_llm_rows is None:
        # Default: access to TenantLLMService.get_my_llms raises. Callers that
        # exercise bare-model resolution must pass an explicit list.
        def _raise(*_args, **_kwargs):
            raise AssertionError(
                "TenantLLMService.get_my_llms was called but the test did "
                "not configure tenant_llm_rows"
            )

        _stub(
            monkeypatch,
            "api.db.services.tenant_llm_service",
            TenantLLMService=SimpleNamespace(get_my_llms=_raise),
        )
    else:
        rows_snapshot = list(tenant_llm_rows)

        def _return_rows(*_args, **_kwargs):
            return [dict(row) for row in rows_snapshot]

        _stub(
            monkeypatch,
            "api.db.services.tenant_llm_service",
            TenantLLMService=SimpleNamespace(get_my_llms=_return_rows),
        )

    # joint_services.tenant_model_service is imported by models_api_service at
    # module load for the three ensure_* helpers; stub it as a no-op.
    _stub(
        monkeypatch,
        "api.db.joint_services.tenant_model_service",
        ensure_mineru_from_env=lambda *a, **kw: None,
        ensure_paddleocr_from_env=lambda *a, **kw: None,
        ensure_opendataloader_from_env=lambda *a, **kw: None,
    )
    _stub(
        monkeypatch,
        "api.db.joint_services",
        tenant_model_service=sys.modules["api.db.joint_services.tenant_model_service"],
    )

    _stub(
        monkeypatch,
        "common.constants",
        ActiveStatusEnum=SimpleNamespace(ACTIVE=SimpleNamespace(value=1), INACTIVE=SimpleNamespace(value=0), UNSUPPORTED=SimpleNamespace(value=2)),
        LLMType=SimpleNamespace(EMBEDDING="embedding"),
    )
    _stub(
        monkeypatch,
        "common.settings",
        FACTORY_LLM_INFOS=factory_llm_infos if factory_llm_infos is not None else [],
    )

    module_path = (
        Path(__file__).resolve().parents[5]
        / "api"
        / "apps"
        / "services"
        / "models_api_service.py"
    )
    spec = importlib.util.spec_from_file_location(
        "test_models_api_service_list_tenant_added_models",
        module_path,
    )
    module = importlib.util.module_from_spec(spec)
    monkeypatch.setitem(sys.modules, "test_models_api_service_list_tenant_added_models", module)
    spec.loader.exec_module(module)
    stubs = {
        "user_service": sys.modules["api.db.services.user_service"],
        "tenant_model_provider_service": sys.modules["api.db.services.tenant_model_provider_service"],
        "tenant_model_instance_service": sys.modules["api.db.services.tenant_model_instance_service"],
        "tenant_model_service": sys.modules["api.db.services.tenant_model_service"],
        "tenant_llm_service": sys.modules["api.db.services.tenant_llm_service"],
    }
    return module, stubs


def _make_model_record(model_name, model_type="embedding", status=1):
    """Build a `SimpleNamespace` mimicking a `TenantModel` ORM row.

    The default provider_id/instance_id pair (`provider-1`/`instance-1`)
    matches the fixtures wired up by `_load_module` so the resulting key
    `provider-1@instance-1@<model_name>` round-trips through
    `list_tenant_added_models` as expected.

    Args:
        model_name: Model name; may contain `@` characters.
        model_type: Model type filter (default `embedding`).
        status: `ActiveStatusEnum` value (default `1` = ACTIVE).

    Returns:
        A `SimpleNamespace` with the fields read by
        `list_tenant_added_models`.
    """
    return SimpleNamespace(
        provider_id="provider-1",
        instance_id="instance-1",
        model_name=model_name,
        model_type=model_type,
        status=status,
    )


@pytest.mark.p2
def test_list_tenant_added_models_handles_at_symbol_in_model_name(monkeypatch):
    """Regression: model_name containing '@' must not raise ValueError.

    LM Studio exposes embedding model IDs like
    `text-embedding-nomic-embed-text-v1.5@q8_0`. RAGFlow used to build a
    composite key `provider@instance@model_name` and then unpack it back with
    `split("@")`, which raised ValueError when the model_name itself contained
    `@`. After the fix, the key is split with `rsplit("@", 2)` so any extra
    `@` characters stay attached to model_name.
    """
    record = _make_model_record("text-embedding-nomic-embed-text-v1.5@q8_0")
    module, _ = _load_module(monkeypatch, tenant_model_records=[record])

    success, result = module.list_tenant_added_models("tenant-1", "embedding")

    assert success is True
    assert isinstance(result, list)
    assert len(result) == 1
    added = result[0]
    assert added["name"] == "text-embedding-nomic-embed-text-v1.5@q8_0"
    assert added["provider_id"] == "provider-1"
    assert added["instance_id"] == "instance-1"
    assert added["provider_name"] == "LM-Studio"
    assert added["instance_name"] == "default"


@pytest.mark.p2
def test_list_tenant_added_models_handles_multiple_at_symbols_in_model_name(monkeypatch):
    """Same regression with multiple '@' characters in the model_name."""
    record = _make_model_record("foo@bar@baz@q4_k_m")
    module, _ = _load_module(monkeypatch, tenant_model_records=[record])

    success, result = module.list_tenant_added_models("tenant-1", "embedding")

    assert success is True
    assert len(result) == 1
    assert result[0]["name"] == "foo@bar@baz@q4_k_m"
    assert result[0]["provider_id"] == "provider-1"
    assert result[0]["instance_id"] == "instance-1"


@pytest.mark.p2
def test_list_tenant_added_models_preserves_full_model_name_with_at(monkeypatch):
    """The trailing model_name must absorb any extra '@' characters.

    Verifies that split('@', 2) (not rsplit) is used so the right-hand side
    is preserved. If rsplit were used, the trailing '@q8_0' would become the
    full model_name and the prefix would be lost.
    """
    record = _make_model_record("a/b@q8_0")
    module, _ = _load_module(monkeypatch, tenant_model_records=[record])

    success, result = module.list_tenant_added_models("tenant-1", "embedding")

    assert success is True
    assert len(result) == 1
    assert result[0]["name"] == "a/b@q8_0"
    assert result[0]["provider_id"] == "provider-1"
    assert result[0]["instance_id"] == "instance-1"


@pytest.mark.p2
def test_list_tenant_added_models_still_works_for_plain_model_names(monkeypatch):
    """Sanity check: the rsplit change must not break the standard case."""
    record = _make_model_record("gemma-4-12b-it-qat", model_type="chat")
    module, _ = _load_module(monkeypatch, tenant_model_records=[record])

    success, result = module.list_tenant_added_models("tenant-1", "chat")

    assert success is True
    assert len(result) == 1
    assert result[0]["name"] == "gemma-4-12b-it-qat"


# NOTE: A unit test for the defensive try/except in the production code is
# intentionally omitted. In the current production flow every key built by
# `f"{provider_id}@{instance_id}@{model_name}"` has exactly 3 '@'-separated
# parts, so the defensive branch is unreachable. The branch is left in as
# insurance for future code paths that might construct keys differently.
# If a future change makes the branch reachable, add a focused unit test for
# it at that time.


@pytest.mark.p2
def test_get_model_info_resolves_bare_model_via_enrolled_llm(monkeypatch):
    """Bare model name (no '@') should resolve via TenantLLMService.

    The composite key ``provider-1@instance-1@model-name`` exists for newly
    enrolled models, but legacy tenants persisted a bare default such as
    ``gemma-4-12b-it-qat`` before composite keys became the convention.
    ``_get_model_info`` must look up the matching ``tenant_llm`` row and
    use its ``llm_factory`` as the provider name so the downstream
    provider/instance lookups can succeed.
    """
    bare_model = "gemma-4-12b-it-qat"
    enrolled = [{
        "llm_name": bare_model,
        "model_type": "embedding",
        "llm_factory": "LM-Studio",
    }]
    module, _ = _load_module(
        monkeypatch,
        tenant_model_records=[],
        tenant_llm_rows=enrolled,
    )

    # Override the provider/instance services so we observe what the bare
    # branch fed them. Both should see the resolved provider "LM-Studio"
    # and the default instance.
    captured = {}

    def fake_get_provider(tenant_id, provider_name):
        captured["provider_name"] = provider_name
        return SimpleNamespace(id="provider-1", provider_name=provider_name)

    def fake_get_instance(provider_id, instance_name):
        captured["instance_name"] = instance_name
        return SimpleNamespace(id="instance-1", provider_id=provider_id, instance_name=instance_name)

    def fake_get_model(*args):
        captured["model_args"] = args
        # status=1 == ACTIVE
        return SimpleNamespace(status=1)

    provider_svc = sys.modules["api.db.services.tenant_model_provider_service"].TenantModelProviderService
    instance_svc = sys.modules["api.db.services.tenant_model_instance_service"].TenantModelInstanceService
    model_svc = sys.modules["api.db.services.tenant_model_service"].TenantModelService
    monkeypatch.setattr(provider_svc, "get_by_tenant_id_and_provider_name", staticmethod(fake_get_provider))
    monkeypatch.setattr(instance_svc, "get_by_provider_id_and_instance_name", staticmethod(fake_get_instance))
    monkeypatch.setattr(
        model_svc,
        "get_by_provider_id_and_instance_id_and_model_type_and_model_name",
        staticmethod(fake_get_model),
    )

    result = module._get_model_info("tenant-1", bare_model, "embedding")

    assert result is not None
    assert result["model_provider"] == "LM-Studio"
    assert result["model_instance"] == "default"
    assert result["model_name"] == bare_model
    # And the resolved provider/instance were used downstream:
    assert captured["provider_name"] == "LM-Studio"
    assert captured["instance_name"] == "default"
    # The model_name passed downstream must be the bare name intact.
    assert captured["model_args"][3] == bare_model


@pytest.mark.p2
def test_get_model_info_returns_none_when_bare_model_no_unique_match(monkeypatch):
    """Bare model name with no enrolled match must return None (not crash).

    When the tenant has no ``tenant_llm`` row matching the bare name (or has
    multiple ambiguous matches), ``_resolve_bare_model_provider`` returns
    ``""`` and ``_get_model_info`` should fall through to a normal provider
    lookup that fails, returning None rather than guessing.
    """
    bare_model = "gemma-4-12b-it-qat"

    # Stub the downstream provider lookup to return None so the function
    # cleanly returns None without raising.
    module, _ = _load_module(
        monkeypatch,
        tenant_model_records=[],
        tenant_llm_rows=[],
    )
    provider_svc = sys.modules["api.db.services.tenant_model_provider_service"].TenantModelProviderService
    monkeypatch.setattr(
        provider_svc,
        "get_by_tenant_id_and_provider_name",
        staticmethod(lambda *a, **kw: None),
    )

    result = module._get_model_info("tenant-1", bare_model, "embedding")
    assert result is None


@pytest.mark.p2
def test_get_model_info_returns_none_when_bare_model_ambiguous_enrollment(monkeypatch):
    """Bare model name with multiple enrolled factories must return None.

    Two ``tenant_llm`` rows that share the same ``llm_name`` and
    ``model_type`` but disagree on ``llm_factory`` indicate a tenant-side
    data conflict. The helper refuses to guess between them and the
    fallback provider lookup fails, returning None.
    """
    bare_model = "gemma-4-12b-it-qat"
    enrolled = [
        {"llm_name": bare_model, "model_type": "embedding", "llm_factory": "LM-Studio"},
        {"llm_name": bare_model, "model_type": "embedding", "llm_factory": "Ollama"},
    ]
    module, _ = _load_module(
        monkeypatch,
        tenant_model_records=[],
        tenant_llm_rows=enrolled,
    )
    provider_svc = sys.modules["api.db.services.tenant_model_provider_service"].TenantModelProviderService
    monkeypatch.setattr(
        provider_svc,
        "get_by_tenant_id_and_provider_name",
        staticmethod(lambda *a, **kw: None),
    )

    result = module._get_model_info("tenant-1", bare_model, "embedding")
    assert result is None
