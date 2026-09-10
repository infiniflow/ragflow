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
"""Regression tests for validate_dataset_embedding_models() in
api.db.services.knowledgebase_service.

Datasets may store their embedding reference either as a tenant_model id
(hex) or as a legacy composite ``model@instance@provider`` string. Datasets
that resolve to the same base embedding model must validate together even
when the storage forms or provider instances differ (e.g.
``BAAI/bge-m3@renew@SILICONFLOW`` vs ``BAAI/bge-m3@COPY@SILICONFLOW``).

This test loads only the relevant function definitions from the source file
via AST, so it doesn't pull in the full ``api.db.db_models`` import chain and
can run in any minimal pytest environment.
"""

import ast
from pathlib import Path
from types import SimpleNamespace

import pytest

pytestmark = pytest.mark.p2


def _load_validator(resolved_names):
    """Exec the validator + helpers from the production source file with a
    stubbed ``get_composite_model_name_by_ids``."""
    src_path = Path(__file__).resolve().parents[5] / "api" / "db" / "services" / "knowledgebase_service.py"
    tree = ast.parse(src_path.read_text(encoding="utf-8"))
    wanted = {
        "_base_model_name",
        "_kb_embedding_base_name",
        "validate_dataset_embedding_models",
    }
    nodes = [node for node in tree.body if isinstance(node, ast.FunctionDef) and node.name in wanted]
    assert len(nodes) == len(wanted), f"missing functions: {wanted - {n.name for n in nodes}}"
    module = ast.Module(body=nodes, type_ignores=[])
    ns = {"get_composite_model_name_by_ids": lambda _ids: resolved_names}
    exec(compile(module, str(src_path), "exec"), ns)  # noqa: S102 - exec'ing AST-extracted functions from our own source file
    return ns["validate_dataset_embedding_models"]


def test_same_composite_validates():
    validate = _load_validator({})
    kbs = [
        SimpleNamespace(embd_id="BAAI/bge-m3@1@SILICONFLOW", tenant_embd_id=None),
        SimpleNamespace(embd_id="BAAI/bge-m3@1@SILICONFLOW", tenant_embd_id=None),
    ]
    assert validate(kbs) is None


def test_same_base_different_instances_validate():
    """The search-settings bug: same model/provider, different instance name."""
    validate = _load_validator({})
    kbs = [
        SimpleNamespace(embd_id="BAAI/bge-m3@renew@SILICONFLOW", tenant_embd_id=None),
        SimpleNamespace(embd_id="BAAI/bge-m3@COPY@SILICONFLOW", tenant_embd_id=None),
    ]
    assert validate(kbs) is None


def test_different_base_models_rejected():
    validate = _load_validator({})
    kbs = [
        SimpleNamespace(embd_id="BAAI/bge-m3@1@SILICONFLOW", tenant_embd_id=None),
        SimpleNamespace(embd_id="Qwen/Qwen3-Embedding-0.6B@1@SILICONFLOW", tenant_embd_id=None),
    ]
    assert "different embedding models" in validate(kbs)


def test_tenant_model_id_and_composite_resolve_together():
    """A raw tenant_model id and a legacy composite pointing at the same base
    model must validate together."""
    validate = _load_validator({"5fb939ba64d04dd2b6dd9c1c08775d52": "BAAI/bge-m3@1@SILICONFLOW"})
    kbs = [
        SimpleNamespace(
            embd_id="5fb939ba64d04dd2b6dd9c1c08775d52",
            tenant_embd_id="5fb939ba64d04dd2b6dd9c1c08775d52",
        ),
        SimpleNamespace(embd_id="BAAI/bge-m3@2@SILICONFLOW", tenant_embd_id=None),
    ]
    assert validate(kbs) is None


def test_two_tenant_model_ids_resolving_to_same_base_validate():
    validate = _load_validator(
        {
            "5fb939ba64d04dd2b6dd9c1c08775d52": "BAAI/bge-m3@1@SILICONFLOW",
            "fdbbd8f9a0564a6f92d068bd596de861": "BAAI/bge-m3@2@SILICONFLOW",
        }
    )
    kbs = [
        SimpleNamespace(
            embd_id="5fb939ba64d04dd2b6dd9c1c08775d52",
            tenant_embd_id="5fb939ba64d04dd2b6dd9c1c08775d52",
        ),
        SimpleNamespace(
            embd_id="fdbbd8f9a0564a6f92d068bd596de861",
            tenant_embd_id="fdbbd8f9a0564a6f92d068bd596de861",
        ),
    ]
    assert validate(kbs) is None


def test_resolved_different_bases_rejected():
    validate = _load_validator(
        {
            "5fb939ba64d04dd2b6dd9c1c08775d52": "BAAI/bge-m3@1@SILICONFLOW",
            "b346fecb098e4305b58926f1bae6bbc9": "Qwen/Qwen3-Embedding-0.6B@1@SILICONFLOW",
        }
    )
    kbs = [
        SimpleNamespace(
            embd_id="5fb939ba64d04dd2b6dd9c1c08775d52",
            tenant_embd_id="5fb939ba64d04dd2b6dd9c1c08775d52",
        ),
        SimpleNamespace(
            embd_id="b346fecb098e4305b58926f1bae6bbc9",
            tenant_embd_id="b346fecb098e4305b58926f1bae6bbc9",
        ),
    ]
    assert "different embedding models" in validate(kbs)


def test_unresolvable_ids_stay_isolated():
    validate = _load_validator({})
    kbs = [
        SimpleNamespace(embd_id="2d8ff0a97d75431c8c91526549939328", tenant_embd_id="2d8ff0a97d75431c8c91526549939328"),
        SimpleNamespace(embd_id="BAAI/bge-m3@1@SILICONFLOW", tenant_embd_id=None),
    ]
    assert "different embedding models" in validate(kbs)


def test_stale_tenant_id_falls_back_to_composite_base():
    """A KB whose tenant_embd_id no longer resolves still groups by its
    legacy composite embd_id."""
    validate = _load_validator({})
    kbs = [
        SimpleNamespace(embd_id="BAAI/bge-m3@1@SILICONFLOW", tenant_embd_id="2540bad87b7511f197c0d16229aaaa64"),
        SimpleNamespace(embd_id="BAAI/bge-m3@2@SILICONFLOW", tenant_embd_id=None),
    ]
    assert validate(kbs) is None


def test_mixed_embedding_and_none_rejected():
    validate = _load_validator({})
    kbs = [
        SimpleNamespace(embd_id="BAAI/bge-m3@1@SILICONFLOW", tenant_embd_id=None),
        SimpleNamespace(embd_id="", tenant_embd_id=None),
    ]
    assert "some have embedding models and others do not" in validate(kbs)


def test_all_without_embedding_validates():
    validate = _load_validator({})
    kbs = [
        SimpleNamespace(embd_id="", tenant_embd_id=None),
        SimpleNamespace(embd_id="", tenant_embd_id=None),
    ]
    assert validate(kbs) is None
