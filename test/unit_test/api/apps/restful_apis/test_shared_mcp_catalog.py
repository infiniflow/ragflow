import asyncio
import copy
import json
from contextlib import contextmanager
from types import SimpleNamespace
import unittest
from unittest.mock import AsyncMock, Mock, patch

import pytest
from peewee import SqliteDatabase

from api.db.db_models import DB, UserCanvas
from api.db.services.canvas_service import UserCanvasService
from test.testcases.restful_api.test_mcp_routes_unit import _load_mcp_api


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
            id="server",
            tenant_id="team",
            name="Project tools",
            server_type="streamable-http",
            url="https://user:credential-value@example.test/mcp?key=credential-value",
            headers={"Authorization": "credential-value"},
            variables={
                "authorization_token": "credential-value",
                "tools": {
                    "allowed": {"name": "allowed", "inputSchema": {"type": "object"}, "secret_extension": "credential-value"},
                    "unselected": {"name": "unselected"},
                },
            },
        )
        self.server.to_dict = lambda: {"id": self.server.id, "headers": self.server.headers}
        self.mcp = Mock()
        self.mcp.get_by_id.return_value = (True, self.server)
        self.mcp.get_or_none.side_effect = lambda **kw: self.server if kw["tenant_id"] == "team" else None
        self.mcp.get_servers.return_value = []
        self.members = Mock()
        self.members.get_tenants_by_user_id.return_value = [{"tenant_id": "team", "role": "normal"}]
        self.canvases = Mock()
        self.canvases.query.return_value = [
            SimpleNamespace(
                dsl=json.dumps(
                    {
                        "components": {
                            "agent": {
                                "obj": {
                                    "component_name": "Agent",
                                    "params": {
                                        "mcp": [
                                            {"mcp_id": "server", "tools": {"allowed": {}}},
                                        ]
                                    },
                                }
                            },
                        }
                    }
                )
            )
        ]
        self.user = SimpleNamespace(id="member")
        self.request = SimpleNamespace(args=Args())
        self.monkeypatch = pytest.MonkeyPatch()
        self.addCleanup(self.monkeypatch.undo)
        self.module = _load_mcp_api(self.monkeypatch)
        for name, value in {
            "current_user": self.user,
            "request": self.request,
            "MCPServerService": self.mcp,
            "UserTenantService": self.members,
            "UserCanvasService": self.canvases,
            "safe_json_parse": parse_json,
            "get_request_json": AsyncMock(return_value={}),
        }.items():
            self.monkeypatch.setattr(self.module, name, value)
        self.ns = vars(self.module)

    def test_member_gets_only_selected_tool_metadata(self):
        result = self.ns["detail"]("server")
        self.assertEqual(result["code"], 0)
        self.assertEqual(set(result["data"]["variables"]["tools"]), {"allowed"})
        self.assertTrue(result["data"]["read_only"])
        self.assertEqual(result["data"]["url"], "")
        self.assertNotIn("credential-value", json.dumps(result))
        self.assertNotIn("headers", result["data"])
        self.canvases.query.assert_called_once_with(user_id="team", permission="team", canvas_category=self.module.CanvasCategory.Agent)

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

    @contextmanager
    def sqlite_canvases(self):
        database = SqliteDatabase(":memory:")
        with database.bind_ctx([UserCanvas], bind_refs=False, bind_backrefs=False):
            database.create_tables([UserCanvas])
            try:
                with (
                    patch.object(DB, "is_closed", return_value=False),
                    patch.object(DB, "close"),
                    patch.object(DB, "connect", side_effect=AssertionError("Unexpected external database connection")),
                    patch.object(self.module, "UserCanvasService", UserCanvasService),
                ):
                    yield
            finally:
                database.close()

    def test_dataflow_with_agent_component_does_not_grant_access(self):
        with self.sqlite_canvases():
            UserCanvas.create(
                id="dataflow",
                user_id="team",
                permission="team",
                canvas_category=self.module.CanvasCategory.DataFlow,
                dsl=json.loads(self.canvases.query.return_value[0].dsl),
            )
            self.assertEqual([row.id for row in UserCanvasService.query(user_id="team", permission="team")], ["dataflow"])
            self.assertEqual(UserCanvasService.query(user_id="team", permission="team", canvas_category=self.module.CanvasCategory.Agent), [])
            with patch.object(self.module, "safe_json_parse", wraps=parse_json) as parse:
                self.assertEqual(self.ns["detail"]("server")["code"], 102)
                parse.assert_not_called()

    def test_database_query_excludes_dataflow_private_and_other_tenant_canvases(self):
        allowed_dsl = json.loads(self.canvases.query.return_value[0].dsl)
        other_dsl = copy.deepcopy(allowed_dsl)
        other_dsl["components"]["agent"]["obj"]["params"]["mcp"][0]["tools"] = {"unselected": {}}
        with self.sqlite_canvases():
            for cid, owner, permission, category, dsl in (
                ("agent", "team", "team", self.module.CanvasCategory.Agent, allowed_dsl),
                ("dataflow", "team", "team", self.module.CanvasCategory.DataFlow, other_dsl),
                ("private", "team", "me", self.module.CanvasCategory.Agent, other_dsl),
                ("other-tenant", "other", "team", self.module.CanvasCategory.Agent, other_dsl),
            ):
                UserCanvas.create(id=cid, user_id=owner, permission=permission, canvas_category=category, dsl=dsl)
            rows = UserCanvasService.query(user_id="team", permission="team", canvas_category=self.module.CanvasCategory.Agent)
            self.assertEqual([row.id for row in rows], ["agent"])
            result = self.ns["detail"]("server")
            self.assertEqual(result["code"], 0)
            self.assertEqual(set(result["data"]["variables"]["tools"]), {"allowed"})

    def test_other_server_reference_does_not_grant_access(self):
        self.canvases.query.return_value = [
            SimpleNamespace(
                dsl={
                    "components": {
                        "a": {
                            "obj": {
                                "component_name": "Agent",
                                "params": {"mcp": [{"mcp_id": "other", "tools": {"allowed": {}}}]},
                            }
                        }
                    }
                }
            )
        ]
        self.assertEqual(self.ns["detail"]("server")["code"], 102)

    def test_malformed_canvas_dsl_is_ignored(self):
        malformed_dsls = [
            {"components": "not-a-dict"},
            {"components": {"agent": "not-a-dict"}},
            {"components": {"agent": {"obj": "not-a-dict"}}},
            {"components": {"agent": {"obj": {"component_name": "Agent", "params": "not-a-dict"}}}},
            {"components": {"agent": {"obj": {"component_name": "Agent", "params": {"mcp": "not-a-list"}}}}},
            {"components": {"agent": {"obj": {"component_name": "Agent", "params": {"mcp": ["not-a-dict"]}}}}},
            {"components": {"agent": {"obj": {"component_name": "Agent", "params": {"mcp": [{"mcp_id": "server", "tools": "not-a-dict"}]}}}}},
        ]
        for dsl in malformed_dsls:
            with self.subTest(dsl=dsl):
                self.canvases.query.return_value = [SimpleNamespace(dsl=json.dumps(dsl))]
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
