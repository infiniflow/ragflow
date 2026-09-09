"""Contracts for the always-triggered T3 architecture workflow."""

import importlib.util
import json
import os
import re
import subprocess
from pathlib import Path
from unittest.mock import patch

import yaml

ROOT = Path(os.environ.get("ARCHITECTURE_CANDIDATE_ROOT", Path(__file__).resolve().parents[4])).resolve()
WORKFLOW = ROOT / ".github/workflows/architecture.yml"


def _workflow() -> dict:
    return yaml.safe_load(WORKFLOW.read_text(encoding="utf-8"))


def _step(job: dict, name: str) -> dict:
    return next(step for step in job["steps"] if step.get("name") == name)


def _commands(job: dict) -> str:
    return "\n".join(step.get("run", "") for step in job["steps"])


def test_protected_policy_sources_are_tracked():
    policy = json.loads((ROOT / "tools/quality/architecture-policy.json").read_text(encoding="utf-8"))
    protected_sources = sorted({path for lane in policy["lanes"] for path in lane.get("protected_sources", [])})
    completed = subprocess.run(
        ["git", "ls-files", "-z", "--", *protected_sources],
        cwd=ROOT,
        capture_output=True,
        text=True,
        check=False,
    )
    assert completed.returncode == 0, completed.stderr
    tracked_sources = {path for path in completed.stdout.split("\0") if path}
    assert tracked_sources == set(protected_sources)

    requirements_input = ROOT / "services/asr-online-service/architecture-contract-requirements.in"
    requirements_lock = ROOT / "services/asr-online-service/architecture-contract-requirements.txt"
    direct_requirements = [line.strip() for line in requirements_input.read_text(encoding="utf-8").splitlines() if line.strip() and not line.startswith("#")]
    assert direct_requirements == [
        "fastapi==0.115.14",
        "httpx==0.27.2",
        "pydantic-settings==2.15.0",
        "pytest==8.4.2",
        "pytest-asyncio==1.4.0",
        "python-docx==1.2.0",
        "python-multipart==0.0.9",
    ]

    lock_lines = requirements_lock.read_text(encoding="utf-8").splitlines()
    package_blocks = []
    for line in lock_lines:
        if line and not line.startswith((" ", "#")):
            package_blocks.append([line])
        elif package_blocks:
            package_blocks[-1].append(line)
    assert package_blocks
    assert all(re.search(r"--hash=sha256:[0-9a-f]{64}(?:\s|\\|$)", "\n".join(block)) for block in package_blocks)
    locked_requirements = {block[0].split(" \\", 1)[0] for block in package_blocks}
    assert set(direct_requirements) <= locked_requirements
    locked_package_names = {requirement.split("==", 1)[0].lower() for requirement in locked_requirements}
    assert locked_package_names.isdisjoint({"gigaam", "numpy", "tone", "torch", "transformers", "uvicorn", "watchfiles"})

    helper_path = ROOT / "tools/quality/run_isolated_python.py"
    spec = importlib.util.spec_from_file_location("architecture_isolated_runner", helper_path)
    isolated_runner = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(isolated_runner)
    with patch.dict(
        os.environ,
        {
            "PATH": "preserved",
            "NODE_OPTIONS": "--require=attacker.js",
            "NODE_PATH": "attacker-modules",
            "NODE_EXTRA_CA_CERTS": "attacker.pem",
            "GITHUB_ENV": "command-file",
            "ARCHITECTURE_EVIDENCE_DIR": "evidence",
            "API_TOKEN": "secret",
        },
        clear=True,
    ):
        child_environment = isolated_runner.sanitized_child_environment()
    assert child_environment == {"PATH": "preserved"}
    assert "env=sanitized_child_environment()" in (ROOT / "tools/quality/inspect_typescript.py").read_text(encoding="utf-8")
    assert "env=sanitized_child_environment()" in (ROOT / "tools/quality/check_runtime_graph_policy.py").read_text(encoding="utf-8")

    node_install_inputs = {"web/.npmrc", "web/package.json", "web/pnpm-lock.yaml"}
    for lane_id in ("python-architecture", "runtime-graph"):
        lane = next(item for item in policy["lanes"] if item["id"] == lane_id)
        assert node_install_inputs <= set(lane["protected_sources"])
        assert node_install_inputs <= set(lane["selectors"]["paths"])


