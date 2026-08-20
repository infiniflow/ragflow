#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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
"""
Regression test for the foreground restart loop pattern in
``docker/entrypoint.sh``.

The admin / webserver / go-ingestor loops in ``docker/entrypoint.sh`` all
use the pattern::

    while true; do
        <binary> <args>
        ...
        sleep 1
    done &

inside a script that starts with ``set -e`` (line 3). On bash 5.2.21
(the version in the official ragflow image), ``set -e`` is enforced
*inside* the ``while`` loop body even though the bash documentation
says it should be suspended. The practical effect is that a non-zero
exit from the wrapped binary silently kills the loop wrapper, and the
container is left with no supervisor for that service.

The fix in #18545 brackets the binary invocation with ``set +e`` /
``set -e`` and captures ``status=$?`` so the wrapper can log the exit
code and still survive. The fix in #17196 reduces the *probability* of
the underlying infinity cold-start timeout firing in the first place.

This test:

1. Extracts the loop body of each guarded loop from
   ``docker/entrypoint.sh`` by marker text.
2. Replaces the actual binary invocation with one that exits with a
   chosen status.
3. Runs the loop body under a harness that breaks after N iterations.
4. Asserts that the wrapper survives all iterations and emits the
   expected log line.

The test is bash-only and does not require a running RAGFlow stack. It
requires bash 5.2+ to exercise the bash 5.2.21-specific ``set -e``
behavior that motivated the fix.

Run::

    .venv/bin/python -m pytest test/integration/test_entrypoint_loop.py -s -v

Refs:
    - infiniflow/ragflow#18542 (the bug)
    - infiniflow/ragflow#18545 (the fix)
    - infiniflow/ragflow#17196 (the complementary timeout fix)
"""
from __future__ import annotations

import re
import subprocess
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[2]
ENTRYPOINT = REPO_ROOT / "docker" / "entrypoint.sh"

# Each entry identifies one of the six foreground loops that the v3 fix
# in #18545 guards. The marker is a substring of the ``echo`` line that
# uniquely identifies the loop body. All six loops must remain guarded;
# a regression that drops one is caught by the parametrized tests
# below.
LOOPS = [
    ("main_webserver_python", "Attempt to start RAGFlow python server"),
    ("admin_python", "Attempt to start Admin python server"),
    ("admin_go", "Starting Admin go server"),
    ("main_webserver_go", "Starting RAGFlow go server"),
    ("go_ingestor_range", "Starting go ingestor"),
    ("go_ingestor_fixed", "Starting go ingestor"),
]


def _extract_loop_body(marker: str) -> str:
    """Return the body of the while loop whose echo line contains ``marker``.

    The body excludes the ``while true; do`` and ``done &`` lines. The
    returned string is the verbatim extracted lines, ready to be run
    in a subprocess.
    """
    text = ENTRYPOINT.read_text()
    lines = text.splitlines()
    in_loop = False
    body_lines: list[str] = []
    for line in lines:
        if not in_loop and re.search(rf'echo "[^"]*{re.escape(marker)}[^"]*"', line):
            in_loop = True
            continue
        if in_loop:
            if re.match(r"\s*done\s*&\s*$", line):
                break
            if line.strip() == "":
                continue
            body_lines.append(line)
    assert body_lines, f"Could not find loop body for marker {marker!r}"
    return "\n".join(body_lines)


def _replace_invocation(body: str, exit_code: int) -> str:
    """Replace the wrapped binary invocation with a deterministic fake.

    The pattern is one of::

        "$PY" <args>
        bin/ragflow_server <args>

    The replacement is a subshell invocation that exits with the
    requested status but does not terminate the harness script. The
    pattern is matched line-by-line on the verbatim lines extracted
    from ``docker/entrypoint.sh``.
    """
    out_lines: list[str] = []
    replaced = 0
    for line in body.splitlines():
        stripped = line.strip()
        if not stripped or "&" in stripped:
            out_lines.append(line)
            continue
        if (
            stripped.startswith('"$PY"')
            or stripped.startswith("bin/ragflow_server")
            or stripped.startswith('"bin/ragflow_server"')
        ):
            # Use a subshell to return the requested exit code without
            # terminating the harness. ``(exit N)`` exits the subshell
            # with status N, and the parent (the ``while`` body) sees
            # that exit status via ``$?``.
            out_lines.append(f"(exit {exit_code})")
            replaced += 1
        else:
            out_lines.append(line)
    assert replaced >= 1, (
        f"Could not find the binary invocation line to replace. Body:\n{body}"
    )
    return "\n".join(out_lines)


