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

import importlib
import sys
import types

import pytest

np = pytest.importorskip("numpy")

from api.utils.validation_utils import RaptorConfig
from pydantic import ValidationError


@pytest.fixture()
def raptor_module(monkeypatch):
    class TaskCanceledException(Exception):
        pass

    class DummyLimiter:
        async def __aenter__(self):
            return self

        async def __aexit__(self, exc_type, exc, tb):
            return False

    class DummyGaussianMixture:
        def __init__(self, *args, **kwargs):
            pass

        def fit(self, embeddings):
            return self

        def bic(self, embeddings):
            return 0

        def predict_proba(self, embeddings):
            return np.ones((len(embeddings), 1))

    class DummyAgglomerativeClustering:
        def __init__(self, n_clusters=None, distance_threshold=None, compute_distances=False, linkage="ward"):
            self.n_clusters = n_clusters
            self.distance_threshold = distance_threshold
            self.compute_distances = compute_distances
            self.linkage = linkage
            self.distances_ = np.array([0.1, 0.2, 1.0])

        def fit(self, embeddings):
            self.labels_ = self.fit_predict(embeddings)
            return self

        def fit_predict(self, embeddings):
            if self.n_clusters is None:
                return np.zeros(len(embeddings), dtype=int)
            return np.array([idx % self.n_clusters for idx in range(len(embeddings))])

    class DummyUMAP:
        def __init__(self, *args, **kwargs):
            pass

        def fit_transform(self, embeddings):
            raise AssertionError("Psi tree builder must use original embeddings, not UMAP")

    sklearn_module = types.ModuleType("sklearn")
    mixture_module = types.ModuleType("sklearn.mixture")
    mixture_module.GaussianMixture = DummyGaussianMixture
    cluster_module = types.ModuleType("sklearn.cluster")
    cluster_module.AgglomerativeClustering = DummyAgglomerativeClustering
    umap_module = types.ModuleType("umap")
    umap_module.UMAP = DummyUMAP
    task_service_module = types.ModuleType("api.db.services.task_service")
    task_service_module.has_canceled = lambda task_id: False
    connection_utils_module = types.ModuleType("common.connection_utils")
    connection_utils_module.timeout = lambda seconds: lambda fn: fn
    exceptions_module = types.ModuleType("common.exceptions")
    exceptions_module.TaskCanceledException = TaskCanceledException
    token_utils_module = types.ModuleType("common.token_utils")
    token_utils_module.truncate = lambda text, max_len: text[:max_len]
    graphrag_utils_module = types.ModuleType("rag.graphrag.utils")
    graphrag_utils_module.chat_limiter = DummyLimiter()
    graphrag_utils_module.get_embed_cache = lambda *args, **kwargs: None
    graphrag_utils_module.get_llm_cache = lambda *args, **kwargs: None
    graphrag_utils_module.set_embed_cache = lambda *args, **kwargs: None
    graphrag_utils_module.set_llm_cache = lambda *args, **kwargs: None

    async def thread_pool_exec(fn, *args, **kwargs):
        return fn(*args, **kwargs)

    misc_utils_module = types.ModuleType("common.misc_utils")
    misc_utils_module.thread_pool_exec = thread_pool_exec

    monkeypatch.setitem(sys.modules, "sklearn", sklearn_module)
    monkeypatch.setitem(sys.modules, "sklearn.mixture", mixture_module)
    monkeypatch.setitem(sys.modules, "sklearn.cluster", cluster_module)
    monkeypatch.setitem(sys.modules, "umap", umap_module)
    monkeypatch.setitem(sys.modules, "api.db.services.task_service", task_service_module)
    monkeypatch.setitem(sys.modules, "common.connection_utils", connection_utils_module)
    monkeypatch.setitem(sys.modules, "common.exceptions", exceptions_module)
    monkeypatch.setitem(sys.modules, "common.token_utils", token_utils_module)
    monkeypatch.setitem(sys.modules, "rag.graphrag.utils", graphrag_utils_module)
    monkeypatch.setitem(sys.modules, "common.misc_utils", misc_utils_module)
    monkeypatch.delitem(sys.modules, "rag.raptor", raising=False)
    module = importlib.import_module("rag.raptor")
    yield module
    monkeypatch.delitem(sys.modules, "rag.raptor", raising=False)


class FakeChatModel:
    llm_name = "fake-chat"
    max_length = 4096

    def __init__(self):
        self.calls = []

    async def async_chat(self, system, history, gen_conf):
        self.calls.append(history[0]["content"])
        return f"summary-{len(self.calls)}"


class FakeEmbeddingModel:
    llm_name = "fake-embedding"

    def encode(self, texts):
        embeddings = []
        for text in texts:
            checksum = sum(ord(ch) for ch in text)
            embeddings.append(np.array([len(text), checksum % 17 + 1], dtype=float))
        return embeddings, len(texts)


