"""Shadow check planner/executor; never substitutes for the existing release gate."""

from __future__ import annotations

import argparse
import fnmatch
import json
import os
from pathlib import Path
import shutil
import shlex
import signal
import subprocess
import sys
import time
import xml.etree.ElementTree as ET

from candidate import canonical, digest


ROOT = Path(__file__).resolve().parents[2]
CONTROL = ("tools/", ".github/", "test/", "docker/", "deployment/", "ragflow_deps/")
SHARED = {"pyproject.toml", "uv.lock", "go.mod", "go.sum", "Dockerfile", "build.sh", "lefthook.yml", ".dockerignore"}
WEB_SHARED = ("web/src/utils/", "web/src/services/", "web/src/routes", "web/src/main.")


def changed_paths(root: Path, base: str) -> list[str]:
    # --no-renames includes both old and new names, including deleted paths.
    paths = set()
    for args in (
        ("diff", "--name-only", "--no-renames", "-z", base, "--"),
        ("diff", "--cached", "--name-only", "--no-renames", "-z", base, "--"),
        ("diff", "--name-only", "--no-renames", "-z", "--"),
        ("ls-files", "--others", "--exclude-standard", "-z"),
    ):
        output = subprocess.check_output(["git", "-C", str(root), *args])
        paths.update(p.decode("utf-8") for p in output.split(b"\0") if p)
    return sorted(paths)


def select(paths: list[str], checks: list[dict], mode: str, owners: dict[str, str]) -> tuple[list[str], str]:
    if mode != "quick":
        return [check["id"] for check in checks], f"{mode}: all configured checks"
    selected_owners = set()
    for path in paths:
        if path in SHARED or path.startswith((*CONTROL, *WEB_SHARED)) or fnmatch.fnmatch(path, "web/*lock*") or "config" in Path(path).name:
            return [check["id"] for check in checks], "shared inputs or selection policy changed: conservative full plan"
        owner = owners.get(path)
        if owner is not None:
            selected_owners.add(owner)
        elif path.startswith("web/"):
            selected_owners.add("web")
        elif path.startswith(("api/", "rag/", "agent/", "deepdoc/", "common/", "sdk/")):
            selected_owners.add("python")
        elif path.startswith(("internal/", "cmd/")):
            selected_owners.add("go")
        else:
            return [check["id"] for check in checks], "unknown path: conservative full plan"
    selected = [check["id"] for check in checks if selected_owners.intersection(check["owners"])]
    # An owner with no configured executable checks is not a reason to skip.
    if any(not any(owner in check["owners"] for check in checks) for owner in selected_owners):
        return [check["id"] for check in checks], "owner not covered: conservative full plan"
    return selected, "shadow ownership selection; existing CI remains authoritative"


def coverage_providers(checks: list[dict], selected: list[str]) -> dict[str, str]:
    """Coalesce only the known plain pytest subset into the unchanged full unit command.

    Different flags, wrappers or policies disable this optimization. This is
    evidence sharing within one run, never reuse of an earlier passing result.
    """
    by_id = {check["id"]: check for check in checks}
    if not {"process-contracts", "python-unit"}.issubset(selected):
        return {}
    subset, full = by_id["process-contracts"], by_id["python-unit"]
    if (
        subset["command"] != ["{python}", "-m", "pytest", "test/unit_test/tools", "test/unit_test/deployment", "test/unit_test/test_live_model_profile.py", "-q", "--junitxml={result}"]
        or full["command"] != ["{python}", "run_tests.py", "-i"]
        or full.get("test_options") != ["--junitxml={result}"]
        or subset.get("test_options")
        or any(check.get(key) for check in (subset, full) for key in ("cwd", "env", "isolated", "platform", "tools", "files"))
        or subset["result"] != "junit"
        or full["result"] != "junit"
    ):
        return {}
    return {"process-contracts": "python-unit"}


