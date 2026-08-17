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

import json
import types
from datetime import date, datetime, time, timezone
from decimal import Decimal

import pytest

from common.data_source import bigquery_connector as bq_mod
from common.data_source.bigquery_connector import BigQueryConnector
from common.data_source.config import DocumentSource
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
)


# ---------------------------------------------------------------------------
# Fakes for the google-cloud-bigquery client surface
# ---------------------------------------------------------------------------


class _FakeSchemaField:
    def __init__(self, name, field_type):
        self.name = name
        self.field_type = field_type


class _FakeScalarQueryParameter:
    def __init__(self, name, type_, value):
        self.name = name
        self.type_ = type_
        self.value = value


class _FakeQueryJobConfig:
    def __init__(self):
        self.use_legacy_sql = None
        self.use_query_cache = None
        self.maximum_bytes_billed = None
        self.job_timeout_ms = None
        self.query_parameters = None
        self.dry_run = False


class _FakeJob:
    def __init__(self, rows=None, schema=None, total_bytes=123):
        self._rows = rows if rows is not None else []
        self.schema = schema or []
        self.total_bytes_processed = total_bytes

    def result(self, page_size=None):
        return iter(self._rows)


class _FakeClient:
    def __init__(self, rows=None, result_schema=None, table_schema=None):
        self._rows = rows if rows is not None else []
        self._result_schema = result_schema or []
        self._table_schema = table_schema or []
        self.queries = []

    def query(self, query, job_config=None, location=None):
        self.queries.append((query, job_config, location))
        return _FakeJob(rows=self._rows, schema=self._result_schema)

    def get_table(self, ref):
        return types.SimpleNamespace(schema=self._table_schema)


@pytest.fixture(autouse=True)
def _patch_bigquery(monkeypatch):
    fake = types.SimpleNamespace(
        QueryJobConfig=_FakeQueryJobConfig,
        ScalarQueryParameter=_FakeScalarQueryParameter,
    )
    monkeypatch.setattr(bq_mod, "bigquery", fake)
    monkeypatch.setattr(bq_mod, "service_account", types.SimpleNamespace())


SERVICE_ACCOUNT = {"type": "service_account", "project_id": "p"}


def _make_connector(client=None, **overrides):
    kwargs = dict(
        project_id="my-proj",
        dataset_id="ds",
        table_id="tbl",
        content_columns="name,description",
        metadata_columns="status",
        id_column="id",
        timestamp_column=None,
        batch_size=2,
    )
    kwargs.update(overrides)
    connector = BigQueryConnector(**kwargs)
    connector._credentials = {"service_account_info": SERVICE_ACCOUNT}
    if client is not None:
        connector._client = client
    return connector


# ---------------------------------------------------------------------------
# Credentials
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_load_credentials_accepts_json_string():
    connector = BigQueryConnector(project_id="p", content_columns="name")
    connector.load_credentials({"service_account_json": json.dumps(SERVICE_ACCOUNT)})
    assert connector._credentials["service_account_info"] == SERVICE_ACCOUNT


@pytest.mark.p2
def test_load_credentials_accepts_dict():
    connector = BigQueryConnector(project_id="p", content_columns="name")
    connector.load_credentials({"service_account_json": SERVICE_ACCOUNT})
    assert connector._credentials["service_account_info"] == SERVICE_ACCOUNT


@pytest.mark.p2
def test_missing_credentials_raises():
    connector = BigQueryConnector(project_id="p", content_columns="name")
    with pytest.raises(ConnectorMissingCredentialError):
        connector.load_credentials({})


@pytest.mark.p2
def test_invalid_credentials_json_raises():
    connector = BigQueryConnector(project_id="p", content_columns="name")
    with pytest.raises(ConnectorMissingCredentialError):
        connector.load_credentials({"service_account_json": "{not-json"})


# ---------------------------------------------------------------------------
# Query construction
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_table_query_quotes_identifiers():
    connector = _make_connector()
    assert connector._build_base_query() == "SELECT * FROM `my-proj.ds.tbl`"


@pytest.mark.p2
def test_custom_query_strips_trailing_semicolon():
    connector = _make_connector(query="SELECT * FROM t WHERE x = 1;")
    assert connector._build_base_query() == "SELECT * FROM t WHERE x = 1"


@pytest.mark.p2
def test_missing_table_and_query_raises():
    connector = _make_connector(dataset_id="", table_id="", query="")
    with pytest.raises(ConnectorValidationError):
        connector._build_base_query()


