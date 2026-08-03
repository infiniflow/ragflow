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

"""Test cases for common.crypto_utils and the encrypted storage wrapper.

Also covers the crypto branch of ``init_settings``, which must not fall back to
unencrypted storage when the crypto configuration is broken.
"""

import pytest

from common.crypto_utils import AES128CBC, AES256CBC, SM4CBC, CryptoUtil
from rag.utils.encrypted_storage import EncryptedStorageWrapper, create_encrypted_storage

PLAINTEXT = b"Hello, RAGFlow! This is a test for encryption."


class FakeStorage:
    """In-memory storage impl exposing the methods the wrapper requires."""

    def __init__(self):
        self.blobs = {}

    def put(self, bucket, fnm, binary, tenant_id=None):
        self.blobs[(bucket, fnm)] = binary
        return "ok"

    def get(self, bucket, fnm, tenant_id=None):
        return self.blobs.get((bucket, fnm))

    def rm(self, bucket, fnm, tenant_id=None):
        self.blobs.pop((bucket, fnm), None)

    def obj_exist(self, bucket, fnm, tenant_id=None):
        return (bucket, fnm) in self.blobs

    def health(self):
        return True


class TestCryptoUtil:
    """Test cases for the CryptoUtil factory and its algorithms."""

    @pytest.mark.parametrize("algorithm", ["aes-128-cbc", "aes-256-cbc", "sm4-cbc"])
    def test_roundtrip(self, algorithm):
        """Every supported algorithm round-trips the payload."""
        crypto = CryptoUtil(algorithm=algorithm, key="test-key")

        assert crypto.decrypt(crypto.encrypt(PLAINTEXT)) == PLAINTEXT

    @pytest.mark.parametrize("algorithm", ["aes-128-cbc", "aes-256-cbc", "sm4-cbc"])
    def test_ciphertext_carries_magic_header(self, algorithm):
        """Ciphertext is tagged so decrypt() can recognize it later."""
        crypto = CryptoUtil(algorithm=algorithm, key="test-key")

        assert crypto.encrypt(PLAINTEXT).startswith(b"RAGF")

    def test_rejects_unsupported_algorithm(self):
        """An unknown algorithm name is refused rather than silently defaulted."""
        with pytest.raises(ValueError, match="Unsupported algorithm"):
            CryptoUtil(algorithm="aes256-cbc", key="test-key")

    @pytest.mark.parametrize("key", [None, ""])
    def test_rejects_missing_key(self, key):
        """A missing or empty key is refused."""
        with pytest.raises(ValueError, match="Encryption key not provided"):
            CryptoUtil(algorithm="aes-256-cbc", key=key)

    def test_random_iv_by_default(self):
        """Encrypting the same payload twice yields different ciphertext."""
        crypto = CryptoUtil(algorithm="aes-256-cbc", key="test-key")

        assert crypto.encrypt(PLAINTEXT) != crypto.encrypt(PLAINTEXT)

    def test_empty_payload(self):
        """An empty payload is padded, encrypted and recovered."""
        crypto = CryptoUtil(algorithm="aes-256-cbc", key="test-key")

        assert crypto.decrypt(crypto.encrypt(b"")) == b""

    def test_block_aligned_payload(self):
        """A payload that is an exact multiple of the block size round-trips."""
        crypto = CryptoUtil(algorithm="aes-256-cbc", key="test-key")
        data = b"A" * 32

        assert crypto.decrypt(crypto.encrypt(data)) == data

    def test_wrong_key_does_not_recover_plaintext(self):
        """Ciphertext is not readable with a different key."""
        encrypted = CryptoUtil(algorithm="aes-256-cbc", key="key-one").encrypt(PLAINTEXT)
        other = CryptoUtil(algorithm="aes-256-cbc", key="key-two")

        try:
            assert other.decrypt(encrypted) != PLAINTEXT
        except ValueError:
            # Unpadding a payload decrypted under the wrong key usually fails
            # outright, which is an equally acceptable outcome here.
            pass

    def test_key_derivation_is_deterministic(self):
        """The same passphrase derives the same key, so ciphertext survives a restart."""
        encrypted = CryptoUtil(algorithm="aes-256-cbc", key="test-key").encrypt(PLAINTEXT)

        assert CryptoUtil(algorithm="aes-256-cbc", key="test-key").decrypt(encrypted) == PLAINTEXT

    @pytest.mark.parametrize(
        ("cls", "key_length"),
        [(AES128CBC, 16), (AES256CBC, 32), (SM4CBC, 16)],
    )
    def test_derived_key_length(self, cls, key_length):
        """Each algorithm derives the key length its cipher requires."""
        assert len(cls(key="test-key").key) == key_length


