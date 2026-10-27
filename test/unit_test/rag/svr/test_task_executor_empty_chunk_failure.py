"""Regression tests for infiniflow/ragflow#18013 (rework of closed PR #18263).

A parser that signals failure via ``progress_callback(-1, ...)`` during
``build_chunks`` and then returns zero chunks must not be overwritten to
``progress = 1.0`` ("DONE") by the empty-chunk branch in ``do_handle_task``.

The rework avoids ``TaskService.get_task`` entirely: that method is not a
read-only getter (it mutates progress/retry_count and can abandon the task),
which was the maintainer's design objection to the original PR. Instead the
``progress_callback`` closure records a same-run failure signal locally.
"""

import ast
import textwrap
from pathlib import Path

TASK_EXECUTOR = (
    Path(__file__).resolve().parents[4] / "rag" / "svr" / "task_executor.py"
)
SOURCE = TASK_EXECUTOR.read_text()
TREE = ast.parse(SOURCE)


def _do_handle_task() -> ast.AsyncFunctionDef:
    for node in ast.walk(TREE):
        if isinstance(node, ast.AsyncFunctionDef) and node.name == "do_handle_task":
            return node
    raise AssertionError("do_handle_task not found in task_executor.py")


def _empty_chunk_if(func: ast.AsyncFunctionDef) -> ast.If:
    """Find the ``if not chunks:`` guard in do_handle_task."""
    for node in ast.walk(func):
        if isinstance(node, ast.If) and isinstance(node.test, ast.UnaryOp):
            if isinstance(node.test.operand, ast.Name) and node.test.operand.id == "chunks":
                return node
    raise AssertionError("empty-chunk branch not found")


def test_no_get_task_call_in_do_handle_task():
    """The maintainer-blocked pattern must not come back: no get_task()."""
    for node in ast.walk(_do_handle_task()):
        if isinstance(node, ast.Call):
            f = node.func
            if isinstance(f, ast.Attribute) and f.attr == "get_task":
                raise AssertionError(
                    "do_handle_task calls TaskService.get_task (has write side effects)"
                )


def test_callback_wrapper_records_failure_signal():
    """A local wrapper around set_progress must record prog < 0."""
    func = _do_handle_task()
    wrappers = [
        n for n in func.body
        if isinstance(n, ast.FunctionDef) and n.name == "progress_callback"
    ]
    assert wrappers, "progress_callback wrapper not defined in do_handle_task"
    src = ast.get_source_segment(SOURCE, wrappers[0])
    assert "prog is not None and prog < 0" in src, "wrapper does not detect prog < 0"
    assert 'run_state["failure_signaled"] = True' in src, "wrapper does not record the failure"
    assert "_set_progress(prog=prog, msg=msg)" in src, "wrapper must forward to set_progress"


def test_empty_chunk_branch_guards_on_failure_signal():
    """The branch must only write the message when a failure was signaled."""
    branch = _empty_chunk_if(_do_handle_task())
    inner = branch.body[0]
    assert isinstance(inner, ast.If), "inner failure-guard If not found"
    inner_src = ast.get_source_segment(SOURCE, inner)
    assert 'run_state["failure_signaled"]' in inner_src, "branch does not check the failure signal"
    then_src = ast.get_source_segment(SOURCE, inner.body[0])
    assert "progress_callback(msg=" in then_src, "failure path must be message-only"
    assert "1.0" not in then_src, "failure path must not write progress=1.0"
    else_src = ast.get_source_segment(SOURCE, inner.orelse[0])
    assert "progress_callback(1.0" in else_src, "genuinely-empty path must still complete at 1.0"


def _extract_runtime_harness() -> str:
    """Build an executable harness from the REAL wrapper and branch source."""
    wrapper = next(
        n for n in _do_handle_task().body
        if isinstance(n, ast.FunctionDef) and n.name == "progress_callback"
    )
    wrapper_src = textwrap.dedent(ast.get_source_segment(SOURCE, wrapper))
    branch_src = textwrap.dedent(
        ast.get_source_segment(SOURCE, _empty_chunk_if(_do_handle_task()))
    )
    branch_src = textwrap.indent(branch_src, "    ").rstrip()
    assert branch_src.endswith("return"), "extracted branch should end in return"
    branch_src = branch_src[: -len("return")] + "return calls"
    return (
        "def simulate(fail_first):\n"
        "    calls = []\n"
        "\n"
        "    def _set_progress(prog=None, msg=\"Processing...\"):\n"
        "        calls.append(prog)\n"
        "\n"
        "    run_state = {\"failure_signaled\": False}\n"
        "\n"
        + textwrap.indent(wrapper_src, "    ")
        + "\n\n"
        "    if fail_first:\n"
        "        progress_callback(-1, msg=\"parser exploded\")\n"
        "\n"
        "    chunks = []\n"
        "    task_document_name = \"doc.pdf\"\n"
        + branch_src
        + "\n"
    )


def _run_simulation(fail_first: bool):
    namespace = {}
    exec(_extract_runtime_harness(), namespace)
    return namespace["simulate"](fail_first)


def test_runtime_failed_parser_keeps_failure_state():
    """Parser signaled -1, then zero chunks: no progress=1.0 may be written."""
    first = _run_simulation(True)
    second = _run_simulation(True)
    assert first == second, "simulation is not deterministic"
    assert 1.0 not in first, f"progress=1.0 written over a failure: {first}"
    assert first == [-1, None], f"expected [-1, msg-only], got {first}"


def test_runtime_genuinely_empty_document_completes():
    """No failure signal, zero chunks: terminal progress=1.0 is written."""
    first = _run_simulation(False)
    second = _run_simulation(False)
    assert first == second, "simulation is not deterministic"
    assert first == [1.0], f"expected [1.0], got {first}"
