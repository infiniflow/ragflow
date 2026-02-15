from unittest.mock import patch

from core.config import AppConfig


def test_oauth_legacy_yaml_compatibility():
    """Test that legacy top-level 'oauth' is moved to 'third_party.oauth'."""

    legacy_yaml = {
        "oauth": {
            "oauth2": {"client_id": "old_oauth2_id", "client_secret": "old_oauth2_secret"},
            "oidc": {"client_id": "old_oidc_id", "client_secret": "old_oidc_secret"},
            "github": {"client_id": "old_github_id", "client_secret": "old_github_secret"},
            "feishu": {"app_id": "old_feishu_app", "app_secret": "old_feishu_secret"}
        }
    }

    with patch("core.config.app.load_yaml", return_value=legacy_yaml):
        cfg = AppConfig()

    assert "oauth" not in cfg

    oauth_cfg = cfg.third_party.oauth
    assert oauth_cfg.oauth2.client_id == "old_oauth2_id"
    assert oauth_cfg.oauth2.client_secret == "old_oauth2_secret"
    assert oauth_cfg.oidc.client_id == "old_oidc_id"
    assert oauth_cfg.oidc.client_secret == "old_oidc_secret"
    assert oauth_cfg.github.client_id == "old_github_id"
    assert oauth_cfg.github.client_secret == "old_github_secret"
    assert oauth_cfg.feishu.app_id == "old_feishu_app"
    assert oauth_cfg.feishu.app_secret == "old_feishu_secret"


def test_oauth_new_yaml_no_change():
    """Test that new 'third_party.oauth' configuration is preserved."""

    new_yaml = {
        "third_party": {
            "oauth": {
                "oauth2": {"client_id": "new_oauth2_id", "client_secret": "new_oauth2_secret"},
                "oidc": {"client_id": "new_oidc_id", "client_secret": "new_oidc_secret"},
                "github": {"client_id": "new_github_id", "client_secret": "new_github_secret"},
                "feishu": {"app_id": "new_feishu_app", "app_secret": "new_feishu_secret"}
            }
        }
    }

    with patch("core.config.app.load_yaml", return_value=new_yaml):
        cfg = AppConfig()

    oauth_cfg = cfg.third_party.oauth

    assert oauth_cfg.oauth2.client_id == "new_oauth2_id"
    assert oauth_cfg.oauth2.client_secret == "new_oauth2_secret"
    assert oauth_cfg.oidc.client_id == "new_oidc_id"
    assert oauth_cfg.oidc.client_secret == "new_oidc_secret"
    assert oauth_cfg.github.client_id == "new_github_id"
    assert oauth_cfg.github.client_secret == "new_github_secret"
    assert oauth_cfg.feishu.app_id == "new_feishu_app"
    assert oauth_cfg.feishu.app_secret == "new_feishu_secret"


def test_tcadp_old_yaml():
    """Test that legacy top-level tcadp config is migrated to third_party.tcadp"""
    return_value = {
        "tcadp-config": {
            "secret_id": "old_secret_id",
            "secret_key": "old_secret_key",
            "region": "https://old-api.example.com",
        }
    }
    with patch("core.config.app.load_yaml", return_value=return_value):
        config = AppConfig()

    # tcadp should now exist under third_party
    tcadp_cfg = config.third_party.tcadp
    assert tcadp_cfg.region == "https://old-api.example.com"
    assert tcadp_cfg.secret_id == "old_secret_id"
    assert tcadp_cfg.secret_key == "old_secret_key"


def test_tcadp_new_yaml():
    """Test that tcadp config under third_party is loaded correctly"""
    return_value = {
        "third_party": {
            "tcadp": {
                "secret_id": "new_secret_id",
                "secret_key": "new_secret_key",
                "region": "https://new-api.example.com",
            }
        }
    }
    with patch("core.config.app.load_yaml", return_value=return_value):
        config = AppConfig()

    tcadp_cfg = config.third_party.tcadp
    assert tcadp_cfg.region == "https://new-api.example.com"
    assert tcadp_cfg.secret_id == "new_secret_id"
    assert tcadp_cfg.secret_key == "new_secret_key"
