"""Static HTTP-contract regression for the governed business-document API.

The production API modules are loaded dynamically by ``api.apps`` and importing
that application bootstrap in a unit test also initializes infrastructure.  An
AST contract test keeps this lane deterministic while still proving that the
real route module exposes the expected protected surface.
"""

from __future__ import annotations

import ast
from pathlib import Path


API_PATH = Path(__file__).parents[5] / "api" / "apps" / "restful_apis" / "business_document_api.py"


EXPECTED_ROUTES = {
    "search_eva_business_document_sources": ("/business-documents/eva/sources", ("GET",)),
    "create_eva_business_document_change": ("/business-documents/eva/changes", ("POST",)),
    "list_eva_business_document_changes": ("/business-documents/eva/changes", ("GET",)),
    "get_eva_business_document_change": ("/business-documents/eva/changes/<change_id>", ("GET",)),
    "generate_eva_business_document_change_draft": ("/business-documents/eva/changes/<change_id>/generate", ("POST",)),
    "approve_eva_business_document_change": ("/business-documents/eva/changes/<change_id>/approve", ("POST",)),
    "prepare_eva_business_document_change": ("/business-documents/eva/changes/<change_id>/prepare", ("POST",)),
    "publish_eva_business_document_change": ("/business-documents/eva/changes/<change_id>/publish", ("POST",)),
    "create_business_document": ("/business-documents", ("POST",)),
    "list_business_documents": ("/business-documents", ("GET",)),
    "list_business_document_catalog": ("/business-documents/catalog", ("GET",)),
    "get_business_document": ("/business-documents/<document_id>", ("GET",)),
    "delete_business_document": ("/business-documents/<document_id>", ("DELETE",)),
    "pull_business_document_from_eva": ("/business-documents/<document_id>/eva/pull", ("POST",)),
    "rebind_business_document_to_eva": ("/business-documents/<document_id>/eva/rebind", ("POST",)),
    "create_business_document_eva_change": ("/business-documents/<document_id>/eva/changes", ("POST",)),
    "execute_business_document_command": ("/business-documents/<document_id>/commands", ("POST",)),
    "list_business_document_revisions": ("/business-documents/<document_id>/revisions", ("GET",)),
    "get_business_document_revision": ("/business-documents/<document_id>/revisions/<revision_id>", ("GET",)),
    "list_business_document_jobs": ("/business-documents/<document_id>/jobs", ("GET",)),
    "list_business_document_exports": ("/business-documents/<document_id>/exports", ("GET",)),
    "download_business_document_export": ("/business-documents/<document_id>/exports/<artifact_id>/download", ("GET",)),
}


def _module() -> ast.Module:
    return ast.parse(API_PATH.read_text(encoding="utf-8"), filename=str(API_PATH))


def _literal(value: ast.AST):
    return ast.literal_eval(value)


def _route_contract(function: ast.AsyncFunctionDef) -> tuple[str, tuple[str, ...]]:
    route = next(decorator for decorator in function.decorator_list if isinstance(decorator, ast.Call) and isinstance(decorator.func, ast.Attribute) and decorator.func.attr == "route")
    path = _literal(route.args[0])
    methods_keyword = next(keyword for keyword in route.keywords if keyword.arg == "methods")
    return path, tuple(_literal(methods_keyword.value))


def test_http_surface_is_exact_and_every_route_requires_login():
    functions = {node.name: node for node in _module().body if isinstance(node, ast.AsyncFunctionDef) and node.name in EXPECTED_ROUTES}

    assert set(functions) == set(EXPECTED_ROUTES)
    for name, expected in EXPECTED_ROUTES.items():
        function = functions[name]
        assert _route_contract(function) == expected
        assert any(isinstance(decorator, ast.Name) and decorator.id == "login_required" for decorator in function.decorator_list)


def test_mutating_routes_read_json_and_all_routes_map_domain_errors():
    functions = {node.name: node for node in _module().body if isinstance(node, ast.AsyncFunctionDef) and node.name in EXPECTED_ROUTES}

    for name, function in functions.items():
        calls = [node for node in ast.walk(function) if isinstance(node, ast.Call)]
        called_names = {call.func.id for call in calls if isinstance(call.func, ast.Name)}
        handlers = [node for node in ast.walk(function) if isinstance(node, ast.ExceptHandler)]
        assert any(isinstance(handler.type, ast.Name) and handler.type.id == "BusinessDocumentError" for handler in handlers)
        assert "_error" in called_names
        assert "thread_pool_exec" in called_names
        if name in {
            "create_eva_business_document_change",
            "generate_eva_business_document_change_draft",
            "approve_eva_business_document_change",
            "prepare_eva_business_document_change",
            "publish_eva_business_document_change",
            "create_business_document",
            "pull_business_document_from_eva",
            "rebind_business_document_to_eva",
            "create_business_document_eva_change",
            "execute_business_document_command",
        }:
            assert "get_request_json" in called_names


def test_http_success_and_error_envelopes_remain_explicit():
    tree = _module()
    helpers = {node.name: node for node in tree.body if isinstance(node, ast.FunctionDef) and node.name in {"_success", "_error"}}

    assert set(helpers) == {"_success", "_error"}
    success_constants = {node.value for node in ast.walk(helpers["_success"]) if isinstance(node, ast.Constant)}
    error_attributes = {node.attr for node in ast.walk(helpers["_error"]) if isinstance(node, ast.Attribute)}
    assert {"code", "data"}.issubset(success_constants)
    assert {"code", "message", "details", "status"}.issubset(error_attributes)
