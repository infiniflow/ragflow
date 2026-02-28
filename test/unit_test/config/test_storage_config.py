import pytest
from unittest.mock import patch
from core.config import AppConfig
from core.config.types import ObjectStorageType

# ------------------------
# Defaults & Active Override
# ------------------------

def test_storage_defaults(monkeypatch):
    monkeypatch.delenv("STORAGE_IMPL", raising=False)
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()
    assert cfg.storage.active == ObjectStorageType.MINIO


def test_storage_active_override(monkeypatch):
    monkeypatch.setenv("STORAGE_IMPL", "oss")
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()
    assert cfg.storage.active == ObjectStorageType.OSS


# ------------------------
# MinIO
# ------------------------

def test_minio_defaults(monkeypatch):
    monkeypatch.setenv("STORAGE_IMPL", "minio")
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{}, {}]
        cfg = AppConfig()

    minio_cfg = cfg.storage.minio
    assert minio_cfg.host == "localhost:9000"
    assert minio_cfg.user == "rag_flow"
    assert minio_cfg.password is None
    assert minio_cfg.bucket is None  # empty_to_none validator
    assert minio_cfg.prefix_path == ""


def test_minio_override(monkeypatch):
    monkeypatch.delenv("STORAGE_IMPL", raising=False)
    override = {
        "host": "127.0.0.1",
        "user": "minio",
        "password": "minio",
        "bucket": "minio",
        "prefix_path": "minio",
    }
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{"storage": {"active": "minio", "minio": override}}, {}]
        cfg = AppConfig()

    minio_cfg = cfg.storage.minio
    for k, v in override.items():
        assert getattr(minio_cfg, k) == v


# ------------------------
# OSS
# ------------------------

def test_oss_defaults(monkeypatch):
    monkeypatch.setenv("STORAGE_IMPL", "oss")
    with patch("core.config.app.load_yaml", return_value={}):
        cfg = AppConfig()
    oss_cfg = cfg.storage.oss
    assert cfg.storage.active == ObjectStorageType.OSS
    assert oss_cfg.access_key == ""
    assert oss_cfg.secret_key == ""
    assert oss_cfg.endpoint_url == ""
    assert oss_cfg.region == ""
    assert oss_cfg.bucket == ""


def test_oss_override(monkeypatch):
    monkeypatch.delenv("STORAGE_IMPL", raising=False)
    override = {
        "access_key": "myak",
        "secret_key": "mysk",
        "endpoint_url": "http://oss.example.com",
        "region": "cn-shanghai",
        "bucket": "mybucket",
    }
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{"storage": {"active": "oss", "oss": override}}, {}]
        cfg = AppConfig()

    oss_cfg = cfg.storage.oss
    for k, v in override.items():
        assert getattr(oss_cfg, k) == v


# ------------------------
# Generic current / active type test
# ------------------------

@pytest.mark.parametrize(
    "impl, enum_val",
    [
        ("minio", ObjectStorageType.MINIO),
        ("s3", ObjectStorageType.S3),
        ("oss", ObjectStorageType.OSS),
        ("azure_sas", ObjectStorageType.AZURE_SAS),
        ("azure_spn", ObjectStorageType.AZURE_SPN),
        ("opendal", ObjectStorageType.OPENDAL),
    ]
)
def test_storage_active_types(impl, enum_val, monkeypatch):
    monkeypatch.setenv("STORAGE_IMPL", impl)
    with patch("core.config.app.load_yaml", return_value={}):
        cfg = AppConfig()
    assert cfg.storage.active == enum_val


@pytest.mark.parametrize(
    "key, override",
    [
        ("s3", {"access_key": "ak", "secret_key": "sk", "bucket": "b"}),
        ("oss", {"access_key": "ak", "secret_key": "sk", "bucket": "b"}),
        ("azure_sas", {"container_url": "url", "sas_token": "token"}),
        ("azure_spn", {"account_url": "url", "client_id": "id", "secret": "sec",
                       "tenant_id": "tid", "container_name": "c"}),
        ("opendal", {"scheme": "mysql", "config": {"oss_table": "table"}}),
    ]
)
def test_storage_yaml_override(key, override, monkeypatch):
    monkeypatch.delenv("STORAGE_IMPL", raising=False)
    with patch("core.config.app.load_yaml") as mock_load:
        mock_load.side_effect = [{"storage": {key: override}}, {}]
        cfg = AppConfig()
    storage_cfg = getattr(cfg.storage, key)
    for k, v in override.items():
        assert getattr(storage_cfg, k) == v


@pytest.mark.parametrize(
    "impl",
    ["minio", "s3", "oss", "azure_sas", "azure_spn", "opendal"]
)
def test_storage_current(impl, monkeypatch):
    monkeypatch.setenv("STORAGE_IMPL", impl)
    with patch("core.config.app.load_yaml", return_value={}):
        cfg = AppConfig()
    current = cfg.storage.current
    assert current == getattr(cfg.storage, impl)


# ------------------------
# YAML vs Env priority
# ------------------------

def test_storage_yaml_overrides_env(monkeypatch):
    """Test that YAML values take priority over environment variable STORAGE_IMPL."""
    monkeypatch.setenv("STORAGE_IMPL", "oss")  # env says OSS
    yaml_cfg = {"storage": {"active": "minio"}}  # YAML says MinIO
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()
    # YAML should win
    assert cfg.storage.active == ObjectStorageType.MINIO


def test_storage_env_applied_if_yaml_missing(monkeypatch):
    """Test that environment variable applies only if YAML does not specify storage.active."""
    monkeypatch.setenv("STORAGE_IMPL", "oss")
    yaml_cfg = {}  # YAML does not specify storage.active
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()
    # Env variable should be applied because YAML is missing the field
    assert cfg.storage.active == ObjectStorageType.OSS


def test_minio_host_port_parsing(monkeypatch):
    """Test that MinIO host:port parsing works and YAML overrides correctly."""
    monkeypatch.delenv("STORAGE_IMPL", raising=False)
    yaml_cfg = {"storage": {"active": "minio", "minio": {"host": "yamlhost:9999"}}}
    with patch("core.config.app.load_yaml", return_value=yaml_cfg):
        cfg = AppConfig()
    minio_cfg = cfg.storage.minio
    assert minio_cfg.host == "yamlhost"
    assert minio_cfg.port == 9999
