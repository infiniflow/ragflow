import json
from pathlib import Path
import tomllib

import yaml


ROOT = Path(__file__).resolve().parents[4]

ASSIGNMENT_PATHS = {
    "api/apps/business_documents/adapters/__init__.py",
    "api/apps/business_documents/adapters/assignment.py",
    "business_documents/application/__init__.py",
    "business_documents/application/assign_document.py",
    "business_documents/domain/access.py",
    "business_documents/domain/assignment.py",
    "business_documents/domain/workflow.py",
    "test/unit_test/business_documents/application/test_assign_document.py",
    "test/unit_test/tools/quality/test_business_document_assignment_metadata.py",
}

COVERAGE_TESTS = [
    "test/unit_test/business_documents",
    "test/unit_test/api/apps/business_documents",
    "test/unit_test/api/apps/restful_apis/test_business_document_api_contract.py",
]


def _python_policy() -> dict:
    return yaml.safe_load((ROOT / "tools/quality/python-boundaries.yaml").read_text(encoding="utf-8"))


def test_assignment_packages_data_and_coverage_roots_are_explicit():
    project = tomllib.loads((ROOT / "pyproject.toml").read_text(encoding="utf-8"))
    packages = project["tool"]["setuptools"]["packages"]

    assert {package for package in packages if package == "business_documents" or package.startswith("business_documents.")} == {
        "business_documents",
        "business_documents.domain",
        "business_documents.application",
    }
    assert project["tool"]["setuptools"]["package-data"]["business_documents"] == ["domain/*.json"]
    assert "business_documents/" in project["tool"]["coverage"]["run"]["source"]


def test_assignment_paths_and_import_contracts_are_exact():
    module_map = yaml.safe_load((ROOT / "tools/quality/module-map.yaml").read_text(encoding="utf-8"))
    owners = {path: [module["id"] for module in module_map["modules"] if path in module["paths"]] for path in ASSIGNMENT_PATHS}
    assert owners == {path: ["business-documents"] for path in ASSIGNMENT_PATHS}

    policy = _python_policy()
    connection = next(item for item in policy["connections"] if item["id"] == "business-documents-package-wiring")
    assignment_sources = {
        "api.apps.business_documents.adapters.assignment",
        "api.apps.business_documents.authorization",
        "api.apps.business_documents.exports",
        "api.apps.business_documents.service",
    }
    assignment_targets = {
        "business_documents.application.assign_document",
        "business_documents.domain.access",
        "business_documents.domain.assignment",
        "business_documents.domain.workflow",
    }
    observed_edges = {
        (edge["source"], edge["target"], tuple(edge["symbols"])) for edge in connection["allowed_reverse_imports"] if edge["source"] in assignment_sources and edge["target"] in assignment_targets
    }
    assert observed_edges == {
        (
            "api.apps.business_documents.adapters.assignment",
            "business_documents.application.assign_document",
            ("AssignDocument", "AssignDocumentCommand", "AssignDocumentError", "AssignmentUnitOfWork", "DocumentAssignedEvent"),
        ),
        (
            "api.apps.business_documents.adapters.assignment",
            "business_documents.domain.access",
            ("BusinessDocumentRole",),
        ),
        (
            "api.apps.business_documents.adapters.assignment",
            "business_documents.domain.assignment",
            ("DocumentOwnership",),
        ),
        (
            "api.apps.business_documents.adapters.assignment",
            "business_documents.domain.workflow",
            ("ACTIVE_JOB_STATUSES",),
        ),
        (
            "api.apps.business_documents.authorization",
            "business_documents.domain.access",
            ("BusinessDocumentRole", "can_assign_document", "normalize_role"),
        ),
        (
            "api.apps.business_documents.service",
            "business_documents.domain.access",
            ("BusinessDocumentRole", "normalize_role"),
        ),
        (
            "api.apps.business_documents.exports",
            "business_documents.domain.workflow",
            ("OperationState",),
        ),
        (
            "api.apps.business_documents.service",
            "business_documents.domain.workflow",
            ("ACTIVE_JOB_STATUSES", "OperationState", "is_operation_quiescent"),
        ),
    }

    import_probe = next(item for item in policy["runtime_probes"] if item["id"] == "business-documents-domain-import")
    targets = {target["module"]: target["required_symbols"] for target in import_probe["targets"]}
    assert targets["business_documents.domain.access"] == ["BusinessDocumentRole", "normalize_role", "can_assign_document"]
    assert targets["business_documents.domain.assignment"] == [
        "DocumentOwnership",
        "AssignmentDecision",
        "AssignmentOperationInProgress",
        "StateVersionMismatch",
        "decide_assignment",
    ]
    assert targets["business_documents.domain.workflow"] == [
        "OperationState",
        "ACTIVE_JOB_STATUSES",
        "is_operation_quiescent",
    ]
    assert targets["business_documents.application.assign_document"] == [
        "AssignDocumentCommand",
        "AssignDocumentResult",
        "DocumentAssignedEvent",
        "AssignDocumentError",
        "AssignmentUnitOfWork",
        "AssignDocument",
    ]

    http_probe = next(item for item in policy["runtime_probes"] if item["id"] == "business-documents-http-registration")
    adapter_stub = next(item for item in http_probe["stub_modules"] if item["module"] == "api.apps.business_documents.adapters.assignment")
    assert adapter_stub["symbols"] == {"assign_business_document": "function"}


def test_document_coverage_lane_includes_assignment_tests_and_both_source_roots():
    checks = json.loads((ROOT / "tools/quality/checks.yaml").read_text(encoding="utf-8"))["checks"]
    coverage = next(check for check in checks if check["id"] == "document-coverage")["command"]
    assert [argument for argument in coverage if argument.startswith("test/")] == COVERAGE_TESTS
    assert [argument for argument in coverage if argument.startswith("--cov=")] == [
        "--cov=api/apps/business_documents",
        "--cov=business_documents",
    ]

    fragment = (
        "test/unit_test/business_documents test/unit_test/api/apps/business_documents "
        "test/unit_test/api/apps/restful_apis/test_business_document_api_contract.py "
        "--cov=api/apps/business_documents --cov=business_documents --cov-branch --cov-fail-under=79"
    )
    for path in (
        ".github/workflows/tests.yml",
        ".github/workflows/sep-tests.yml",
        "docs/develop/architecture-checks-ru.md",
        "test/REGRESSION.md",
    ):
        assert fragment in (ROOT / path).read_text(encoding="utf-8")

    import run_tests

    runner = run_tests.TestRunner()
    runner.coverage = True
    command = runner.build_pytest_command()
    coverage_sources = [Path(command[index + 1]).resolve() for index, argument in enumerate(command) if argument == "--cov"]
    assert coverage_sources == [(ROOT / source).resolve() for source in ("api", "rag", "common", "deepdoc", "agent", "business_documents")]
