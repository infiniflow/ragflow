"""Plan or execute T2 Go package profiles through the repository build driver."""

from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path, PurePosixPath
import platform
import re
import shutil
import subprocess
import sys

import yaml

from capture_inventory import capture, git, safe_path


VERSION = "0.2.0"
REPORT_ONLY = "T2_REPORT_ONLY"


def _file(value: object, label: str) -> str:
    if not isinstance(value, str) or not value or "\\" in value:
        raise ValueError(f"{label} must be a repository-relative POSIX file")
    path = PurePosixPath(value)
    if path.is_absolute() or ".." in path.parts or path.as_posix().startswith("./"):
        raise ValueError(f"Unsafe {label}: {value}")
    return path.as_posix()


def _prefix(value: object, label: str) -> str:
    if not isinstance(value, str) or not value or "\\" in value:
        raise ValueError(f"{label} must be a repository-relative POSIX directory")
    path = PurePosixPath(value)
    if path.is_absolute() or ".." in path.parts or path.as_posix().startswith("./"):
        raise ValueError(f"Unsafe {label}: {value}")
    return path.as_posix().rstrip("/") + "/"


def _package(value: object, label: str) -> str:
    if not isinstance(value, str) or not value.startswith("./") or "\\" in value:
        raise ValueError(f"{label} must start with ./ and use POSIX separators")
    body = value[2:]
    suffix = "/..." if body.endswith("/...") else ""
    path = PurePosixPath(body[: -len(suffix)] if suffix else body)
    if not path.parts or path.is_absolute() or ".." in path.parts:
        raise ValueError(f"Unsafe {label}: {value}")
    normalized = "./" + path.as_posix()
    return normalized + suffix


