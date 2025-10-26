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

import base64
import functools
import hashlib
import time
from pathlib import Path


def encode_avatar(image_path):
    with Path.open(image_path, "rb") as file:
        binary_data = file.read()
    base64_encoded = base64.b64encode(binary_data).decode("utf-8")
    return base64_encoded


def compare_by_hash(file1, file2, algorithm="sha256"):
    def _calc_hash(file_path):
        hash_func = hashlib.new(algorithm)
        with open(file_path, "rb") as f:
            while chunk := f.read(8192):
                hash_func.update(chunk)
        return hash_func.hexdigest()

    return _calc_hash(file1) == _calc_hash(file2)


def wait_for(timeout=10, interval=1, error_msg="Timeout"):
    def decorator(func):
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            start_time = time.time()
            while True:
                result = func(*args, **kwargs)
                if result is True:
                    return result
                elapsed = time.time() - start_time
                if elapsed > timeout:
                    assert False, error_msg
                time.sleep(interval)

        return wrapper

    return decorator


def is_sorted(data, field, descending=True):
    timestamps = [ds[field] for ds in data]
    return all(a >= b for a, b in zip(timestamps, timestamps[1:])) if descending else all(a <= b for a, b in zip(timestamps, timestamps[1:]))
