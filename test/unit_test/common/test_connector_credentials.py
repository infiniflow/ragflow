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
"""AES-256-GCM encryption of connector ``config["credentials"]``.

The Go runtime reads and writes the same column, so the stored format is
fixed: ``"enc:v1:" + base64(nonce[12] || ciphertext || tag[16])``.
"""

import base64
import copy
import json

import pytest
from Cryptodome.Cipher import AES

from common import settings
from common.connector_credentials import (
    ConnectorCredentialsError,
    connector_key,
    decrypt_connector_config,
    encrypt_connector_config,
    has_encrypted_credentials,
)

# Bytes 0..31. The Go tests decrypt the same vector.
_KEY = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
_VECTOR = "enc:v1:ZGVmZ2hpamtsbW5vMzm/FhC2IvFVBzHK4EVIiS2pKzu5X9Feh/PZO57Rh3K0yyGkLTDmruUEeZB6LtRoLedehJBJUg=="
_OTHER_KEY = base64.b64encode(bytes(range(1, 33))).decode()


@pytest.fixture
def key(monkeypatch):
    monkeypatch.setenv("RAGFLOW_CONNECTOR_KEY", _KEY)


@pytest.fixture
def no_key(monkeypatch):
    monkeypatch.delenv("RAGFLOW_CONNECTOR_KEY", raising=False)


@pytest.mark.p2
@pytest.mark.parametrize("value", [None, ""])
def test_unset_or_empty_key_turns_encryption_off(monkeypatch, value):
    if value is None:
        monkeypatch.delenv("RAGFLOW_CONNECTOR_KEY", raising=False)
    else:
        monkeypatch.setenv("RAGFLOW_CONNECTOR_KEY", value)
    assert connector_key() is None


@pytest.mark.p2
def test_valid_key_decodes_to_32_bytes(key):
    assert connector_key() == bytes(range(32))


@pytest.mark.p2
@pytest.mark.parametrize(
    "value, reason",
    [
        ("not base64!", "base64"),
        (_KEY.rstrip("="), "base64"),
        (_KEY + "\n", "base64"),
        (_KEY + "\r\n", "base64"),
        (base64.b64encode(bytes(16)).decode(), "32 bytes"),
        (base64.b64encode(bytes(33)).decode(), "32 bytes"),
    ],
)
def test_malformed_key_is_rejected_without_echoing_it(monkeypatch, value, reason):
    monkeypatch.setenv("RAGFLOW_CONNECTOR_KEY", value)
    with pytest.raises(ValueError, match=reason) as exc:
        connector_key()
    assert "RAGFLOW_CONNECTOR_KEY" in str(exc.value)
    assert value not in str(exc.value)


@pytest.mark.p2
def test_init_settings_fails_on_a_malformed_key_before_any_other_setup(monkeypatch):
    def _setup_ran(*_args, **_kwargs):
        raise AssertionError("init_settings must validate RAGFLOW_CONNECTOR_KEY first")

    monkeypatch.setenv("RAGFLOW_CONNECTOR_KEY", "not base64!")
    monkeypatch.setattr(settings, "normalize_database_type", _setup_ran)
    with pytest.raises(ValueError, match="RAGFLOW_CONNECTOR_KEY"):
        settings.init_settings()


@pytest.mark.p2
def test_decrypts_the_cross_runtime_vector(key):
    config = {"feed_url": "https://example.com", "credentials": _VECTOR}
    assert decrypt_connector_config(config) == {
        "feed_url": "https://example.com",
        "credentials": {"api_token": "tok-123", "user": "ada"},
    }
    assert config["credentials"] == _VECTOR


@pytest.mark.p2
def test_round_trip_encrypts_only_credentials_and_keeps_the_input(key):
    config = {"credentials": {"api_token": "s3cret-token"}, "sync_deleted_files": True}
    original = copy.deepcopy(config)

    encrypted = encrypt_connector_config(config)

    assert config == original
    assert encrypted["credentials"].startswith("enc:v1:")
    assert encrypted["sync_deleted_files"] is True
    assert "s3cret-token" not in json.dumps(encrypted)
    assert decrypt_connector_config(encrypted) == original


@pytest.mark.p2
def test_stored_value_uses_a_12_byte_nonce_and_a_16_byte_tag(key):
    credentials = {"api_token": "tok-123", "user": "ada"}
    token = encrypt_connector_config({"credentials": credentials})["credentials"]

    raw = base64.b64decode(token.removeprefix("enc:v1:"), validate=True)

    assert len(raw) == 12 + len(json.dumps(credentials).encode()) + 16