def normalize_policy(raw: object) -> dict:
    if not isinstance(raw, dict) or raw.get("schema_version") != 1:
        raise ValueError("Unsupported Go build profile schema")
    profiles = raw.get("profiles")
    if not isinstance(profiles, list) or not profiles:
        raise ValueError("Go build policy must contain profiles")
    profile_ids: set[str] = set()
    all_prefixes: list[tuple[str, str]] = []
    normalized_profiles = []
    for source in profiles:
        if not isinstance(source, dict):
            raise ValueError("Go build profile must be an object")
        profile_id = source.get("id")
        if not isinstance(profile_id, str) or not profile_id or profile_id in profile_ids:
            raise ValueError(f"Invalid or duplicate Go build profile ID: {profile_id}")
        profile_ids.add(profile_id)
        if source.get("rule_id") != "BUILD-01" or source.get("platform") != "linux":
            raise ValueError(f"Profile {profile_id} must use BUILD-01 on linux")
        driver = _file(source.get("driver"), f"Profile {profile_id} driver")
        if driver != "build.sh":
            raise ValueError(f"Profile {profile_id} must use the repository build.sh")
        tools = source.get("required_tools")
        if not isinstance(tools, list) or any(not isinstance(item, str) or not item for item in tools):
            raise ValueError(f"Profile {profile_id} has invalid required_tools")
        if not {"bash", "go"}.issubset(tools):
            raise ValueError(f"Profile {profile_id} must require bash and go")
        compilers = source.get("c_compilers")
        if not isinstance(compilers, list) or not {"clang", "gcc"}.issubset(compilers):
            raise ValueError(f"Profile {profile_id} must accept the build.sh C compilers clang and gcc")
        native = source.get("native_dependencies")
        if not isinstance(native, list) or not {"office_oxide", "pdfium-static", "pdf_oxide"}.issubset(native):
            raise ValueError(f"Profile {profile_id} omits required native dependencies")
        timeout = source.get("timeout_seconds")
        if type(timeout) is not int or timeout < 1 or timeout > 1800:
            raise ValueError(f"Profile {profile_id} has invalid timeout_seconds")
        test_args = source.get("test_args")
        if not isinstance(test_args, list) or any(not isinstance(item, str) or not item for item in test_args) or "-json" not in test_args:
            raise ValueError(f"Profile {profile_id} must request structured go test JSON")
        if any(item in {"./...", "--", "--test", "-t"} or item.startswith("./") for item in test_args):
            raise ValueError(f"Profile {profile_id} test_args may not contain packages or driver options")
        narrowing = ("-run", "-skip", "-list", "-bench", "-short", "-failfast", "-count=0")
        if any(item == option or item.startswith(option + "=") for item in test_args for option in narrowing):
            raise ValueError(f"Profile {profile_id} test_args may not narrow or skip the declared package tests")
        targets = source.get("targets")
        if not isinstance(targets, list) or not targets:
            raise ValueError(f"Profile {profile_id} must contain targets")
        target_ids: set[str] = set()
        normalized_targets = []
        for target in targets:
            if not isinstance(target, dict):
                raise ValueError(f"Profile {profile_id} target must be an object")
            target_id = target.get("id")
            if not isinstance(target_id, str) or not target_id or target_id in target_ids:
                raise ValueError(f"Invalid or duplicate target ID in {profile_id}: {target_id}")
            target_ids.add(target_id)
            owners = target.get("owners")
            if not isinstance(owners, list) or not owners or any(not isinstance(item, str) or not item for item in owners):
                raise ValueError(f"Target {target_id} has invalid owners")
            prefixes = target.get("path_prefixes")
            packages = target.get("packages")
            build_args = target.get("build_args")
            if not isinstance(prefixes, list) or not prefixes:
                raise ValueError(f"Target {target_id} must contain path_prefixes")
            if (packages is None) == (build_args is None):
                raise ValueError(f"Target {target_id} must contain exactly one of packages or build_args")
            normalized_prefixes = [_prefix(item, f"Target {target_id} prefix") for item in prefixes]
            normalized_packages = []
            normalized_build_args = []
            normalized_artifacts = []
            if packages is not None:
                if not isinstance(packages, list) or not packages:
                    raise ValueError(f"Target {target_id} has invalid packages")
                normalized_packages = [_package(item, f"Target {target_id} package") for item in packages]
                if target.get("artifacts") is not None:
                    raise ValueError(f"Test target {target_id} may not declare artifacts")
            else:
                if build_args != ["--go"]:
                    raise ValueError(f"Build target {target_id} must use the repository --go driver action")
                artifacts = target.get("artifacts")
                if not isinstance(artifacts, list) or not artifacts:
                    raise ValueError(f"Build target {target_id} must declare artifacts")
                normalized_build_args = list(build_args)
                normalized_artifacts = [_file(item, f"Target {target_id} artifact") for item in artifacts]
            for prefix in normalized_prefixes:
                for previous, previous_id in all_prefixes:
                    if prefix.startswith(previous) or previous.startswith(prefix):
                        raise ValueError(f"Overlapping Go prefixes: {previous_id} and {target_id}")
                all_prefixes.append((prefix, target_id))
            normalized_target = {**target, "owners": list(dict.fromkeys(owners)), "path_prefixes": normalized_prefixes}
            if normalized_packages:
                normalized_target["packages"] = list(dict.fromkeys(normalized_packages))
            else:
                normalized_target["build_args"] = normalized_build_args
                normalized_target["artifacts"] = list(dict.fromkeys(normalized_artifacts))
            normalized_targets.append(normalized_target)
        normalized_profiles.append(
            {
                **source,
                "driver": driver,
                "required_tools": list(dict.fromkeys(tools)),
                "c_compilers": list(dict.fromkeys(compilers)),
                "native_dependencies": list(dict.fromkeys(native)),
                "targets": normalized_targets,
            }
        )
    return {**raw, "profiles": normalized_profiles}


def load_policy(path: Path) -> dict:
    return normalize_policy(yaml.safe_load(path.read_text(encoding="utf-8")))


def _package_directory(package: str) -> str:
    value = package[2:]
    return value[:-4] if value.endswith("/...") else value


def _required_go_tests(root: Path, records: list[dict]) -> list[dict]:
    required = []
    pattern = re.compile(r"^func\s+(Test[A-Za-z0-9_]+)\s*\(\s*[A-Za-z_][A-Za-z0-9_]*\s+\*testing\.T\s*\)", re.MULTILINE)
    for record in records:
        path = record["path"]
        if not path.endswith("_test.go") or not record.get("working_file", {}).get("present"):
            continue
        source = safe_path(root, path).read_text(encoding="utf-8")
        package = "./" + PurePosixPath(path).parent.as_posix()
        required.extend({"path": path, "package": package, "test": name} for name in pattern.findall(source))
    return required