def test_workflow_is_unconditional_report_only_and_fail_closed():
    workflow = _workflow()
    assert workflow["permissions"] == {"contents": "read"}
    events = workflow[True]
    assert set(events) == {"pull_request", "push", "workflow_dispatch"}
    assert set(events["pull_request"]["types"]) == {"opened", "synchronize", "reopened", "ready_for_review", "edited"}
    assert "paths" not in events["pull_request"] and "paths-ignore" not in events["pull_request"]
    assert "paths" not in events["push"] and "paths-ignore" not in events["push"]

    jobs = workflow["jobs"]
    assert list(jobs) == ["architecture-policy-plan", "architecture-policy-analysis", "architecture-policy"]
    assert all("permissions" not in job for job in jobs.values())
    assert [job_id for job_id, job in jobs.items() if job["name"] == "architecture-policy"] == ["architecture-policy"]
    assert jobs["architecture-policy"]["if"] == "always()"
    assert set(jobs["architecture-policy"]["needs"]) == {"architecture-policy-plan", "architecture-policy-analysis"}
    assert all("continue-on-error" not in step for job in jobs.values() for step in job["steps"])
    assert all("|| true" not in step.get("run", "") for job in jobs.values() for step in job["steps"])
    assert {job_id: [step.get("name") for step in job["steps"]] for job_id, job in jobs.items()} == {
        "architecture-policy-plan": [
            "Check out candidate for static planning",
            "Set up policy interpreter",
            "Create fresh plan directory",
            "Resolve comparison base",
            "Materialize authoritative policy bundle",
            "Select authoritative architecture lanes",
            "Evaluate candidate policy with authoritative checker",
            "Reject candidate policy coverage reduction",
            "Upload architecture plan",
        ],
        "architecture-policy-analysis": [
            "Check out candidate for analysis",
            "Download architecture plan",
            "Create fresh analysis directory",
            "Reject selected lanes with untrusted sources",
            "Set up pnpm",
            "Set up Node.js",
            "Set up Python and uv",
            "Materialize protected producer sources",
            "Prepare root Python contracts",
            "Prepare ASR architecture contract environment",
            "Exercise architecture policy fixtures",
            "Prepare locked TypeScript parser",
            "Classify TypeScript and Go runtime graphs",
            "Plan changed Go build profiles",
            "Check configured Python architecture",
            "Upload architecture analysis",
        ],
        "architecture-policy": [
            "Create fresh final evidence directory",
            "Require a completed trusted plan",
            "Require completed analysis",
            "Check out candidate for final static verification",
            "Set up final policy interpreter",
            "Download trusted plan",
            "Download analysis reports",
            "Resolve and verify comparison base",
            "Re-materialize authoritative policy bundle",
            "Recompute authoritative selection",
            "Re-evaluate candidate policy",
            "Recheck candidate policy coverage",
            "Stage downloaded reports beside fresh plan",
            "Aggregate architecture evidence",
            "Upload final architecture evidence",
        ],
    }


def test_workflow_uses_base_policy_as_authoritative_when_protocol_is_available():
    job = _workflow()["jobs"]["architecture-policy-plan"]
    assert job["runs-on"] == "ubuntu-latest"
    assert job["timeout-minutes"] == 15
    checkout = next(step for step in job["steps"] if step.get("uses", "").startswith("actions/checkout@"))
    assert checkout["with"]["fetch-depth"] == 0
    assert "astral-sh/setup-uv@v6" in {step.get("uses") for step in job["steps"]}

    source = _step(job, "Materialize authoritative policy bundle")["run"]
    assert "git show" in source
    assert "TRUSTED_BASE_PROTOCOL = 1" in source
    assert 'source="base"' in source
    assert 'source="candidate-bootstrap"' in source
    assert "NOT_ENABLED" in source
    selection = _step(job, "Select authoritative architecture lanes")["run"]
    comparison = _step(job, "Reject candidate policy coverage reduction")["run"]
    assert '"${POLICY_RUNNER}"' in selection and '"${POLICY_FILE}"' in selection
    assert "--github-output" in selection
    assert "--base-plan" in comparison and "--candidate-plan" in comparison

    commands = _commands(job)
    for forbidden in (
        "uv sync",
        "pnpm ",
        "pytest",
        ".venv/bin/python",
        "tools/quality/check_architecture.py",
        "tools/quality/inspect_typescript.py",
        "tools/quality/inspect_go.py",
        "tools/quality/check_runtime_graph_policy.py",
        "tools/quality/check_go_build_profiles.py",
    ):
        assert forbidden not in commands
    upload = _step(job, "Upload architecture plan")
    assert upload["if"] == "always() && steps.evidence.outcome == 'success'"
    assert upload["with"]["name"].startswith("architecture-policy-plan-")
    assert upload["with"]["include-hidden-files"] is True


