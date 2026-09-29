#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
from rag.advanced_rag.harness.chunk_utils import strip_chunk_vectors, strip_kbinfos_vectors
from rag.advanced_rag.harness.memory import add


class _Tools:
    def __init__(self):
        self.kbinfos = {}


def test_strip_chunk_vectors_removes_dense_arrays():
    chunks = [{"chunk_id": "a", "content": "hello", "vector": [0.1, 0.2, 0.3]}]
    strip_chunk_vectors(chunks)
    assert "vector" not in chunks[0]


def test_strip_kbinfos_vectors():
    kbinfos = {"chunks": [{"id": "1", "vector": [1.0] * 128}], "doc_aggs": []}
    strip_kbinfos_vectors(kbinfos)
    assert "vector" not in kbinfos["chunks"][0]


def test_memory_add_does_not_store_vectors():
    tools = _Tools()
    add(tools, [{"chunk_id": "c1", "content": "fact", "vector": [0.5] * 64}])
    assert len(tools.kbinfos["memory"]) == 1
    assert "vector" not in tools.kbinfos["memory"][0]
