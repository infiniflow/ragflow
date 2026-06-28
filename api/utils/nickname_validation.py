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
import logging
import re

from api.constants import NICKNAME_MAX_LENGTH
from common.constants import RetCode

# Match frontend NICKNAME_PATTERN: letters, numbers, space, and . _ ' -
_NICKNAME_PATTERN = re.compile(r"^[\w ._'-]+$", re.UNICODE)


def _reject_nickname(message: str) -> tuple[str, int]:
    logging.warning("Nickname validation failed: %s", message)
    return message, RetCode.ARGUMENT_ERROR


def validate_nickname(nickname: str | None) -> tuple[str | None, int | None]:
    """
    Validate a user nickname/display name.

    Returns:
        A tuple of (error_message, error_code) if validation fails,
        or (None, None) if validation passes.
    """
    if not isinstance(nickname, (str, type(None))):
        return _reject_nickname("Nickname must be a string.")
    if nickname is None:
        return _reject_nickname("Nickname is required.")

    nickname = nickname.strip()
    if not nickname:
        return _reject_nickname("Nickname cannot be empty.")
    if len(nickname) > NICKNAME_MAX_LENGTH:
        return _reject_nickname(f"Nickname must be at most {NICKNAME_MAX_LENGTH} characters.")
    if not _NICKNAME_PATTERN.fullmatch(nickname):
        return _reject_nickname("Nickname contains invalid characters.")
    return None, None