def test_analysis_job_contains_candidate_lifecycle_and_publishes_reports():
    job = _workflow()["jobs"]["architecture-policy-analysis"]
    assert job["needs"] == "architecture-policy-plan"
    assert job["if"] == "needs.architecture-policy-plan.result == 'success'"
    assert job["runs-on"] == "ubuntu-latest"
    assert job["timeout-minutes"] == 45
    uses = {step.get("uses") for step in job["steps"]}
    assert {
        "actions/checkout@v6",
        "actions/download-artifact@v4",
        "pnpm/action-setup@v4",
        "actions/setup-node@v4",
        "astral-sh/setup-uv@v6",
        "actions/upload-artifact@v4",
    }.issubset(uses)

    commands = _commands(job)
    source_gate = _step(job, "Reject selected lanes with untrusted sources")
    assert set(source_gate["env"]) == {
        "PYTHON_SELECTED",
        "PYTHON_RUNNABLE",
        "RUNTIME_SELECTED",
        "RUNTIME_RUNNABLE",
        "GO_SELECTED",
        "GO_RUNNABLE",
        "FIXTURES_SELECTED",
        "FIXTURES_RUNNABLE",
    }
    assert 'if [[ "${selected}" == "true" && "${runnable}" != "true" ]]' in source_gate["run"]
    assert "Selected architecture lane has no trusted source bundle" in source_gate["run"]
    assert "uv sync --python 3.13 --group test --frozen --no-install-project" in commands
    assert "uv venv --clear services/asr-online-service/.architecture-venv --python 3.13" in commands
    assert ("uv pip sync --python services/asr-online-service/.architecture-venv/bin/python --require-hashes --strict services/asr-online-service/architecture-contract-requirements.txt") in commands
    assert "uv sync --project services/asr-online-service" not in commands
    assert "pnpm --dir web install" not in commands
    parser_step = _step(job, "Prepare locked TypeScript parser")
    assert 'trusted_web="${EVIDENCE_DIR}/protected-sources/web"' in parser_step["run"]
    assert 'install_web="${RUNNER_TEMP}/architecture-typescript-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}"' in parser_step["run"]
    assert 'candidate_modules="${GITHUB_WORKSPACE}/web/node_modules"' in parser_step["run"]
    assert "--frozen-lockfile --ignore-scripts --ignore-pnpmfile --ignore-workspace" in parser_step["run"]
    assert 'cp "${trusted_web}/package.json" "${trusted_web}/pnpm-lock.yaml" "${trusted_web}/.npmrc" "${install_web}/"' in parser_step["run"]
    assert "Refusing stale TypeScript install or pre-existing candidate node_modules" in parser_step["run"]
    assert 'cp -RL "${install_web}/node_modules/typescript" "${candidate_modules}/typescript"' in parser_step["run"]
    assert 'test -f "${candidate_modules}/typescript/lib/typescript.js"' in parser_step["run"]
    assert "tools/quality/check_architecture.py" in commands
    assert "tools/quality/check_runtime_graph_policy.py" in commands
    assert "tools/quality/check_go_build_profiles.py" in commands
    materialize = _step(job, "Materialize protected producer sources")["run"]
    assert '"${POLICY_RUNNER}"' in materialize and " materialize " in materialize.replace("\n", " ")
    assert '--output-directory "${EVIDENCE_DIR}/protected-sources"' in materialize
    assert commands.count('python -I -B "${isolated_runner}"') == 5
    assert commands.count('"${trusted_root}" tools/quality/') == 5
    assert commands.count('--root "${GITHUB_WORKSPACE}"') >= 6
    assert " fixtures " in commands.replace("\n", " ")
    assert "--junit-output" in commands
    assert commands.count("--selection-sha256") == 3
    step_names = [step.get("name") for step in job["steps"]]
    fixture_index = step_names.index("Exercise architecture policy fixtures")
    for name in ("Prepare locked TypeScript parser", "Classify TypeScript and Go runtime graphs", "Plan changed Go build profiles", "Check configured Python architecture"):
        assert fixture_index < step_names.index(name)
    assert _step(job, "Exercise architecture policy fixtures")["if"] == "${{ success() && needs.architecture-policy-plan.outputs.policy_fixtures == 'true' }}"
    assert _step(job, "Classify TypeScript and Go runtime graphs")["if"] == "${{ success() && needs.architecture-policy-plan.outputs.runtime_graph == 'true' }}"
    assert _step(job, "Plan changed Go build profiles")["if"] == "${{ success() && needs.architecture-policy-plan.outputs.go_build_plan == 'true' }}"
    assert _step(job, "Check configured Python architecture")["if"] == "${{ success() && needs.architecture-policy-plan.outputs.python_architecture == 'true' }}"
    for name in ("Set up pnpm", "Set up Node.js", "Prepare locked TypeScript parser"):
        assert "policy_fixtures" not in _step(job, name)["if"]
    upload = _step(job, "Upload architecture analysis")
    assert upload["if"] == "always() && steps.evidence.outcome == 'success'"
    assert upload["with"]["name"].startswith("architecture-policy-analysis-")
    assert upload["with"]["include-hidden-files"] is True