@pytest.mark.p2
def test_time_filtered_query_compound_cursor_with_id_column():
    client = _FakeClient(table_schema=[_FakeSchemaField("updated_at", "TIMESTAMP"), _FakeSchemaField("id", "STRING")])
    connector = _make_connector(client=client, timestamp_column="updated_at", id_column="id")
    start = datetime(2026, 1, 1, tzinfo=timezone.utc)
    end = datetime(2026, 2, 1, tzinfo=timezone.utc)

    query, params = connector._build_time_filtered_query(connector._build_base_query(), start, end, start_id="last-id")

    assert "(ragflow_src.updated_at > @start_cursor OR (ragflow_src.updated_at = @start_cursor AND ragflow_src.id > @start_cursor_id))" in query
    assert "ragflow_src.updated_at <= @end_cursor" in query
    assert [(p.name, p.type_, p.value) for p in params] == [
        ("start_cursor", "TIMESTAMP", start),
        ("start_cursor_id", "STRING", "last-id"),
        ("end_cursor", "TIMESTAMP", end),
    ]


@pytest.mark.p2
def test_time_filtered_query_uses_gte_without_id_column():
    client = _FakeClient(table_schema=[_FakeSchemaField("updated_at", "TIMESTAMP")])
    connector = _make_connector(client=client, timestamp_column="updated_at", id_column=None)
    start = datetime(2026, 1, 1, tzinfo=timezone.utc)
    end = datetime(2026, 2, 1, tzinfo=timezone.utc)

    query, params = connector._build_time_filtered_query(connector._build_base_query(), start, end)

    assert "ragflow_src.updated_at >= @start_cursor" in query
    assert "ragflow_src.updated_at <= @end_cursor" in query
    assert [(p.name, p.type_, p.value) for p in params] == [
        ("start_cursor", "TIMESTAMP", start),
        ("end_cursor", "TIMESTAMP", end),
    ]


@pytest.mark.p2
def test_cursor_param_type_resolves_for_date_and_int():
    date_client = _FakeClient(table_schema=[_FakeSchemaField("d", "DATE")])
    int_client = _FakeClient(table_schema=[_FakeSchemaField("n", "INTEGER")])

    date_conn = _make_connector(client=date_client, timestamp_column="d")
    int_conn = _make_connector(client=int_client, timestamp_column="n")

    assert date_conn._resolve_cursor_param_type() == "DATE"
    assert int_conn._resolve_cursor_param_type() == "INT64"


@pytest.mark.p2
def test_unsupported_cursor_type_raises():
    client = _FakeClient(table_schema=[_FakeSchemaField("ts", "BOOL")])
    connector = _make_connector(client=client, timestamp_column="ts")
    with pytest.raises(ConnectorValidationError):
        connector._resolve_cursor_param_type()


@pytest.mark.p2
def test_slim_query_uses_id_column():
    connector = _make_connector()
    slim = connector._build_slim_query(connector._build_base_query())
    assert "ragflow_src.id" in slim


# ---------------------------------------------------------------------------
# Value rendering & row conversion
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_value_serialization_types():
    assert BigQueryConnector._render_content_value(Decimal("1.50")) == "1.50"
    assert BigQueryConnector._render_content_value(b"binary") is None
    assert BigQueryConnector._render_content_value(date(2026, 1, 2)) == "2026-01-02"
    assert BigQueryConnector._render_content_value(time(3, 4, 5)) == "03:04:05"
    assert BigQueryConnector._render_content_value({"a": 1, "b": [1, 2]}) == '{"a": 1, "b": [1, 2]}'
    assert BigQueryConnector._render_content_value("POINT(1 2)") == "POINT(1 2)"  # GEOGRAPHY WKT passes through

    # Metadata base64-encodes bytes instead of skipping.
    assert BigQueryConnector._render_metadata_value(b"hi") == "aGk="


@pytest.mark.p2
def test_row_to_document_content_metadata_id_timestamp():
    connector = _make_connector(timestamp_column="updated_at")
    ts = datetime(2026, 3, 4, 5, 6, 7, tzinfo=timezone.utc)
    row = {
        "id": 42,
        "name": "Acme",
        "description": "A company",
        "status": "active",
        "updated_at": ts,
    }

    doc = connector._row_to_document(row)

    assert doc.id == "bigquery:my-proj:ds.tbl:42"
    assert "【name】:\nAcme" in doc.blob.decode("utf-8")
    assert "【description】:\nA company" in doc.blob.decode("utf-8")
    assert doc.metadata == {"status": "active"}
    assert doc.source == DocumentSource.BIGQUERY
    assert doc.extension == ".txt"
    assert doc.doc_updated_at == ts
    assert doc.semantic_identifier == "Acme"


@pytest.mark.p2
def test_document_id_falls_back_to_content_hash_without_id_column():
    connector = _make_connector(id_column=None)
    row = {"name": "Acme", "description": "A company", "status": "active"}
    doc_id = connector._build_document_id_from_row(row)
    assert doc_id.startswith("bigquery:my-proj:ds.tbl:")
    assert len(doc_id.rsplit(":", 1)[1]) == 32  # md5 hex


@pytest.mark.p2
def test_custom_query_mode_id_prefix():
    connector = _make_connector(dataset_id="", table_id="", query="SELECT * FROM t")
    row = {"id": 7, "name": "x"}
    assert connector._build_document_id_from_row(row) == "bigquery:my-proj:query:7"


