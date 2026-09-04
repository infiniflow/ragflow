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

Covers: fail-closed shared-secret authentication (503 when no token is
configured unless the explicit unauthenticated opt-in flag is set, 401 with a
wrong token, success via Authorization: Bearer and X-Sandbox-Token), the
pre-auth rate limit applied to all /run traffic, the authenticated execution
quota, the standalone Compose environment contract, and the docker network
flag used for sandbox runner containers.
"""

import asyncio
import base64
from pathlib import Path

import pytest
from fastapi.testclient import TestClient
from models.enums import ResultStatus, SupportLanguage
from models.schemas import CodeExecutionResult

API_TOKEN_ENV = "SANDBOX_EXECUTOR_MANAGER_API_TOKEN"
ALLOW_UNAUTH_ENV = "SANDBOX_EXECUTOR_MANAGER_ALLOW_UNAUTHENTICATED"
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

    def test_unset_token_fails_closed_with_503(self, client, monkeypatch):
        monkeypatch.delenv(API_TOKEN_ENV, raising=False)
        monkeypatch.delenv(ALLOW_UNAUTH_ENV, raising=False)
        response = client.post("/run", json=_payload())
        assert response.status_code == 503
        assert "SANDBOX_EXECUTOR_MANAGER_API_TOKEN" in response.json()["detail"]

    def test_blank_token_fails_closed_with_503(self, client, monkeypatch):
        monkeypatch.setenv(API_TOKEN_ENV, "   ")
        monkeypatch.delenv(ALLOW_UNAUTH_ENV, raising=False)
        assert client.post("/run", json=_payload()).status_code == 503

    def test_explicit_opt_in_restores_open_endpoint(self, client, monkeypatch):
        monkeypatch.delenv(API_TOKEN_ENV, raising=False)
        monkeypatch.setenv(ALLOW_UNAUTH_ENV, "true")
        response = client.post("/run", json=_payload())
        assert response.status_code == 200

    @pytest.mark.parametrize("flag", ["1", "TRUE", "Yes", "on "])
    def test_opt_in_accepts_truthy_flag_spellings(self, client, monkeypatch, flag):
        monkeypatch.delenv(API_TOKEN_ENV, raising=False)
        monkeypatch.setenv(ALLOW_UNAUTH_ENV, flag)
        assert client.post("/run", json=_payload()).status_code == 200

    @pytest.mark.parametrize("flag", ["false", "0", "no", "unexpected"])
    def test_opt_in_rejects_non_truthy_flag_spellings(self, client, monkeypatch, flag):
        monkeypatch.delenv(API_TOKEN_ENV, raising=False)
        monkeypatch.setenv(ALLOW_UNAUTH_ENV, flag)
        assert client.post("/run", json=_payload()).status_code == 503

    def test_opt_in_is_ignored_when_a_token_is_configured(self, client, monkeypatch):
        monkeypatch.setenv(API_TOKEN_ENV, TEST_TOKEN)
        monkeypatch.setenv(ALLOW_UNAUTH_ENV, "true")
        # The flag never weakens a configured token: bad credentials still 401.
        assert client.post("/run", json=_payload(), headers={"Authorization": "Bearer wrong"}).status_code == 401

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

    def test_repeated_unauthenticated_requests_eventually_get_429(self, client, monkeypatch):
        # The pre-auth limiter throttles ALL /run traffic before authentication,
        # so an invalid-token flood cannot generate unbounded authentication
        # work. The limiter is shrunk to 3/minute for this test (conftest
        # resets it afterwards).
        import services.preauth as preauth_module

        monkeypatch.setenv(API_TOKEN_ENV, TEST_TOKEN)
        monkeypatch.setattr(
            preauth_module,
            "preauth_limiter",
            preauth_module.PreAuthRateLimiter(preauth_module._parse_rate_limit("3/minute")),
        )
        for _ in range(3):
            assert client.post("/run", json=_payload()).status_code == 401
        response = client.post("/run", json=_payload())
        assert response.status_code == 429
        body = response.json()
        assert body["exit_code"] == -429

    def test_single_auth_failures_still_return_401_under_generous_preauth(self, client, monkeypatch):
        monkeypatch.setenv(API_TOKEN_ENV, TEST_TOKEN)
        assert client.post("/run", json=_payload()).status_code == 401
        assert client.post("/run", json=_payload(), headers={"X-Sandbox-Token": "wrong"}).status_code == 401


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


class TestStandaloneComposeEnvironment:
    """Regression: the standalone Compose file must actually deliver the
    documented environment contract (SANDBOX_RUN_RATE_LIMIT, the pre-auth
    limit, and the explicit unauthenticated opt-in flag) into the container."""

    COMPOSE_PATH = Path(__file__).resolve().parents[2] / "docker-compose.yml"
    ENV_EXAMPLE_PATH = Path(__file__).resolve().parents[2] / ".env.example"

    def test_documented_rate_limit_reaches_the_container(self):
        compose = self.COMPOSE_PATH.read_text(encoding="utf-8")
        assert "- SANDBOX_RUN_RATE_LIMIT=${SANDBOX_RUN_RATE_LIMIT:-120/minute}" in compose
        assert "- SANDBOX_RUN_PREAUTH_RATE_LIMIT=${SANDBOX_RUN_PREAUTH_RATE_LIMIT:-30/minute}" in compose

    def test_unauthenticated_opt_in_flag_is_forwarded(self):
        compose = self.COMPOSE_PATH.read_text(encoding="utf-8")
        assert "- SANDBOX_EXECUTOR_MANAGER_ALLOW_UNAUTHENTICATED=${SANDBOX_EXECUTOR_MANAGER_ALLOW_UNAUTHENTICATED:-false}" in compose

    def test_env_example_matches_compose_defaults(self):
        example = self.ENV_EXAMPLE_PATH.read_text(encoding="utf-8")
        for name in (
            "SANDBOX_RUN_RATE_LIMIT",
            "SANDBOX_RUN_PREAUTH_RATE_LIMIT",
            "SANDBOX_EXECUTOR_MANAGER_ALLOW_UNAUTHENTICATED",
            "SANDBOX_EXECUTOR_MANAGER_API_TOKEN",
        ):
            assert name in example, f"{name} missing from .env.example"
