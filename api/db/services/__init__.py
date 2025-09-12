#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import re
from pathlib import PurePath

from .user_service import UserService as UserService


def split_name_counter(filename: str) -> tuple[str, int | None]:
    """
    Splits a filename into main part and counter (if present in parentheses).

    Args:
        filename: Input filename string to be parsed

    Returns:
        A tuple containing:
        - The main filename part (string)
        - The counter from parentheses (integer) or None if no counter exists
    """
    pattern = re.compile(r"^(.*?)\((\d+)\)$")

    match = pattern.search(filename)
    if match:
        main_part = match.group(1).rstrip()
        bracket_part = match.group(2)
        return main_part, int(bracket_part)

    return filename, None


def duplicate_name(query_func, **kwargs) -> str:
    """
    Generates a unique filename by appending/incrementing a counter when duplicates exist.

    Continuously checks for name availability using the provided query function,
    automatically appending (1), (2), etc. until finding an available name or
    reaching maximum retries.

    Args:
        query_func: Callable that accepts keyword arguments and returns:
                  - True if name exists (should be modified)
                  - False if name is available
        **kwargs: Must contain 'name' key with original filename to check

    Returns:
        str: Available filename, either:
            - Original name (if available)
            - Modified name with counter (e.g., "file(1).txt")

    Raises:
        KeyError: If 'name' key not provided in kwargs
        RuntimeError: If unable to generate unique name after maximum retries

    Example:
        >>> def name_exists(name): return name in existing_files
        >>> duplicate_name(name_exists, name="document.pdf")
        'document(1).pdf'  # If original exists
    """
    MAX_RETRIES = 1000

    if "name" not in kwargs:
        raise KeyError("Arguments must contain 'name' key")

    original_name = kwargs["name"]
    current_name = original_name
    retries = 0

    while retries < MAX_RETRIES:
        if not query_func(**kwargs):
            return current_name

        path = PurePath(current_name)
        stem = path.stem
        suffix = path.suffix

        main_part, counter = split_name_counter(stem)
        counter = counter + 1 if counter else 1

        new_name = f"{main_part}({counter}){suffix}"

        kwargs["name"] = new_name
        current_name = new_name
        retries += 1

    raise RuntimeError(f"Failed to generate unique name within {MAX_RETRIES} attempts. Original: {original_name}")