def parse_result(kind: str, path: Path) -> tuple[str, str]:
    if kind == "exit":
        return "INCOMPLETE", "exit success without structured test evidence; not accepted as a test gate"
    if not path.is_file():
        return "INCOMPLETE", "required result missing"
    try:
        if kind == "junit":
            root = ET.parse(path).getroot()
            if root.tag not in {"testsuites", "testsuite"}:
                return "INCOMPLETE", "invalid JUnit root"
            for suite in root.iter():
                if suite.tag not in {"testsuites", "testsuite"}:
                    continue
                if any(int(suite.get(key, "0")) > 0 for key in ("failures", "errors")):
                    return "FAIL", "JUnit suite failure/error"
                if int(suite.get("skipped", "0")) > 0:
                    return "INCOMPLETE", "JUnit suite skipped cases"
            cases = list(root.iter("testcase"))
            if not cases:
                return "INCOMPLETE", "no test cases"
            if any(case.find("failure") is not None or case.find("error") is not None for case in cases):
                return "FAIL", "test failure/error"
            if any(case.find("skipped") is not None for case in cases):
                return "INCOMPLETE", "required cases skipped"
            return "PASS", f"{len(cases)} test cases"
        if kind == "jest":
            data = json.loads(path.read_bytes())
            if not isinstance(data, dict):
                return "INCOMPLETE", "invalid Jest object"
            for key in ("numTotalTests", "numPassedTests", "numFailedTests", "numFailedTestSuites", "numPendingTests", "numTodoTests", "numPendingTestSuites"):
                value = data.get(key, 0)
                if type(value) is not int or value < 0:
                    return "INCOMPLETE", "invalid Jest counts"
            if data.get("numFailedTests", 0) or data.get("numFailedTestSuites", 0) or data.get("success") is False:
                return "FAIL", "Jest failures"
            if data.get("numPendingTests", 0) or data.get("numTodoTests", 0) or data.get("numPendingTestSuites", 0):
                return "INCOMPLETE", "Jest cases not executed"
            if not data.get("numPassedTests") or data.get("numPassedTests") != data.get("numTotalTests") or data.get("success") is not True:
                return "INCOMPLETE", "incomplete Jest results"
            return "PASS", f"{data['numPassedTests']} test cases"
        return "INCOMPLETE", "unknown result format"
    except (ValueError, OSError, ET.ParseError):
        return "INCOMPLETE", "unreadable result"


def stop_tree(process: subprocess.Popen) -> None:
    if os.name == "nt":
        subprocess.run(["taskkill", "/PID", str(process.pid), "/T", "/F"], capture_output=True, check=False)
    else:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
    process.wait(timeout=10)


