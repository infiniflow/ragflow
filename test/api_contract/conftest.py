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
API contract tests for RAGFlow.

Light endpoint validation for critical paths. These tests ensure that HTTP/SDK/Web APIs
maintain consistent contracts (request/response schemas).

Per AGENTS.md: Contract tests are thin layer on top of integration tests.
They validate interface stability, not business logic (business logic tested in integration/).
"""

import sys
from pathlib import Path

import pytest

# Add testcases to path for common utilities
testcases_path = Path(__file__).parent.parent / "testcases"
sys.path.insert(0, str(testcases_path))

# Add test directory to path
test_path = Path(__file__).parent.parent
sys.path.insert(0, str(test_path))

# Import after path setup
from libs.auth import RAGFlowHttpApiAuth


@pytest.fixture
def api_client(token):
    """HTTP API client with authentication."""
    return RAGFlowHttpApiAuth(token)