# ---------------------------------------------------------------------------
# Batching & cursor round-trip
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_batches_accumulate_to_batch_size():
    rows = [{"id": i, "name": f"n{i}", "description": "d", "status": "s"} for i in range(5)]
    client = _FakeClient(rows=rows)
    connector = _make_connector(client=client, batch_size=2)

    batches = list(connector.load_from_state())

    assert [len(b) for b in batches] == [2, 2, 1]


@pytest.mark.p2
def test_cursor_serialize_deserialize_roundtrip():
    dt = datetime(2026, 1, 1, 12, 0, tzinfo=timezone.utc)
    d = date(2026, 1, 1)
    t = time(12, 0)
    dec = Decimal("1.23")

    assert BigQueryConnector.deserialize_cursor_value(BigQueryConnector.serialize_cursor_value(dt)) == dt
    assert BigQueryConnector.deserialize_cursor_value(BigQueryConnector.serialize_cursor_value(d)) == d
    assert BigQueryConnector.deserialize_cursor_value(BigQueryConnector.serialize_cursor_value(t)) == t
    assert BigQueryConnector.deserialize_cursor_value(BigQueryConnector.serialize_cursor_value(dec)) == dec
    assert BigQueryConnector.serialize_cursor_value(42) == 42
    assert BigQueryConnector.deserialize_cursor_value(42) == 42


@pytest.mark.p2
def test_load_from_cursor_range_empty_without_end():
    connector = _make_connector(timestamp_column="updated_at")
    assert list(connector.load_from_cursor_range(start_value=None, end_value=None)) == []


@pytest.mark.p2
def test_load_from_cursor_range_empty_when_end_not_after_start():
    connector = _make_connector(timestamp_column="updated_at")
    start = datetime(2026, 2, 1, tzinfo=timezone.utc)
    end = datetime(2026, 1, 1, tzinfo=timezone.utc)
    assert list(connector.load_from_cursor_range(start, end)) == []


# ---------------------------------------------------------------------------
# Slim docs & validation
# ---------------------------------------------------------------------------


@pytest.mark.p2
def test_slim_docs_use_id_column():
    rows = [{"id": 1}, {"id": 2}]
    client = _FakeClient(rows=rows)
    connector = _make_connector(client=client, batch_size=10)

    slim_batches = list(connector.retrieve_all_slim_docs_perm_sync())
    ids = [doc.id for batch in slim_batches for doc in batch]

    assert ids == ["bigquery:my-proj:ds.tbl:1", "bigquery:my-proj:ds.tbl:2"]


@pytest.mark.p2
def test_validation_detects_missing_content_column():
    client = _FakeClient(table_schema=[_FakeSchemaField("other", "STRING")])
    connector = _make_connector(client=client, content_columns="name")
    with pytest.raises(ConnectorValidationError, match="name"):
        connector.validate_connector_settings()


@pytest.mark.p2
def test_validation_detects_missing_metadata_column():
    client = _FakeClient(table_schema=[_FakeSchemaField("name", "STRING")])
    connector = _make_connector(client=client, content_columns="name", metadata_columns="status")
    with pytest.raises(ConnectorValidationError, match="status"):
        connector.validate_connector_settings()


@pytest.mark.p2
def test_validation_detects_missing_id_column():
    client = _FakeClient(table_schema=[_FakeSchemaField("name", "STRING")])
    connector = _make_connector(client=client, content_columns="name", metadata_columns="", id_column="id")
    with pytest.raises(ConnectorValidationError, match="id"):
        connector.validate_connector_settings()


@pytest.mark.p2
def test_validation_detects_missing_timestamp_column():
    client = _FakeClient(table_schema=[_FakeSchemaField("name", "STRING")])
    connector = _make_connector(client=client, content_columns="name", metadata_columns="", id_column="", timestamp_column="ts")
    with pytest.raises(ConnectorValidationError, match="ts"):
        connector.validate_connector_settings()


@pytest.mark.p2
def test_validation_detects_unsupported_cursor_type_early():
    client = _FakeClient(table_schema=[_FakeSchemaField("name", "STRING"), _FakeSchemaField("ts", "BOOL")])
    connector = _make_connector(client=client, content_columns="name", metadata_columns="", id_column="", timestamp_column="ts")
    with pytest.raises(ConnectorValidationError, match="not supported as a cursor"):
        connector.validate_connector_settings()


@pytest.mark.p2
def test_validation_missing_credentials_raises():
    connector = BigQueryConnector(project_id="p", content_columns="name")
    with pytest.raises(ConnectorMissingCredentialError):
        connector.validate_connector_settings()


@pytest.mark.p2
def test_validation_dry_run_failure_becomes_validation_error():
    class _FailingClient(_FakeClient):
        def query(self, query, job_config=None, location=None):
            raise RuntimeError("bad query / would scan too much")

    connector = _make_connector(client=_FailingClient())
    with pytest.raises(ConnectorValidationError):
        connector.validate_connector_settings()
