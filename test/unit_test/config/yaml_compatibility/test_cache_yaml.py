from unittest.mock import patch

from core.config import AppConfig


def test_cache_old_yaml():
    return_value = {
        "redis": {"host": "127.0.0.1", "port": 6379, "db": 1, "password": "oldpass"}
    }
    with patch("core.config.app.load_yaml", return_value=return_value):
        config = AppConfig()
    redis_cfg = config.cache.redis
    assert redis_cfg.host == "127.0.0.1"
    assert redis_cfg.db == 1

def test_cache_new_yaml():
    return_value = {
        "cache": {
            "redis": {"host": "127.0.0.2", "port": 6380, "db": 2, "password": "newpass"}
        }
    }
    with patch("core.config.app.load_yaml", return_value=return_value):
        config = AppConfig()
    redis_cfg = config.cache.redis
    assert redis_cfg.host == "127.0.0.2"
    assert redis_cfg.db == 2
