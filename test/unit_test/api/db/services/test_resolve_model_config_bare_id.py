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
"""Regression tests for issue #18398.

``resolve_model_config`` is the single resolution point every model
config call goes through. The default contract is the composite form
``name@instance@provider``; the RAGFlow frontend builds that form when
the user selects a model. Legacy / API-scripted deployments can store
the bare ``tenant_model.id`` (32-char UUID) instead -- the bare form
has no ``@`` so ``split_model_name`` returns an empty provider_name and
the provider-instance lookup raises ``Provider not found for model
<uuid>.``.

The fix: when ``get_model_config_by_id`` raises ``LookupError`` and the
``model_ref`` has no ``@``, build the composite form from the row
directly via a new helper ``_composite_from_bare_model_id`` and try
the provider-instance lookup with that. None falls through to the
original failing path so the behaviour for genuinely missing models
is unchanged.

The test exercises both paths with monkeypatched DB returns (the
``@DB.connection_context`` decorator skips the real DB on the test
thread, but the model / instance / provider / tenant service lookups
are not decorated and would otherwise hit the database). The
``resolve_model_config`` happy path (composite ``name@instance@provider``
model_ref) is also covered as a regression guard.
"""

import importlib

import pytest


def _reload_module():
    """Reload the module under test so monkeypatched DB returns take
    effect through the captured ``TenantModelService`` / etc. names."""
    import api.db.joint_services.tenant_model_service as mod

    return importlib.reload(mod)


@pytest.fixture
def tms(monkeypatch):
    """Return the tenant_model_service module with a fresh import,
    ready for monkeypatched TenantModelService / TenantModelProviderService
    / TenantModelInstanceService / TenantService lookups.
    """
    return _reload_module()


# ---------------------------------------------------------------------------
# resolve_model_config: composite name passes through
# ---------------------------------------------------------------------------


def test_resolve_model_config_composite_passes_through(tms, monkeypatch):
    """A composite model_ref ('name@instance@provider') is the
    canonical form. resolve_model_config returns the right config;
    pre-fix this worked, post-fix it still works (the bare-id
    fallback is only taken when the model_ref has no '@').
    """
    by_provider_called = {"hit": False}

    def _by_id(tenant_id, model_type, model_id):
        # The id-based path can succeed for a composite model_ref
        # too (TenantModelService.get_by_id is keyed by the model's
        # composite form too). Whichever path succeeds is fine.
        raise LookupError("composite model_ref does not match a tenant_model row")

    def _by_provider(tenant_id, model_type, model_name):
        by_provider_called["hit"] = True
        return {"llm_factory": "OpenAI", "llm_name": "gpt-4", "model_type": "chat", "api_key": "sk", "api_base": "https://api.openai.com/v1"}

    monkeypatch.setattr(tms, "get_model_config_by_id", _by_id)
    monkeypatch.setattr(tms, "get_model_config_from_provider_instance", _by_provider)

    from api.db.joint_services.tenant_model_service import LLMType

    out = tms.resolve_model_config("tenant-A", LLMType.CHAT, "gpt-4@default@OpenAI")
    assert out["llm_name"] == "gpt-4"
    assert by_provider_called["hit"], "a composite model_ref must reach the provider-instance lookup (the bare-id fallback is only taken when model_ref has no '@')"


# ---------------------------------------------------------------------------
# resolve_model_config: bare id with primary id-based success
# ---------------------------------------------------------------------------


def test_resolve_model_config_bare_id_primary_succeeds(tms, monkeypatch):
    """A bare id that the primary id-based lookup can resolve should
    use that path. The bare-id composite fallback should NOT be
    touched. This is the common case (the row exists, type matches,
    status is active, tenant has access).
    """
    fallback_called = {"hit": False}

    def _by_id(tenant_id, model_type, model_id):
        return {"llm_factory": "OpenAI", "llm_name": "gpt-4", "model_type": "chat", "api_key": "sk", "api_base": "https://api.openai.com/v1"}

    def _by_provider(tenant_id, model_type, model_name):
        fallback_called["hit"] = True
        return {}

    monkeypatch.setattr(tms, "get_model_config_by_id", _by_id)
    monkeypatch.setattr(tms, "get_model_config_from_provider_instance", _by_provider)

    from api.db.joint_services.tenant_model_service import LLMType

    out = tms.resolve_model_config("tenant-A", LLMType.CHAT, "abc123uuid")
    assert out["llm_name"] == "gpt-4"
    assert not fallback_called["hit"]


# ---------------------------------------------------------------------------
# resolve_model_config: bare id with primary id-based failure, then composite fallback
# ---------------------------------------------------------------------------


