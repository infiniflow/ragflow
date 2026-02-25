import ast
import json
import os
import sys
from pathlib import Path
import pytest

# Add project root to sys.path
sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), "../..")))

from common.config_utils import get_base_config
from common.doc_store.doc_store_base import DocStoreConnection, MatchDenseExpr, MatchTextExpr
from rag.utils.opensearch_conn import OSConnection

# ----------------- Helper functions ------------------------------


def _ensure_opensearch_config() -> bool:
    """
    Ensure settings.OS is populated when running tests outside the app.
    init_settings() is only called when the server starts; when running pytest
    directly, settings.OS stays empty. Load from service_conf.yaml if needed.
    """
    from common import settings

    # Pytest can import this file before the application calls init_settings(),
    # which leaves settings.OS empty even when service_conf.yaml has OpenSearch.
    if not settings.OS:
        os_cfg = get_base_config("os", {})
        if isinstance(os_cfg, dict):
            settings.OS = os_cfg

    return bool(isinstance(settings.OS, dict) and settings.OS.get("hosts"))


def _enshure_live_opensearch_connection() -> bool:
    """
    Test that a connection to a live OpenSearch instance can be established.
    Skips when OpenSearch is unreachable; passes when the cluster responds.
    """

    try:
        if not _ensure_opensearch_config():
            return False

        os_conn = OSConnection()
        if os_conn.os.ping() and "version" in os_conn.info:
            return True
        else:
            return False

    except Exception as e:
        print(f"No OpenSearch instance found: {e}")
        return False


# ----------------- Tests ------------------------------
def test_opensearch_connection_parity():
    """
    Verify OpenSearch connector parity with expected non-generic behavior.

    This test is static and does not require a live OpenSearch instance.
    """
    current_dir = Path(__file__).resolve().parent
    project_root = current_dir.parent.parent.parent
    opensearch_conn_path = project_root / "rag" / "utils" / "opensearch_conn.py"
    source_code = opensearch_conn_path.read_text(encoding="utf-8")

    tree = ast.parse(source_code)
    os_connection_class = next(node for node in tree.body if isinstance(node, ast.ClassDef) and node.name == "OSConnection")
    class_methods = {node.name for node in os_connection_class.body if isinstance(node, ast.FunctionDef)}

    # Keep only behaviorally relevant parity checks after generic rollback.
    assert "create_doc_meta_idx" in class_methods, "OSConnection should implement create_doc_meta_idx"
    assert "search" in class_methods, "OSConnection should implement search"
    assert "insert" in class_methods, "OSConnection should implement insert"
    assert "update" in class_methods, "OSConnection should implement update"
    assert "delete" in class_methods, "OSConnection should implement delete"

    # Verify compatibility alias remains in constructor source.
    assert "self.es = self.os" in source_code, "OSConnection should keep .es alias for compatibility paths"


def test_metadata_mapping_exists():
    """
    Verify that the OpenSearch metadata mapping file exists.
    """
    # Find project root independently because get_project_base_directory()
    # might be polluted by other tests.
    current_dir = Path(__file__).resolve().parent
    project_root = current_dir.parent.parent.parent
    mapping_path = project_root / "conf" / "doc_meta_os_mapping.json"
    assert os.path.exists(mapping_path), f"Metadata mapping file not found at {mapping_path}"


def test_opensearch_mapping_supports_3072_knn_vector():
    """
    Verify OpenSearch mapping includes dynamic template for 3072-d vectors.
    """
    current_dir = Path(__file__).resolve().parent
    project_root = current_dir.parent.parent.parent
    mapping_path = project_root / "conf" / "os_mapping.json"
    assert mapping_path.exists(), f"OpenSearch mapping file not found at {mapping_path}"

    mapping_config = json.loads(mapping_path.read_text(encoding="utf-8"))
    dynamic_templates = mapping_config.get("mappings", {}).get("dynamic_templates", [])

    has_3072_knn_template = any(
        template.get("knn_vector", {}).get("match") == "*_3072_vec"
        and template.get("knn_vector", {}).get("mapping", {}).get("type") == "knn_vector"
        and template.get("knn_vector", {}).get("mapping", {}).get("dimension") == 3072
        for template in dynamic_templates
    )

    assert has_3072_knn_template, "OpenSearch mapping must define a knn_vector dynamic template for *_3072_vec"


def test_memory_opensearch_connector_implements_docstore_abstract_methods():
    """
    Ensure memory OpenSearch connector implements all abstract DocStore methods.
    """
    current_dir = Path(__file__).resolve().parent
    project_root = current_dir.parent.parent.parent
    memory_conn_path = project_root / "memory" / "utils" / "opensearch_conn.py"

    tree = ast.parse(memory_conn_path.read_text(encoding="utf-8"))
    os_connection_class = next(node for node in tree.body if isinstance(node, ast.ClassDef) and node.name == "OSConnection")
    class_methods = {node.name for node in os_connection_class.body if isinstance(node, ast.FunctionDef)}

    required_methods = set(DocStoreConnection.__abstractmethods__)
    missing_methods = sorted(required_methods - class_methods)

    assert not missing_methods, f"memory.utils.opensearch_conn.OSConnection misses required DocStoreConnection methods: {missing_methods}"


