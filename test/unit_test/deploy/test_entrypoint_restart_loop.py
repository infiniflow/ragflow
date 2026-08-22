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
"""Regression tests for issue #18542.

``docker/entrypoint.sh`` runs each service under a ``while true; do ...
done &`` loop. When the inner command crashed (e.g. ``api/ragflow_server.py``
aborted on the infinity 120s readiness timeout during cold start), the
web server's loop wrapper silently disappeared -- leaving nginx serving
HTML with no backend on port 9380. The admin / task-executor / data-sync
loops restarted on the same crash, but the web server's loop did not.

The fix wraps the inner command of each restart loop with
``|| echo "X server exited, restarting in 1s..."`` so the loop body
always exits 0 and the loop continues. The misleading ``X server started``
log line (which printed AFTER the inner command exited, not after it
started) is removed.

These tests parse ``docker/entrypoint.sh`` with a small regex-based
scanner and pin the loop-body structure. They run as pytest tests
without importing the script (the script lives at the repo root and
is not a Python module). The repo-root path is derived from
``Path(__file__).resolve().parents[4]``.
"""

import re
import shutil
import subprocess
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[3]
ENTRYPOINT = REPO_ROOT / "docker" / "entrypoint.sh"


def _read_entrypoint():
    if not ENTRYPOINT.is_file():
        pytest.fail(f"missing script: {ENTRYPOINT}")
    return ENTRYPOINT.read_text(encoding="utf-8")


def _restart_loop_bodies(source, *, require_background=True, require_no_wait=True):
    """Yield the body of each ``while true; do ... done &`` restart loop,
    in source order. The body's text is the full literal block between
    ``do`` and ``done`` on the same indentation level.

    Filters:
    - ``require_background``: only yield loops whose ``done`` ends in ``&``
      (the loop is itself backgrounded). The fix for issue #18542 only
      targets these -- a synchronous ``while true`` inside a function
      is supervised by ``wait`` and does not have the bug.
    - ``require_no_wait``: only yield loops whose body does not contain
      ``wait``. The ``sync_data_source.py`` / ``task_executor.py``
      patterns use ``wait`` to supervise the backgrounded inner
      process -- ``wait`` itself absorbs the failed exit code so the
      ``while`` body's last command (``sleep 1``) is always successful
      and the loop continues. Those loops are robust without the
      ``|| echo ...`` guard.
    """
    lines = source.splitlines()
    i = 0
    while i < len(lines):
        m = re.search(r"\bwhile\s+true\s*;\s*do\b", lines[i])
        if not m:
            i += 1
            continue
        # Find the matching `done` at the same indentation. The loop in
        # entrypoint.sh is short (3-5 lines) and the `done` is the next
        # line starting with the same indent as the `while`.
        while_indent = len(lines[i]) - len(lines[i].lstrip())
        j = i + 1
        while j < len(lines):
            stripped = lines[j].lstrip()
            line_indent = len(lines[j]) - len(stripped)
            if stripped.startswith("done") and line_indent == while_indent:
                # Yield the body lines (between `do` and `done`).
                body = "\n".join(lines[i + 1 : j])
                is_backgrounded = stripped.endswith("&")
                is_supervised = "wait" in body
                if (not require_background or is_backgrounded) and (not require_no_wait or not is_supervised):
                    yield body
                i = j + 1
                break
            j += 1
        else:
            return  # no matching `done` found


# ---------------------------------------------------------------------------
# Pin the restart-loop structure: each loop must (a) call the inner command
# with `|| echo "... exited, restarting in 1s..."` so a non-zero exit does
# not break the loop, and (b) NOT include the misleading `X server started`
# log line that printed AFTER the process exited.
# ---------------------------------------------------------------------------


EXPECTED_RESTART_LOOP_MARKERS = [
    "admin/server/admin_server.py",
    "bin/ragflow_server --admin",
    "api/ragflow_server.py",
    "bin/ragflow_server --api",
    "bin/ragflow_server --ingestor",
]


