import logging
from unittest.mock import patch

from core.config.app import AppConfig


def test_ragflow_defaults(monkeypatch):
    """Default RAGFlowConfig via AppConfig"""
    # Ensure env vars are cleared
    monkeypatch.delenv("REGISTER_ENABLED", raising=False)
    monkeypatch.delenv("RAGFLOW_SECRET_KEY", raising=False)

    with patch("core.config.app.load_yaml", return_value={}):
        cfg = AppConfig()

    # Web settings
    assert cfg.ragflow.host == "0.0.0.0"
    assert cfg.ragflow.http_port == 9380
    assert cfg.ragflow.max_content_length == 1024 * 1024 * 1024
    assert cfg.ragflow.response_timeout == 600
    assert cfg.ragflow.body_timeout == 600
    assert cfg.ragflow.strong_test_count == 8

    # Superuser defaults
    assert cfg.ragflow.default_superuser_nickname == "admin"
    assert cfg.ragflow.default_superuser_email == "admin@ragflow.io"
    assert cfg.ragflow.default_superuser_password == "admin"

    # Feature flags
    assert cfg.ragflow.register_enabled is True
    assert cfg.ragflow.crypto_enabled is False
    assert cfg.ragflow.use_docling is False

    # Secret key auto-generated
    assert cfg.ragflow.secret_key is not None
    assert len(cfg.ragflow.secret_key) == 64


def test_ragflow_env_override(monkeypatch):
    """RAGFlowConfig via AppConfig with environment variable overrides"""
    monkeypatch.setenv("REGISTER_ENABLED", "0")
    monkeypatch.setenv(
        "RAGFLOW_SECRET_KEY",
        "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    )

    with patch("core.config.app.load_yaml", return_value={}):
        cfg = AppConfig()

    assert cfg.ragflow.register_enabled is False
    assert cfg.ragflow.secret_key == "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"


def test_ragflow_str_to_bool(monkeypatch):
    """Test string-to-boolean conversion via AppConfig"""
    with patch("core.config.app.load_yaml", return_value={"ragflow": {"register_enabled": "true"}}):
        cfg = AppConfig()
    assert cfg.ragflow.register_enabled is True

    with patch("core.config.app.load_yaml", return_value={"ragflow": {"register_enabled": "False"}}):
        cfg2 = AppConfig()
    assert cfg2.ragflow.register_enabled is False


def test_ragflow_secret_key_autogenerate(monkeypatch):
    """Secret key auto-generation when missing or too short via AppConfig"""
    monkeypatch.delenv("RAGFLOW_SECRET_KEY", raising=False)

    with patch("core.config.app.load_yaml", return_value={"ragflow": {"secret_key": None}}):
        with patch.object(logging, "warning") as mock_warn:
            cfg = AppConfig()
            assert cfg.ragflow.secret_key is not None
            assert len(cfg.ragflow.secret_key) == 64
            mock_warn.assert_called_once()

    with patch("core.config.app.load_yaml", return_value={"ragflow": {"secret_key": "short"}}):
        with patch.object(logging, "warning") as mock_warn2:
            cfg2 = AppConfig()
            assert cfg2.ragflow.secret_key is not None
            assert len(cfg2.ragflow.secret_key) == 64
            mock_warn2.assert_called_once()


def test_ragflow_custom_values(monkeypatch):
    """Direct custom values via AppConfig"""
    yaml_override = {
        "ragflow": {
            "host": "127.0.0.1",
            "http_port": 8080,
            "max_content_length": 10,
            "strong_test_count": 100,
            "default_superuser_email": "test@test.com",
            "register_enabled": False,
            "crypto_enabled": True,
            "use_docling": True,
        }
    }

    with patch("core.config.app.load_yaml", return_value=yaml_override):
        cfg = AppConfig()

    assert cfg.ragflow.host == "127.0.0.1"
    assert cfg.ragflow.http_port == 8080
    assert cfg.ragflow.max_content_length == 10
    assert cfg.ragflow.strong_test_count == 100
    assert cfg.ragflow.default_superuser_email == "test@test.com"
    assert cfg.ragflow.register_enabled is False
    assert cfg.ragflow.crypto_enabled is True
    assert cfg.ragflow.use_docling is True
