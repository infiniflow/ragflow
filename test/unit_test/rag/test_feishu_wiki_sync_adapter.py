import importlib.util
import sys
from datetime import datetime, timezone
from pathlib import Path
from types import ModuleType


class FakeConnector:
    def __init__(self):
        self.poll_calls = []
        self.full_calls = 0

    def load_from_state(self):
        self.full_calls += 1
        return iter((["full"],))

    def poll_source(self, start, end):
        self.poll_calls.append((start, end))
        return iter((["incremental"],))


fake_connector = FakeConnector()


class FakeConnectorFactory:
    configs = []

    @classmethod
    def build_connector(cls, config):
        cls.configs.append(config)
        return fake_connector


def _load_adapter_module():
    repo_root = Path(__file__).resolve().parents[3]
    adapter_path = repo_root / "rag" / "svr" / "feishu_wiki_sync.py"
    if not adapter_path.exists():
        return None

    saved_data_source = sys.modules.get("common.data_source")
    data_source_stub = ModuleType("common.data_source")
    data_source_stub.FeishuWikiConnector = FakeConnectorFactory
    sys.modules["common.data_source"] = data_source_stub
    try:
        spec = importlib.util.spec_from_file_location(
            "_feishu_wiki_sync_adapter_under_test",
            adapter_path,
        )
        module = importlib.util.module_from_spec(spec)
        assert spec.loader is not None
        spec.loader.exec_module(module)
        return module
    finally:
        if saved_data_source is None:
            sys.modules.pop("common.data_source", None)
        else:
            sys.modules["common.data_source"] = saved_data_source


adapter_module = _load_adapter_module()


def _build_generator():
    assert adapter_module is not None, "Feishu Wiki sync adapter is not implemented"
    return adapter_module.build_feishu_wiki_generator


def setup_function():
    fake_connector.poll_calls.clear()
    fake_connector.full_calls = 0
    FakeConnectorFactory.configs.clear()


def test_manual_rebuild_scans_from_the_root():
    build_generator = _build_generator()
    config = {"space_id": "space-1"}

    connector, batches = build_generator(
        config,
        {"reindex": "1", "poll_range_start": None},
        window_end=datetime(2026, 1, 2, tzinfo=timezone.utc),
    )

    assert connector is fake_connector
    assert list(batches) == [["full"]]
    assert fake_connector.full_calls == 1
    assert fake_connector.poll_calls == []
    assert FakeConnectorFactory.configs == [config]


def test_automatic_sync_uses_the_previous_successful_window():
    build_generator = _build_generator()

    connector, batches = build_generator(
        {"space_id": "space-1"},
        {
            "reindex": "0",
            "poll_range_start": datetime(2026, 1, 1, tzinfo=timezone.utc),
        },
        window_end=datetime(2026, 1, 2, tzinfo=timezone.utc),
    )

    assert connector is fake_connector
    assert list(batches) == [["incremental"]]
    assert fake_connector.full_calls == 0
    assert fake_connector.poll_calls == [(1767225600.0, 1767312000.0)]

