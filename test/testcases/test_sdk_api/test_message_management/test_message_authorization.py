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

"""Cross-tenant access uses a second unprivileged API key, never another tenant's data."""

import os

import pytest
from ragflow_sdk import Memory, RAGFlow

from test.testcases.test_sdk_api.test_message_management.message_helpers import assert_content

pytestmark = pytest.mark.integration


@pytest.fixture
def other_client(integration_settings):
    key = os.getenv("RAGFLOW_SDK_OTHER_API_KEY")
    tenant_id = os.getenv("RAGFLOW_SDK_OTHER_TENANT_ID")
    if not key or key == integration_settings["api_key"] or not tenant_id or tenant_id == integration_settings["tenant_id"]:
        pytest.fail("Cross-tenant checks require RAGFLOW_SDK_OTHER_API_KEY and RAGFLOW_SDK_OTHER_TENANT_ID for a separate unprivileged tenant")
    return RAGFlow(key, integration_settings["base_url"])


@pytest.mark.parametrize("operation", ["list", "recent", "search", "content", "status", "forget", "add"])
def test_private_memory_not_accessible_to_other_tenant(other_client, populated_memory, operation):
    memory = populated_memory["memory"]
    row = populated_memory["rows"][0]
    foreign = Memory(other_client, {"id": memory.id})
    if operation == "recent":
        assert other_client.get_recent_messages([memory.id]) == []
    elif operation == "search":
        assert other_client.search_message("", [memory.id]) == []
    elif operation == "add":
        # Copying a legitimate conversation subject must not grant memory access.
        with pytest.raises(Exception, match="Some messages failed to add.*Memory not found"):
            other_client.add_message(**populated_memory["payloads"][0])
    else:
        with pytest.raises(Exception, match="^Resource not found$"):
            if operation == "list":
                foreign.list_memory_messages()
            elif operation == "content":
                foreign.get_message_content(row["message_id"])
            elif operation == "status":
                foreign.update_message_status(row["message_id"], False)
            else:
                foreign.forget_message(row["message_id"])
    content = memory.get_message_content(row["message_id"])
    assert_content(content, populated_memory["payloads"][0], memory.id)
    assert content["status"] is True
    assert content.get("forget_at") in (None, "", "-")
    assert memory.list_memory_messages()["messages"]["total_count"] == 5