def test_final_aggregate_runs_on_fresh_trusted_job():
    job = _workflow()["jobs"]["architecture-policy"]
    assert job["runs-on"] == "ubuntu-latest"
    assert job["timeout-minutes"] == 15
    assert _step(job, "Create fresh final evidence directory")
    analysis_gate = _step(job, "Require completed analysis")
    assert analysis_gate["env"]["ANALYSIS_RESULT"] == "${{ needs.architecture-policy-analysis.result }}"
    assert 'if [[ "${ANALYSIS_RESULT}" != "success" ]]' in analysis_gate["run"]
    checkout = next(step for step in job["steps"] if step.get("uses", "").startswith("actions/checkout@"))
    assert checkout["with"]["fetch-depth"] == 0

    source = _step(job, "Re-materialize authoritative policy bundle")["run"]
    assert "git show" in source
    assert "TRUSTED_BASE_PROTOCOL = 1" in source
    assert 'source="candidate-bootstrap"' in source
    assert "NOT_ENABLED" in source
    assert "PLANNED_SOURCE" in source
    recompute = _step(job, "Recompute authoritative selection")["run"]
    assert '"${POLICY_RUNNER}"' in recompute and '"${POLICY_FILE}"' in recompute
    assert "cmp" in recompute and "architecture-policy-plan-final-input/selection.json" in recompute
    comparison = _step(job, "Recheck candidate policy coverage")["run"]
    assert "--base-plan" in comparison and "--candidate-plan" in comparison
    assert "cmp" in comparison and "architecture-policy-plan-final-input/policy-compatibility.json" in comparison
    aggregate = _step(job, "Aggregate architecture evidence")["run"]
    assert '"${POLICY_RUNNER}"' in aggregate and '"${POLICY_FILE}"' in aggregate
    assert " aggregate " in aggregate.replace("\n", " ")
    assert '--candidate-plan "${EVIDENCE_DIR}/candidate-policy-selection.json"' in aggregate
    assert '--compatibility "${EVIDENCE_DIR}/policy-compatibility.json"' in aggregate

    commands = _commands(job)
    for forbidden in (
        "uv sync",
        "pnpm ",
        "pytest",
        ".venv/bin/python",
        "tools/quality/check_architecture.py",
        "tools/quality/inspect_typescript.py",
        "tools/quality/inspect_go.py",
        "tools/quality/check_runtime_graph_policy.py",
        "tools/quality/check_go_build_profiles.py",
    ):
        assert forbidden not in commands


def test_artifacts_flow_from_plan_and_analysis_to_fresh_final_aggregate():
    jobs = _workflow()["jobs"]
    plan = jobs["architecture-policy-plan"]
    analysis = jobs["architecture-policy-analysis"]
    final = jobs["architecture-policy"]
    plan_name = _step(plan, "Upload architecture plan")["with"]["name"]
    analysis_plan_name = _step(analysis, "Download architecture plan")["with"]["name"]
    final_plan_name = _step(final, "Download trusted plan")["with"]["name"]
    analysis_name = _step(analysis, "Upload architecture analysis")["with"]["name"]
    final_analysis_name = _step(final, "Download analysis reports")["with"]["name"]
    assert plan_name == analysis_plan_name == final_plan_name
    assert analysis_name == final_analysis_name

    staging = _step(final, "Stage downloaded reports beside fresh plan")["run"]
    for report in ("architecture-python.json", "runtime-graph.json", "go-build-plan.json", "policy-fixtures.json"):
        assert report in staging
    assert "policy-compatibility.json" in _step(final, "Recheck candidate policy coverage")["run"]
    assert "--candidate-plan" in _step(final, "Aggregate architecture evidence")["run"]
    assert "--compatibility" in _step(final, "Aggregate architecture evidence")["run"]
    upload = _step(final, "Upload final architecture evidence")
    assert upload["if"] == "always() && steps.evidence.outcome == 'success'"
    assert upload["with"]["name"].startswith("architecture-policy-${{ github.run_id }}-")
    assert upload["with"]["include-hidden-files"] is True
