def test_env_keys_and_defaults_present():
    from common.constants import MISTRAL_OCR_ENV_KEYS, MISTRAL_OCR_DEFAULT_CONFIG

    assert "MISTRAL_OCR_API_KEY" in MISTRAL_OCR_ENV_KEYS
    assert "MISTRAL_OCR_BASE_URL" in MISTRAL_OCR_ENV_KEYS
    assert MISTRAL_OCR_DEFAULT_CONFIG["MISTRAL_OCR_BASE_URL"] == "https://api.mistral.ai/v1"


def test_collect_env_config_returns_none_without_env(monkeypatch):
    from common.constants import MISTRAL_OCR_ENV_KEYS, MISTRAL_OCR_DEFAULT_CONFIG
    from api.db.joint_services.tenant_model_service import _collect_env_config

    for k in MISTRAL_OCR_ENV_KEYS:
        monkeypatch.delenv(k, raising=False)
    assert _collect_env_config(MISTRAL_OCR_ENV_KEYS, MISTRAL_OCR_DEFAULT_CONFIG) is None


def test_collect_env_config_populated_when_key_set(monkeypatch):
    from common.constants import MISTRAL_OCR_ENV_KEYS, MISTRAL_OCR_DEFAULT_CONFIG
    from api.db.joint_services.tenant_model_service import _collect_env_config

    monkeypatch.setenv("MISTRAL_OCR_API_KEY", "sk-live")
    cfg = _collect_env_config(MISTRAL_OCR_ENV_KEYS, MISTRAL_OCR_DEFAULT_CONFIG)
    assert cfg["MISTRAL_OCR_API_KEY"] == "sk-live"
