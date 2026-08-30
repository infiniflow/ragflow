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


def minimum_should_match_percent(fraction):
    """
    Convert a 0..1 minimum_should_match fraction to the percent-string the
    full-text doc-store connectors (es_conn/opensearch_conn/ob_conn/infinity_conn)
    pass through to their query engine, e.g. 0.57 -> "57%".

    Rounds rather than truncates: a base-10 fraction like 0.29 is not exactly
    representable in IEEE-754 (it is stored as 0.28999999999999998...), so
    int(fraction * 100) silently sends a stricter-than-requested threshold for
    many two-decimal fractions (0.29 -> "28%", not "29%").

    Examples:
        >>> minimum_should_match_percent(0.29)
        '29%'
        >>> minimum_should_match_percent(0.5)
        '50%'
    """
    return f"{round(fraction * 100)}%"
