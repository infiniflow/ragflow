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


def get_float(v):
    """
    Convert a value to float, handling None and exceptions gracefully.

    Attempts to convert the input value to a float. If the value is None or
    cannot be converted to float, returns negative infinity as a default value.

    Args:
        v: The value to convert to float. Can be any type that float() accepts,
           or None.

    Returns:
        float: The converted float value if successful, otherwise float('-inf').

    Examples:
        >>> get_float("3.14")
        3.14
        >>> get_float(None)
        -inf
        >>> get_float("invalid")
        -inf
        >>> get_float(42)
        42.0
    """
    if v is None:
        return float("-inf")
    try:
        return float(v)
    except Exception:
        return float("-inf")


def normalize_overlapped_percent(overlapped_percent):
    try:
        value = float(overlapped_percent)
    except (TypeError, ValueError):
        return 0
    if 0 < value < 1:
        value *= 100
    value = int(value)
    return max(0, min(value, 90))


def format_minimum_should_match_percent(fraction: float) -> str:
    """Convert a fraction to a percentage string for full-text search.

    Floating-point fractions such as 0.29 are not exactly representable in
    IEEE-754, so `int(fraction * 100)` truncates toward zero and can produce a
    string one percentage point stricter than requested. This helper rounds
    half-up to the nearest whole percent instead.

    Args:
        fraction: A fraction in the range [0, 1].

    Returns:
        A percentage string like "29%".

    Examples:
        >>> format_minimum_should_match_percent(0.29)
        '29%'
        >>> format_minimum_should_match_percent(0.57)
        '57%'
        >>> format_minimum_should_match_percent(0.125)
        '13%'
    """
    return str(int(fraction * 100 + 0.5)) + "%"
