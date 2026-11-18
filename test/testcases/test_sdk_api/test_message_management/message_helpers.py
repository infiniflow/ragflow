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
import time


def wait_until(probe, predicate, timeout=30.0):
    """Retry incomplete results, never hide API errors behind retries."""
    deadline = time.monotonic() + timeout
    while True:
        result = probe()
        if predicate(result):
            return result
        remaining = deadline - time.monotonic()
        assert remaining > 0, f"Timed out waiting for message state; last result: {result!r}"
        time.sleep(min(0.25, remaining))


def assert_identity(row, payload, memory_id):
    assert row["memory_id"] == memory_id, row
    assert row["message_type"] == "raw", row
    for field in ("agent_id", "session_id", "user_id"):
        assert row[field] == payload[field].strip(), row
    assert str(row["message_id"]).isdigit(), row


def assert_content(row, payload, memory_id):
    assert_identity(row, payload, memory_id)
    assert row["content"] == f"User Input: {payload['user_input']}\nAgent Response: {payload['agent_response']}", row
