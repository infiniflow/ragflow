"""Tests for data-source connector construction."""

import pytest

from common import settings  # noqa: F401 - initialize shared settings before connector imports
from common.constants import FileSource
from common.data_source import BlobStorageConnector, build_connector_for_source


def test_s3_connector_uses_configured_compatible_bucket_type(monkeypatch):
    captured = {}
    expected = object()

    def build_connector(config, *, bucket_type):
        captured["config"] = config
        captured["bucket_type"] = bucket_type
        return expected

    monkeypatch.setattr(BlobStorageConnector, "build_connector", build_connector)
    config = {"bucket_name": "database", "bucket_type": "s3_compatible"}

    connector = build_connector_for_source(FileSource.S3, config)

    assert connector is expected
    assert captured == {"config": config, "bucket_type": "s3_compatible"}


def test_s3_connector_defaults_bucket_type_to_source(monkeypatch):
    captured = {}

    def build_connector(_config, *, bucket_type):
        captured["bucket_type"] = bucket_type
        return object()

    monkeypatch.setattr(BlobStorageConnector, "build_connector", build_connector)

    build_connector_for_source(FileSource.S3, {"bucket_name": "database"})

    assert captured["bucket_type"] == FileSource.S3


@pytest.mark.parametrize("configured_bucket_type", [None, ""])
def test_s3_connector_defaults_empty_bucket_type_to_source(monkeypatch, configured_bucket_type):
    captured = {}

    def build_connector(_config, *, bucket_type):
        captured["bucket_type"] = bucket_type
        return object()

    monkeypatch.setattr(BlobStorageConnector, "build_connector", build_connector)

    build_connector_for_source(FileSource.S3, {"bucket_name": "database", "bucket_type": configured_bucket_type})

    assert captured["bucket_type"] == FileSource.S3
