#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""Integration tests for :func:`PipelineOperationLogService.create`.

The actual bug fix from #18306 lives inside ``create()``: when a PARSE
dataflow task has a valid pipeline DSL with a Parser component, the
persisted ``parser_id`` should reflect the operator's chosen parser
(e.g. "docling"), not the KB default the document inherited (typically
"DeepDOC"). These tests mock the heavy service dependencies so the
real ``create()`` method runs end-to-end and we assert on the captured
log payload.
"""

import importlib.util
import json
import sys
import types
from unittest.mock import MagicMock

import pytest


# --------------------------------------------------------------------------- #
# Helper: load pipeline_operation_log_service with heavy dependencies stubbed
# --------------------------------------------------------------------------- #
#
# ``create()`` pulls in DocumentService, UserCanvasService,
# KnowledgebaseService, and the peewee-backed DB. Stubbing those lets us
# exercise the real create() flow without standing up the full stack.


def _install_stubs(monkeypatch):
    # peewee.fn is referenced at module import time.
    peewee_mod = types.ModuleType("peewee")
    peewee_mod.fn = lambda *a, **kw: None
    monkeypatch.setitem(sys.modules, "peewee", peewee_mod)

    # Common settings/config_utils import chain — keep minimal.
    for mod in [
        "common", "common.constants", "common.settings", "common.config_utils",
        "common.misc_utils", "common.time_utils",
        "api", "api.db", "api.db.db_models", "api.db.services",
        "api.db.services.common_service", "api.db.services.document_service",
        "api.db.services.knowledgebase_service", "api.db.services.canvas_service",
        "api.db.services.task_service",
    ]:
        monkeypatch.setitem(sys.modules, mod, types.ModuleType(mod))

    # Peewee DB stub with the @connection_context decorator the create()
    # method uses. The decorator must preserve the wrapped function's
    # return value so the create() flow runs to completion.
    import contextlib

    @contextlib.contextmanager
    def _noop_connection_context(*a, **kw):
        yield

    DB = MagicMock()
    DB.connection_context = _noop_connection_context
    sys.modules["api.db.db_models"].DB = DB

    sys.modules["api.db.db_models"].Document = object
    # Use a MagicMock for PipelineOperationLog so cls.model.select(),
    # cls.model.delete(), etc. don't blow up the create() flow after
    # our fake cls.save captures the log payload.
    _PolModel = MagicMock()
    _PolModel.select.return_value.where.return_value.count.return_value = 0
    sys.modules["api.db.db_models"].PipelineOperationLog = _PolModel

    sys.modules["api.db.services.canvas_service"].UserCanvasService = MagicMock()
    sys.modules["api.db.services.document_service"].DocumentService = MagicMock()
    sys.modules["api.db.services.knowledgebase_service"].KnowledgebaseService = MagicMock()
    sys.modules["api.db.services.task_service"].GRAPH_RAPTOR_FAKE_DOC_ID = "fake"
    sys.modules["api.db.services.task_service"].TaskService = MagicMock()
    sys.modules["api.db.services.common_service"].CommonService = MagicMock()
    sys.modules["common.misc_utils"].get_uuid = lambda: "uuid"
    sys.modules["common.time_utils"].current_timestamp = lambda: "ts"
    sys.modules["common.time_utils"].datetime_format = lambda x: "dt"

    # rag.flow.parser.parser is required for the suffix map derivation.
    import enum

    class _PP:
        setups = {
            "pdf": {"suffix": ["pdf"]},
            "markdown": {"suffix": ["md", "markdown", "mdx"]},
            "image": {"suffix": ["jpg", "jpeg", "png", "gif"]},
        }

    rag_mod = types.ModuleType("rag"); sys.modules["rag"] = rag_mod
    flow_mod = types.ModuleType("rag.flow"); sys.modules["rag.flow"] = flow_mod
    parser_pkg = types.ModuleType("rag.flow.parser")
    parser_pkg.__path__ = ["/dev/null"]
    sys.modules["rag.flow.parser"] = parser_pkg
    parser_sub = types.ModuleType("rag.flow.parser.parser")
    parser_sub.ParserParam = _PP
    sys.modules["rag.flow.parser.parser"] = parser_sub

    # Build proper class stubs so monkeypatch.setattr can target
    # ``class.method`` without hitting ``object``'s read-only namespace.
    # Pre-declare methods so monkeypatch has something to replace.
    def _make_class_stub(name, methods):
        cls = type(name, (object,), {m: classmethod(lambda cls, *a, **kw: None) for m in methods})
        return cls

    sys.modules["api.db.services.canvas_service"].UserCanvasService = _make_class_stub(
        "UserCanvasService", ["get_by_id"]
    )
    sys.modules["api.db.services.document_service"].DocumentService = _make_class_stub(
        "DocumentService", ["get_by_id", "update_progress_immediately"]
    )
    sys.modules["api.db.services.knowledgebase_service"].KnowledgebaseService = _make_class_stub(
        "KnowledgebaseService", ["get_by_id"]
    )
    sys.modules["api.db.services.task_service"].TaskService = _make_class_stub(
        "TaskService", ["get_by_id"]
    )
    sys.modules["api.db.services.common_service"].CommonService = _make_class_stub(
        "CommonService", ["save"]
    )

    # Stub PipelineTaskType so the create() branching works.
    class PipelineTaskType(str, enum.Enum):
        PARSE = "Parse"
        DOWNLOAD = "Download"
        RAPTOR = "RAPTOR"
        GRAPH_RAG = "GraphRAG"
        MINDMAP = "Mindmap"
        MEMORY = "Memory"
        ARTIFACT = "Wiki"
        SKILL = "Skill"
        STRUCTURE_GRAPH = "StructureGraph"
        STRUCTURE_MINDMAP = "StructureMindmap"
        TIMELINE = "Timeline"
        SESSION_GRAPH = "SessionGraph"
        SESSION_ESSENCE = "SessionEssence"
        STRUCTURE = "Structure"

    class TaskStatus(str, enum.Enum):
        CANCEL = "cancel"
        DONE = "done"
        FAIL = "fail"

    sys.modules["common.constants"].PipelineTaskType = PipelineTaskType
    sys.modules["common.constants"].TaskStatus = TaskStatus
    sys.modules["api.db"].VALID_PIPELINE_TASK_TYPES = {
        PipelineTaskType.PARSE, PipelineTaskType.RAPTOR, PipelineTaskType.GRAPH_RAG,
    }

    # Load the actual module under test.
    import importlib.util
    import os
    os.chdir("/tmp/opencode/repos/ragflow")
    spec = importlib.util.spec_from_file_location(
        "_pol_test", "api/db/services/pipeline_operation_log_service.py",
    )
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


# --------------------------------------------------------------------------- #
# Test helpers
# --------------------------------------------------------------------------- #


def _make_document(suffix: str = "pdf", parser_id: str = "DeepDOC"):
    """Build a MagicMock that quacks like a Document ORM row."""
    doc = MagicMock()
    doc.parser_id = parser_id
    doc.suffix = suffix
    doc.name = f"sample.{suffix}"
    doc.kb_id = "kb-1"
    doc.run = "1"
    doc.progress = 1.0
    doc.progress_msg = ""
    doc.process_begin_at = None
    doc.process_duration = 0.0
    doc.type = "pdf"
    doc.source_type = "local/upload"
    doc.to_dict = MagicMock(return_value={"id": "doc-1"})
    return doc


def _make_user_pipeline(pipeline_id: str = "pipe-1", title: str = "My pipeline"):
    pipe = MagicMock()
    pipe.id = pipeline_id
    pipe.user_id = "tenant-1"
    pipe.title = title
    pipe.avatar = None
    return pipe


def _parse_dsl(setup_key: str, parse_method: str) -> str:
    return json.dumps(
        {
            "components": {
                "parser-node": {
                    "obj": {
                        "component_name": "Parser",
                        "params": {"setups": {setup_key: {"parse_method": parse_method}}},
                    },
                },
            },
            "path": [],
        }
    )


# --------------------------------------------------------------------------- #
# Integration tests for create() (issue #18306 regression coverage)
# --------------------------------------------------------------------------- #


@pytest.fixture
def pol_module(monkeypatch):
    return _install_stubs(monkeypatch)


@pytest.fixture
def captured_log(monkeypatch, pol_module):
    """Replace ``PipelineOperationLogService.save`` so we can capture the
    log dict create() tries to persist, then return it without writing
    to the DB."""
    captured = {}

    def _fake_save(cls, **kwargs):
        captured["log"] = kwargs
        # Return a MagicMock to satisfy the caller.
        return MagicMock()

    monkeypatch.setattr(pol_module.PipelineOperationLogService, "save", classmethod(_fake_save))
    return captured


def test_create_prefers_pipeline_parser_for_parse_task(pol_module, captured_log, monkeypatch):
    """Issue #18306: a PARSE task with a valid pipeline DSL must persist
    the pipeline's Parser choice (e.g. "docling"), not the document's
    inherited KB default ("DeepDOC")."""
    document = _make_document(suffix="pdf", parser_id="DeepDOC")
    user_pipeline = _make_user_pipeline()
    monkeypatch.setattr(
        pol_module.DocumentService, "get_by_id", MagicMock(return_value=(True, document))
    )
    monkeypatch.setattr(
        pol_module.UserCanvasService, "get_by_id", MagicMock(return_value=(True, user_pipeline))
    )

    dsl = _parse_dsl("pdf", "docling")
    pol_module.PipelineOperationLogService.create(
        document_id="doc-1",
        pipeline_id="pipe-1",
        task_type=pol_module.PipelineTaskType.PARSE,
        task_id="task-1",
        referred_document_id="doc-1",
        dsl=dsl,
    )

    log = captured_log["log"]
    assert log["parser_id"] == "docling", (
        f"create() must prefer the pipeline's Parser choice over "
        f"document.parser_id; got parser_id={log['parser_id']!r}"
    )


def test_create_uses_document_parser_id_when_dsl_has_no_parser(pol_module, captured_log, monkeypatch):
    """If the pipeline DSL has no Parser component, create() must fall
    back to document.parser_id (the previous behavior). The #18306
    fix only changes the path where a Parser is configured."""
    document = _make_document(suffix="pdf", parser_id="DeepDOC")
    user_pipeline = _make_user_pipeline()
    monkeypatch.setattr(
        pol_module.DocumentService, "get_by_id", MagicMock(return_value=(True, document))
    )
    monkeypatch.setattr(
        pol_module.UserCanvasService, "get_by_id", MagicMock(return_value=(True, user_pipeline))
    )

    # DSL with no Parser component at all.
    dsl = json.dumps({"components": {"some-other-node": {"obj": {"component_name": "Begin"}}}, "path": []})

    pol_module.PipelineOperationLogService.create(
        document_id="doc-1",
        pipeline_id="pipe-1",
        task_type=pol_module.PipelineTaskType.PARSE,
        task_id="task-1",
        referred_document_id="doc-1",
        dsl=dsl,
    )

    log = captured_log["log"]
    assert log["parser_id"] == "DeepDOC"
    # Demoted to DEBUG — no warning line expected in the captured log
    # record, but the parser_id fallback must still happen silently.
    assert log["parser_id"] == document.parser_id


def test_create_persists_sanitized_dsl(pol_module, captured_log, monkeypatch):
    """create() must persist a sanitized copy of the DSL (without
    q_<dim>_vec keys) in the dsl column — verified via the captured
    log payload, not via a DB write."""
    document = _make_document(suffix="md", parser_id="DeepDOC")
    user_pipeline = _make_user_pipeline()
    monkeypatch.setattr(
        pol_module.DocumentService, "get_by_id", MagicMock(return_value=(True, document))
    )
    monkeypatch.setattr(
        pol_module.UserCanvasService, "get_by_id", MagicMock(return_value=(True, user_pipeline))
    )

    dsl_with_vectors = json.dumps(
        {
            "components": {
                "Tokenizer:0": {
                    "obj": {
                        "params": {
                            "outputs": {
                                "chunks": {"value": [{"text": "x", "q_1024_vec": [0.1, 0.2]}]},
                            },
                        },
                    },
                },
                "parser-node": {
                    "obj": {
                        "component_name": "Parser",
                        "params": {"setups": {"markdown": {"parse_method": "docling"}}},
                    },
                },
            },
            "path": [],
        }
    )

    pol_module.PipelineOperationLogService.create(
        document_id="doc-1",
        pipeline_id="pipe-1",
        task_type=pol_module.PipelineTaskType.PARSE,
        task_id="task-1",
        referred_document_id="doc-1",
        dsl=dsl_with_vectors,
    )

    log = captured_log["log"]
    persisted_dsl = log["dsl"]
    # The q_1024_vec key must have been stripped by _remove_embedding_vectors.
    chunk = persisted_dsl["components"]["Tokenizer:0"]["obj"]["params"]["outputs"]["chunks"]["value"][0]
    assert "q_1024_vec" not in chunk
    assert chunk == {"text": "x"}
    # Parser resolution still worked.
    assert log["parser_id"] == "docling"


def test_create_falls_back_to_empty_dsl_for_malformed_dsl(pol_module, captured_log, monkeypatch, caplog):
    """Malformed DSL must not crash create() — it falls back to {} for
    the dsl column, the WARNING fires (per CodeRabbit review), and the
    parser_id falls back to document.parser_id."""
    document = _make_document(suffix="pdf", parser_id="DeepDOC")
    user_pipeline = _make_user_pipeline()
    monkeypatch.setattr(
        pol_module.DocumentService, "get_by_id", MagicMock(return_value=(True, document))
    )
    monkeypatch.setattr(
        pol_module.UserCanvasService, "get_by_id", MagicMock(return_value=(True, user_pipeline))
    )

    with caplog.at_level(pol_module.logging.WARNING, logger="root"):
        pol_module.PipelineOperationLogService.create(
            document_id="doc-1",
            pipeline_id="pipe-1",
            task_type=pol_module.PipelineTaskType.PARSE,
            task_id="task-1",
            referred_document_id="doc-1",
            dsl="not-json",  # malformed
        )

    log = captured_log["log"]
    # Malformed DSL yields {} (not None / not a crash).
    assert log["dsl"] == {}
    # Fall back to document.parser_id.
    assert log["parser_id"] == "DeepDOC"
    # The "Pipeline DSL is missing or malformed" WARNING must fire.
    assert any("Pipeline DSL is missing or malformed" in rec.message for rec in caplog.records)


def test_create_demotes_debug_log_when_dsl_has_no_parser(pol_module, captured_log, monkeypatch, caplog):
    """Per the CodeRabbit review follow-up: when the pipeline DSL is valid
    but has no Parser component (a common case for pipelines that don't
    need text extraction), the message must NOT spam the warning
    channel. Only the malformed-DSL warning is at WARNING; this one
    is at DEBUG."""
    document = _make_document(suffix="pdf", parser_id="DeepDOC")
    user_pipeline = _make_user_pipeline()
    monkeypatch.setattr(
        pol_module.DocumentService, "get_by_id", MagicMock(return_value=(True, document))
    )
    monkeypatch.setattr(
        pol_module.UserCanvasService, "get_by_id", MagicMock(return_value=(True, user_pipeline))
    )

    dsl = json.dumps({"components": {"some-other-node": {"obj": {"component_name": "Begin"}}}, "path": []})

    with caplog.at_level(pol_module.logging.DEBUG, logger="root"):
        pol_module.PipelineOperationLogService.create(
            document_id="doc-1",
            pipeline_id="pipe-1",
            task_type=pol_module.PipelineTaskType.PARSE,
            task_id="task-1",
            referred_document_id="doc-1",
            dsl=dsl,
        )

    # DEBUG-level: must NOT appear in the warning channel.
    assert not any(
        rec.levelno >= pol_module.logging.WARNING
        and "Could not resolve pipeline parser from DSL" in rec.message
        for rec in caplog.records
    )
    # But the parser_id fallback still happened.
    log = captured_log["log"]
    assert log["parser_id"] == "DeepDOC"
