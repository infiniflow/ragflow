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

"""Unit tests for RAPTOR clustering behavior."""

import sys
from types import ModuleType

import pytest

np = pytest.importorskip("numpy")
pytest.importorskip("sklearn")


def _install_raptor_import_stubs(monkeypatch):
    task_service = ModuleType("api.db.services.task_service")
    task_service.has_canceled = lambda _task_id: False
    monkeypatch.setitem(sys.modules, "api.db.services.task_service", task_service)

    connection_utils = ModuleType("common.connection_utils")
    connection_utils.timeout = lambda _seconds: lambda func: func
    monkeypatch.setitem(sys.modules, "common.connection_utils", connection_utils)

    exceptions = ModuleType("common.exceptions")
    exceptions.TaskCanceledException = RuntimeError
    monkeypatch.setitem(sys.modules, "common.exceptions", exceptions)

    token_utils = ModuleType("common.token_utils")
    token_utils.truncate = lambda text, _max_length: text
    monkeypatch.setitem(sys.modules, "common.token_utils", token_utils)

    graphrag_utils = ModuleType("rag.graphrag.utils")
    graphrag_utils.chat_limiter = None
    graphrag_utils.get_embed_cache = lambda *_args, **_kwargs: None
    graphrag_utils.get_llm_cache = lambda *_args, **_kwargs: None
    graphrag_utils.set_embed_cache = lambda *_args, **_kwargs: None
    graphrag_utils.set_llm_cache = lambda *_args, **_kwargs: None
    monkeypatch.setitem(sys.modules, "rag.graphrag.utils", graphrag_utils)

    misc_utils = ModuleType("common.misc_utils")

    async def thread_pool_exec(func, *args, **kwargs):
        return func(*args, **kwargs)

    misc_utils.thread_pool_exec = thread_pool_exec
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils)

    umap_mod = ModuleType("umap")
    umap_mod.UMAP = object
    monkeypatch.setitem(sys.modules, "umap", umap_mod)


def _make_raptor(monkeypatch, max_cluster, clustering_method="ahc"):
    _install_raptor_import_stubs(monkeypatch)
    monkeypatch.delitem(sys.modules, "rag.raptor", raising=False)
    from rag.raptor import RecursiveAbstractiveProcessing4TreeOrganizedRetrieval

    return RecursiveAbstractiveProcessing4TreeOrganizedRetrieval(
        max_cluster=max_cluster,
        llm_model=None,
        embd_model=None,
        prompt="summarize",
        clustering_method=clustering_method,
    )


def test_ahc_clustering_respects_single_max_cluster(monkeypatch):
    raptor = _make_raptor(monkeypatch, max_cluster=1)

    embeddings = np.array(
        [
            [0.0, 0.0],
            [0.1, 0.0],
            [10.0, 10.0],
            [10.1, 10.0],
        ]
    )

    labels = raptor._get_clusters_ahc(embeddings)

    assert labels.tolist() == [0, 0, 0, 0]


def test_ahc_clustering_does_not_exceed_max_cluster(monkeypatch):
    raptor = _make_raptor(monkeypatch, max_cluster=2)

    embeddings = np.array(
        [
            [0.0, 0.0],
            [0.1, 0.0],
            [10.0, 10.0],
            [10.1, 10.0],
            [20.0, 20.0],
            [20.1, 20.0],
        ]
    )

    labels = raptor._get_clusters_ahc(embeddings)

    assert len(np.unique(labels)) <= 2
