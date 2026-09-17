#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
"""use_kg must call kg_retriever.retrieval exactly once in _retrieve_kb.

Regression: agent/tools/retrieval.py historically called
settings.kg_retriever.retrieval twice when use_kg=True (once inside the
``if kbs:`` branch with raw ``kb_ids``, once after with ``filtered_kb_ids``)
and prepended both results. The LLM then saw duplicate KG context and the
request paid for a second graph retrieval. The surviving call must use
``filtered_kb_ids`` and normalize ``content_with_weight`` → ``content``.
"""

import ast
from pathlib import Path

import pytest

pytestmark = pytest.mark.p2

_REPO_ROOT = Path(__file__).resolve().parents[4]
_RETRIEVAL = _REPO_ROOT / "agent" / "tools" / "retrieval.py"


def _retrieve_kb_method():
    tree = ast.parse(_RETRIEVAL.read_text(encoding="utf-8"))
    for node in tree.body:
        if isinstance(node, ast.ClassDef) and node.name == "Retrieval":
            for item in node.body:
                if isinstance(item, (ast.FunctionDef, ast.AsyncFunctionDef)) and item.name == "_retrieve_kb":
                    return item
    raise AssertionError("_retrieve_kb not found on Retrieval")


def _kg_retrieval_calls(method: ast.AST):
    """Return Call nodes that invoke *.kg_retriever.retrieval(...)."""
    calls = []
    for node in ast.walk(method):
        if not isinstance(node, ast.Call):
            continue
        func = node.func
        # settings.kg_retriever.retrieval(...)
        if (
            isinstance(func, ast.Attribute)
            and func.attr == "retrieval"
            and isinstance(func.value, ast.Attribute)
            and func.value.attr == "kg_retriever"
        ):
            calls.append(node)
    return calls


def test_retrieve_kb_calls_kg_retriever_exactly_once():
    method = _retrieve_kb_method()
    calls = _kg_retrieval_calls(method)
    assert len(calls) == 1, (
        f"_retrieve_kb must call kg_retriever.retrieval exactly once; found {len(calls)}. "
        "Duplicate calls prepend the KG chunk twice when use_kg=True."
    )


def test_retrieve_kb_kg_call_uses_filtered_kb_ids_and_normalizes_content():
    """The single KG path must pass filtered_kb_ids and map content_with_weight → content."""
    method = _retrieve_kb_method()
    src = ast.get_source_segment(_RETRIEVAL.read_text(encoding="utf-8"), method)
    assert src is not None
    assert "filtered_kb_ids" in src
    # Must normalize the KG chunk field the way bot/chunk APIs expect for prompts.
    assert 'ck["content"] = ck["content_with_weight"]' in src or "ck['content'] = ck['content_with_weight']" in src
    assert 'del ck["content_with_weight"]' in src or "del ck['content_with_weight']" in src
    # Must not still pass the unfiltered raw kb_ids list into kg retrieval.
    # Accept filtered_kb_ids only as the kb id argument near kg_retriever.retrieval.
    calls = _kg_retrieval_calls(method)
    assert len(calls) == 1
    call = calls[0]
    # Third positional arg is kb_ids (query, tenant_ids, kb_ids, ...)
    assert len(call.args) >= 3, ast.dump(call)
    kb_arg = call.args[2]
    assert isinstance(kb_arg, ast.Name) and kb_arg.id == "filtered_kb_ids", (
        f"kg_retriever.retrieval kb_ids arg should be filtered_kb_ids, got {ast.dump(kb_arg)}"
    )
