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

from unittest.mock import patch

from core.config.app import AppConfig
from core.config.components.base.cache import RedisConfig, CacheConfig
from core.config.types import CacheType


# ------------------------
# Default values
# ------------------------

def test_cache_defaults(monkeypatch):
    """Test that default cache type is Redis when no env var is set."""
    monkeypatch.delenv("CACHE_TYPE", raising=False)
    with patch("core.config.app.load_yaml", return_value={}):
        cfg = AppConfig()
    assert cfg.cache.active == CacheType.REDIS
    assert cfg.cache.current.host == "localhost"
    assert cfg.cache.current.port == 6379
    assert cfg.cache.current.username is None
    assert cfg.cache.current.password is None
    assert cfg.cache.current.db == 1

    params = cfg.cache.current.connection_params
    assert "username" not in params
    assert "password" not in params


def test_cache_current_valid_and_is_redis():
    """Test CacheConfig.current property and is_redis helper."""
    cache = CacheConfig()
    assert cache.current == cache.redis
    assert cache.is_redis


# ------------------------
# YAML override
# ------------------------

def test_cache_yaml_override():
    """Test that YAML values override default Redis configuration."""
    yaml_cfg = {
        "cache": {"redis": {"host": "1.2.3.4", "port": 6380, "db": 2, "password": "secret"}}
    }
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()

    redis_cfg = cfg.cache.redis
    assert redis_cfg.host == "1.2.3.4"
    assert redis_cfg.port == 6380
    assert redis_cfg.db == 2
    assert redis_cfg.password == "secret"
    # DSN reflects password
    assert f":{redis_cfg.password}@" in str(redis_cfg.dsn)
    # connection_params includes password, excludes username if not set
    conn = redis_cfg.connection_params
    assert conn["password"] == "secret"
    assert "username" not in conn


def test_yaml_priority_over_env(monkeypatch):
    """YAML values should take precedence over environment variables."""
    monkeypatch.setenv("CACHE_TYPE", "memcached")  # Env would normally override
    yaml_cfg = {"cache": {"active": "redis", "redis": {"host": "10.0.0.1"}}}
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()
    # YAML wins for cache type
    assert cfg.cache.active == CacheType.REDIS
    assert cfg.cache.redis.host == "10.0.0.1"


# ------------------------
# Redis host parsing
# ------------------------

def test_redis_handle_host_colon(monkeypatch):
    """Test RedisConfig parses 'host:port' format correctly."""
    cfg = RedisConfig(host="1.2.3.4:6380")
    assert cfg.host == "1.2.3.4"
    assert cfg.port == 6380


def test_redis_handle_host_only():
    """Test RedisConfig keeps host and port if given separately."""
    cfg = RedisConfig(host="1.2.3.4", port=6379)
    assert cfg.host == "1.2.3.4"
    assert cfg.port == 6379


# ------------------------
# Redis helper properties
# ------------------------

def test_redis_endpoint_property():
    """Test RedisConfig.endpoint returns 'host:port' string."""
    cfg = RedisConfig(host="127.0.0.1", port=6379)
    assert cfg.endpoint == "127.0.0.1:6379"


def test_redis_build_dsn_with_password():
    """Test Redis DSN includes password if provided."""
    cfg = RedisConfig(host="1.2.3.4", port=6379, db=0, password="secret")
    assert str(cfg.dsn) == "redis://:secret@1.2.3.4:6379/0"


def test_redis_build_dsn_without_password():
    """Test Redis DSN omits password if not provided."""
    cfg = RedisConfig(host="1.2.3.4", port=6379, db=0)
    assert str(cfg.dsn) == "redis://1.2.3.4:6379/0"


# ------------------------
# Redis connection parameters
# ------------------------

def test_redis_connection_params_full():
    """Test RedisConfig.connection_params includes all fields when username/password are set."""
    cfg = RedisConfig(host="1.2.3.4", port=6379, db=1, username="u", password="p")
    expected = {
        "host": "1.2.3.4",
        "port": 6379,
        "db": 1,
        "username": "u",
        "password": "p",
        "decode_responses": True
    }
    assert cfg.connection_params == expected


def test_redis_connection_params_minimal():
    """Test RedisConfig.connection_params with minimal config (no username/password)."""
    cfg = RedisConfig(host="1.2.3.4", port=6379, db=1)
    expected = {"host": "1.2.3.4", "port": 6379, "db": 1, "decode_responses": True}
    assert cfg.connection_params == expected


# ------------------------
# Optional: test connection_params reflects YAML overrides
# ------------------------

def test_redis_connection_params_yaml_override():
    """Test that YAML overrides for Redis username/password appear in connection_params."""
    yaml_cfg = {
        "cache": {"redis": {"username": "yamluser", "password": "yamlpass"}}
    }
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()
    redis_cfg = cfg.cache.redis
    conn = redis_cfg.connection_params
    assert conn["username"] == "yamluser"
    assert conn["password"] == "yamlpass"