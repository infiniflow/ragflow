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

import os
import re
import subprocess
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[2]
ENTRYPOINT = REPO_ROOT / "docker" / "entrypoint.sh"

# Each entry identifies one of the six foreground loops that the v3 fix
# in #18545 guards. The marker is a substring of the ``echo`` line that
# identifies the loop body. The two go-ingestor loops share the same
# ``echo "Starting go ingestor..."`` line; we disambiguate them with
# ``occurrence`` (1-based: the first match is 1, the second is 2, etc.).
# All six loops must remain guarded; a regression that drops one is
# caught by the parametrized tests below.
LOOPS = [
    ("main_webserver_python", "Attempt to start RAGFlow python server", 1),
    ("admin_python", "Attempt to start Admin python server", 1),
    ("admin_go", "Starting Admin go server", 1),
    ("main_webserver_go", "Starting RAGFlow go server", 1),
    # Two loops in the entrypoint both start with "Starting go ingestor..."
    # (one under range workers, one under fixed workers). They are
    # structurally identical but live in different ``if`` branches of the
    # entrypoint, so each one gets its own test.
    ("go_ingestor_range", "Starting go ingestor", 1),
    ("go_ingestor_fixed", "Starting go ingestor", 2),
]


def _extract_loop_body(marker: str, occurrence: int = 1) -> str:
    """Return the body of the Nth while loop whose echo line contains ``marker``.

    ``occurrence`` is 1-based: 1 returns the first match, 2 the second, etc.
    This is needed for markers that appear multiple times in the file
    (e.g. the go-ingestor loop body is duplicated under both the
    range-workers and fixed-workers branches of ``ENABLE_TASKEXECUTOR``).

    The body excludes the ``while true; do`` and ``done &`` lines. The
    returned string is the verbatim extracted lines, ready to be run
    in a subprocess.
    """
    text = ENTRYPOINT.read_text()
    lines = text.splitlines()
    in_loop = False
    matches_seen = 0
    body_lines: list[str] = []
    for line in lines:
        if not in_loop and re.search(rf'echo "[^"]*{re.escape(marker)}[^"]*"', line):
            matches_seen += 1
            if matches_seen != occurrence:
                continue
            in_loop = True
            continue
        if in_loop:
            if re.match(r"\s*done\s*&\s*$", line):
                break
            if line.strip() == "":
                continue
            body_lines.append(line)
    assert body_lines, (
        f"Could not find loop body for marker {marker!r} occurrence {occurrence} "
        f"(only {matches_seen} occurrences found)"
    )
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

    The test specifically exercises the bash 5.2.21 ``set -e`` behavior
    that motivated the fix. On bash < 5.2, ``set -e`` *is* suspended
    inside a ``while`` loop body, and the v3 fix would be unnecessary.
    We refuse to run the test on bash < 5.2 to make the regression
    detection semantically meaningful.
    """
    # The bash subprocess inherits our env, so BASH_VERSINFO reflects
    # the version of the bash that will actually run the test.
    if (
        int(os.environ.get("BASH_VERSINFO_0", "0")) < 5
        or (
            int(os.environ.get("BASH_VERSINFO_0", "0")) == 5
            and int(os.environ.get("BASH_VERSINFO_1", "0")) < 2
        )
    ):
        # Fall back to invoking bash directly. Note that `$BASH_VERSINFO`
        # expands to the first element of the array (the major version),
        # so we must index explicitly. We also strip the debug and
        # build suffixes (e.g. "5.2.21(1)-release") by reading the
        # major/minor fields directly.
        probe = subprocess.run(
            ["bash", "-c", "echo ${BASH_VERSINFO[0]} ${BASH_VERSINFO[1]}"],
            capture_output=True,
            text=True,
            timeout=5,
        )
        try:
            parts = probe.stdout.strip().split()
            major = int(parts[0])
            minor = int(parts[1]) if len(parts) > 1 else 0
        except (ValueError, IndexError):
            pytest.skip(
                f"could not determine bash version from output: {probe.stdout!r}"
            )
        if major < 5 or (major == 5 and minor < 2):
            pytest.skip(
                f"test requires bash 5.2+ to exercise the bash 5.2.21 set -e behavior "
                f"that motivated the fix; found {major}.{minor}"
            )

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


@pytest.mark.parametrize(
    "loop_name,marker,occurrence",
    LOOPS,
    ids=[name for name, _, _ in LOOPS],
)
def test_loop_wrapper_survives_non_zero_exit(
    loop_name: str, marker: str, occurrence: int
) -> None:
    """The loop wrapper must survive a non-zero exit from the wrapped process.

    Without the ``set +e`` / ``set -e`` bracket introduced in #18545,
    bash 5.2.21's ``set -e`` enforcement inside a ``while`` loop body
    kills the wrapper on the first non-zero exit. This is the bug from
    #18542.
    """
    body = _extract_loop_body(marker, occurrence=occurrence)
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


@pytest.mark.parametrize(
    "loop_name,marker,occurrence",
    LOOPS,
    ids=[name for name, _, _ in LOOPS],
)
def test_loop_wrapper_survives_clean_zero_exit(
    loop_name: str, marker: str, occurrence: int
) -> None:
    """The loop wrapper must also survive a clean exit 0 from the wrapped process.

    Exercises the ``if [ "$status" -eq 0 ]`` branch of the if/else. On
    the broken pre-#18545 v1 polish (capture ``status=$?`` directly
    with no ``set +e`` / ``set -e`` bracket), the ``false`` exit
    propagates through ``$?`` and the test fails; the clean-exit
    branch alone would have passed on the broken v1, so this test
    primarily guards the v3 polish regression where the if/else
    branches both need to keep their separate log messages.
    """
    body = _extract_loop_body(marker, occurrence=occurrence)
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


@pytest.mark.parametrize(
    "loop_name,marker,occurrence",
    LOOPS,
    ids=[name for name, _, _ in LOOPS],
)
def test_each_loop_has_own_set_plus_e_set_minus_e_guard(
    loop_name: str, marker: str, occurrence: int
) -> None:
    """Each of the six guarded loops must have its own ``set +e`` / ``set -e`` pair.

    A global count of ``set +e`` occurrences in the file is not
    sufficient: a future refactor could drop one of the guards but
    keep the count right by accident (e.g. by adding a guard to the
    wrong place). This test extracts each loop's body individually
    and asserts the body has its own bracket.
    """
    body = _extract_loop_body(marker, occurrence=occurrence)
    assert "set +e" in body, (
        f"[{loop_name}] extracted loop body does not contain `set +e`. "
        f"Body:\n{body}"
    )
    assert "set -e" in body, (
        f"[{loop_name}] extracted loop body does not contain `set -e`. "
        f"Body:\n{body}"
    )