def test_resolve_model_config_bare_id_falls_back_to_composite(tms, monkeypatch):
    """When get_model_config_by_id raises LookupError AND the model_ref
    has no '@', resolve_model_config should build the composite form
    from the row directly and try the provider-instance lookup with
    that. This is the fix for issue #18398.
    """
    seen_composite = {}

    def _by_id(tenant_id, model_type, model_id):
        raise LookupError(f"TenantModel id={model_id} is disabled.")

    def _by_provider(tenant_id, model_type, model_name):
        seen_composite["composite"] = (model_name, model_type)
        return {"llm_factory": "OpenAI", "llm_name": "gpt-4", "model_type": "embedding", "api_key": "sk", "api_base": "https://api.openai.com/v1"}

    # _composite_from_bare_model_id reads the row. Patch the service
    # lookups to return a valid row.
    def _tenant_model_get_by_id(model_id):
        from types import SimpleNamespace
        from api.db.joint_services.tenant_model_service import calculate_model_type

        return True, SimpleNamespace(
            model_name="gpt-4",
            model_type=calculate_model_type("embedding"),
            status=1,  # ACTIVE
            provider_id="prov-1",
            instance_id="inst-1",
        )

    def _provider_get_by_id(provider_id):
        from types import SimpleNamespace

        return True, SimpleNamespace(tenant_id="tenant-A", provider_name="OpenAI")

    def _instance_get_by_id(instance_id):
        from types import SimpleNamespace

        return True, SimpleNamespace(instance_name="default")

    monkeypatch.setattr(tms, "get_model_config_by_id", _by_id)
    monkeypatch.setattr(tms, "get_model_config_from_provider_instance", _by_provider)
    monkeypatch.setattr(tms.TenantModelService, "get_by_id", staticmethod(_tenant_model_get_by_id))
    monkeypatch.setattr(tms.TenantModelProviderService, "get_by_id", staticmethod(_provider_get_by_id))
    monkeypatch.setattr(tms.TenantModelInstanceService, "get_by_id", staticmethod(_instance_get_by_id))

    from api.db.joint_services.tenant_model_service import LLMType

    out = tms.resolve_model_config("tenant-A", LLMType.EMBEDDING, "abc123uuid")
    assert out["llm_name"] == "gpt-4"
    composite_used = seen_composite["composite"][0]
    assert composite_used == "gpt-4@default@OpenAI", (
        f"the provider-instance lookup must be called with the composite 'name@instance@provider' form built from the row, not the bare id; got {composite_used!r}"
    )


# ---------------------------------------------------------------------------
# resolve_model_config: bare id with primary failure, but the row itself is
# unavailable -> original error path is preserved
# ---------------------------------------------------------------------------


def test_resolve_model_config_bare_id_row_unavailable_preserves_original_error(tms, monkeypatch):
    """If the row doesn't exist (genuine 'model not found' case), the
    fix must NOT mask the LookupError from the provider-instance
    fallback. The user gets a clear 'Provider not found for model
    <id>.' error -- the same error they got pre-fix for a truly
    missing model.
    """

    def _by_id(tenant_id, model_type, model_id):
        raise LookupError(f"TenantModel id={model_id} not found.")

    def _by_provider(tenant_id, model_type, model_name):
        raise LookupError(f"Provider  not found for model {model_name}.")

    def _tenant_model_get_by_id(model_id):
        return False, None  # row does not exist

    monkeypatch.setattr(tms, "get_model_config_by_id", _by_id)
    monkeypatch.setattr(tms, "get_model_config_from_provider_instance", _by_provider)
    monkeypatch.setattr(tms.TenantModelService, "get_by_id", staticmethod(_tenant_model_get_by_id))

    from api.db.joint_services.tenant_model_service import LLMType

    with pytest.raises(LookupError, match="Provider  not found for model abc123uuid"):
        tms.resolve_model_config("tenant-A", LLMType.EMBEDDING, "abc123uuid")


# ---------------------------------------------------------------------------
# _composite_from_bare_model_id: shape of the helper
# ---------------------------------------------------------------------------


def test_composite_from_bare_model_id_returns_name_instance_provider(tms, monkeypatch):
    """The helper returns the composite form 'name@instance@provider'."""
    from types import SimpleNamespace
    from api.db.joint_services.tenant_model_service import calculate_model_type

    def _tenant_model_get_by_id(model_id):
        return True, SimpleNamespace(
            model_name="gpt-4",
            model_type=calculate_model_type("embedding"),
            status=1,
            provider_id="prov-1",
            instance_id="inst-1",
        )

    def _provider_get_by_id(provider_id):
        return True, SimpleNamespace(tenant_id="tenant-A", provider_name="OpenAI")

    def _instance_get_by_id(instance_id):
        return True, SimpleNamespace(instance_name="my-instance")

    monkeypatch.setattr(tms.TenantModelService, "get_by_id", staticmethod(_tenant_model_get_by_id))
    monkeypatch.setattr(tms.TenantModelProviderService, "get_by_id", staticmethod(_provider_get_by_id))
    monkeypatch.setattr(tms.TenantModelInstanceService, "get_by_id", staticmethod(_instance_get_by_id))

    from api.db.joint_services.tenant_model_service import LLMType

    out = tms._composite_from_bare_model_id("tenant-A", LLMType.EMBEDDING, "abc123uuid")
    assert out == "gpt-4@my-instance@OpenAI"


def test_composite_from_bare_model_id_returns_none_when_row_missing(tms, monkeypatch):
    """If the tenant_model row does not exist, the helper returns None
    so the caller can fall through to the original failing path.
    """

    def _tenant_model_get_by_id(model_id):
        return False, None

    monkeypatch.setattr(tms.TenantModelService, "get_by_id", staticmethod(_tenant_model_get_by_id))

    from api.db.joint_services.tenant_model_service import LLMType

    assert tms._composite_from_bare_model_id("tenant-A", LLMType.EMBEDDING, "missing-id") is None


def test_composite_from_bare_model_id_returns_none_on_type_mismatch(tms, monkeypatch):
    """A model of a different type (e.g. CHAT looked up as EMBEDDING)
    must not be resolved. The helper returns None so the original
    LookupError path is preserved.
    """
    from types import SimpleNamespace
    from api.db.joint_services.tenant_model_service import calculate_model_type

    def _tenant_model_get_by_id(model_id):
        # model is a CHAT model only (no EMBEDDING bit)
        return True, SimpleNamespace(
            model_name="gpt-4",
            model_type=calculate_model_type("chat"),
            status=1,
            provider_id="prov-1",
            instance_id="inst-1",
        )

    monkeypatch.setattr(tms.TenantModelService, "get_by_id", staticmethod(_tenant_model_get_by_id))

    from api.db.joint_services.tenant_model_service import LLMType

    assert tms._composite_from_bare_model_id("tenant-A", LLMType.EMBEDDING, "chat-only-id") is None