_DEFAULT_TREE_BUILDER = object()


def _make_raptor(raptor_module, max_cluster=64, tree_builder=_DEFAULT_TREE_BUILDER, **kwargs):
    if tree_builder is _DEFAULT_TREE_BUILDER:
        kwargs["tree_builder"] = raptor_module.PSI_TREE_BUILDER
    else:
        kwargs["tree_builder"] = tree_builder
    return raptor_module.RecursiveAbstractiveProcessing4TreeOrganizedRetrieval(
        max_cluster,
        FakeChatModel(),
        FakeEmbeddingModel(),
        "{cluster_content}",
        max_token=32,
        threshold=0.1,
        **kwargs,
    )


def _chunks():
    return [
        ("alpha first", np.array([1.0, 0.0])),
        ("alpha second", np.array([0.99, 0.01])),
        ("alpha third", np.array([0.98, 0.02])),
    ]


def test_default_tree_builder_remains_original_raptor(raptor_module):
    raptor = _make_raptor(raptor_module, tree_builder=None)

    assert raptor._tree_builder == raptor_module.RAPTOR_TREE_BUILDER


def test_unknown_tree_builder_is_rejected(raptor_module):
    with pytest.raises(ValueError, match="Unsupported RAPTOR tree builder"):
        _make_raptor(raptor_module, tree_builder="ahc")


def test_raptor_config_accepts_hidden_psi_tree_builder():
    assert RaptorConfig().tree_builder == "raptor"
    assert RaptorConfig().clustering_method == "gmm"
    assert RaptorConfig(clustering_method="ahc").clustering_method == "ahc"
    assert RaptorConfig(tree_builder="psi").tree_builder == "psi"

    with pytest.raises(ValidationError):
        RaptorConfig(tree_builder="ahc")
    with pytest.raises(ValidationError):
        RaptorConfig(clustering_method="psi")


def test_ahc_clustering_method_is_supported_in_original_tree_builder(raptor_module):
    raptor = _make_raptor(raptor_module, tree_builder=raptor_module.RAPTOR_TREE_BUILDER, clustering_method="ahc")

    labels = raptor._get_clusters_ahc(np.array([[0.0, 0.0], [0.1, 0.0], [10.0, 10.0], [10.1, 10.0]]))

    assert raptor._tree_builder == raptor_module.RAPTOR_TREE_BUILDER
    assert raptor._clustering_method == "ahc"
    assert len(labels) == 4


def test_unknown_clustering_method_is_rejected(raptor_module):
    with pytest.raises(ValueError, match="Unsupported RAPTOR clustering method"):
        _make_raptor(raptor_module, clustering_method="psi")


def test_psi_tree_builder_ranks_all_leaf_pairs_by_original_cosine_similarity(raptor_module):
    raptor = _make_raptor(raptor_module)
    leaves = [
        raptor_module._PsiTreeNode(index=0, embedding=np.array([1.0, 0.0])),
        raptor_module._PsiTreeNode(index=1, embedding=np.array([0.0, 1.0])),
        raptor_module._PsiTreeNode(index=2, embedding=np.array([0.99, 0.01])),
        raptor_module._PsiTreeNode(index=3, embedding=np.array([-1.0, 0.0])),
    ]

    ranked_pairs = raptor._rank_leaf_pairs(leaves)

    assert len(ranked_pairs) == 6
    assert tuple(ranked_pairs[0]) == (2, 0)


def test_psi_tree_builder_uses_cosine_similarity_not_vector_magnitude(raptor_module):
    raptor = _make_raptor(raptor_module)
    leaves = [
        raptor_module._PsiTreeNode(index=0, embedding=np.array([100.0, 0.0])),
        raptor_module._PsiTreeNode(index=1, embedding=np.array([1.0, 1.0])),
        raptor_module._PsiTreeNode(index=2, embedding=np.array([0.1, 0.0])),
    ]

    ranked_pairs = raptor._rank_leaf_pairs(leaves)

    assert tuple(ranked_pairs[0]) == (2, 0)


def test_psi_tree_builder_handles_zero_vectors_in_cosine_ranking(raptor_module):
    raptor = _make_raptor(raptor_module)
    leaves = [
        raptor_module._PsiTreeNode(index=0, embedding=np.array([0.0, 0.0])),
        raptor_module._PsiTreeNode(index=1, embedding=np.array([1.0, 0.0])),
        raptor_module._PsiTreeNode(index=2, embedding=np.array([0.9, 0.1])),
    ]

    ranked_pairs = raptor._rank_leaf_pairs(leaves)

    assert tuple(ranked_pairs[0]) == (2, 1)