def execute(check: dict, root: Path, output: Path, isolated: bool) -> dict:
    result = output / f"{check['id']}.result"
    log = output / f"{check['id']}.log"
    replacements = {"python": sys.executable, "node": shutil.which("node") or "node", "result": str(result)}
    command = [arg.format(**replacements) for arg in check["command"]]
    entry = {"id": check["id"], "command": command, "status": "INCOMPLETE", "seconds": 0}
    missing = []
    if check.get("platform") and check["platform"] != sys.platform:
        missing.append("platform")
    if check.get("isolated") and not isolated:
        missing.append("explicit disposable-stack authorization")
    missing.extend(f"env:{name}" for name in check.get("env", []) if not os.environ.get(name))
    missing.extend(f"file:{name}" for name in check.get("files", []) if not (root / name).is_file())
    missing.extend(f"tool:{name}" for name in [command[0], *check.get("tools", [])] if not shutil.which(name))
    if missing:
        return {**entry, "reason": "missing prerequisites: " + ", ".join(missing)}
    if result.exists() or log.exists():
        return {**entry, "reason": "refuse stale result/log reuse"}
    start = time.monotonic()
    # Local -k/-m/--ignore/--collect-only must not silently narrow a declared lane.
    env = {**os.environ, "PATH": str(Path(sys.executable).parent) + os.pathsep + os.environ.get("PATH", ""), "PYTEST_ADDOPTS": ""}
    if "test_options" in check:
        env["PYTEST_ADDOPTS"] = shlex.join(arg.format(**{**replacements, "result": result.as_posix()}) for arg in check["test_options"])
    with log.open("xb") as stream:
        try:
            process = subprocess.Popen(command, cwd=root / check.get("cwd", "."), env=env, stdout=stream, stderr=subprocess.STDOUT, start_new_session=os.name != "nt")
        except OSError:
            return {**entry, "reason": "process could not start; inspect cwd and executable prerequisites", "seconds": time.monotonic() - start}
        try:
            code = process.wait(timeout=check["timeout"])
        except subprocess.TimeoutExpired:
            stop_tree(process)
            return {**entry, "status": "INCOMPLETE", "reason": "deadline exceeded; process tree terminated", "seconds": time.monotonic() - start}
        except KeyboardInterrupt:
            stop_tree(process)
            return {**entry, "status": "CANCELLED", "reason": "interrupted; process tree terminated", "seconds": time.monotonic() - start}
    entry.update(seconds=time.monotonic() - start, exit_code=code, log=str(log))
    if code:
        return {**entry, "status": "FAIL", "reason": "command failed"}
    status, reason = parse_result(check["result"], result)
    if check["id"] == "frontend-types":
        status, reason = "PASS", "TypeScript compiler completed"
    return {**entry, "status": status, "reason": reason}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["plan", "run", "report"])
    parser.add_argument("--mode", choices=["quick", "candidate", "full"], default="quick")
    parser.add_argument("--base", default="HEAD")
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--isolated-stack", action="store_true", help="operator confirms disposable test resources; never use for shared data")
    args = parser.parse_args()
    if args.action == "report":
        print((args.output / "report.json").read_text())
        return 0
    config = json.loads((ROOT / "tools/quality/checks.yaml").read_bytes())
    if config.get("schema") != 1 or config.get("rollout") != "shadow":
        raise ValueError("unsupported check policy")
    paths = changed_paths(ROOT, args.base)
    # Existing provenance inventory is a responsibility map, not an import graph.
    inventory = json.loads((ROOT / "tools/quality/file-inventory.json").read_bytes())
    owners = {record["path"]: record.get("module", "unknown") for record in inventory["records"]}
    selected, reason = select(paths, config["checks"], args.mode, owners)
    covered_by = coverage_providers(config["checks"], selected)
    plan = {
        "schema": 1,
        "mode": args.mode,
        "rollout": "shadow",
        "paths": paths,
        "selected": selected,
        "execution": [name for name in selected if name not in covered_by],
        "covered_by": covered_by,
        "reason": reason,
        "policy_sha256": digest(canonical(config)),
        "baseline_checks": [check["id"] for check in config["checks"]],
        "required_external_evidence": config["required_external_evidence"],
    }
    args.output.mkdir(parents=True, exist_ok=False)
    (args.output / "plan.json").write_bytes(canonical(plan) + b"\n")
    if args.action == "plan":
        print(json.dumps(plan, indent=2))
        return 0
    results = []
    for check in config["checks"]:
        if check["id"] in covered_by:
            continue
        if check["id"] not in selected:
            results.append({"id": check["id"], "status": "NOT_APPLICABLE", "reason": reason})
            continue
        print(f"START {check['id']}", flush=True)
        entry = execute(check, ROOT, args.output.resolve(), args.isolated_stack)
        results.append(entry)
        print(f"{entry['status']} {check['id']} {entry['seconds']:.1f}s", flush=True)
        if entry["status"] == "CANCELLED":
            break
    for check_id, provider in covered_by.items():
        evidence = next((entry for entry in results if entry["id"] == provider), None)
        results.append(
            {
                "id": check_id,
                "covered_by": provider,
                "status": evidence["status"] if evidence else "INCOMPLETE",
                "seconds": 0,
                "reason": f"Included in the same run's {provider}; no separate invocation",
                "log": evidence.get("log") if evidence else None,
            }
        )
    # In shadow mode absence of full parity evidence can never produce release PASS.
    report = {**plan, "results": results, "release_status": "INCOMPLETE", "reason": "shadow rollout; external evidence not evaluated"}
    (args.output / "report.json").write_bytes(canonical(report) + b"\n")
    print(json.dumps(report, indent=2))
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
