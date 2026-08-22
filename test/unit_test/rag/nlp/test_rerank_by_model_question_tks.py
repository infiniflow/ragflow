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
"""Regression tests for issue #18472.

The three rerank paths in ``Dealer`` (``rag/nlp/search.py``) build a
per-chunk token list (``ins_tw``) that feeds both the term-similarity
score and, for ``rerank_by_model``, the document string the
cross-encoder scores. Two of the three paths included the chunk's
generated-questions field; the third (``rerank_by_model``) silently
dropped it. The cost was paid (the field was fetched from the index)
and the value discarded.

The fix adds ``question_tks`` to ``rerank_by_model``'s per-chunk token
list. It is added unweighted, consistent with how ``title_tks`` and
``important_kwd`` are handled in that path (the other two paths
multiply by 2/5/6 to weight the term-similarity score, but here the
tokens are joined into a single document string and passed to the
cross-encoder -- repeating a field would distort the model's own
scoring).

The test pins the fix at the AST level: it walks
``Dealer.rerank_by_model`` and asserts that ``question_tks`` is
fetched from ``sres.field[i]`` and included in the per-chunk ``tks``
list, and that the other two rerank paths still include it (regression
guard).
"""

import ast
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[4]
SEARCH_PY = REPO_ROOT / "rag" / "nlp" / "search.py"


def _dealer_class_node():
    """Parse ``rag/nlp/search.py`` and return the ``Dealer`` class AST node.

    Fails the test (not the import) if the source file is missing or the
    class has been renamed, so a future refactor that breaks the
    contract surfaces here rather than as a silent regression.
    """
    if not SEARCH_PY.is_file():
        pytest.fail(f"missing source: {SEARCH_PY}")
    tree = ast.parse(SEARCH_PY.read_text(encoding="utf-8"))
    for node in ast.walk(tree):
        if isinstance(node, ast.ClassDef) and node.name == "Dealer":
            return node
    pytest.fail("`Dealer` class not found in rag/nlp/search.py")


def _method_node(dealer_node, name):
    """Return the AST node of the ``name`` method on ``Dealer``, or fail."""
    for stmt in dealer_node.body:
        if isinstance(stmt, ast.FunctionDef) and stmt.name == name:
            return stmt
    pytest.fail(f"`Dealer.{name}` method not found in rag/nlp/search.py")


def _assigns_in_loop(method_node, target_id):
    """Walk the method body looking for an Assign to a Name with the
    given id, anywhere inside any for-loop. The return value is the
    right-hand side of the assignment, or None if not found.
    """
    for stmt in ast.walk(method_node):
        if not isinstance(stmt, ast.For):
            continue
        for sub in ast.walk(stmt):
            if isinstance(sub, ast.Assign) and len(sub.targets) == 1 and isinstance(sub.targets[0], ast.Name) and sub.targets[0].id == target_id:
                return sub.value
    return None


def _name_nodes_in_value(node):
    """Yield every Name node that appears anywhere in ``node``. Used to
    check whether a specific name (e.g. ``question_tks``) is referenced
    in an expression.
    """
    if isinstance(node, ast.Name):
        yield node.id
    for child in ast.iter_child_nodes(node):
        yield from _name_nodes_in_value(child)


# ---------------------------------------------------------------------------
# rerank_by_model: must include question_tks
# ---------------------------------------------------------------------------


def test_rerank_by_model_extracts_question_tks_from_field():
    """``Dealer.rerank_by_model`` must read ``question_tks`` from the
    chunk's field map. Pre-fix, only ``title_tks`` and ``important_kwd``
    were read; ``question_tks`` was fetched from the index and discarded.
    """
    dealer = _dealer_class_node()
    method = _method_node(dealer, "rerank_by_model")
    qtk_assign = _assigns_in_loop(method, "question_tks")
    assert qtk_assign is not None, (
        "`Dealer.rerank_by_model` no longer reads `question_tks` from `sres.field[i]`; the cross-encoder is now scoring without the chunk's generated questions. See issue #18472."
    )
    # The reading pattern should be `question_tks = [t for t in sres.field[i].get("question_tks", "").split() if t]`
    # (matching the other two rerank paths). Pin the source: it must
    # call sres.field[i].get("question_tks", ...).
    src = ast.unparse(qtk_assign)
    assert "sres.field[i]" in src and "question_tks" in src, (
        f"`Dealer.rerank_by_model`'s question_tks extraction must read `question_tks` from `sres.field[i]` (matching the pattern used by `rerank` and `rerank_with_knn`); got: {src!r}"
    )


