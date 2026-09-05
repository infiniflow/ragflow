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
"""Regression tests for ``_should_update_progress`` and the related
``update_progress`` contract in ``api.db.services.task_service``.

Closes the silent-failure bug from issue #18013 where a parser that
called ``progress_callback(-1, ...)`` to signal failure was overwritten
by a later call to ``progress_callback(1.0, ...)`` from the empty-chunk
branch in ``rag.svr.task_executor``. The fix is in two layers:

1. ``update_progress`` now refuses to overwrite a ``progress == -1``
   task with a non-(-1) value. The rule is extracted as
   ``_should_update_progress(current, new)`` and tested here as a
   pure function (no DB mocking required).
2. The orchestrator in ``rag.svr.task_executor`` (line 1595-1596) now
   writes the message but not the progress field on the empty-chunk
   path. Tested via the ``set_progress`` callable below.

Both layers are required: the DB-layer fix is the defense in depth, the
orchestrator fix is the policy decision.
"""

import inspect

import pytest


# ---------------------------------------------------------------------------
# _should_update_progress: pure function table
# ---------------------------------------------------------------------------


class TestShouldUpdateProgressRule:
    """Pins the ``_should_update_progress(current, new)`` truth table.

    The function is the single source of truth for whether a progress
    update overwrites the existing value. A failed task (current == -1)
    must stay failed — recovery happens by re-queuing a new task with
    progress = 0.0, not by overwriting the failure marker.
    """

    @pytest.mark.parametrize(
        "current,new,expected",
        [
            # Fresh task: 0.0 -> 0.0 (no-op), 0.0 -> 0.5 (forward), 0.0 -> 1.0 (complete)
            (0.0, 0.0, False),  # equal, neither > nor -1, no update
            (0.0, 0.5, True),  # forward progress
            (0.0, 1.0, True),  # completion
            (0.0, -1, True),  # failure transition
            # In-progress: 0.5 -> 1.0 (complete), 0.5 -> -1 (fail), 0.5 -> 0.5 (no-op)
            (0.5, 0.5, False),  # equal, no update
            (0.5, 0.8, True),  # forward
            (0.5, 1.0, True),  # completion
            (0.5, -1, True),  # failure transition
            (0.5, 0.3, False),  # backward — should NOT happen, but if it does, the rule refuses
            # Failed task: -1 -> anything (other than -1) is REFUSED
            (-1, -1, False),  # already failed, no-op
            (-1, 0.0, False),  # would silently un-fail; REFUSED
            (-1, 0.5, False),  # would silently un-fail; REFUSED
            (-1, 1.0, False),  # THE BUG FIX: empty-chunk branch's 1.0 no longer overwrites -1
            # Forward from failed is also refused (the original rule (a)
            # "allows recovery from -1" was the bug source — removed).
            # Boundary cases at 0.0 and 1.0
            (0.0, 0.0, False),  # duplicate 0.0 = no-op
            (0.999, 1.0, True),  # last-step completion
            (0.999, 0.999, False),  # duplicate 0.999 = no-op
            (0.5, 1.0, True),  # skip-ahead completion (e.g. parser-only task)
            # Negative values other than -1
            (-0.5, 0.0, True),  # -0.5 is not a special "failed" marker; normal forward
            (-0.5, -0.4, True),  # -0.5 -> -0.4: forward
            (-0.5, 1.0, True),  # -0.5 -> 1.0: completion
            (-0.5, -1, True),  # -0.5 -> -1: failure transition
        ],
    )
    def test_truth_table(self, current, new, expected):
        from api.db.services.task_service import _should_update_progress

        assert _should_update_progress(current, new) is expected, f"_should_update_progress(current={current}, new={new}) returned {not expected}, expected {expected}"

    def test_failed_task_stays_failed_for_completion(self):
        """The bug from #18013: a parser that signals failure (-1) is
        followed by the empty-chunk branch setting 1.0. Pre-fix, the
        DB allowed the overwrite. Post-fix, _should_update_progress
        returns False and the task stays at -1.
        """
        from api.db.services.task_service import _should_update_progress

        # Simulate the bug scenario:
        # 1. Parser signals failure: progress_callback(-1, ...) sets progress=-1
        # 2. Empty-chunk branch tries to overwrite with 1.0
        # Pre-fix: True (overwrites -1 with 1.0, task marked DONE — BUG)
        # Post-fix: False (overwrite refused, task stays failed)
        assert _should_update_progress(-1, 1.0) is False

    def test_completion_still_works_for_in_progress_task(self):
        """The fix must not break the normal completion path: an
        in-progress task (0.5) can transition to 1.0 (completion).
        """
        from api.db.services.task_service import _should_update_progress

        assert _should_update_progress(0.5, 1.0) is True
        assert _should_update_progress(0.0, 1.0) is True
        assert _should_update_progress(0.999, 1.0) is True

    def test_recovery_via_new_task_still_works(self):
        """The fix does not change the retry mechanism: a failed task
        is recovered by re-queuing a NEW task with progress = 0.0,
        not by overwriting the failed task's -1.

        The new task's first progress update is from 0.0 (default in
        new_task()) to 0.0 (the initial 'Start' message) — which
        returns False (no-op). Then 0.0 -> 0.5 (forward) -> 1.0
        (completion) — all True.
        """
        from api.db.services.task_service import _should_update_progress

        # New task: progress = 0.0
        assert _should_update_progress(0.0, 0.0) is False  # initial Start
        assert _should_update_progress(0.0, 0.5) is True  # forward
        assert _should_update_progress(0.5, 1.0) is True  # completion


