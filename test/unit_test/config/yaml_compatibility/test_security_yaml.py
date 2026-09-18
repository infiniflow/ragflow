from unittest.mock import patch

from core.config import AppConfig


def test_security_old_yaml_compatibility():
    """Test that legacy top-level security fields are normalized correctly."""
    return_value = {
        "password": {
            "encrypt_enabled": True,
            "encrypt_module": "module#func",
            "private_key": "old-private-key",
        },
        "authentication": {
            "client": {
                "switch": True,
                "http_app_key": "appkey",
                "http_secret_key": "secretkey",
            },
            "site": {
                "switch": True
            },
        },
        "permission": {
            "switch": True,
            "component": True,
            "dataset": False,
        }
    }

    with patch("core.config.app.load_yaml", return_value=return_value):
        config = AppConfig()

    security_cfg = config.security

    # password
    pwd_cfg = security_cfg.password
    assert pwd_cfg.encrypt_enabled is True
    assert pwd_cfg.encrypt_module == "module#func"
    assert pwd_cfg.private_key == "old-private-key"

    # authentication
    auth_cfg = security_cfg.authentication
    assert auth_cfg.client.switch is True
    assert auth_cfg.client.http_app_key == "appkey"
    assert auth_cfg.client.http_secret_key == "secretkey"
    assert auth_cfg.site.switch is True

    # permission
    perm_cfg = security_cfg.permission
    assert perm_cfg.switch is True
    assert perm_cfg.component is True
    assert perm_cfg.dataset is False


def test_security_new_yaml():
    """Test that new-format security config is loaded correctly."""
    return_value = {
        "security": {
            "password": {
                "encrypt_enabled": False,
                "encrypt_module": "module#func",
                "private_key": "new-private-key",
            },
            "authentication": {
                "client": {
                    "switch": False,
                    "http_app_key": "appkey2",
                    "http_secret_key": "secretkey2",
                },
                "site": {"switch": False}
            },
            "permission": {
                "switch": False,
                "component": False,
                "dataset": True,
            }
        }
    }

    with patch("core.config.app.load_yaml", return_value=return_value):
        config = AppConfig()

    security_cfg = config.security

    # password
    pwd_cfg = security_cfg.password
    assert pwd_cfg.encrypt_enabled is False
    assert pwd_cfg.private_key == "new-private-key"

    # authentication
    auth_cfg = security_cfg.authentication
    assert auth_cfg.client.switch is False
    assert auth_cfg.client.http_app_key == "appkey2"
    assert auth_cfg.client.http_secret_key == "secretkey2"
    assert auth_cfg.site.switch is False

    # permission
    perm_cfg = security_cfg.permission
    assert perm_cfg.switch is False
    assert perm_cfg.component is False
    assert perm_cfg.dataset is True