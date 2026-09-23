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
import os
import subprocess
import sys
from pathlib import Path
from types import SimpleNamespace

import pytest

pytestmark = pytest.mark.p2


CONFIG_ENV_KEYS = {
    "API_PROXY_SCHEME",
    "GO_HTTP_PORT",
    "HOST_ADDRESS",
    "SVR_HTTP_PORT",
}


def _host_address(**env_overrides: str) -> str:
    env = os.environ.copy()
    env.update(
        {
            "PYTHONPATH": ".",
            "ZHIPU_AI_API_KEY": "dummy",
            "SILICONFLOW_API_KEY": "dummy",
        }
    )
    for key in CONFIG_ENV_KEYS:
        env.pop(key, None)
    env.update(env_overrides)
    return subprocess.check_output(
        [sys.executable, "-c", "from test.testcases.configs import HOST_ADDRESS; print(HOST_ADDRESS)"],
        env=env,
        text=True,
    ).strip()


def test_explicit_host_address_wins():
    assert _host_address(HOST_ADDRESS="http://example.test:1234", API_PROXY_SCHEME="go") == "http://example.test:1234"


def test_default_host_address_only_changes_for_go_proxy_scheme():
    assert _host_address(API_PROXY_SCHEME="python") == "http://127.0.0.1:9380"
    assert _host_address(API_PROXY_SCHEME="python", SVR_HTTP_PORT="19380") == "http://127.0.0.1:9380"
    assert _host_address(API_PROXY_SCHEME="go") == "http://127.0.0.1:9384"
    assert _host_address(API_PROXY_SCHEME="go", GO_HTTP_PORT="19384") == "http://127.0.0.1:19384"


def test_default_host_address_uses_docker_go_port(monkeypatch, tmp_path):
    from test.testcases import configs

    docker_env = tmp_path / "docker.env"
    docker_env.write_text("GO_HTTP_PORT=19384\n")
    monkeypatch.setattr(configs, "_DOCKER_ENV", Path(docker_env))
    monkeypatch.setattr(configs, "API_PROXY_SCHEME", "go")
    for key in CONFIG_ENV_KEYS:
        monkeypatch.delenv(key, raising=False)

    assert configs._default_host_address() == "http://127.0.0.1:19384"


def test_non_json_setup_response_is_only_tolerated_in_go_mode(monkeypatch):
    from test.testcases import conftest

    response = SimpleNamespace(
        json=lambda: (_ for _ in ()).throw(ValueError("not json")),
        text="Internal Server Error",
        reason="Internal Server Error",
        status_code=500,
    )

    monkeypatch.setattr(conftest, "API_PROXY_SCHEME", "go")
    assert conftest._response_json_or_warning(response, "setup") == {
        "code": 500,
        "message": "setup returned non-JSON response: Internal Server Error",
    }

    monkeypatch.setattr(conftest, "API_PROXY_SCHEME", "python")
    with pytest.raises(ValueError, match="not json"):
        conftest._response_json_or_warning(response, "setup")


def test_go_model_setup_adds_missing_models_without_replacing_existing_instances(monkeypatch):
    from test.testcases import conftest

    calls = []
    model_lists = {
        ("ZHIPU-AI", "CI"): [{"name": "glm-4.5-air"}],
        ("SILICONFLOW", "CI"): [{"name": "BAAI/bge-reranker-v2-m3"}],
    }

    class Response:
        status_code = 200
        reason = "OK"
        text = ""

        def __init__(self, payload):
            self.payload = payload

        def json(self):
            return self.payload

    def request(method, url, **kwargs):
        calls.append((method, url, kwargs))
        if method == "GET":
            provider, instance = url.rsplit("/providers/", 1)[1].split("/instances/")
            instance = instance.removesuffix("/models")
            return Response({"code": 0, "data": model_lists[(provider, instance)]})
        return Response({"code": 0, "message": "success"})

    monkeypatch.setattr(conftest, "HOST_ADDRESS", "http://go.test")
    monkeypatch.setattr(conftest.requests, "get", lambda url, **kwargs: request("GET", url, **kwargs))
    monkeypatch.setattr(conftest.requests, "post", lambda url, **kwargs: request("POST", url, **kwargs))
    monkeypatch.setattr(conftest, "ZHIPU_AI_API_KEY", "zhipu-secret")
    monkeypatch.setattr(conftest, "SILICONFLOW_API_KEY", "silicon-secret")

    conftest._ensure_go_model_setup("auth")

    added = [call for call in calls if call[0] == "POST" and "/models" in call[1]]
    assert {call[2]["json"]["model_name"] for call in added} == {
        "glm-4-flash",
        "glm-4",
        "embedding-3",
        "BAAI/bge-m3",
    }
    assert all(call[2]["json"]["model_type"] for call in added)
    assert not any(call[0] == "POST" and call[1].endswith("/instances") for call in calls)