def evaluate_plan(root: Path, policy: dict, snapshot: dict, selected_profiles: set[str] | None = None) -> dict:
    profiles = [profile for profile in policy["profiles"] if selected_profiles is None or profile["id"] in selected_profiles]
    known = {profile["id"] for profile in policy["profiles"]}
    if selected_profiles and selected_profiles - known:
        raise ValueError("Unknown Go build profile: " + ", ".join(sorted(selected_profiles - known)))
    records = [record for record in snapshot["records"] if record["path"].endswith(".go")]
    findings = []
    incomplete_reasons = []
    mapped = []
    selected_targets = []
    packages = []
    for record in records:
        matches = []
        for profile in profiles:
            for target in profile["targets"]:
                if any(record["path"].startswith(prefix) for prefix in target["path_prefixes"]):
                    matches.append((profile, target))
        if not matches:
            findings.append({"rule_id": "BUILD-01", "path": record["path"], "owner": record["module"], "reason": "changed Go file has no build target"})
            continue
        profile, target = matches[0]
        if record["module"] not in target["owners"]:
            findings.append(
                {
                    "rule_id": "BUILD-01",
                    "path": record["path"],
                    "owner": record["module"],
                    "target": target["id"],
                    "reason": "build target does not declare the provenance owner",
                }
            )
            continue
        mapped.append({"path": record["path"], "origin": record["origin"], "owner": record["module"], "profile": profile["id"], "target": target["id"]})
        key = (profile["id"], target["id"])
        if key not in {(item["profile"], item["target"]) for item in selected_targets}:
            selected = {"profile": profile["id"], "target": target["id"]}
            if "packages" in target:
                selected.update(kind="test", packages=target["packages"])
                packages.extend(package for package in target["packages"] if package not in packages)
            else:
                selected.update(kind="build", build_args=target["build_args"], artifacts=target["artifacts"])
            selected_targets.append(selected)
    for item in selected_targets:
        for package in item.get("packages", []):
            directory = _package_directory(package)
            if not safe_path(root, directory).is_dir():
                incomplete_reasons.append(f"package directory is missing: {directory}")
    drivers = {profile["driver"] for profile in profiles}
    if records and len(drivers) != 1:
        incomplete_reasons.append("selected profiles do not resolve to one build driver")
    driver = next(iter(drivers), "build.sh")
    if records and not safe_path(root, driver).is_file():
        incomplete_reasons.append(f"build driver is missing: {driver}")
    test_args = []
    for profile in profiles:
        if any(item["profile"] == profile["id"] for item in selected_targets):
            if test_args and test_args != profile["test_args"]:
                incomplete_reasons.append("selected profiles use different go test arguments")
            test_args = profile["test_args"]
    commands = []
    for item in selected_targets:
        if item["kind"] == "build":
            commands.append(
                {
                    "kind": "build",
                    "target": item["target"],
                    "argv": ["bash", driver, *item["build_args"]],
                    "artifacts": item["artifacts"],
                }
            )
    if packages:
        commands.append(
            {"kind": "test", "targets": [item["target"] for item in selected_targets if item["kind"] == "test"], "argv": ["bash", driver, "--test", *test_args, *packages], "packages": packages}
        )
    required_tests = _required_go_tests(root, records)
    status = "INCOMPLETE" if incomplete_reasons else "FAIL" if findings else "NOT_APPLICABLE" if not records else "READY"
    return {
        "analysis_status": status,
        "policy_status": "NOT_EVALUATED" if status in {"READY", "NOT_APPLICABLE"} else status,
        "scope": "All changed .go paths in the current T0 provenance inventory",
        "go_files": mapped,
        "selected_targets": selected_targets,
        "required_tests": required_tests,
        "commands": commands,
        "findings": findings,
        "incomplete_reasons": incomplete_reasons,
    }


