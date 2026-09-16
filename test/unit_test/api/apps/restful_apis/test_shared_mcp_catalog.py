import ast
import asyncio
import copy
import json
import os
from pathlib import Path
from types import SimpleNamespace
import unittest
from unittest.mock import AsyncMock, Mock


SOURCE = Path(os.environ.get("RAGFLOW_TEST_MCP_SOURCE", Path(__file__).resolve().parents[5] / "api/apps/restful_apis/mcp_api.py"))
FUNCTIONS = {"_get_mcp_ids_from_args", "_shared_mcp_metadata", "_export_mcp_servers", "detail", "list_mcp", "update", "rm"}


class Args(dict):
    def getlist(self, key):
        value = self.get(key, [])
        return value if isinstance(value, list) else [value]


def parse_json(value):
    if isinstance(value, dict):
        return value
    try:
        return json.loads(value)
    except (ValueError, TypeError):
        return {}


class SharedMcpCatalogTests(unittest.TestCase):
    def setUp(self):
        self.server = SimpleNamespace(
            id="server", tenant_id="team", name="Project tools", server_type="streamable-http",
            url="https://user:credential-value@example.test/mcp?key=credential-value",
            headers={"Authorization": "credential-value"},
            variables={"authorization_token": "credential-value", "tools": {
                "allowed": {"name": "allowed", "inputSchema": {"type": "object"}, "secret_extension": "credential-value"},
                "unselected": {"name": "unselected"},
            }},
        )
        self.server.to_dict = lambda: {"id": self.server.id, "headers": self.server.headers}
        self.mcp = Mock()
        self.mcp.get_by_id.return_value = (True, self.server)
        self.mcp.get_or_none.side_effect = lambda **kw: self.server if kw["tenant_id"] == "team" else None
        self.mcp.get_servers.return_value = []
        self.members = Mock()
        self.members.get_tenants_by_user_id.return_value = [{"tenant_id": "team", "role": "normal"}]
        self.canvases = Mock()
        self.canvases.query.return_value = [SimpleNamespace(dsl=json.dumps({"components": {
            "agent": {"obj": {"component_name": "Agent", "params": {"mcp": [
                {"mcp_id": "server", "tools": {"allowed": {}}},
            ]}}},
        }}))]
        self.user = SimpleNamespace(id="member")
        self.request = SimpleNamespace(args=Args())
        self.ns = {
            "Response": object, "current_user": self.user, "request": self.request,
            "MCPServerService": self.mcp, "UserTenantService": self.members, "UserCanvasService": self.canvases,
            "safe_json_parse": parse_json,
            "get_json_result": lambda data: {"code": 0, "data": data},
            "get_data_error_result": lambda message: {"code": 102, "message": message},
            "server_error_response": lambda exc: {"code": 100, "message": str(exc)},
            "get_request_json": AsyncMock(return_value={}),
            "DEFAULT_PAGE": 1, "DEFAULT_PAGE_SIZE": 30,
            "validate_rest_api_page": int, "validate_rest_api_page_size": int,
            "validate_rest_api_ids": lambda *_: None,
        }
        nodes = []
        for node in ast.parse(SOURCE.read_text()).body:
            if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and node.name in FUNCTIONS:
                node.decorator_list = []
                nodes.append(node)
        exec(compile(ast.Module(body=nodes, type_ignores=[]), str(SOURCE), "exec"), self.ns)

    def test_member_gets_only_selected_tool_metadata(self):
        result = self.ns["detail"]("server")
        self.assertEqual(result["code"], 0)
        self.assertEqual(set(result["data"]["variables"]["tools"]), {"allowed"})
        self.assertTrue(result["data"]["read_only"])
        self.assertEqual(result["data"]["url"], "")
        self.assertNotIn("credential-value", json.dumps(result))
        self.assertNotIn("headers", result["data"])
        self.canvases.query.assert_called_once_with(user_id="team", permission="team")

    def test_owner_management_response_is_unchanged(self):
        self.user.id = "team"
        result = self.ns["detail"]("server")
        self.assertEqual(result["data"], self.server.to_dict())
        self.members.get_tenants_by_user_id.assert_not_called()

    def test_nonmember_invited_and_other_tenant_are_denied(self):
        for membership in ([], [{"tenant_id": "team", "role": "invite"}], [{"tenant_id": "other", "role": "normal"}]):
            with self.subTest(membership=membership):
                self.members.get_tenants_by_user_id.return_value = membership
                self.assertEqual(self.ns["detail"]("server")["code"], 102)
        self.canvases.query.assert_not_called()

    def test_revoked_membership_is_not_cached(self):
        self.assertEqual(self.ns["detail"]("server")["code"], 0)
        self.members.get_tenants_by_user_id.return_value = []
        self.assertEqual(self.ns["detail"]("server")["code"], 102)

    def test_server_not_referenced_by_shared_agent_is_denied(self):
        self.canvases.query.return_value = []
        self.assertEqual(self.ns["detail"]("server")["code"], 102)

    def test_other_server_reference_does_not_grant_access(self):
        self.canvases.query.return_value = [SimpleNamespace(dsl={"components": {"a": {"obj": {
            "component_name": "Agent", "params": {"mcp": [{"mcp_id": "other", "tools": {"allowed": {}}}]},
        }}}})]
        self.assertEqual(self.ns["detail"]("server")["code"], 102)

    def test_member_cannot_download_credentials(self):
        self.request.args["mode"] = "download"
        result = self.ns["detail"]("server")
        self.assertEqual(result["code"], 102)
        self.assertNotIn("credential-value", json.dumps(result))

    def test_member_cannot_modify_or_delete_server(self):
        for operation in ("update", "rm"):
            with self.subTest(operation=operation):
                self.assertEqual(asyncio.run(self.ns[operation]("server"))["code"], 102)
        self.mcp.filter_update.assert_not_called()
        self.mcp.delete_by_ids.assert_not_called()

    def test_id_filtered_listing_returns_sanitized_metadata_once(self):
        self.request.args["mcp_ids"] = ["server", "server"]
        result = asyncio.run(self.ns["list_mcp"]())
        self.assertEqual(result["data"]["total"], 1)
        self.assertNotIn("credential-value", json.dumps(result))
        self.assertTrue(result["data"]["mcp_servers"][0]["read_only"])

    def test_unfiltered_listing_does_not_enumerate_shared_servers(self):
        result = asyncio.run(self.ns["list_mcp"]())
        self.assertEqual(result["data"], {"total": 0, "mcp_servers": []})
        self.members.get_tenants_by_user_id.assert_not_called()

    def test_shared_listing_honors_keyword(self):
        self.request.args.update({"mcp_id": "server", "keywords": "unmatched"})
        result = asyncio.run(self.ns["list_mcp"]())
        self.assertEqual(result["data"]["total"], 0)

    def test_shared_read_does_not_mutate_stored_credentials(self):
        before = copy.deepcopy(self.server.variables)
        self.ns["detail"]("server")
        self.assertEqual(self.server.variables, before)


if __name__ == "__main__":
    unittest.main()
