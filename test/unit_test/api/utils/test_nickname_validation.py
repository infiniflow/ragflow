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

import pytest

from api.constants import NICKNAME_MAX_LENGTH
from api.utils.nickname_validation import validate_nickname
from common.constants import RetCode


@pytest.mark.parametrize(
    "nickname",
    [
        "John Doe",
        "张三",
        "O'Brien",
        "valid-name_123",
        "a" * NICKNAME_MAX_LENGTH,
    ],
)
def test_validate_nickname_accepts_valid_values(nickname):
    message, code = validate_nickname(nickname)
    assert message is None
    assert code is None


@pytest.mark.parametrize(
    "nickname, expected_message",
    [
        (None, "Nickname is required."),
        ("", "Nickname cannot be empty."),
        ("   ", "Nickname cannot be empty."),
        ("carh!@#$%^&*()_+WFAGD", "Nickname contains invalid characters."),
        ("John\tDoe", "Nickname contains invalid characters."),
        ("John\nDoe", "Nickname contains invalid characters."),
        ("a" * (NICKNAME_MAX_LENGTH + 1), f"Nickname must be at most {NICKNAME_MAX_LENGTH} characters."),
    ],
)
def test_validate_nickname_rejects_invalid_values(nickname, expected_message):
    message, code = validate_nickname(nickname)
    assert message == expected_message
    assert code == RetCode.ARGUMENT_ERROR


def test_validate_nickname_rejects_non_string_input():
    message, code = validate_nickname(123)
    assert message == "Nickname must be a string."
    assert code == RetCode.ARGUMENT_ERROR