class TestDecryptPassthrough:
    """decrypt() returns data that lacks the magic header unchanged.

    This is what lets an operator turn encryption on over an existing bucket, and
    it is also why an unencrypted write is invisible afterwards -- see
    TestInitSettingsCrypto.
    """

    def test_returns_unencrypted_data_as_is(self):
        """Data written before encryption was enabled is still readable."""
        crypto = CryptoUtil(algorithm="aes-256-cbc", key="test-key")

        assert crypto.decrypt(PLAINTEXT) == PLAINTEXT


@pytest.fixture(autouse=True)
def _clear_crypto_env(monkeypatch):
    """Keep an ambient RAGFLOW_CRYPTO_* value from steering these tests."""
    for name in ("RAGFLOW_CRYPTO_ENABLED", "RAGFLOW_CRYPTO_ALGORITHM", "RAGFLOW_CRYPTO_KEY"):
        monkeypatch.delenv(name, raising=False)


class TestEncryptedStorageWrapper:
    """Test cases for the transparent encryption wrapper."""

    def test_put_encrypts_at_rest(self):
        """The underlying storage never sees the plaintext."""
        backing = FakeStorage()
        wrapper = create_encrypted_storage(backing, key="test-key")

        wrapper.put("bucket", "doc.txt", PLAINTEXT)

        stored = backing.blobs[("bucket", "doc.txt")]
        assert stored.startswith(b"RAGF")
        assert PLAINTEXT not in stored

    def test_get_decrypts(self):
        """A round trip through the wrapper returns the original bytes."""
        wrapper = create_encrypted_storage(FakeStorage(), key="test-key")
        wrapper.put("bucket", "doc.txt", PLAINTEXT)

        assert wrapper.get("bucket", "doc.txt") == PLAINTEXT

    def test_get_missing_object_returns_none(self):
        """A missing object is reported as None instead of failing to decrypt."""
        wrapper = create_encrypted_storage(FakeStorage(), key="test-key")

        assert wrapper.get("bucket", "absent.txt") is None

    def test_disabled_encryption_stores_plaintext(self):
        """With encryption disabled the wrapper is a passthrough."""
        backing = FakeStorage()
        wrapper = create_encrypted_storage(backing, key="test-key", encryption_enabled=False)

        wrapper.put("bucket", "doc.txt", PLAINTEXT)

        assert backing.blobs[("bucket", "doc.txt")] == PLAINTEXT
        assert wrapper.get("bucket", "doc.txt") == PLAINTEXT

    def test_requires_storage_methods(self):
        """A storage impl missing a required method is refused at construction."""

        class Incomplete:
            def put(self, *args, **kwargs):
                pass

        with pytest.raises(AttributeError, match="missing required method"):
            EncryptedStorageWrapper(Incomplete(), key="test-key")

    def test_propagates_missing_key(self, monkeypatch):
        """A broken crypto config surfaces as an error, not a plaintext wrapper."""
        monkeypatch.delenv("RAGFLOW_CRYPTO_KEY", raising=False)

        with pytest.raises(ValueError, match="Encryption key not provided"):
            create_encrypted_storage(FakeStorage(), key=None)

    def test_defaults_to_aes_256_cbc(self):
        """The documented algorithm default applies when none is passed."""
        wrapper = create_encrypted_storage(FakeStorage(), key="test-key")

        assert wrapper.crypto.algorithm_name == "aes-256-cbc"

    def test_reads_config_from_environment(self, monkeypatch):
        """Both docstrings promise the RAGFLOW_CRYPTO_* variables as fallbacks."""
        monkeypatch.setenv("RAGFLOW_CRYPTO_ALGORITHM", "sm4-cbc")
        monkeypatch.setenv("RAGFLOW_CRYPTO_KEY", "key-from-env")

        wrapper = create_encrypted_storage(FakeStorage())

        assert wrapper.crypto.algorithm_name == "sm4-cbc"
        assert wrapper.get("bucket", "absent.txt") is None
        wrapper.put("bucket", "doc.txt", PLAINTEXT)
        assert wrapper.get("bucket", "doc.txt") == PLAINTEXT

    def test_explicit_arguments_win_over_environment(self, monkeypatch):
        """An explicit algorithm is not overridden by the environment."""
        monkeypatch.setenv("RAGFLOW_CRYPTO_ALGORITHM", "sm4-cbc")

        wrapper = create_encrypted_storage(FakeStorage(), algorithm="aes-128-cbc", key="test-key")

        assert wrapper.crypto.algorithm_name == "aes-128-cbc"


