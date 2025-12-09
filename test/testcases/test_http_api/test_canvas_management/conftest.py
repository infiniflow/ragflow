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
"""Shared fixtures and utilities for canvas permissions HTTP API tests."""

from __future__ import annotations

import importlib.util
from pathlib import Path
from typing import Any, List

import pytest

from common import delete_canvas
from libs.auth import RAGFlowWebApiAuth


# ---------------------------------------------------------------------------
# Import cleanup helpers from root and team-management conftests
# ---------------------------------------------------------------------------

_root_conftest_path = Path(__file__).parent.parent.parent / "conftest.py"
_root_spec = importlib.util.spec_from_file_location(
    "root_test_conftest", _root_conftest_path
)
_root_conftest_module = importlib.util.module_from_spec(_root_spec)
assert _root_spec.loader is not None
_root_spec.loader.exec_module(_root_conftest_module)
delete_user_from_db = _root_conftest_module.delete_user_from_db

_team_conftest_path = (
    Path(__file__).parent.parent / "test_team_management" / "conftest.py"
)
_team_spec = importlib.util.spec_from_file_location(
    "team_test_conftest", _team_conftest_path
)
_team_conftest_module = importlib.util.module_from_spec(_team_spec)
assert _team_spec.loader is not None
_team_spec.loader.exec_module(_team_conftest_module)
delete_team_from_db = _team_conftest_module.delete_team_from_db


# ---------------------------------------------------------------------------
# Cleanup fixtures for teams, users, and canvases
# ---------------------------------------------------------------------------


@pytest.fixture(scope="function")
def clear_teams(request: pytest.FixtureRequest) -> List[str]:
    """Fixture to clean up teams (tenants) created during canvas tests."""
    created_team_ids: List[str] = []

    def cleanup() -> None:
        for tenant_id in created_team_ids:
            try:
                delete_team_from_db(tenant_id)
            except Exception as exc:  # pragma: no cover - best-effort cleanup
                print(f"[clear_teams] Failed to delete test tenant {tenant_id}: {exc}")

    request.addfinalizer(cleanup)
    return created_team_ids


@pytest.fixture(scope="function")
def clear_team_users(request: pytest.FixtureRequest) -> List[str]:
    """Fixture to clean up users created in canvas-permissions tests.

    Tests/fixtures should append created user *emails* to the returned list.
    After each test, this fixture hard-deletes those users via the shared
    `delete_user_from_db` helper used by the user-management tests.
    """
    created_user_emails: List[str] = []

    def cleanup() -> None:
        for email in created_user_emails:
            try:
                delete_user_from_db(email)
            except Exception as exc:  # pragma: no cover - best-effort cleanup
                print(f"[clear_team_users] Failed to delete user {email}: {exc}")

    request.addfinalizer(cleanup)
    return created_user_emails


@pytest.fixture(scope="function")
def clear_canvases(
    request: pytest.FixtureRequest,
    web_api_auth: RAGFlowWebApiAuth,
) -> List[str]:
    """Fixture to clean up canvases created during canvas-permissions tests."""
    created_canvas_ids: List[str] = []

    def cleanup() -> None:
        if not created_canvas_ids:
            return
        try:
            delete_payload: dict[str, Any] = {"canvas_ids": created_canvas_ids}
            res: dict[str, Any] = delete_canvas(web_api_auth, delete_payload)
            if res.get("code") != 0:
                print(
                    f"[clear_canvases] Failed to delete canvases {created_canvas_ids}: {res}"
                )
        except Exception as exc:  # pragma: no cover - best-effort cleanup
            print(f"[clear_canvases] Exception while deleting canvases: {exc}")

    request.addfinalizer(cleanup)
    return created_canvas_ids


