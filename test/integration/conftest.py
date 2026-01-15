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
"""
Integration test fixtures for RAGFlow.

These fixtures provide shared setup/teardown for integration tests that validate
cross-interface workflows (HTTP API, SDK, Web API all testing the same business logic).

Per AGENTS.md: Tests focus on service-layer logic, not API endpoint structure.
"""

import sys
from pathlib import Path

# Add testcases to path for common utilities
testcases_path = Path(__file__).parent.parent / "testcases"
sys.path.insert(0, str(testcases_path))

# Add test directory to path
test_path = Path(__file__).parent.parent
sys.path.insert(0, str(test_path))

import pytest  # noqa: E402
from libs.auth import RAGFlowHttpApiAuth  # noqa: E402

# Re-export common fixtures from testcases
from testcases.conftest import *  # noqa: F401, F403, E402


@pytest.fixture
def api_client(token):
    """HTTP API client with authentication."""
    return RAGFlowHttpApiAuth(token)
