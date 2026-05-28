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

from common.log_utils import extract_upstream_error_message


def test_extract_upstream_error_message_reads_common_provider_fields():
    response = {
        "status_code": 500,
        "request_id": "req-1",
        "error": {"code": "BadGateway", "message": "provider is unavailable"},
        "reason": "upstream timeout",
    }

    message = extract_upstream_error_message(response)

    assert "status_code: 500" in message
    assert "code: BadGateway" in message
    assert "message: provider is unavailable" in message
    assert "reason: upstream timeout" in message
    assert "request_id: req-1" in message