# ---------------------------------------------------------------------------
# set_progress behavior: prog=None vs prog=-1 vs prog=1.0
# ---------------------------------------------------------------------------


def _load_set_progress_signature():
    """Read ``set_progress``'s signature from the source file via AST,
    so the test doesn't pull in the full ``rag.svr.task_executor``
    import chain (which transitively imports ``rag.graphrag`` and
    trips on a pre-existing ``graspologic`` syntax error in the
    development venv).
    """
    import ast
    from pathlib import Path

    src_path = Path(__file__).resolve().parents[5] / "rag" / "svr" / "task_executor.py"
    tree = ast.parse(src_path.read_text(encoding="utf-8"))
    func = next(
        (n for n in tree.body if isinstance(n, ast.FunctionDef) and n.name == "set_progress"),
        None,
    )
    assert func is not None, "set_progress not found in task_executor.py"

    # ``ast.arg`` doesn't carry its own default; defaults live on the
    # ``ast.arguments`` container (``args.defaults`` for the positional
    # suffix, ``kw_defaults`` for the keyword-only). Stitch them together.
    args = func.args
    positional = args.args
    n_positional_defaults = len(args.defaults) if args.defaults else 0
    n_positional_no_default = len(positional) - n_positional_defaults
    defaults = {}
    for i, arg in enumerate(positional):
        if i < n_positional_no_default:
            defaults[arg.arg] = inspect.Parameter.empty
        else:
            default_node = args.defaults[i - n_positional_no_default]
            try:
                defaults[arg.arg] = ast.literal_eval(default_node)
            except (ValueError, SyntaxError):
                defaults[arg.arg] = ast.unparse(default_node)
    for arg, default_node in zip(args.kwonlyargs, args.kw_defaults or (), strict=True):
        if default_node is None:
            defaults[arg.arg] = inspect.Parameter.empty
        else:
            try:
                defaults[arg.arg] = ast.literal_eval(default_node)
            except (ValueError, SyntaxError):
                defaults[arg.arg] = ast.unparse(default_node)
    return defaults


