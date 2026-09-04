import ast
import re
from pathlib import Path
from types import SimpleNamespace

import pytest


class _FakeFrame:
    def __init__(self, data):
        self._data = data
        self.columns = list(data)

    @property
    def empty(self):
        return not self._data or not len(next(iter(self._data.values())))

    def __len__(self):
        return len(next(iter(self._data.values()))) if self._data else 0

    def __getitem__(self, key):
        return self._data[key]


@pytest.fixture
def get_highlight():
    repo_root = Path(__file__).resolve().parents[3]
    source = (repo_root / "common" / "doc_store" / "infinity_conn_base.py").read_text(encoding="utf-8")
    tree = ast.parse(source)
    class_node = next(node for node in tree.body if isinstance(node, ast.ClassDef) and node.name == "InfinityConnectionBase")
    method = next(node for node in class_node.body if isinstance(node, ast.FunctionDef) and node.name == "get_highlight")
    namespace = {
        "pd": SimpleNamespace(DataFrame=object),
        "re": re,
        "is_english": lambda _parts: False,
    }
    exec(compile(ast.Module(body=[method], type_ignores=[]), "infinity_conn_base.py", "exec"), namespace)
    return namespace["get_highlight"]


def test_get_highlight_uses_dataframe_from_infinity_tuple_and_content_fallback(get_highlight):
    frame = _FakeFrame({"id": ["doc-1"], "content": ["hello needle"]})

    result = get_highlight(object(), (frame, 99), ["needle"], "content_with_weight")

    assert result == {"doc-1": "hello <em>needle</em>"}


def test_get_highlight_returns_empty_for_empty_dataframe(get_highlight):
    frame = _FakeFrame({"id": [], "content": []})

    assert get_highlight(object(), (frame, 99), ["needle"], "content") == {}
