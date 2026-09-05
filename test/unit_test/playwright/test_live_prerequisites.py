from unittest.mock import MagicMock

import pytest

from test.playwright.conftest import _is_malformed_tenant_model_value, _normalize_tenant_model_value, browser, flow_context
from test.playwright.helpers._auth_helpers import ensure_authed


@pytest.mark.parametrize("value", [
    "qwen2.5:7b-instruct", "qwen2.5:7b-instruct@Ollama",
    "qwen2.5:7b-instruct@QA@Ollama",
    "text-embedding-nomic-embed-text-v1.5@q8_0@lmstudio@LM-Studio",
])
def test_modern_model_identifiers_are_preserved(value):
    assert not _is_malformed_tenant_model_value(value)
    assert _normalize_tenant_model_value(value) == value


@pytest.mark.parametrize("value", ["@Ollama", "model@", "model@@Ollama", "@QA@Ollama", "model@ @Ollama"])
def test_missing_identifier_parts_are_rejected(value):
    assert _is_malformed_tenant_model_value(value)
    assert _normalize_tenant_model_value(value) == ""


def test_suffix_normalization_preserves_instance_and_provider():
    assert _is_malformed_tenant_model_value("model@QA@Ollama#extra")
    assert _normalize_tenant_model_value("model@QA@Ollama#extra") == "model@QA@Ollama"


@pytest.mark.parametrize("url", ["about:blank", "data:text/html,blank"])
def test_auth_does_not_access_storage_on_opaque_origins(url):
    page = MagicMock()
    page.url = url
    page.goto.side_effect = lambda *args, **kwargs: setattr(page, "url", "http://qa/home")
    ensure_authed(page, "http://qa/login", MagicMock(), MagicMock(), ("qa@example.org", "qa-only"))
    page.wait_for_function.assert_not_called()
    page.goto.assert_called_once()


def test_browser_reuses_supplied_playwright_runtime(monkeypatch):
    monkeypatch.setenv("PW_BROWSER", "chromium")
    runtime = MagicMock()
    fixture = browser.__wrapped__(runtime)
    assert next(fixture) is runtime.chromium.launch.return_value
    fixture.close()
    runtime.chromium.launch.return_value.close.assert_called_once()


def test_context_configuration_errors_are_not_silently_ignored():
    instance = MagicMock()
    request = MagicMock()
    request.getfixturevalue.side_effect = RuntimeError("broken context profile")
    with pytest.raises(RuntimeError, match="broken context profile"):
        next(flow_context.__wrapped__(instance, request))
    instance.new_context.assert_not_called()