@pytest.mark.skipif(not _enshure_live_opensearch_connection(), reason="No OpenSearch configuration or instance found")
def test_live_opensearch_metadata_ops():
    """
    Live tests for metadata operations.
    Only runs if RUN_LIVE_OS_TESTS is set.
    """

    os_conn = OSConnection()
    tenant_id = "pytest_tenant"
    index_name = f"ragflow_doc_meta_{tenant_id}"
    kb_id = "test_kb_123"

    try:
        # 1. Create Index
        print(f"\n[LIVE] Creating metadata index {index_name}...")
        os_conn.create_doc_meta_idx(index_name)
        assert os_conn.index_exist(index_name)

        # 2. Insert Multiple Documents
        docs = [
            {"id": "doc_1", "kb_id": kb_id, "meta_fields": {"tags": ["a", "b"], "status": "new"}},
            {"id": "doc_2", "kb_id": kb_id, "meta_fields": {"tags": ["b", "c"], "status": "active"}},
            {"id": "doc_3", "kb_id": "other_kb", "meta_fields": {"tags": ["d"], "status": "new"}},
        ]
        print(f"[LIVE] Inserting {len(docs)} documents...")
        res = os_conn.insert(docs, index_name)
        assert not res, f"Insert failed: {res}"

        # 3. Refresh and Count
        os_conn.refresh_idx(index_name)
        count = os_conn.count_idx(index_name)
        assert count == 3, f"Expected 3 documents, got {count}"

        # 4. Search Metadata
        print("[LIVE] Testing search on metadata index...")
        search_res = os_conn.search(
            select_fields=["*"],
            highlight_fields=[],
            condition={"status": "new"},
            match_expressions=[],
            order_by=None,
            offset=0,
            limit=10,
            index_names=index_name,
            knowledgebase_ids=[kb_id, "other_kb"],
        )
        total = os_conn.get_total(search_res)
        assert total == 2, f"Expected 2 'new' documents, got {total}"

        # 5. Partial Update
        print("[LIVE] Testing partial update...")
        os_conn.update_doc_metadata_field(index_name, "doc_1", {"meta_fields": {"status": "updated", "new_key": "val"}})
        os_conn.refresh_idx(index_name)
        doc1 = os_conn.get("doc_1", index_name, [kb_id])
        assert doc1["meta_fields"]["status"] == "updated"
        assert doc1["meta_fields"]["new_key"] == "val"
        assert "a" in doc1["meta_fields"]["tags"]

        # 6. Delete Specific Metadata
        print("[LIVE] Testing deletion...")
        deleted = os_conn.delete({"id": "doc_2"}, index_name, kb_id)
        assert deleted == 1
        os_conn.refresh_idx(index_name)
        assert os_conn.count_idx(index_name) == 2

        # 7. Batch Update (Legacy style)
        print("[LIVE] Testing legacy update...")
        os_conn.update({"id": "doc_3"}, {"status": "batch_updated"}, index_name, "other_kb")
        os_conn.refresh_idx(index_name)
        doc3 = os_conn.get("doc_3", index_name, ["other_kb"])
        assert doc3["status"] == "batch_updated"

    finally:
        # Cleanup
        if os_conn.index_exist(index_name):
            print(f"[LIVE] Cleaning up index {index_name}...")
            os_conn.delete_idx(index_name, "")


@pytest.mark.skipif(not _enshure_live_opensearch_connection(), reason="No OpenSearch configuration or instance found")
def test_live_opensearch_retrival_ops():
    """
    Live tests for retrieval operations on the regular OpenSearch doc index.
    """
    os_conn = OSConnection()
    index_name = "ragflow_pytest_retrieval"
    kb_id = "test_kb_retrieval"

    vec_3072_a = [0.01] * 3072
    vec_3072_b = [0.0] * 3072
    vec_3072_b[0] = 1.0

    docs = [
        {
            "id": "chunk_1",
            "kb_id": kb_id,
            "content_ltks": "opensearch retrieval smoke test",
            "q_3072_vec": vec_3072_a,
            "available_int": 1,
        },
        {
            "id": "chunk_2",
            "kb_id": kb_id,
            "content_ltks": "another chunk for retrieval",
            "q_3072_vec": vec_3072_b,
            "available_int": 1,
        },
    ]

    try:
        if os_conn.index_exist(index_name):
            os_conn.delete_idx(index_name, "")

        # 1. Create regular index with vector support and insert chunks.
        os_conn.create_idx(index_name, kb_id, 3072)
        assert os_conn.index_exist(index_name), f"Failed to create retrieval index {index_name}"

        insert_res = os_conn.insert(docs, index_name)
        assert not insert_res, f"Insert failed: {insert_res}"
        os_conn.refresh_idx(index_name)
        assert os_conn.count_idx(index_name) == 2

        # 2. Text retrieval check.
        text_res = os_conn.search(
            select_fields=["id", "kb_id", "content_ltks"],
            highlight_fields=[],
            condition={},
            match_expressions=[
                MatchTextExpr(
                    fields=["content_ltks"],
                    matching_text="opensearch",
                    topn=5,
                    extra_options={"minimum_should_match": 0.0},
                )
            ],
            order_by=None,
            offset=0,
            limit=5,
            index_names=index_name,
            knowledgebase_ids=[kb_id],
        )
        assert os_conn.get_total(text_res) >= 1
        assert "chunk_1" in os_conn.get_doc_ids(text_res)

        # 3. Vector retrieval check on q_3072_vec.
        vector_res = os_conn.search(
            select_fields=["id", "kb_id"],
            highlight_fields=[],
            condition={},
            match_expressions=[
                MatchDenseExpr(
                    vector_column_name="q_3072_vec",
                    embedding_data=vec_3072_a,
                    embedding_data_type="float",
                    distance_type="cosine",
                    topn=2,
                    extra_options={"similarity": 0.0},
                )
            ],
            order_by=None,
            offset=0,
            limit=2,
            index_names=index_name,
            knowledgebase_ids=[kb_id],
        )
        assert os_conn.get_total(vector_res) >= 1
        assert len(os_conn.get_doc_ids(vector_res)) >= 1

    finally:
        if os_conn.index_exist(index_name):
            os_conn.delete_idx(index_name, "")