def test_psi_tree_builder_collapses_leaf_into_ranked_pair_parent(raptor_module):
    raptor = _make_raptor(raptor_module, max_cluster=64)

    root, leaves = raptor._build_psi_structure(_chunks())

    assert len(root.children) == 3
    assert {child.index for child in root.children} == {0, 1, 2}
    assert all(leaf.parent is root for leaf in leaves)


def test_psi_tree_builder_collapses_leaf_at_matching_rank(monkeypatch, raptor_module):
    raptor = _make_raptor(raptor_module, max_cluster=64)
    chunks = [
        ("node 0", np.array([1.0, 0.0])),
        ("node 1", np.array([0.9, 0.1])),
        ("node 2", np.array([-1.0, 0.0])),
        ("node 3", np.array([-0.9, -0.1])),
        ("node 4", np.array([0.8, 0.2])),
    ]
    monkeypatch.setattr(
        raptor,
        "_rank_leaf_pairs",
        lambda _leaves: np.array([[0, 1], [2, 3], [0, 2], [4, 0]]),
    )

    root, leaves = raptor._build_psi_structure(chunks)

    assert leaves[4].parent is leaves[0].parent
    assert leaves[4].parent is not root
    assert len(root.children) == 2


def test_psi_union_find_clamps_out_of_bounds_parent_rank(caplog, raptor_module):
    union_find = raptor_module._PsiUnionFind(2)
    union_find._node_ids[1] = [1]
    union_find._rank[0] = 2

    with caplog.at_level("WARNING"):
        union_find._build(0, 1, insert_point=1)

    assert union_find.tree[0] == 1
    assert "rank index" in caplog.text


def test_psi_tree_builder_rebalances_nodes_over_max_children(raptor_module):
    raptor = _make_raptor(raptor_module, max_cluster=2)

    root, _ = raptor._build_psi_structure(_chunks())

    assert all(len(node.children) <= 2 for node in raptor._iter_nodes(root))
    assert len(root.children) == 2
    assert any(child.children for child in root.children)


def test_psi_tree_builder_uses_bucketed_structure_for_large_inputs(monkeypatch, raptor_module):
    chunks = [(f"node {idx}", np.array([float(idx), float(idx % 3 + 1)])) for idx in range(8)]
    raptor = _make_raptor(
        raptor_module,
        max_cluster=3,
        psi_exact_max_leaves=3,
        psi_bucket_size=2,
    )
    ranked_sizes = []
    original_rank = raptor._rank_leaf_pairs

    def track_rank(nodes):
        ranked_sizes.append(len(nodes))
        return original_rank(nodes)

    monkeypatch.setattr(raptor, "_rank_leaf_pairs", track_rank)

    root, leaves = raptor._build_psi_structure(chunks)

    assert len(leaves) == len(chunks)
    assert all(leaf.parent is not None for leaf in leaves)
    assert all(len(node.children) <= 3 for node in raptor._iter_nodes(root))
    assert max(ranked_sizes) <= 3


@pytest.mark.asyncio
async def test_psi_tree_builder_materializes_rebalanced_summary_layers_without_umap(monkeypatch, raptor_module):
    def fail_umap(*args, **kwargs):
        raise AssertionError("Psi tree builder must use original embeddings, not UMAP")

    monkeypatch.setattr(raptor_module.umap, "UMAP", fail_umap)
    raptor = _make_raptor(raptor_module, max_cluster=2)

    chunks, layers = await raptor(_chunks(), random_state=0)

    assert len(chunks) == 5
    assert layers == [(0, 3), (3, 4), (4, 5)]
    assert [chunk[0] for chunk in chunks[3:]] == ["summary-1", "summary-2"]


@pytest.mark.asyncio
async def test_psi_tree_builder_skips_failed_node_summary(monkeypatch, raptor_module):
    raptor = _make_raptor(raptor_module, max_cluster=2)

    async def fail_summary(*args, **kwargs):
        return None

    monkeypatch.setattr(raptor, "_summarize_texts", fail_summary)

    chunks, layers = await raptor(_chunks(), random_state=0)

    assert len(chunks) == 3
    assert [chunk[0] for chunk in chunks] == [chunk[0] for chunk in _chunks()]
    assert layers == [(0, 3)]


@pytest.mark.asyncio
async def test_original_raptor_stops_when_transient_summary_fails(monkeypatch, raptor_module):
    raptor = _make_raptor(raptor_module, tree_builder=raptor_module.RAPTOR_TREE_BUILDER)

    async def fail_summary(*args, **kwargs):
        return None

    monkeypatch.setattr(raptor, "_summarize_texts", fail_summary)

    input_chunks = _chunks()[:2]
    chunks, layers = await raptor(input_chunks, random_state=0)

    assert len(chunks) == 2
    assert [chunk[0] for chunk in chunks] == [chunk[0] for chunk in input_chunks]
    assert layers == [(0, 2)]
