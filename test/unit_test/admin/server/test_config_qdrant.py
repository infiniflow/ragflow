import os
from types import SimpleNamespace

os.environ.setdefault("JOBLIB_MULTIPROCESSING", "0")

from admin.server import config as admin_config
from api.utils import health_utils


def test_load_configurations_includes_qdrant(monkeypatch):
    monkeypatch.setattr(admin_config, "read_config", lambda _: {
        "qdrant": {
            "host": "qdrant",
            "http_port": 6333,
            "grpc_port": 6334,
            "https": False,
            "prefer_grpc": True,
        }
    })

    configs = admin_config.load_configurations("service_conf.yaml")

    assert len(configs) == 1
    config = configs[0]
    assert config.name == "qdrant"
    assert config.retrieval_type == "qdrant"
    assert config.host == "qdrant"
    assert config.port == 6333
    assert config.grpc_port == 6334
    assert config.prefer_grpc is True
    assert config.detail_func_name == "get_qdrant_status"


def test_get_qdrant_status_uses_docstore_health(monkeypatch):
    expected = {"type": "qdrant", "status": "green"}

    monkeypatch.setenv("DOC_ENGINE", "qdrant")
    monkeypatch.setattr(health_utils.settings, "docStoreConn", SimpleNamespace(health=lambda: expected))

    status = health_utils.get_qdrant_status()

    assert status == {"status": "alive", "message": expected}
