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
import os
from pathlib import Path
from Cryptodome.PublicKey import RSA
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from common.file_utils import get_project_base_directory


class CryptPayloadError(ValueError):
    """Raised when a client-supplied ciphertext payload is malformed.

    Distinguished from server-side faults (missing or invalid private-key
    file, key import failures), which propagate unchanged so callers can
    treat them as server errors rather than bad credentials.
    """


def crypt(line):
    """
    decrypt(crypt(input_string)) == base64(input_string), which frontend and ragflow_cli use.
    """
    file_path = os.path.join(get_project_base_directory(), "conf", "public.pem")
    rsa_key = RSA.importKey(Path(file_path).read_text(), "Welcome")
    cipher = Cipher_pkcs1_v1_5.new(rsa_key)
    password_base64 = base64.b64encode(line.encode("utf-8")).decode("utf-8")
    encrypted_password = cipher.encrypt(password_base64.encode())
    return base64.b64encode(encrypted_password).decode("utf-8")


def decrypt(line):
    file_path = os.path.join(get_project_base_directory(), "conf", "private.pem")
    # Key-file read/import failures are server faults and propagate as-is.
    rsa_key = RSA.importKey(Path(file_path).read_text(), "Welcome")
    cipher = Cipher_pkcs1_v1_5.new(rsa_key)
    # Everything below concerns the client-supplied payload. Strip internal
    # whitespace first: line-wrapped base64 (as test fixtures and PEM-style
    # senders produce) is legal base64, but b64decode(validate=True) would
    # reject the embedded newlines. After the strip, validate=True still
    # catches genuinely malformed payloads (non-alphabet characters).
    try:
        ciphertext = base64.b64decode("".join(line.split()), validate=True)
    except ValueError as e:
        raise CryptPayloadError("password payload is not valid base64") from e
    try:
        plaintext = cipher.decrypt(ciphertext, None)
    except ValueError as e:
        # e.g. pycryptodome's "Ciphertext with incorrect length" for
        # well-formed base64 that is not a valid ciphertext block.
        raise CryptPayloadError("password payload failed RSA decryption") from e
    if plaintext is None:
        raise CryptPayloadError("password payload failed RSA decryption")
    try:
        return plaintext.decode("utf-8")
    except UnicodeDecodeError as e:
        raise CryptPayloadError("decrypted password payload is not valid UTF-8") from e
