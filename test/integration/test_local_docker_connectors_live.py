"""Opt-in live EVA and OpenMetadata checks for the local Docker QA contour.

Run this file inside the RAGFlow application container with
``RAGFLOW_LOCAL_DOCKER_CONNECTOR_TEST=1``.  Credentials are read from the
container's existing non-production connector rows and are never serialized.
Every external mutation is either restored exactly or deleted and verified.
"""

from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import os
import threading
from uuid import uuid4

import pytest
import requests


def _require_live_contour() -> None:
    if os.environ.get("RAGFLOW_LOCAL_DOCKER_CONNECTOR_TEST") != "1":
        pytest.skip("Requires the explicitly enabled local Docker connector contour")


def _connector_by_name(name: str):
    from api.db.db_models import Connector

    connector = Connector.get_or_none(Connector.name == name)
    assert connector is not None, f"Local QA connector is missing: {name}"
    config = dict(connector.config)
    credentials = config.pop("credentials", None)
    assert isinstance(credentials, dict) and any(credentials.values()), f"Local QA connector has no credentials: {name}"
    config.pop("sync_deleted_files", None)
    return config, credentials


class _FaultOnceSession:
    """Inject one retryable GET failure, then delegate to the real service."""

    def __init__(self):
        self.inner = requests.Session()
        self.failed_once = False

    def post(self, *args, **kwargs):
        return self.inner.post(*args, **kwargs)

    def request(self, method, *args, **kwargs):
        if method.upper() == "GET" and not self.failed_once:
            self.failed_once = True
            response = requests.Response()
            response.status_code = 503
            response._content = b'{"error":"injected transient"}'
            response.headers["Content-Type"] = "application/json"
            return response
        return self.inner.request(method, *args, **kwargs)


@pytest.mark.p0
def test_openmetadata_live_read_write_retry_and_restore():
    _require_live_contour()
    from api.apps.services.openmetadata_copilot_service import OpenMetadataClient, OpenMetadataConfig
    from common.data_source.openmetadata_connector import OpenMetadataConnector

    config, credentials = _connector_by_name(os.environ.get("RAGFLOW_OPENMETADATA_TEST_CONNECTOR", "OpenMetadata Catalog Sync"))
    connector = OpenMetadataConnector(**config)
    connector.load_credentials(credentials)
    connector.validate_connector_settings()
    rows = connector._list_tables()
    requested_fqn = os.environ.get("RAGFLOW_OPENMETADATA_TEST_TABLE", "docker_postgres_eva.eva.public.cmf_testcase")
    target = next((row for row in rows if row.get("fullyQualifiedName") == requested_fqn), None)
    assert target is not None, f"Local OpenMetadata QA table is missing: {requested_fqn}"

    client_config = OpenMetadataConfig(
        base_url=config["base_url"],
        public_url=config["public_url"],
        username=str(credentials.get("openmetadata_username") or ""),
        password=str(credentials.get("openmetadata_password") or ""),
        jwt_token=str(credentials.get("openmetadata_jwt_token") or ""),
        timeout_seconds=12,
        retries=2,
        cache_ttl_seconds=10,
        stale_after_hours=168,
        max_entities=10,
        max_results=10,
        write_enabled=True,
        confirmation_ttl_seconds=60,
    )
    fault_session = _FaultOnceSession()
    retry_client = OpenMetadataClient(client_config, session=fault_session)
    assert retry_client.get("/api/v1/system/version").get("version")
    assert fault_session.failed_once

    client = OpenMetadataClient(client_config)
    entity_id = str(target["id"])
    original_description = client.get_table(entity_id).get("description")
    marker = f"T1 local Docker write/recovery fixture {uuid4().hex}"
    restored = False
    try:
        operation = "replace" if original_description is not None else "add"
        changed = client.patch(f"/api/v1/tables/{entity_id}", [{"op": operation, "path": "/description", "value": marker}])
        assert changed.get("description") == marker
        assert client.get_table(entity_id).get("description") == marker
    finally:
        if original_description is None:
            client.patch(f"/api/v1/tables/{entity_id}", [{"op": "remove", "path": "/description"}])
        else:
            client.patch(f"/api/v1/tables/{entity_id}", [{"op": "replace", "path": "/description", "value": original_description}])
        restored = client.get_table(entity_id).get("description") == original_description
    assert restored


@pytest.mark.p0
def test_eva_live_retry_create_publish_update_and_cleanup():
    _require_live_contour()
    from common.data_source.eva_wiki_connector import EvaWikiConnector, EvaWikiMutationClient

    config, credentials = _connector_by_name(os.environ.get("RAGFLOW_EVA_TEST_CONNECTOR", "EVA Wiki native smoke"))
    config["include_attachments"] = False
    target_base = config["api_base_url"]
    state = {"attempts": 0, "failed_once": False}

    class FaultProxy(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            return

        def do_POST(self):  # noqa: N802 - BaseHTTPRequestHandler protocol name
            state["attempts"] += 1
            if not state["failed_once"]:
                state["failed_once"] = True
                body = b'{"error":"injected transient"}'
                self.send_response(503)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)
                return
            length = int(self.headers.get("Content-Length", "0"))
            request_body = self.rfile.read(length)
            headers = {key: value for key, value in self.headers.items() if key.lower() not in {"host", "content-length", "connection", "accept-encoding"}}
            response = requests.post(target_base + self.path, data=request_body, headers=headers, timeout=30, allow_redirects=False)
            self.send_response(response.status_code)
            for key, value in response.headers.items():
                if key.lower() not in {"transfer-encoding", "connection", "content-encoding", "content-length"}:
                    self.send_header(key, value)
            self.send_header("Content-Length", str(len(response.content)))
            self.end_headers()
            self.wfile.write(response.content)

    server = ThreadingHTTPServer(("127.0.0.1", 0), FaultProxy)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    try:
        retry_config = {**config, "api_base_url": f"http://127.0.0.1:{server.server_port}", "verify_ssl": False, "retry_count": 2}
        retry_reader = EvaWikiConnector(**retry_config)
        retry_reader.load_credentials(credentials)
        retry_reader.get_project()
        assert state["failed_once"] and state["attempts"] >= 2
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=5)

    reader = EvaWikiConnector(**config)
    reader.load_credentials(credentials)
    writer = EvaWikiMutationClient(
        api_base_url=config["api_base_url"],
        project_id=config["project_id"],
        verify_ssl=config.get("verify_ssl", True),
        retry_count=config.get("retry_count", 3),
    )
    writer.load_credentials(credentials)
    key = uuid4().hex[:12]
    name = f"T1 local synthetic {key}"
    created = reader._rpc(
        "CmfDocument.create",
        {},
        call_kwargs={"project": config["project_id"], "name": name, "code": f"T1-{key}", "text_draft": "<p>synthetic first revision</p>"},
    )
    deleted = False
    try:
        writer.publish_document(created)
        first = reader.get_document_for_edit(created)
        assert "synthetic first revision" in first["html"]
        writer.update_document_draft(created, "<p>synthetic recovered revision</p>", name=name + " updated")
        writer.publish_document(created)
        second = reader.get_document_for_edit(created)
        assert second["name"] == name + " updated"
        assert "synthetic recovered revision" in second["html"]
    finally:
        reader._rpc("CmfDocument.delete", {}, args=[created])
        deleted = reader._get_entity_by_id("CmfDocument", reader._EDITABLE_DOCUMENT_FIELDS, created) is None
    assert deleted
