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
"""AES-256-GCM encryption of connector ``config["credentials"]`` at rest.

The Go server reads and writes the same column, so the stored format is fixed:
``"enc:v1:" + base64(nonce[12] || ciphertext || tag[16])`` with an empty AAD.
"""

import base64
import json
import os

from Cryptodome.Cipher import AES
from Cryptodome.Random import get_random_bytes

_KEY_ENV = "RAGFLOW_CONNECTOR_KEY"
_PREFIX = "enc:v1:"
# Go's cipher.NewGCM uses a 12-byte nonce; pycryptodome defaults to 16.
_NONCE_SIZE = 12
_TAG_SIZE = 16


class ConnectorCredentialsError(ValueError):
    pass


def connector_key() -> bytes | None:
    """Return the key from RAGFLOW_CONNECTOR_KEY, or None when encryption is off."""
    value = os.environ.get(_KEY_ENV, "")
    if not value:
        return None
    try:
        key = base64.b64decode(value, validate=True)
    except ValueError:
        raise ValueError(f"{_KEY_ENV} is not valid base64") from None
    if len(key) != 32:
        raise ValueError(f"{_KEY_ENV} must decode to 32 bytes, got {len(key)}")
    return key


def has_encrypted_credentials(config) -> bool:
    """True when credentials hold a value in any encrypted format, which a client must never send."""
    return isinstance(config, dict) and isinstance(config.get("credentials"), str) and config["credentials"].startswith("enc:")


def _is_encrypted(credentials) -> bool:
    return isinstance(credentials, str) and credentials.startswith(_PREFIX)


def encrypt_connector_config(config):
    """Return a copy of config with credentials encrypted, or config itself when there is nothing to encrypt."""
    if not isinstance(config, dict) or "credentials" not in config or _is_encrypted(config["credentials"]):
        return config
    key = connector_key()
    if key is None:
        return config
    cipher = AES.new(key, AES.MODE_GCM, nonce=get_random_bytes(_NONCE_SIZE))
    ciphertext, tag = cipher.encrypt_and_digest(json.dumps(config["credentials"]).encode())
    return {**config, "credentials": _PREFIX + base64.b64encode(cipher.nonce + ciphertext + tag).decode()}


def _decrypt(key: bytes, token: str) -> bytes:
    raw = base64.b64decode(token, validate=True)
    if len(raw) < _NONCE_SIZE + _TAG_SIZE:
        raise ValueError("ciphertext too short")
    cipher = AES.new(key, AES.MODE_GCM, nonce=raw[:_NONCE_SIZE])
    return cipher.decrypt_and_verify(raw[_NONCE_SIZE:-_TAG_SIZE], raw[-_TAG_SIZE:])


def decrypt_connector_config(config):
    """Return a copy of config with credentials decrypted, or config itself when they are not encrypted."""
    if not isinstance(config, dict) or not _is_encrypted(config.get("credentials")):
        return config
    key = connector_key()
    if key is None:
        raise ConnectorCredentialsError(f"connector credentials are encrypted but {_KEY_ENV} is not set")
    try:
        plaintext = _decrypt(key, config["credentials"][len(_PREFIX) :])
    except ValueError:
        raise ConnectorCredentialsError(f"cannot decrypt connector credentials: wrong {_KEY_ENV} or corrupted value") from None
    return {**config, "credentials": json.loads(plaintext)}