@pytest.mark.p2
def test_every_write_draws_a_fresh_nonce(key):
    config = {"credentials": {"api_token": "tok-123"}}
    assert encrypt_connector_config(config)["credentials"] != encrypt_connector_config(config)["credentials"]


@pytest.mark.p2
def test_without_a_key_encrypt_stores_plaintext(no_key):
    config = {"credentials": {"api_token": "tok-123"}}
    assert encrypt_connector_config(config) == {"credentials": {"api_token": "tok-123"}}


@pytest.mark.p2
@pytest.mark.parametrize("config", [{"credentials": _VECTOR, "x": 1}, {"sync_deleted_files": True}, [], None])
def test_encrypt_leaves_encrypted_or_missing_credentials_alone(key, config):
    assert encrypt_connector_config(config) == copy.deepcopy(config)


@pytest.mark.p2
@pytest.mark.parametrize("config", [{"credentials": {"api_token": "tok-123"}}, {"credentials": "plain-token"}, {"sync_deleted_files": True}, [], None])
def test_decrypt_leaves_plaintext_or_missing_credentials_alone(key, config):
    assert decrypt_connector_config(config) == copy.deepcopy(config)


@pytest.mark.p2
def test_decrypt_without_a_key_fails(no_key):
    with pytest.raises(ConnectorCredentialsError, match="encrypted but RAGFLOW_CONNECTOR_KEY is not set"):
        decrypt_connector_config({"credentials": _VECTOR})


@pytest.mark.p2
def test_decrypt_with_the_wrong_key_fails_without_leaking(monkeypatch):
    monkeypatch.setenv("RAGFLOW_CONNECTOR_KEY", _OTHER_KEY)
    with pytest.raises(ConnectorCredentialsError, match="cannot decrypt connector credentials") as exc:
        decrypt_connector_config({"credentials": _VECTOR})
    message = str(exc.value)
    for secret in (_KEY, _OTHER_KEY, _VECTOR, "tok-123"):
        assert secret not in message


def _tampered(value: str) -> str:
    raw = bytearray(base64.b64decode(value.removeprefix("enc:v1:")))
    raw[20] ^= 0x01
    return "enc:v1:" + base64.b64encode(bytes(raw)).decode()


@pytest.mark.p2
@pytest.mark.parametrize("credentials", [_tampered(_VECTOR), "enc:v1:AAAA", "enc:v1:!!!!", "enc:v1:"])
def test_decrypt_rejects_a_corrupted_value(key, credentials):
    with pytest.raises(ConnectorCredentialsError, match="cannot decrypt connector credentials"):
        decrypt_connector_config({"credentials": credentials})


def _encrypt_bytes(plaintext: bytes) -> str:
    cipher = AES.new(bytes(range(32)), AES.MODE_GCM, nonce=bytes(12))
    ciphertext, tag = cipher.encrypt_and_digest(plaintext)
    return "enc:v1:" + base64.b64encode(cipher.nonce + ciphertext + tag).decode()


@pytest.mark.p2
def test_decrypts_raw_utf8_as_go_writes_it(key):
    # Go's json.Marshal writes non-ASCII as raw UTF-8; Python writes \u escapes.
    stored = _encrypt_bytes('{"password": "p\u00e4ssword"}'.encode())
    assert decrypt_connector_config({"credentials": stored}) == {"credentials": {"password": "p\u00e4ssword"}}


@pytest.mark.p2
@pytest.mark.parametrize("plaintext", [b"not json", b"\x80"])
def test_decrypt_rejects_an_authentic_payload_that_is_not_json(key, plaintext):
    with pytest.raises(ConnectorCredentialsError) as exc:
        decrypt_connector_config({"credentials": _encrypt_bytes(plaintext)})
    assert str(exc.value) == "cannot decrypt connector credentials: wrong RAGFLOW_CONNECTOR_KEY or corrupted value"


@pytest.mark.p2
@pytest.mark.parametrize(
    "config, expected",
    [
        ({"credentials": _VECTOR}, True),
        ({"credentials": "enc:v2:anything"}, True),
        ({"credentials": "plain-token"}, False),
        ({"credentials": {"api_token": "enc:v1:x"}}, False),
        ({}, False),
        ("enc:v1:x", False),
        (None, False),
    ],
)
def test_has_encrypted_credentials(config, expected):
    assert has_encrypted_credentials(config) is expected
