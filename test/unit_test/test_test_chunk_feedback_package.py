import importlib
import sys
from types import ModuleType


def test_chunk_feedback_package_import_is_safe_when_common_is_shadowed(monkeypatch):
    shadow_common = ModuleType("common")
    monkeypatch.setitem(sys.modules, "common", shadow_common)
    monkeypatch.delitem(
        sys.modules,
        "test.testcases.test_web_api.test_chunk_feedback",
        raising=False,
    )

    module = importlib.import_module("test.testcases.test_web_api.test_chunk_feedback")

    assert module is not None
