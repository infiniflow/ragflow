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
"""Shared fixtures and utilities for team management HTTP API tests."""

from __future__ import annotations

import os
import subprocess
import sys
from pathlib import Path
from typing import List

import importlib.util
import pytest


# ---------------------------------------------------------------------------
# Import user cleanup helper from root test conftest
# ---------------------------------------------------------------------------

_root_conftest_path = Path(__file__).parent.parent.parent / "conftest.py"
_root_spec = importlib.util.spec_from_file_location(
    "root_test_conftest", _root_conftest_path
)
_root_conftest_module = importlib.util.module_from_spec(_root_spec)
assert _root_spec.loader is not None
_root_spec.loader.exec_module(_root_conftest_module)
delete_user_from_db = _root_conftest_module.delete_user_from_db


def delete_team_from_db(tenant_id: str) -> bool:
    """Hard-delete a test team (tenant) and related records directly from DB.

    This is used only from tests to ensure that team-related test data does not
    accumulate across runs. It does not touch application code or APIs.
    """
    try:
        current_dir: str = os.path.dirname(os.path.abspath(__file__))
        # Project root: test/testcases/test_http_api/test_team_management/../../../../
        project_root: str = os.path.abspath(
            os.path.join(current_dir, "..", "..", "..", "..")
        )

        delete_script: str = f"""
import sys
sys.path.insert(0, '{project_root}')

# Remove test directories from path to avoid conflicts
test_paths = [p for p in sys.path if 'test/testcases' in p or 'testcases' in p]
for p in test_paths:
    if p in sys.path:
        sys.path.remove(p)

try:
    from api.db.db_models import DB, Tenant, UserTenant, File

    tenants = list(Tenant.select().where(Tenant.id == '{tenant_id}'))
    if tenants:
        with DB.atomic():
            for tenant in tenants:
                tid = tenant.id
                try:
                    # Delete user-team relationships
                    UserTenant.delete().where(UserTenant.tenant_id == tid).execute()

                    # Delete files associated with this tenant
                    File.delete().where(File.tenant_id == tid).execute()

                    # Finally delete the tenant itself
                    Tenant.delete().where(Tenant.id == tid).execute()
                except Exception as e:
                    print(f"Warning during team cleanup: {{e}}")
        print(f"DELETED_TENANT:{tenant_id}")
    else:
        print(f"TENANT_NOT_FOUND:{tenant_id}")
except Exception as e:
    print(f"ERROR:{{e}}")
    import traceback
    traceback.print_exc()
    sys.exit(1)
"""

        result = subprocess.run(
            [sys.executable, "-c", delete_script],
            capture_output=True,
            text=True,
            timeout=30,
        )

        output: str = result.stdout + result.stderr

        if "DELETED_TENANT:" in output:
            print(f"Successfully deleted tenant {tenant_id} from database")
            return True
        if "TENANT_NOT_FOUND:" in output:
            print(f"Tenant {tenant_id} not found in database")
            return False

        print("Failed to delete tenant from database")
        if output:
            print(f"Output: {output}")
        return False

    except subprocess.TimeoutExpired:
        print("Timeout while trying to delete tenant from database")
        return False
    except Exception as exc:  # pragma: no cover - best-effort cleanup
        print(f"Failed to delete tenant from database: {exc}")
        return False


@pytest.fixture(scope="function")
def clear_teams(request: pytest.FixtureRequest) -> List[str]:
    """Fixture to clean up teams (tenants) created during tests.

    Tests should append created team IDs to the returned list. After each test
    this fixture will hard-delete those teams directly from the database.
    """
    created_team_ids: List[str] = []

    def cleanup() -> None:
        for tenant_id in created_team_ids:
            try:
                delete_team_from_db(tenant_id)
            except Exception as exc:  # pragma: no cover - best-effort cleanup
                print(
                    f"[clear_teams] Failed to delete test tenant {tenant_id}: {exc}"
                )

    request.addfinalizer(cleanup)
    return created_team_ids


@pytest.fixture(scope="function")
def clear_team_users(request: pytest.FixtureRequest) -> List[str]:
    """Fixture to clean up users created in team-management tests.

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


