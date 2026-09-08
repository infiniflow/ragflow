"""Contracts for the always-triggered T3 architecture workflow."""

from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parents[4]
WORKFLOW = ROOT / ".github/workflows/architecture.yml"


def test_workflow_is_unconditional_report_only_and_fail_closed():
    raw = WORKFLOW.read_text(encoding="utf-8")
    workflow = yaml.safe_load(raw)
    events = workflow[True]
    assert set(events) == {"pull_request", "push", "workflow_dispatch"}
    assert "paths" not in events["pull_request"] and "paths-ignore" not in events["pull_request"]
    assert "paths" not in events["push"] and "paths-ignore" not in events["push"]

    jobs = workflow["jobs"]
    assert list(jobs) == ["architecture-policy"]
    job = jobs["architecture-policy"]
    assert job["name"] == "architecture-policy"
    assert "if" not in job
    assert job["runs-on"] == ["self-hosted", "ragflow-test"]

    steps = job["steps"]
    assert all("continue-on-error" not in step for step in steps)
    checkout = next(step for step in steps if step.get("uses", "").startswith("actions/checkout@"))
    assert checkout["with"]["fetch-depth"] == 0
    aggregate = next(step for step in steps if step.get("name") == "Aggregate architecture evidence")
    assert aggregate["if"] == "always() && steps.selection.outcome == 'success'"
    assert "check_architecture_policy.py" in aggregate["run"]
    assert " aggregate " in aggregate["run"].replace("\n", " ")

    commands = "\n".join(step.get("run", "") for step in steps)
    assert "contains(github.event.pull_request.labels" not in raw
    assert "--write" not in commands
    assert "git add" not in commands
    assert "check_architecture.py" in commands
    assert "check_runtime_graph_policy.py" in commands
    assert "check_go_build_profiles.py" in commands
    assert "--junitxml" in commands
    assert "actions/upload-artifact@v4" in {step.get("uses") for step in steps}