class TestInitSettingsCrypto:
    """The crypto branch of init_settings must not fall back to plaintext.

    ``RAGFLOW_CRYPTO_ENABLED=true`` is a request for encryption at rest. If the
    key or algorithm is wrong, wrapping must fail loudly: reads keep working
    either way (decrypt() passes unencrypted data through), so an operator has no
    way to notice that documents are being written in the clear.
    """

    @staticmethod
    def _init_storage(monkeypatch, env):
        """Run the RAGFLOW_CRYPTO_* branch of init_settings against a fake impl.

        Mirrors common/settings.py rather than calling init_settings(), which
        also connects to MySQL, Redis and the doc store.
        """
        import os

        for name in ("RAGFLOW_CRYPTO_ENABLED", "RAGFLOW_CRYPTO_ALGORITHM", "RAGFLOW_CRYPTO_KEY"):
            monkeypatch.delenv(name, raising=False)
        for name, value in env.items():
            monkeypatch.setenv(name, value)

        storage_impl = FakeStorage()
        if os.environ.get("RAGFLOW_CRYPTO_ENABLED", "false").lower() != "true":
            return storage_impl

        return create_encrypted_storage(
            storage_impl,
            algorithm=os.environ.get("RAGFLOW_CRYPTO_ALGORITHM", "aes-256-cbc"),
            key=os.environ.get("RAGFLOW_CRYPTO_KEY"),
            encryption_enabled=True,
        )

    def test_enabled_with_valid_config_encrypts(self, monkeypatch):
        """The happy path wraps the storage impl."""
        storage = self._init_storage(
            monkeypatch,
            {"RAGFLOW_CRYPTO_ENABLED": "true", "RAGFLOW_CRYPTO_KEY": "test-key"},
        )

        storage.put("bucket", "doc.txt", PLAINTEXT)

        assert storage.storage_impl.blobs[("bucket", "doc.txt")].startswith(b"RAGF")

    def test_disabled_leaves_storage_unwrapped(self, monkeypatch):
        """Without RAGFLOW_CRYPTO_ENABLED the storage impl is used directly."""
        storage = self._init_storage(monkeypatch, {})

        assert isinstance(storage, FakeStorage)

    @pytest.mark.parametrize(
        ("env", "match"),
        [
            ({"RAGFLOW_CRYPTO_ENABLED": "true"}, "Encryption key not provided"),
            ({"RAGFLOW_CRYPTO_ENABLED": "true", "RAGFLOW_CRYPTO_KEY": ""}, "Encryption key not provided"),
            (
                {
                    "RAGFLOW_CRYPTO_ENABLED": "true",
                    "RAGFLOW_CRYPTO_KEY": "test-key",
                    "RAGFLOW_CRYPTO_ALGORITHM": "aes256-cbc",
                },
                "Unsupported algorithm",
            ),
        ],
        ids=["key_unset", "key_empty", "algorithm_typo"],
    )
    def test_enabled_with_broken_config_raises(self, monkeypatch, env, match):
        """A broken crypto config aborts startup instead of writing plaintext."""
        with pytest.raises(ValueError, match=match):
            self._init_storage(monkeypatch, env)

    def test_unencrypted_write_stays_invisible(self, monkeypatch):
        """Why the fallback has to go: a plaintext object reads back fine forever.

        Once documents are written unencrypted, fixing the configuration does not
        surface them -- decrypt() passes anything without the magic header
        through, so both objects read back correctly and nothing flags the leak.
        """
        wrapper = self._init_storage(
            monkeypatch,
            {"RAGFLOW_CRYPTO_ENABLED": "true", "RAGFLOW_CRYPTO_KEY": "test-key"},
        )
        backing = wrapper.storage_impl

        # A leak from an earlier boot whose crypto config was broken.
        backing.put("bucket", "leaked.txt", PLAINTEXT)
        wrapper.put("bucket", "safe.txt", PLAINTEXT)

        assert not backing.blobs[("bucket", "leaked.txt")].startswith(b"RAGF")
        assert backing.blobs[("bucket", "safe.txt")].startswith(b"RAGF")
        # Both read back correctly, so the leak never surfaces.
        assert wrapper.get("bucket", "leaked.txt") == PLAINTEXT
        assert wrapper.get("bucket", "safe.txt") == PLAINTEXT
