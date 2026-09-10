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

from common import config_utils
from common.config_utils import decrypt_database_config


class TestDecryptDatabaseConfig:
    def test_missing_password_key_does_not_raise(self):
        """A config section without `password` must not crash (regression #11051)."""
        database = {"user": "root", "host": "localhost"}
        result = decrypt_database_config(database=database)
        # Returns the section untouched and does NOT fabricate a password key.
        assert result == {"user": "root", "host": "localhost"}
        assert "password" not in result

    def test_explicit_empty_dict_is_honoured(self):
        """An explicitly supplied empty dict must not fall back to global config."""
        result = decrypt_database_config(database={})
        assert result == {}

    def test_none_database_falls_back_to_global_config(self, monkeypatch):
        """When no database is passed, the global config is used."""
        monkeypatch.setattr(
            config_utils,
            "get_base_config",
            lambda name, default=None: {"user": "root", "host": "localhost"},
        )
        result = decrypt_database_config(name="mysql")
        assert result == {"user": "root", "host": "localhost"}
        assert "password" not in result

    def test_none_database_with_missing_section(self, monkeypatch):
        """A missing config section resolves to an empty dict, not a crash."""
        monkeypatch.setattr(
            config_utils,
            "get_base_config",
            lambda name, default=None: default,
        )
        result = decrypt_database_config(name="nonexistent")
        assert result == {}

    def test_password_is_decrypted(self, monkeypatch):
        """When `password` is present it is passed through the decryptor."""
        monkeypatch.setattr(
            config_utils,
            "decrypt_database_password",
            lambda password: f"decrypted::{password}",
        )
        database = {"user": "root", "password": "secret"}
        result = decrypt_database_config(database=database)
        assert result["password"] == "decrypted::secret"
        assert result["user"] == "root"

    def test_empty_password_value_is_preserved(self, monkeypatch):
        """An empty password string is a valid value and must round-trip."""
        monkeypatch.setattr(
            config_utils,
            "decrypt_database_password",
            lambda password: password,
        )
        database = {"user": "root", "password": ""}
        result = decrypt_database_config(database=database)
        assert result["password"] == ""

    def test_custom_passwd_key(self, monkeypatch):
        """A non-default passwd_key is respected for both presence check and decode."""
        monkeypatch.setattr(
            config_utils,
            "decrypt_database_password",
            lambda password: f"decrypted::{password}",
        )
        # Missing custom key -> untouched.
        assert decrypt_database_config(database={"user": "root"}, passwd_key="secret_key") == {"user": "root"}
        # Present custom key -> decrypted.
        result = decrypt_database_config(database={"secret_key": "abc"}, passwd_key="secret_key")
        assert result["secret_key"] == "decrypted::abc"