def test_rerank_by_model_includes_question_tks_in_tks_list():
    """The per-chunk ``tks`` list in ``rerank_by_model`` must include
    ``question_tks``. Pre-fix, the list was
    ``content_ltks + title_tks + important_kwd`` -- missing
    ``question_tks``. The fix adds it (unweighted) so the cross-encoder
    scores the chunk's questions too.
    """
    dealer = _dealer_class_node()
    method = _method_node(dealer, "rerank_by_model")
    tks_assign = _assigns_in_loop(method, "tks")
    assert tks_assign is not None, (
        "`Dealer.rerank_by_model` no longer assigns a `tks` list inside the loop; the fix may have been moved or the function refactored away from the per-chunk token-list shape."
    )
    src = ast.unparse(tks_assign)
    assert "question_tks" in src, (
        f"`Dealer.rerank_by_model`'s per-chunk `tks` list does not include "
        f"`question_tks`. The pre-fix list was `content_ltks + title_tks + "
        f"important_kwd` and silently dropped the chunk's generated "
        f"questions. got: {src!r}. See issue #18472."
    )


# ---------------------------------------------------------------------------
# Regression guard: the other two rerank paths still include question_tks
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("method_name", ["rerank", "rerank_with_knn"])
def test_other_rerank_paths_still_include_question_tks(method_name):
    """``Dealer.rerank`` and ``Dealer.rerank_with_knn`` already included
    ``question_tks`` in their per-chunk token list (with the ``*6``
    multiplier). The fix to ``rerank_by_model`` must not regress the
    other two paths.
    """
    dealer = _dealer_class_node()
    method = _method_node(dealer, method_name)
    tks_assign = _assigns_in_loop(method, "tks")
    assert tks_assign is not None, f"`Dealer.{method_name}` no longer assigns a `tks` list inside the loop; the per-chunk token-list shape may have been refactored."
    src = ast.unparse(tks_assign)
    assert "question_tks" in src, (
        f"`Dealer.{method_name}`'s per-chunk `tks` list no longer includes "
        f"`question_tks`. This is a regression of the existing behaviour "
        f"(the other two rerank paths have always included it). got: {src!r}"
    )


# ---------------------------------------------------------------------------
# Pin: the question_tks is added unweighted, not multiplied
# ---------------------------------------------------------------------------


def test_rerank_by_model_adds_question_tks_unweighted():
    """``rerank_by_model`` must add ``question_tks`` unweighted, not
    multiplied. The other two paths use ``* 2`` for title_tks, ``* 5``
    for important_kwd, and ``* 6`` for question_tks to weight the
    term-similarity score. But in this path the tokens are joined into
    a single document string and passed to the cross-encoder --
    repeating a field would distort the model's own scoring. A
    future refactor that copies the ``* 6`` multiplier from the other
    paths is caught here.
    """
    dealer = _dealer_class_node()
    method = _method_node(dealer, "rerank_by_model")
    tks_assign = _assigns_in_loop(method, "tks")
    assert tks_assign is not None
    src = ast.unparse(tks_assign)
    # The pre-fix shape was `content_ltks + title_tks + important_kwd`
    # (no question_tks). The fix adds `+ question_tks` (no `* N`).
    # Pin: the binary add of `question_tks` appears without a
    # multiplication factor.
    assert "+ question_tks" in src or "+question_tks" in src, (
        f"`Dealer.rerank_by_model`'s per-chunk `tks` list must add "
        f"`question_tks` unweighted (the path joins tokens into a single "
        f"string for the cross-encoder; repeating a field would distort "
        f"the model's scoring). got: {src!r}"
    )
    # And the multiplication-factor variant must not be present.
    assert "* 6" not in src and "*6" not in src, (
        f"`Dealer.rerank_by_model` must not multiply `question_tks` by "
        f"6 (the other two paths do that to weight the term-similarity "
        f"score, but this path passes the joined string to the "
        f"cross-encoder -- repeating a field would distort the model's "
        f"scoring). got: {src!r}"
    )
