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
import importlib


class DecryptPasswordError(Exception): ...


def decrypt_password(password: str, password_conf) -> str:
    """
    Decrypt a password if encryption is enabled in security config.

    Raises:
        ValueError: if encryption is enabled but required settings are missing.
    """
    if not password or not password_conf.encrypt_enabled:
        return password

    if not password_conf.private_key or not password_conf.encrypt_module:
        raise DecryptPasswordError(
            "Database encryption enabled but PRIVATE_KEY or ENCRYPT_MODULE missing"
        )

    try:
        module_name, func_name = password_conf.encrypt_module.split("#")
        func = getattr(importlib.import_module(module_name), func_name)
        return func(password_conf.private_key, password)
    except Exception as e:
        raise DecryptPasswordError(f"Failed to decrypt password: {e}") from e
