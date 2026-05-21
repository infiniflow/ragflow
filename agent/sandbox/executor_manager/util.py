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
import os
import re


def is_enabled(value: str) -> bool:
    return str(value).strip().lower() in {"1", "true", "yes", "on"}


def env_setting_enabled(env_key: str, default: str = "false") -> bool:
    value = os.getenv(env_key, default)
    return is_enabled(value)


def is_valid_memory_limit(mem: str | None) -> bool:
    """
    Return True if the input string is a valid Docker memory limit (e.g. '256m', '1g').
    Units allowed: b, k, m, g (case-insensitive).
    Disallows zero or negative values.
    """
    if not mem or not isinstance(mem, str):
        return False

    mem = mem.strip().lower()

    return re.fullmatch(r"[1-9]\d*(b|k|m|g)", mem) is not None


def parse_timeout_duration(timeout: str | None, default_seconds: int = 10) -> int:
    """
    Parses a string like '90s', '2m', '1m30s' into total seconds (int).
    Supports 's', 'm' (lower or upper case). Returns default if invalid.
    '1m30s' -> 90
    """
    if not timeout or not isinstance(timeout, str):
        return default_seconds

    timeout = timeout.strip().lower()

    pattern = r"^(?:(\d+)m)?(?:(\d+)s)?$"
    match = re.fullmatch(pattern, timeout)
    if not match:
        return default_seconds

    minutes = int(match.group(1)) if match.group(1) else 0
    seconds = int(match.group(2)) if match.group(2) else 0
    total = minutes * 60 + seconds

    return total if total > 0 else default_seconds


def format_timeout_duration(seconds: int) -> str:
    """
    Formats an integer number of seconds into a string like '1m30s'.
    90 -> '1m30s'
    """
    if seconds < 60:
        return f"{seconds}s"
    minutes, sec = divmod(seconds, 60)
    if sec == 0:
        return f"{minutes}m"
    return f"{minutes}m{sec}s"
