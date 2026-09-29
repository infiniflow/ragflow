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
"""Regression tests for agent Retrieval KB/memory embedding compatibility.

Datasets may store a tenant_model UUID or a ``model@instance@provider``
composite in ``embd_id``. Chat/assistant selection treats those as compatible
when they share a tenant_embd_id / base name. The Retrieval canvas component
must use the same ``validate_dataset_embedding_models`` comparison rather than
comparing raw ``embd_id`` strings.

This test loads only the relevant function definitions via AST so it does not
pull in the full ``api.db.db_models`` import chain.
"""

import ast
from pathlib import Path
from types import SimpleNamespace

import pytest

pytestmark = pytest.mark.p2

_REPO_ROOT = Path(__file__).resolve().parents[4]
_KB_SERVICE = _REPO_ROOT / "api" / "db" / "services" / "knowledgebase_service.py"
_RETRIEVAL = _REPO_ROOT / "agent" / "tools" / "retrieval.py"

TENANT_EMBD_ID = "aa6f9604a1d411f1a1fc65466754f1f9"
COMPOSITE = "bge-m3:latest@Ollama local@Ollama"
OTHER_COMPOSITE = "Qwen/Qwen3-Embedding-0.6B@1@SILICONFLOW"


def _exec_functions(src_path, names, ns):
    tree = ast.parse(src_path.read_text(encoding="utf-8"))
    nodes = [node for node in tree.body if isinstance(node, ast.FunctionDef) and node.name in names]
    missing = set(names) - {n.name for n in nodes}
    assert not missing, f"missing functions in {src_path}: {missing}"
    module = ast.Module(body=nodes, type_ignores=[])
    exec(compile(module, str(src_path), "exec"), ns)  # noqa: S102 - exec'ing AST-extracted functions from our own source file
    return ns


def _load_shared_embedding_id(resolved_names):
    ns = {"get_composite_model_name_by_ids": lambda _ids: resolved_names}
    _exec_functions(
        _KB_SERVICE,
        ("_base_model_name", "_kb_embedding_base_name", "validate_dataset_embedding_models"),
        ns,
    )
    _exec_functions(_RETRIEVAL, ("_shared_embedding_id",), ns)
    return ns["_shared_embedding_id"]


def test_uuid_and_composite_with_same_tenant_embd_id_are_compatible():
    check = _load_shared_embedding_id({TENANT_EMBD_ID: COMPOSITE})
    kbs = [
        SimpleNamespace(embd_id=TENANT_EMBD_ID, tenant_embd_id=TENANT_EMBD_ID),
        SimpleNamespace(embd_id=COMPOSITE, tenant_embd_id=TENANT_EMBD_ID),
    ]
    embd_id = check(kbs, "Knowledge bases use different embedding models.")
    assert embd_id in {TENANT_EMBD_ID, COMPOSITE}


def test_uuid_and_composite_return_a_stored_model_id_not_a_base_name():
    check = _load_shared_embedding_id({TENANT_EMBD_ID: COMPOSITE})
    kbs = [
        SimpleNamespace(embd_id=TENANT_EMBD_ID, tenant_embd_id=TENANT_EMBD_ID),
        SimpleNamespace(embd_id=COMPOSITE, tenant_embd_id=TENANT_EMBD_ID),
    ]
    embd_id = check(kbs, "Knowledge bases use different embedding models.")
    assert embd_id != "bge-m3:latest"
    assert "@" in embd_id or embd_id == TENANT_EMBD_ID


def test_different_embedding_models_still_fail():
    check = _load_shared_embedding_id(
        {
            TENANT_EMBD_ID: COMPOSITE,
            "b346fecb098e4305b58926f1bae6bbc9": OTHER_COMPOSITE,
        }
    )
    kbs = [
        SimpleNamespace(embd_id=TENANT_EMBD_ID, tenant_embd_id=TENANT_EMBD_ID),
        SimpleNamespace(embd_id="b346fecb098e4305b58926f1bae6bbc9", tenant_embd_id="b346fecb098e4305b58926f1bae6bbc9"),
    ]
    with pytest.raises(Exception, match="Knowledge bases use different embedding models"):
        check(kbs, "Knowledge bases use different embedding models.")


def test_unresolved_uuid_does_not_match_a_different_composite():
    check = _load_shared_embedding_id({})
    kbs = [
        SimpleNamespace(embd_id=TENANT_EMBD_ID, tenant_embd_id=TENANT_EMBD_ID),
        SimpleNamespace(embd_id=COMPOSITE, tenant_embd_id=None),
    ]
    with pytest.raises(Exception, match="Knowledge bases use different embedding models"):
        check(kbs, "Knowledge bases use different embedding models.")


def test_empty_names_do_not_make_different_models_compatible():
    check = _load_shared_embedding_id({})
    kbs = [
        SimpleNamespace(embd_id=COMPOSITE, tenant_embd_id=None),
        SimpleNamespace(embd_id="", tenant_embd_id=None),
    ]
    with pytest.raises(Exception, match="Knowledge bases use different embedding models"):
        check(kbs, "Knowledge bases use different embedding models.")


def test_memory_uuid_and_composite_with_same_tenant_embd_id_are_compatible():
    check = _load_shared_embedding_id({TENANT_EMBD_ID: COMPOSITE})
    memories = [
        SimpleNamespace(embd_id=TENANT_EMBD_ID, tenant_embd_id=TENANT_EMBD_ID),
        SimpleNamespace(embd_id=COMPOSITE, tenant_embd_id=TENANT_EMBD_ID),
    ]
    assert check(memories, "Memory use different embedding models.") in {TENANT_EMBD_ID, COMPOSITE}


def test_memory_different_embedding_models_still_fail():
    check = _load_shared_embedding_id({})
    memories = [
        SimpleNamespace(embd_id=COMPOSITE, tenant_embd_id=None),
        SimpleNamespace(embd_id=OTHER_COMPOSITE, tenant_embd_id=None),
    ]
    with pytest.raises(Exception, match="Memory use different embedding models"):
        check(memories, "Memory use different embedding models.")