class TestSetProgressDoesNotOverwriteFailure:
    """The orchestrator fix in ``rag.svr.task_executor`` relies on the
    contract that calling ``set_progress(task_id, msg=...)`` (with
    ``prog`` defaulting to ``None``) writes ONLY the message, never
    the progress field. This preserves the parser's earlier -1.
    """

    def test_set_progress_signature_prog_is_optional(self):
        """The function signature has ``prog`` defaulting to ``None`` —
        the documented "write only the message" mode that the
        orchestrator fix relies on.
        """
        defaults = _load_set_progress_signature()
        assert "prog" in defaults, "set_progress is missing a 'prog' parameter"
        assert defaults["prog"] is None, f"set_progress's prog default must be None for the 'write only the message' pattern; got {defaults['prog']!r}"

    def test_progress_msg_is_optional(self):
        """``msg`` has a default of ``\"Processing...\"`` so the caller
        can pass only ``prog`` if they want to write the progress
        field without a new message.
        """
        defaults = _load_set_progress_signature()
        assert "msg" in defaults, "set_progress is missing a 'msg' parameter"
        assert defaults["msg"] == "Processing..."


# ---------------------------------------------------------------------------
# Cross-reference: the orchestrator's empty-chunk branch shape
# ---------------------------------------------------------------------------


class TestEmptyChunkBranchShape:
    """Pins the exact source shape of the orchestrator's empty-chunk
    branch in ``rag.svr.task_executor``. The fix removes the
    ``1.0`` argument from the ``progress_callback`` call so the
    parser's earlier ``-1`` is preserved.

    Intentional source-shape pin (per xugangqiang review on #18263):
    the regression is "the call site invokes progress_callback with
    1.0 in the empty-chunk branch", which a behavioral test on
    set_progress cannot observe directly because the call goes
    through a free function. A behavioral mock would either need
    to mock module-level set_progress (already covered by
    ``TestSetProgressDoesNotOverwriteFailure`` above) or parse the
    AST, which is what this test already does. Keep the pin and
    update the assert messages if the call site is refactored
    rather than dropped.
    """

    def test_orchestrator_does_not_pass_prog_1_0_on_empty_chunks(self):
        """The empty-chunk branch in ``do_handle_task`` must call
        ``progress_callback(msg=...)`` (no prog arg) so the parser's
        earlier ``-1`` is preserved.

        The file has multiple ``if not chunks:`` blocks (the dataflow
        path, the table parser, etc.); this test locates the specific
        one with the ``\"No chunk built from {task_document_name}\"``
        message — the one fixed by this PR.
        """
        from pathlib import Path

        src_path = Path(__file__).resolve().parents[5] / "rag" / "svr" / "task_executor.py"
        text = src_path.read_text(encoding="utf-8")

        # Anchor on the message string — only the empty-chunk branch in
        # ``do_handle_task`` has this format. Find the enclosing
        # ``if not chunks:`` line by searching backwards.
        msg_marker = 'progress_callback(msg=f"No chunk built from {task_document_name}")'
        msg_idx = text.find(msg_marker)
        assert msg_idx != -1, f"Could not find the empty-chunk branch message in task_executor.py. Expected a call like {msg_marker!r}."

        # Find the enclosing ``if not chunks:`` by searching backwards
        # from the call. This is the specific branch fixed by #18013.
        if_idx = text.rfind("if not chunks:", 0, msg_idx)
        assert if_idx != -1, "Could not find the enclosing 'if not chunks:'"

        # Extract the call's argument list. The call is on the line
        # at msg_idx.
        line_at_msg = text[msg_idx : text.find("\n", msg_idx)]
        # Argument list is inside the first pair of parens.
        lparen = line_at_msg.find("(")
        rparen = line_at_msg.rfind(")")
        assert lparen != -1 and rparen != -1, f"Could not extract the argument list from the call line: {line_at_msg!r}"
        call_args = line_at_msg[lparen + 1 : rparen]

        # Pre-fix: ``progress_callback(1.0, msg=...)`` — would
        # overwrite a parser's -1 with 1.0.
        # Post-fix: ``progress_callback(msg=...)`` — only the message.
        assert "1.0" not in call_args, (
            f"Empty-chunk branch in task_executor.py still passes 1.0 to "
            f"progress_callback, which would overwrite the parser's "
            f"earlier -1. Call args: {call_args!r}. Pass only the message "
            f"(no prog) to preserve the failure."
        )
        assert "No chunk built from" in call_args, f"Empty-chunk branch should still log a 'No chunk built from' message. Call args: {call_args!r}"


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