@pytest.mark.parametrize("inner_command", EXPECTED_RESTART_LOOP_MARKERS)
def test_restart_loop_guards_inner_command_with_or_echo(inner_command):
    """The inner command for each restart loop must be guarded with
    ``|| echo "... exited, restarting in 1s..."`` so a crash does not
    break the loop. Pre-fix, the inner command was un-guarded; on
    certain bash versions / configurations the failed command in the
    while-body would silently exit the backgrounded loop wrapper,
    leaving the service down with no automatic recovery.
    """
    source = _read_entrypoint()
    bodies = list(_restart_loop_bodies(source, require_background=True))
    matching = [b for b in bodies if inner_command in b]
    assert matching, (
        f"no restart loop in entrypoint.sh runs the inner command {inner_command!r}; "
        f"loops seen: {len(bodies)}. This typically means the script was "
        f"refactored; if the new structure still works, the marker list needs "
        f"to be updated."
    )
    body = matching[0]
    assert "exited, restarting in 1s..." in body, (
        f"the restart loop running {inner_command!r} must guard the inner "
        f"command with `|| echo '... exited, restarting in 1s...'`. The "
        f"un-guarded form let the backgrounded loop wrapper silently "
        f"disappear after a crash (issue #18542). body:\n{body}"
    )


@pytest.mark.parametrize("inner_command", EXPECTED_RESTART_LOOP_MARKERS)
def test_restart_loop_drops_misleading_started_log_line(inner_command):
    """The pre-fix script logged ``X server started`` AFTER the inner
    command exited, so the message printed on crash too. The fix
    removes that misleading line; the body must NOT contain it.
    """
    source = _read_entrypoint()
    bodies = list(_restart_loop_bodies(source, require_background=True))
    matching = [b for b in bodies if inner_command in b]
    assert matching, f"no restart loop runs the inner command {inner_command!r}"
    body = matching[0]
    assert "server started" not in body, (
        f"the restart loop running {inner_command!r} must not contain the "
        f"misleading 'X server started' log line -- pre-fix, that line "
        f"printed AFTER the inner command exited (crash or normal), so "
        f"a crash-restart looked identical to a healthy start. body:\n{body}"
    )


def test_every_restart_loop_uses_the_protected_pattern():
    """No restart loop is allowed to skip the protection. A regression
    that adds a new service loop (or removes the guard from an existing
    one) is caught here.
    """
    source = _read_entrypoint()
    bodies = list(_restart_loop_bodies(source, require_background=True))
    assert len(bodies) >= 4, (
        f"expected at least 4 backgrounded restart loops in entrypoint.sh "
        f"(admin python, admin go, web python, web go); found {len(bodies)}. "
        f"If the script's structure changed, this test needs to be "
        f"updated alongside the change."
    )
    for idx, body in enumerate(bodies):
        assert "exited, restarting in 1s..." in body, (
            f"restart loop #{idx + 1} does not guard its inner command with `|| echo '... exited, restarting in 1s...'`. The un-guarded form is the bug from issue #18542. body:\n{body}"
        )
        assert "server started" not in body, f"restart loop #{idx + 1} contains the misleading 'X server started' log line that printed on crash too. body:\n{body}"


# ---------------------------------------------------------------------------
# Bash syntax check
# ---------------------------------------------------------------------------


def test_entrypoint_sh_is_syntactically_valid_bash():
    """The fix changes the loop bodies; ``bash -n`` catches any syntax
    regression before it ships.
    """
    if shutil.which("bash") is None:
        pytest.skip("bash is not available on this platform")
    result = subprocess.run(["bash", "-n", str(ENTRYPOINT)], capture_output=True, text=True, check=False)
    assert result.returncode == 0, f"bash -n failed on {ENTRYPOINT}\nstdout: {result.stdout}\nstderr: {result.stderr}"


# ---------------------------------------------------------------------------
# Sanity: the four expected restart loops are all present
# ---------------------------------------------------------------------------


def test_all_four_expected_restart_loops_are_present():
    """Sanity check: the four expected restart loops are all present in
    the script. A future refactor that drops one (e.g. removes the go
    server) surfaces here, alongside an update to the marker list.
    """
    source = _read_entrypoint()
    bodies = list(_restart_loop_bodies(source, require_background=True))
    bodies_str = "\n".join(bodies)
    for marker in EXPECTED_RESTART_LOOP_MARKERS:
        assert marker in bodies_str, f"expected restart loop running {marker!r} is missing from entrypoint.sh. If the service was removed, drop the marker from EXPECTED_RESTART_LOOP_MARKERS."