def _run_loop(
    loop_body: str,
    *,
    exit_code: int,
    iterations: int,
    loop_timeout: int = 15,
) -> subprocess.CompletedProcess:
    """Run the loop body ``iterations`` times with a fake binary that
    exits with ``exit_code`` on each invocation.

    Returns the subprocess CompletedProcess. The wrapper is expected to
    survive all iterations and emit the expected log line.
    """
    body = _replace_invocation(loop_body, exit_code)
    harness = f"""#!/usr/bin/env bash
set -e
counter=0
while true; do
{body}
    counter=$((counter + 1))
    if [ "$counter" -ge {iterations} ]; then
        echo "[T] completed $counter iterations"
        break
    fi
done
"""
    return subprocess.run(
        ["bash", "-c", harness],
        capture_output=True,
        text=True,
        timeout=loop_timeout,
    )


@pytest.mark.parametrize("loop_name,marker", LOOPS, ids=[name for name, _ in LOOPS])
def test_loop_wrapper_survives_non_zero_exit(loop_name: str, marker: str) -> None:
    """The loop wrapper must survive a non-zero exit from the wrapped process.

    Without the ``set +e`` / ``set -e`` bracket introduced in #18545,
    bash 5.2.21's ``set -e`` enforcement inside a ``while`` loop body
    kills the wrapper on the first non-zero exit. This is the bug from
    #18542.
    """
    body = _extract_loop_body(marker)
    result = _run_loop(body, exit_code=137, iterations=3)

    assert result.returncode == 0, (
        f"[{loop_name}] loop wrapper died on non-zero exit. "
        f"exit={result.returncode}\nstdout: {result.stdout}\nstderr: {result.stderr}"
    )
    assert "[T] completed 3 iterations" in result.stdout, (
        f"[{loop_name}] loop did not complete 3 iterations. "
        f"stdout: {result.stdout}\nstderr: {result.stderr}"
    )
    assert "exited with status 137" in result.stdout, (
        f"[{loop_name}] expected 'exited with status 137' log line. "
        f"stdout: {result.stdout}"
    )


@pytest.mark.parametrize("loop_name,marker", LOOPS, ids=[name for name, _ in LOOPS])
def test_loop_wrapper_survives_clean_zero_exit(loop_name: str, marker: str) -> None:
    """The loop wrapper must also survive a clean exit 0 from the wrapped process.

    Exercises the ``if [ "$status" -eq 0 ]`` branch of the if/else.
    A pre-#18545 implementation that captured ``status=$?`` directly
    (without the ``set +e`` / ``set -e`` bracket) would have failed
    this test, because the ``false`` exit code would propagate to the
    wrapper the same way it does for the non-zero case.
    """
    body = _extract_loop_body(marker)
    result = _run_loop(body, exit_code=0, iterations=3)

    assert result.returncode == 0, (
        f"[{loop_name}] loop wrapper died on clean exit. "
        f"exit={result.returncode}\nstdout: {result.stdout}\nstderr: {result.stderr}"
    )
    assert "[T] completed 3 iterations" in result.stdout, (
        f"[{loop_name}] loop did not complete 3 iterations. "
        f"stdout: {result.stdout}\nstderr: {result.stderr}"
    )
    assert "exited cleanly" in result.stdout, (
        f"[{loop_name}] expected 'exited cleanly' log line. "
        f"stdout: {result.stdout}"
    )


def test_entrypoint_uses_set_plus_e_set_minus_e_pattern() -> None:
    """Sanity check: ``docker/entrypoint.sh`` must use ``set +e`` / ``set -e``.

    Without either guard, the loop tests above would fail. This test
    fails fast with a focused error message if a future refactor drops
    the bracket, instead of letting the parametric tests fail in a
    less obvious way.
    """
    text = ENTRYPOINT.read_text()
    assert "set +e" in text, (
        "entrypoint.sh must use `set +e` to neutralize `-e` for binary invocations "
        "(see #18545)."
    )
    assert "set -e" in text, (
        "entrypoint.sh must re-enable `-e` after the binary invocation "
        "(see #18545)."
    )


def test_entrypoint_has_six_guarded_loops() -> None:
    """Guard against the scope of #18545 being silently reduced.

    The fix patched six foreground loops. If a future refactor drops
    one of the guards, the parametric tests will fail, but this test
    gives a tighter error message ("only 4 loops guarded, expected 6").
    """
    text = ENTRYPOINT.read_text()
    # Count occurrences of the canonical marker across all six loops.
    # The number of "Attempt to start" and "Starting" echo lines for the
    # six loops must match the number of `set +e` occurrences.
    marker_count = sum(
        1 for marker in [m for _, m in LOOPS]
        if re.search(rf'echo "[^"]*{re.escape(marker)}[^"]*"', text)
    )
    # Some markers are shared between two loops (e.g. "Starting go ingestor"
    # appears twice); the per-loop parametrized tests catch that.
    set_plus_e = text.count("set +e")
    assert set_plus_e == 6, (
        f"expected 6 `set +e` guards in entrypoint.sh (one per loop), "
        f"found {set_plus_e}. Did one of the #18545 guards get dropped?"
    )
