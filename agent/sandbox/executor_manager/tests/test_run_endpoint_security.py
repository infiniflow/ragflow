#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
"""Security tests for the executor manager /run endpoint.

Run from the executor_manager directory:

    cd agent/sandbox/executor_manager
    python -m pytest tests/

Covers: shared-secret authentication (401 without/with wrong token, success
via Authorization: Bearer and X-Sandbox-Token, backwards compatibility when
no token is configured), rate limiting on /run, and the docker network flag
used for sandbox runner containers.
"""

import asyncio
import base64

import pytest
from fastapi.testclient import TestClient
from models.enums import ResultStatus, SupportLanguage
from models.schemas import CodeExecutionResult

API_TOKEN_ENV = "SANDBOX_EXECUTOR_MANAGER_API_TOKEN"
TEST_TOKEN = "unit-test-shared-secret"


def _payload() -> dict:
    code = "def main():\n    return 42\n"
    return {"code_b64": base64.b64encode(code.encode("utf-8")).decode("utf-8"), "language": "python", "arguments": {}}


@pytest.fixture()
def client(monkeypatch):
    async def fake_execute_code(req):
        return CodeExecutionResult(status=ResultStatus.SUCCESS, stdout="42", stderr="", exit_code=0)

    monkeypatch.setattr("api.handlers.execute_code", fake_execute_code)

    import core.container as container_module

    # Normally populated by the lifespan hook (which provisions real docker
    # containers); handlers uses the same dict object by reference.
    container_module._CONTAINER_EXECUTION_SEMAPHORES[SupportLanguage.PYTHON] = asyncio.Semaphore(1)
    container_module._CONTAINER_EXECUTION_SEMAPHORES[SupportLanguage.NODEJS] = asyncio.Semaphore(1)

    import main

    # Instantiated without a context manager on purpose: the lifespan hook
    # provisions real docker containers, which these unit tests must avoid.
    return TestClient(main.app)


class TestRunEndpointAuth:
    def test_missing_token_returns_401_when_configured(self, client, monkeypatch):
        monkeypatch.setenv(API_TOKEN_ENV, TEST_TOKEN)
        response = client.post("/run", json=_payload())
        assert response.status_code == 401

    def test_wrong_bearer_token_returns_401(self, client, monkeypatch):
        monkeypatch.setenv(API_TOKEN_ENV, TEST_TOKEN)
        response = client.post("/run", json=_payload(), headers={"Authorization": "Bearer not-the-token"})
        assert response.status_code == 401

    def test_wrong_x_sandbox_token_returns_401(self, client, monkeypatch):
        monkeypatch.setenv(API_TOKEN_ENV, TEST_TOKEN)
        response = client.post("/run", json=_payload(), headers={"X-Sandbox-Token": "also-wrong"})
        assert response.status_code == 401

    def test_correct_bearer_token_is_accepted(self, client, monkeypatch):
        monkeypatch.setenv(API_TOKEN_ENV, TEST_TOKEN)
        response = client.post("/run", json=_payload(), headers={"Authorization": f"Bearer {TEST_TOKEN}"})
        assert response.status_code == 200
        assert response.json()["status"] == ResultStatus.SUCCESS

    def test_correct_x_sandbox_token_is_accepted(self, client, monkeypatch):
        monkeypatch.setenv(API_TOKEN_ENV, TEST_TOKEN)
        response = client.post("/run", json=_payload(), headers={"X-Sandbox-Token": TEST_TOKEN})
        assert response.status_code == 200

    def test_unset_token_keeps_endpoint_open_for_backwards_compatibility(self, client, monkeypatch):
        monkeypatch.delenv(API_TOKEN_ENV, raising=False)
        response = client.post("/run", json=_payload())
        assert response.status_code == 200

    def test_healthz_does_not_require_token(self, client, monkeypatch):
        monkeypatch.setenv(API_TOKEN_ENV, TEST_TOKEN)
        assert client.get("/healthz").status_code == 200
        assert client.get("/").status_code == 200


class TestRunEndpointRateLimit:
    def test_requests_beyond_limit_get_429(self, client, monkeypatch):
        # conftest pins SANDBOX_RUN_RATE_LIMIT to 3/minute.
        monkeypatch.setenv(API_TOKEN_ENV, TEST_TOKEN)
        headers = {"Authorization": f"Bearer {TEST_TOKEN}"}
        for _ in range(3):
            assert client.post("/run", json=_payload(), headers=headers).status_code == 200
        response = client.post("/run", json=_payload(), headers=headers)
        assert response.status_code == 429
        body = response.json()
        assert body["exit_code"] == -429

    def test_unauthenticated_requests_do_not_consume_quota(self, client, monkeypatch):
        monkeypatch.setenv(API_TOKEN_ENV, TEST_TOKEN)
        # Rejected before the endpoint (and its limiter) runs.
        for _ in range(5):
            assert client.post("/run", json=_payload()).status_code == 401
        response = client.post("/run", json=_payload(), headers={"Authorization": f"Bearer {TEST_TOKEN}"})
        assert response.status_code == 200


class TestContainerNetworkIsolation:
    @staticmethod
    def _captured_docker_run_argv(monkeypatch) -> list[list[str]]:
        calls: list[list[str]] = []

        async def fake_run_command(*args, timeout=5):
            calls.append(list(args))
            return 0, "true", ""

        import core.container as container_module

        monkeypatch.setattr(container_module, "async_run_command", fake_run_command)
        assert asyncio.run(container_module.create_container("sandbox_python_0", SupportLanguage.PYTHON)) is True
        return next(argv for argv in calls if argv[:3] == ["docker", "run", "-d"])

    def test_container_defaults_to_network_none(self, monkeypatch):
        monkeypatch.delenv("SANDBOX_CONTAINER_NETWORK", raising=False)
        argv = self._captured_docker_run_argv(monkeypatch)
        assert argv[argv.index("--network") + 1] == "none"

    def test_blank_network_env_falls_back_to_none(self, monkeypatch):
        monkeypatch.setenv("SANDBOX_CONTAINER_NETWORK", "   ")
        argv = self._captured_docker_run_argv(monkeypatch)
        assert argv[argv.index("--network") + 1] == "none"

    def test_network_env_override_is_passed_through(self, monkeypatch):
        monkeypatch.setenv("SANDBOX_CONTAINER_NETWORK", "bridge")
        argv = self._captured_docker_run_argv(monkeypatch)
        assert argv[argv.index("--network") + 1] == "bridge"

    def test_other_hardening_flags_still_present(self, monkeypatch):
        monkeypatch.delenv("SANDBOX_CONTAINER_NETWORK", raising=False)
        argv = self._captured_docker_run_argv(monkeypatch)
        assert "--runtime=runsc" in argv
        assert "--read-only" in argv
        assert argv[argv.index("--user") + 1] == "nobody"