def parse_go_test_json(
    output: str,
    returncode: int,
    requested_packages: list[str] | None = None,
    module_path: str | None = None,
    required_tests: list[dict] | None = None,
) -> tuple[str, str, dict]:
    events = []
    for line in output.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        if isinstance(event, dict) and isinstance(event.get("Action"), str):
            events.append(event)
    failed = sorted({event.get("Package") for event in events if event["Action"] == "fail" and event.get("Package")})
    passed = sorted({event.get("Package") for event in events if event["Action"] == "pass" and event.get("Package")})
    tests = sorted({(event.get("Package"), event.get("Test")) for event in events if event["Action"] == "pass" and event.get("Test")})
    skipped = sorted({(event.get("Package"), event.get("Test")) for event in events if event["Action"] == "skip" and event.get("Test")})
    missing = []
    if requested_packages and module_path:
        for requested in requested_packages:
            relative = requested[2:]
            recursive = relative.endswith("/...")
            relative = relative[:-4] if recursive else relative
            expected = f"{module_path}/{relative}"
            if not any(package == expected or (recursive and package.startswith(expected + "/")) for package in passed):
                missing.append(requested)
    required_missing = []
    if required_tests and module_path:
        passed_tests = set(tests)
        for required in required_tests:
            package = f"{module_path}/{required['package'][2:]}"
            if (package, required["test"]) not in passed_tests:
                required_missing.append({"package": required["package"], "test": required["test"], "path": required["path"]})
    evidence = {
        "events": len(events),
        "packages_passed": passed,
        "packages_failed": failed,
        "requested_packages_missing": missing,
        "required_tests_missing": required_missing,
        "tests_passed": len(tests),
        "tests_skipped": len(skipped),
    }
    if failed:
        return "FAIL", "go test reported failed packages", evidence
    if returncode:
        return "INCOMPLETE", "build driver failed without structured test failure; inspect native/tool prerequisites", evidence
    if missing:
        return "INCOMPLETE", "structured PASS evidence is missing for requested package targets", evidence
    if required_missing:
        return "INCOMPLETE", "structured PASS evidence is missing for tests declared in changed Go test files", evidence
    if not events or not passed:
        return "INCOMPLETE", "build driver returned success without structured package PASS evidence", evidence
    reason = "all requested Go packages and changed-file tests passed through build.sh"
    if skipped:
        reason += f"; {len(skipped)} unrelated tests skipped"
    return "PASS", reason, evidence


def prerequisite_reasons(profile: dict, current_platform: str | None = None) -> list[str]:
    reasons = []
    current = current_platform or sys.platform
    if current != profile["platform"]:
        reasons.append(f"platform:{profile['platform']}")
    reasons.extend(f"tool:{tool}" for tool in profile["required_tools"] if shutil.which(tool) is None)
    if not any(shutil.which(compiler) is not None for compiler in profile["c_compilers"]):
        reasons.append("tool:any-of:" + "|".join(profile["c_compilers"]))
    return reasons


