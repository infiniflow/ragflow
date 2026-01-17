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
from core.types.cache import CacheType


def test_cache_defaults(monkeypatch):
    monkeypatch.delenv("CACHE_TYPE", raising=False)
    assert AppConfig().cache.active == CacheType.REDIS


def test_cache_current_valid_and_is_redis():
    cache = CacheConfig()
    assert cache.current == cache.redis
    assert cache.is_redis


def test_cache_yaml_override():
    return_value = {
        "cache": {"redis": {"host": "1.2.3.4", "port": 6380, "db": 2, "password": "secret"}}
    }
    with patch("core.config.app.load_yaml", return_value=return_value):
        cfg = AppConfig()
    r = cfg.cache.redis
    assert r.host == "1.2.3.4"
    assert r.port == 6380
    assert r.db == 2
    assert r.password == "secret"


def test_redis_handle_host_colon(monkeypatch):
    cfg = RedisConfig(host="1.2.3.4:6380")
    assert cfg.host == "1.2.3.4"
    assert cfg.port == 6380


def test_redis_handle_host_only():
    cfg = RedisConfig(host="1.2.3.4", port=6379)
    assert cfg.host == "1.2.3.4"
    assert cfg.port == 6379


def test_redis_endpoint_property():
    cfg = RedisConfig(host="127.0.0.1", port=6379)
    assert cfg.endpoint == "127.0.0.1:6379"


def test_redis_build_dsn_with_password():
    cfg = RedisConfig(host="1.2.3.4", port=6379, db=0, password="secret")
    assert str(cfg.dsn) == "redis://:secret@1.2.3.4:6379/0"


def test_redis_build_dsn_without_password():
    cfg = RedisConfig(host="1.2.3.4", port=6379, db=0)
    assert str(cfg.dsn) == "redis://1.2.3.4:6379/0"


def test_redis_connection_params_full():
    cfg = RedisConfig(host="1.2.3.4", port=6379, db=1, username="u", password="p")
    params = cfg.connection_params
    assert params == {
        "host": "1.2.3.4", "port": 6379, "db": 1, "username": "u", "password": "p", "decode_responses": True
    }


def test_redis_connection_params_minimal():
    cfg = RedisConfig(host="1.2.3.4", port=6379, db=1)
    params = cfg.connection_params
    assert params == {"host": "1.2.3.4", "port": 6379, "db": 1, "decode_responses": True}
