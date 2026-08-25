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


import logging
import math

logger = logging.getLogger(__name__)


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
    """Convert a [0, 1) overlap ratio to an integer percent in [0, 90].

    Wire configs (parser_config, token chunker DSL) store overlapped_percent as a
    ratio, e.g. 0.3 for 30%. Merge math expects an integer percentage.
    """
    try:
        ratio = float(overlapped_percent)
    except (TypeError, ValueError, OverflowError):
        logger.warning("normalize_overlapped_percent: overlap ratio is not numeric")
        return 0
    if not math.isfinite(ratio):
        logger.warning("normalize_overlapped_percent: overlap ratio is not finite")
        return 0
    if ratio < 0 or ratio >= 1:
        logger.warning("normalize_overlapped_percent: overlap ratio is outside [0, 1)")
        return 0
    percent = ratio * 100
    # Match Go's math.Round (away from zero at .5) rather than Python round().
    rounded = math.floor(percent + 0.5)
    return max(0, min(rounded, 90))
