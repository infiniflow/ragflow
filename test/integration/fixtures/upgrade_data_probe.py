"""Subprocess probe imported against either the archived old or current source."""

import hashlib
import inspect
import json
import logging
import os
from pathlib import Path
import socket
import sys


def main():
    source, mode, output = Path(sys.argv[1]).resolve(), sys.argv[2], Path(sys.argv[3])
    sys.path.insert(0, str(source))
    os.chdir(source)
    config = json.loads(os.environ["RAGFLOW_UPGRADE_PROBE_DB"])
    assert config["name"].startswith("t1_upgrade_")
    socketpair_code = getattr(socket.socketpair, "__code__", None)

    def deny_external_network(event, args):
        if event not in {"socket.connect", "socket.getaddrinfo", "socket.sendto"}:
            return
        caller = sys._getframe(1)
        if event == "socket.connect" and caller.f_code is socketpair_code:
            listener = caller.f_locals.get("lsock")
            if listener is not None and args[1] == listener.getsockname()[:2]:
                return
        raise RuntimeError("Upgrade model probe forbids Python network clients")

    # libpq uses its native connection, configured below exclusively for the test DB.
    # Redis, HTTP and model clients must not connect during these model-only probes.
    sys.addaudithook(deny_external_network)
    from common.config_utils import CONFIGS

    # Supply the isolated runtime configuration before the real settings import.
    CONFIGS["postgres"] = config
    from common import settings

    settings.DATABASE_TYPE = "postgres"
    settings.DATABASE = config
    from api.db import db_models as models
    from api.db.services.user_external_credential_service import UserExternalCredentialService

    assert Path(models.__file__).resolve().is_relative_to(source)

    def snapshot():
        names = {"User", "UserTenant", "Knowledgebase", "UserExternalCredential", "SystemAuditEvent"}
        result = {}
        for name, model in inspect.getmembers(models, inspect.isclass):
            if name in names or name.startswith(("BusinessDocument", "AccessGroup")):
                if issubclass(model, models.DataBaseModel) and model.table_exists():
                    database_columns = {column.name for column in models.DB.get_columns(model._meta.table_name)}
                    fields = [field for field in model._meta.sorted_fields if field.column_name in database_columns]
                    rows = list(model.select(*fields).dicts())
                    result[model._meta.table_name] = sorted(rows, key=lambda value: json.dumps(value, sort_keys=True, default=str))
        return json.loads(json.dumps(result, sort_keys=True, default=str))

    def seed():
        models.User.create(id="t1-user", nickname="Synthetic user", email="t1@example.test")
        models.UserTenant.create(id="t1-membership", tenant_id="t1-tenant", user_id="t1-user", invited_by="t1-user", role="owner")
        models.Knowledgebase.create(id="t1-dataset", tenant_id="t1-tenant", name="Synthetic dataset", embd_id="synthetic", created_by="t1-user", permission="team")
        body = "# Synthetic requirements\n\nEvidence survives upgrades.\n"
        content_hash = "sha256:" + hashlib.sha256(body.encode()).hexdigest()
        document_values = dict(
            id="t1-document",
            tenant_id="t1-tenant",
            owner_id="t1-user",
            chat_id="business-document:t1-document",
            title="Synthetic requirements",
            idea="Preserve synthetic data",
            dataset_ids=["t1-dataset"],
            template_version="1",
            policy_version="1",
            current_revision_id="t1-revision",
            state_version=3,
        )
        if hasattr(models.BusinessDocument, "title_key"):
            document_values["title_key"] = hashlib.sha256("synthetic requirements".encode()).hexdigest()
        models.BusinessDocument.create(**document_values)
        models.BusinessDocumentEvent.create(
            id="t1-event",
            document_id="t1-document",
            sequence=1,
            event_type="DocumentCreated",
            actor_type="USER",
            actor_id="t1-user",
            payload={
                "title": "Synthetic requirements",
                "eva_binding": {
                    "page_url": "https://eva.example.test/project/Document/T1",
                    "status": "CONNECTED",
                    "connector_id": "t1-connector",
                    "eva_origin": "https://eva-api.example.test",
                    "project_id": "t1-project",
                    "document_id": "t1-eva-page",
                    "document_name": "Synthetic requirements",
                },
            },
            correlation_id="t1-operation",
        )
        models.BusinessDocumentEvent.create(
            id="t1-pull-event",
            document_id="t1-document",
            sequence=2,
            event_type="EvaDocumentPulled",
            actor_type="USER",
            actor_id="t1-user",
            payload={
                "remote_version": "7",
                "remote_content_hash": "sha256:t1-remote-content",
                "review_cycle": 2,
            },
            correlation_id="t1-pull",
        )
        models.BusinessDocumentRevision.create(
            id="t1-revision",
            document_id="t1-document",
            author_id="t1-user",
            revision_number=1,
            document_ast={"schema_version": "1", "sections": [{"id": "scope", "title": "Scope", "blocks": []}]},
            body_markdown=body,
            content_hash=content_hash,
            source_event_ids=["t1-event"],
        )
        models.BusinessDocumentJob.create(
            id="t1-job",
            document_id="t1-document",
            tenant_id="t1-tenant",
            job_type="DRAFT",
            dedupe_key="t1-dedupe",
            source_state_version=3,
            payload={"revision_id": "t1-revision"},
            available_at=1_700_000_000_000,
            correlation_id="t1-operation",
        )
        models.BusinessDocumentEvidenceSnapshot.create(
            id="t1-evidence",
            job_id="t1-job",
            document_id="t1-document",
            tenant_id="t1-tenant",
            dataset_ids=["t1-dataset"],
            snapshot={"chunks": [{"id": "chunk-1", "content": "Synthetic supporting text"}]},
            evidence_hash="sha256:" + "e" * 64,
        )
        models.BusinessDocumentCommand.create(
            id="t1-command", document_id="t1-document", tenant_id="t1-tenant", idempotency_key="t1-command", request_hash="sha256:" + "c" * 64, response={"accepted": True, "job_id": "t1-job"}
        )
        models.BusinessDocumentQuestion.create(
            id="t1-question",
            document_id="t1-document",
            stage="INTAKE",
            semantic_tag="scope",
            text="Synthetic question?",
            options=[{"option_id": "yes", "label": "Yes"}],
            source_event_ids=["t1-event"],
            evidence_refs=["chunk-1"],
        )
        models.BusinessDocumentAnswer.create(id="t1-answer", document_id="t1-document", question_id="t1-question", actor_id="t1-user", selected_option_id="yes")
        models.BusinessDocumentProposal.create(
            id="t1-proposal",
            document_id="t1-document",
            review_cycle=1,
            text="Synthetic proposal",
            rationale="Preserve decision",
            source_event_ids=["t1-event"],
            fingerprint="sha256:" + "f" * 64,
            source_scope_hash="sha256:" + "s" * 64,
        )
        models.BusinessDocumentProposalDecision.create(id="t1-decision", document_id="t1-document", proposal_id="t1-proposal", actor_id="t1-user", decision="ACCEPT")
        models.BusinessDocumentComment.create(
            id="t1-comment", document_id="t1-document", review_cycle=1, revision_id="t1-revision", actor_id="t1-user", text="Synthetic comment", anchor={"selected_text": "Evidence"}
        )
        models.BusinessDocumentExportArtifact.create(
            id="t1-export",
            document_id="t1-document",
            tenant_id="t1-tenant",
            owner_id="t1-user",
            revision_id="t1-revision",
            export_format="markdown",
            filename="synthetic.md",
            mime_type="text/markdown",
            size=len(body.encode()),
            content_hash=content_hash,
            storage_bucket="t1-synthetic",
            storage_key="exports/synthetic.md",
        )
        models.SystemAuditEvent.create(
            id="t1-audit",
            tenant_id="t1-tenant",
            actor_type="USER",
            actor_id="t1-user",
            action="DocumentCreated",
            outcome="success",
            correlation_id="t1-operation",
            object_id="t1-document",
            event_metadata={"stage": "fixture"},
        )
        models.BusinessDocumentEvaChange.create(
            id="t1-eva-change",
            tenant_id="t1-tenant",
            owner_id="t1-user",
            connector_id="t1-connector",
            eva_project_id="t1-project",
            eva_document_id="t1-eva-page",
            eva_document_name="Synthetic EVA page",
            eva_web_url="https://eva.example.test/page/t1",
            change_summary="Synthetic change",
            base_version="1",
            base_content_hash=content_hash,
            base_html="<p>Synthetic</p>",
            base_markdown=body,
            draft_markdown=body,
            draft_html="<p>Synthetic</p>",
            draft_content_hash=content_hash,
        )
        models.BusinessDocumentEvaChangeEvent.create(
            id="t1-eva-event", change_id="t1-eva-change", sequence=1, event_type="ChangeCreated", actor_id="t1-user", payload={"source_document_id": "t1-document"}
        )
        UserExternalCredentialService.put_eva_wiki_token("t1-user", "https://eva.example.test", "t1-synthetic-token")

    try:
        if mode == "fail_upgrade":
            before = snapshot()
            assert "access_group" not in before
            # Database-local server fault injection, not disk exhaustion. Sequence
            # advancement survives the failed DDL transaction and proves the hit.
            models.DB.execute_sql("CREATE SEQUENCE public.t1_ddl_failure_hits")
            models.DB.execute_sql("""
                CREATE FUNCTION public.t1_fail_access_group_ddl()
                RETURNS event_trigger LANGUAGE plpgsql AS $$
                BEGIN
                    IF EXISTS (
                        SELECT 1 FROM pg_event_trigger_ddl_commands()
                        WHERE object_identity = 'public.access_group'
                    ) THEN
                        PERFORM nextval('public.t1_ddl_failure_hits');
                        RAISE EXCEPTION 'T1_SERVER_INJECTED_DDL_FAILURE'
                            USING ERRCODE = '53100';
                    END IF;
                END;
                $$
            """)
            models.DB.execute_sql("""
                CREATE EVENT TRIGGER t1_fail_access_group_create
                ON ddl_command_end WHEN TAG IN ('CREATE TABLE')
                EXECUTE FUNCTION public.t1_fail_access_group_ddl()
            """)
            try:
                models.init_database_tables()
            except Exception as error:
                assert "create tables failed" in str(error), type(error).__name__
            else:
                raise AssertionError("Initializer accepted a failed table creation")
            hits, called = models.DB.execute_sql("SELECT last_value, is_called FROM public.t1_ddl_failure_hits").fetchone()
            assert called and hits == 1, "Server-side DDL fault was not reached exactly once"
            assert models.DB.execute_sql("SELECT to_regclass('public.access_group')").fetchone()[0] is None
            after = snapshot()
            assert {name: after[name] for name in before} == before, "Failed DDL changed previous-release rows"
            output.write_text(
                json.dumps({"injected_failure_detected": True, "server_ddl_failure_hits": hits, "injected_sqlstate": "53100", "snapshot": after}, sort_keys=True),
                encoding="utf-8",
            )
            return
        if mode in {"seed", "upgrade", "groups"}:
            models.init_database_tables()
            if mode == "seed":
                seed()
            if mode == "groups":
                from api.db.services.access_group_service import AccessGroupService

                AccessGroupService.create({"name": "T1 preserved group", "user_ids": ["t1-user"], "dataset_ids": ["t1-dataset"], "sections": ["dataset", "business_documents"]})
        token = UserExternalCredentialService.get_eva_wiki_token("t1-user", "https://eva.example.test")
        assert token.secret == "t1-synthetic-token" and token.credential_version == 1
        output.write_text(json.dumps(snapshot(), sort_keys=True), encoding="utf-8")
    finally:
        logging.disable(logging.NOTSET)
        models.DB.close_all()


if __name__ == "__main__":
    main()