def _sha(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _output_evidence(output: str, include_tail: bool = False) -> dict:
    encoded = output.encode("utf-8")
    evidence = {"output_sha256": hashlib.sha256(encoded).hexdigest(), "output_bytes": len(encoded)}
    if include_tail:
        evidence["output_tail"] = output[-12000:]
    return evidence


def _build_artifacts(root: Path, artifacts: list[str]) -> tuple[list[dict], list[str]]:
    evidence = []
    missing = []
    for relative in artifacts:
        path = safe_path(root, relative)
        if not path.is_file():
            missing.append(relative)
            continue
        evidence.append({"path": relative, "sha256": _sha(path), "bytes": path.stat().st_size})
    return evidence, missing


def _go_module(root: Path) -> str:
    for line in (root / "go.mod").read_text(encoding="utf-8").splitlines():
        fields = line.split()
        if len(fields) == 2 and fields[0] == "module":
            return fields[1]
    raise ValueError("go.mod has no module directive")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parents[2])
    parser.add_argument("--policy", type=Path, help="Defaults to tools/quality/go-build-profiles.yaml")
    parser.add_argument("--profile", action="append", dest="profiles", help="Exact profile ID; repeat to select more")
    parser.add_argument("--execute", action="store_true", help="Run the selected packages through build.sh --test")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args(argv)
    root = args.root.resolve()
    output = args.output.resolve()
    policy_path = args.policy.resolve() if args.policy else root / "tools/quality/go-build-profiles.yaml"
    tool_path = Path(__file__).resolve()
    try:
        if output.is_relative_to(root):
            git(root, "check-ignore", "--no-index", "-q", output.relative_to(root).as_posix())
        base = json.loads((root / "tools/quality/upstream-base.json").read_text(encoding="utf-8"))
        module_map = yaml.safe_load((root / "tools/quality/module-map.yaml").read_text(encoding="utf-8"))
        policy = load_policy(policy_path)
        snapshot = capture(root, base, module_map)
        plan = evaluate_plan(root, policy, snapshot, set(args.profiles) if args.profiles else None)
        report = {
            "schema_version": 1,
            "tool": {"name": "check_go_build_profiles", "version": VERSION},
            "mode": REPORT_ONLY,
            "input": {
                "candidate_sha": snapshot["head"],
                "upstream_sha": snapshot["upstream_base"],
                "dirty_snapshot_sha256": snapshot["snapshot_sha256"],
                "policy_sha256": _sha(policy_path),
                "tool_sha256": _sha(tool_path),
                "build_driver_sha256": _sha(root / "build.sh") if (root / "build.sh").is_file() else None,
                "python": platform.python_version(),
                "platform": sys.platform,
            },
            **plan,
        }
        exit_code = 2 if plan["analysis_status"] == "INCOMPLETE" else 1 if plan["analysis_status"] == "FAIL" else 0
        if args.execute and plan["analysis_status"] == "READY":
            selected_profile_ids = {item["profile"] for item in plan["selected_targets"]}
            selected_profiles = [profile for profile in policy["profiles"] if profile["id"] in selected_profile_ids]
            missing = sorted({reason for profile in selected_profiles for reason in prerequisite_reasons(profile)})
            if missing:
                report.update(analysis_status="INCOMPLETE", policy_status="INCOMPLETE", incomplete_reasons=[*report["incomplete_reasons"], *missing])
                exit_code = 2
            else:
                timeout = max(profile["timeout_seconds"] for profile in selected_profiles)
                executions = []
                statuses = []
                for command in plan["commands"]:
                    try:
                        completed = subprocess.run(command["argv"], cwd=root, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=timeout, check=False)
                    except subprocess.TimeoutExpired:
                        executions.append({"kind": command["kind"], "status": "INCOMPLETE", "reason": "build profile timed out", "argv": command["argv"]})
                        statuses.append("INCOMPLETE")
                        break
                    if command["kind"] == "build":
                        artifacts, missing_artifacts = _build_artifacts(root, command["artifacts"])
                        status = "PASS" if completed.returncode == 0 and not missing_artifacts else "INCOMPLETE"
                        reason = "build.sh produced every declared entrypoint artifact" if status == "PASS" else "build driver failed or declared entrypoint artifacts are missing"
                        execution = {
                            "kind": "build",
                            "target": command["target"],
                            "argv": command["argv"],
                            "status": status,
                            "returncode": completed.returncode,
                            "reason": reason,
                            "artifacts": artifacts,
                            "missing_artifacts": missing_artifacts,
                            **_output_evidence(completed.stdout, status != "PASS"),
                        }
                    else:
                        status, reason, evidence = parse_go_test_json(
                            completed.stdout,
                            completed.returncode,
                            command["packages"],
                            _go_module(root),
                            plan["required_tests"],
                        )
                        execution = {
                            "kind": "test",
                            "targets": command["targets"],
                            "argv": command["argv"],
                            "status": status,
                            "returncode": completed.returncode,
                            "reason": reason,
                            **evidence,
                            **_output_evidence(completed.stdout, status != "PASS"),
                        }
                    executions.append(execution)
                    statuses.append(status)
                    if status != "PASS":
                        break
                status = "FAIL" if "FAIL" in statuses else "INCOMPLETE" if "INCOMPLETE" in statuses else "PASS"
                report.update(analysis_status=status, policy_status=status, execution={"commands": executions})
                exit_code = 0 if status == "PASS" else 1 if status == "FAIL" else 2
        after = capture(root, base, module_map)
        if after["head"] != snapshot["head"] or after["snapshot_sha256"] != snapshot["snapshot_sha256"]:
            raise ValueError("Provenance snapshot changed during Go profile analysis")
        report["exit_code"] = exit_code
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(json.dumps(report, ensure_ascii=True, indent=2) + "\n", encoding="utf-8")
        print(
            json.dumps(
                {
                    "analysis_status": report["analysis_status"],
                    "policy_status": report["policy_status"],
                    "go_files": len(report["go_files"]),
                    "targets": len(report["selected_targets"]),
                    "output": str(output),
                }
            )
        )
        return exit_code
    except (OSError, subprocess.CalledProcessError, ValueError, yaml.YAMLError) as exc:
        print(f"INCOMPLETE: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
