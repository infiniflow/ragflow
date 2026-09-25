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
"""Connector.config encrypts credentials on every write and never decrypts on read.

Reads keep the stored ciphertext so that access checks and deletes still work
when the key is lost. The backfill in migrate_db encrypts rows written before
the key was set.
"""

import json

import pytest
from peewee import SqliteDatabase

from api.db import db_models
from api.db.db_models import DB, Connector, ConnectorConfigField, JSONField, migrate_connector_credentials
from common.connector_credentials import decrypt_connector_config

# Bytes 0..31, the same key the Go tests use.
_KEY = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="
_VECTOR = "enc:v1:ZGVmZ2hpamtsbW5vMzm/FhC2IvFVBzHK4EVIiS2pKzu5X9Feh/PZO57Rh3K0yyGkLTDmruUEeZB6LtRoLedehJBJUg=="
_PLAIN = {"credentials": {"api_token": "tok-123", "user": "ada"}, "sync_deleted_files": True}


@pytest.fixture
def key(monkeypatch):
    monkeypatch.setenv("RAGFLOW_CONNECTOR_KEY", _KEY)


@pytest.fixture
def no_key(monkeypatch):
    monkeypatch.delenv("RAGFLOW_CONNECTOR_KEY", raising=False)


@pytest.fixture
def sqlite_db():
    db = SqliteDatabase(":memory:")
    db.bind([Connector], bind_refs=False, bind_backrefs=False)
    db.connect()
    db.create_tables([Connector])
    try:
        yield db
    finally:
        db.drop_tables([Connector])
        db.close()
        DB.bind([Connector], bind_refs=False, bind_backrefs=False)


def _create(connector_id, config):
    Connector.create(id=connector_id, tenant_id="tenant-1", name=connector_id, source="rss", input_type="poll", config=config)


def _raw(db, connector_id):
    return db.execute_sql("SELECT config FROM connector WHERE id = ?", (connector_id,)).fetchone()[0]


def _set_raw(db, connector_id, raw):
    db.execute_sql("UPDATE connector SET config = ? WHERE id = ?", (raw, connector_id))


class TestConnectorConfigField:
    @pytest.mark.p2
    def test_db_value_encrypts_credentials_without_touching_the_input(self, key):
        config = json.loads(json.dumps(_PLAIN))

        stored = json.loads(ConnectorConfigField().db_value(config))

        assert config == _PLAIN
        assert stored["credentials"].startswith("enc:v1:")
        assert stored["sync_deleted_files"] is True
        assert decrypt_connector_config(stored) == _PLAIN

    @pytest.mark.p2
    @pytest.mark.parametrize("value", [_PLAIN, None, {"sync_deleted_files": True}])
    def test_db_value_without_a_key_matches_json_field(self, no_key, value):
        assert ConnectorConfigField().db_value(value) == JSONField().db_value(value)

    @pytest.mark.p2
    def test_python_value_returns_the_ciphertext_as_stored(self, key):
        stored = json.dumps({"credentials": _VECTOR, "x": 1})
        assert ConnectorConfigField().python_value(stored) == {"credentials": _VECTOR, "x": 1}

    @pytest.mark.p2
    def test_connector_config_uses_the_encrypting_field(self):
        assert isinstance(Connector.config, ConnectorConfigField)


class TestConnectorConfigWrites:
    @pytest.mark.p2
    def test_insert_and_update_store_encrypted_credentials(self, sqlite_db, key):
        _create("conn-1", _PLAIN)
        inserted = json.loads(_raw(sqlite_db, "conn-1"))
        assert inserted["credentials"].startswith("enc:v1:")

        # The same call CommonService.update_by_id makes.
        new_config = {"credentials": {"api_token": "tok-456"}}
        Connector.update({"config": new_config}).where(Connector.id == "conn-1").execute()

        updated = json.loads(_raw(sqlite_db, "conn-1"))
        assert updated["credentials"].startswith("enc:v1:")
        assert decrypt_connector_config(updated) == {"credentials": {"api_token": "tok-456"}}
        assert new_config == {"credentials": {"api_token": "tok-456"}}

    @pytest.mark.p2
    def test_loaded_row_keeps_the_ciphertext(self, sqlite_db, key):
        _create("conn-1", _PLAIN)
        loaded = Connector.get_by_id("conn-1").config
        assert loaded["credentials"].startswith("enc:v1:")
        assert decrypt_connector_config(loaded) == _PLAIN


class TestMigrateConnectorCredentials:
    @pytest.mark.p2
    def test_encrypts_plaintext_rows_once(self, sqlite_db, monkeypatch):
        monkeypatch.delenv("RAGFLOW_CONNECTOR_KEY", raising=False)
        _create("conn-plain", _PLAIN)
        _create("conn-empty", {"sync_deleted_files": True})
        empty_raw = _raw(sqlite_db, "conn-empty")

        monkeypatch.setenv("RAGFLOW_CONNECTOR_KEY", _KEY)
        migrate_connector_credentials()

        migrated_raw = _raw(sqlite_db, "conn-plain")
        migrated = json.loads(migrated_raw)
        assert migrated["credentials"].startswith("enc:v1:")
        assert decrypt_connector_config(migrated) == _PLAIN
        assert _raw(sqlite_db, "conn-empty") == empty_raw

        def _no_write(*_args, **_kwargs):
            raise AssertionError("a second run must not write any row")

        monkeypatch.setattr(Connector, "update", _no_write)
        migrate_connector_credentials()
        assert _raw(sqlite_db, "conn-plain") == migrated_raw

    @pytest.mark.p2
    def test_without_a_key_reads_and_changes_nothing(self, sqlite_db, no_key, monkeypatch):
        _create("conn-1", _PLAIN)
        before = _raw(sqlite_db, "conn-1")

        def _scan(*_args, **_kwargs):
            raise AssertionError("every startup with encryption off would scan all connectors")

        monkeypatch.setattr(Connector, "select", _scan)
        migrate_connector_credentials()

        assert _raw(sqlite_db, "conn-1") == before

    @pytest.mark.p2
    @pytest.mark.parametrize("raw", ["not json", '"credentials"', '["credentials"]'])
    def test_skips_rows_that_are_not_a_json_object(self, sqlite_db, key, raw):
        _create("conn-1", {})
        _set_raw(sqlite_db, "conn-1", raw)

        migrate_connector_credentials()

        assert _raw(sqlite_db, "conn-1") == raw

    @pytest.mark.p2
    def test_leaves_a_row_that_changed_after_it_was_read(self, sqlite_db, key, monkeypatch):
        concurrent_raw = json.dumps({"credentials": _VECTOR})
        _create("conn-1", {})
        _set_raw(sqlite_db, "conn-1", json.dumps(_PLAIN))
        encrypt = db_models.encrypt_connector_config
        writes = []

        def _encrypt_after_a_concurrent_write(config):
            if not writes:
                writes.append(config)
                _set_raw(sqlite_db, "conn-1", concurrent_raw)
            return encrypt(config)

        monkeypatch.setattr(db_models, "encrypt_connector_config", _encrypt_after_a_concurrent_write)
        migrate_connector_credentials()

        assert writes == [_PLAIN]
        assert _raw(sqlite_db, "conn-1") == concurrent_raw
