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
from contextlib import contextmanager
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

# --------------------------------------------------------------------------- #
# Module load: stub transitive imports the service module pulls in at top level
# --------------------------------------------------------------------------- #
#
# ``create()`` reaches DocumentService, UserCanvasService,
# KnowledgebaseService, and the peewee-backed DB. Stubbing those lets us
# exercise the real create() flow without standing up the full stack.
# Per-method behavior is patched with ``patch.object`` (and
# ``monkeypatch.setattr`` where pytest's patcher is more ergonomic) so
# the test stays close to ``unittest.mock`` conventions and avoids a
# custom spec-loader dance.


def _install_stubs(monkeypatch):
    # peewee.fn is referenced at module import time.
    peewee_mod = types.ModuleType("peewee")
    peewee_mod.fn = lambda *a, **kw: None
    monkeypatch.setitem(sys.modules, "peewee", peewee_mod)

    # The service's transitive import chain pulls in api.db, common.*,
    # and rag.flow.parser.parser — stub each module before exec so the
    # real service module loads against synthetic bindings.
    for mod in [
        "common",
        "common.constants",
        "common.misc_utils",
        "common.time_utils",
        "api",
        "api.db",
        "api.db.db_models",
        "api.db.services",
        "api.db.services.common_service",
        "api.db.services.document_service",
        "api.db.services.knowledgebase_service",
        "api.db.services.canvas_service",
        "api.db.services.task_service",
    ]:
        monkeypatch.setitem(sys.modules, mod, types.ModuleType(mod))

    @contextmanager
    def _noop_connection_context(*a, **kw):
        yield

    DB = MagicMock()
    DB.connection_context = _noop_connection_context
    sys.modules["api.db.db_models"].DB = DB
    sys.modules["api.db.db_models"].Document = object
    # PipelineOperationLog needs concrete int returns on the count() chain
    # so create()'s `if total > limit` branch is well-typed and the
    # cleanup path doesn't blow up with `MagicMock > int`.
    _PolModel = MagicMock()
    _PolModel.select.return_value.where.return_value.count.return_value = 0
    sys.modules["api.db.db_models"].PipelineOperationLog = _PolModel

    # Build class stubs with the methods the service calls, so
    # ``patch.object`` can target them without fighting ``object``'s
    # read-only namespace.
    def _class_stub(methods):
        return type(
            "_Stub",
            (object,),
            {m: classmethod(lambda cls, *a, **kw: None) for m in methods},
        )

    sys.modules["api.db.services.canvas_service"].UserCanvasService = _class_stub(["get_by_id"])
    sys.modules["api.db.services.document_service"].DocumentService = _class_stub(["get_by_id", "update_progress_immediately"])
    sys.modules["api.db.services.knowledgebase_service"].KnowledgebaseService = _class_stub(["get_by_id"])
    sys.modules["api.db.services.task_service"].TaskService = _class_stub(["get_by_id"])
    sys.modules["api.db.services.task_service"].GRAPH_RAPTOR_FAKE_DOC_ID = "fake"
    sys.modules["api.db.services.common_service"].CommonService = _class_stub(["save"])
    sys.modules["common.misc_utils"].get_uuid = lambda: "uuid"
    sys.modules["common.time_utils"].current_timestamp = lambda: "ts"
    sys.modules["common.time_utils"].datetime_format = lambda x: "dt"

    # rag.flow.parser.parser is required for the suffix map derivation.
    # Build a minimal ParserParam stub with the same shape the runtime
    # uses for dispatch (issue #18306 review: tests must validate the
    # helper, not a hand-maintained copy of the suffix list).
    class _ParserParam:
        def __init__(self):
            self.setups = {
                "pdf": {"suffix": ["pdf"]},
                "markdown": {"suffix": ["md", "markdown", "mdx"]},
                "image": {"suffix": ["jpg", "jpeg", "png", "gif"]},
                "text&code": {"suffix": ["txt", "py", "js"]},
            }

    parser_pkg = types.ModuleType("rag.flow.parser")
    parser_pkg.__path__ = ["/dev/null"]
    parser_sub = types.ModuleType("rag.flow.parser.parser")
    parser_sub.ParserParam = _ParserParam
    monkeypatch.setitem(sys.modules, "rag", types.ModuleType("rag"))
    monkeypatch.setitem(sys.modules, "rag.flow", types.ModuleType("rag.flow"))
    monkeypatch.setitem(sys.modules, "rag.flow.parser", parser_pkg)
    monkeypatch.setitem(sys.modules, "rag.flow.parser.parser", parser_sub)

    # Stub PipelineTaskType/TaskStatus so the create() branching works.
    import enum

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
        PipelineTaskType.PARSE,
        PipelineTaskType.DOWNLOAD,
        PipelineTaskType.RAPTOR,
        PipelineTaskType.GRAPH_RAG,
        PipelineTaskType.MINDMAP,
        PipelineTaskType.ARTIFACT,
        PipelineTaskType.SKILL,
    }

    # Load the module under test. Build the path from this test file's
    # location so the test works regardless of the developer's checkout
    # layout. Layout:
    #   <repo>/test/unit_test/api/db/services/<this_file>
    #         ^^^^^^^^ ^^^^^^                  ^^^^^^
    #         parents[5]  parents[4]           parents[0]
    service_path = Path(__file__).resolve().parents[5] / "api" / "db" / "services" / "pipeline_operation_log_service.py"
    spec = importlib.util.spec_from_file_location("_pol_test", str(service_path))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module._parser_setup_key_by_suffix.cache_clear()
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
        return MagicMock()

    monkeypatch.setattr(pol_module.PipelineOperationLogService, "save", classmethod(_fake_save))
    return captured


def test_create_prefers_pipeline_parser_for_parse_task(pol_module, captured_log, monkeypatch):
    """Issue #18306: a PARSE task with a valid pipeline DSL must persist
    the pipeline's Parser choice (e.g. "docling"), not the document's
    inherited KB default ("DeepDOC")."""
    document = _make_document(suffix="pdf", parser_id="DeepDOC")
    user_pipeline = _make_user_pipeline()

    with patch.object(pol_module.DocumentService, "get_by_id", return_value=(True, document)), patch.object(pol_module.UserCanvasService, "get_by_id", return_value=(True, user_pipeline)):
        pol_module.PipelineOperationLogService.create(
            document_id="doc-1",
            pipeline_id="pipe-1",
            task_type=pol_module.PipelineTaskType.PARSE,
            task_id="task-1",
            referred_document_id="doc-1",
            dsl=_parse_dsl("pdf", "docling"),
        )

    assert captured_log["log"]["parser_id"] == "docling", f"create() must prefer the pipeline's Parser choice over document.parser_id; got parser_id={captured_log['log']['parser_id']!r}"


def test_create_uses_document_parser_id_when_dsl_has_no_parser(pol_module, captured_log, monkeypatch):
    """If the pipeline DSL has no Parser component, create() must fall
    back to document.parser_id. The #18306 fix only changes the path
    where a Parser is configured."""
    document = _make_document(suffix="pdf", parser_id="DeepDOC")
    user_pipeline = _make_user_pipeline()

    # DSL with no Parser component at all.
    dsl = json.dumps({"components": {"some-other-node": {"obj": {"component_name": "Begin"}}}, "path": []})

    with patch.object(pol_module.DocumentService, "get_by_id", return_value=(True, document)), patch.object(pol_module.UserCanvasService, "get_by_id", return_value=(True, user_pipeline)):
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
    assert log["parser_id"] == document.parser_id


def test_create_persists_sanitized_dsl(pol_module, captured_log, monkeypatch):
    """create() must persist a sanitized copy of the DSL (without
    q_<dim>_vec keys) in the dsl column — verified via the captured
    log payload, not via a DB write."""
    document = _make_document(suffix="md", parser_id="DeepDOC")
    user_pipeline = _make_user_pipeline()

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

    with patch.object(pol_module.DocumentService, "get_by_id", return_value=(True, document)), patch.object(pol_module.UserCanvasService, "get_by_id", return_value=(True, user_pipeline)):
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
    chunk = persisted_dsl["components"]["Tokenizer:0"]["obj"]["params"]["outputs"]["chunks"]["value"][0]
    assert "q_1024_vec" not in chunk
    assert chunk == {"text": "x"}
    assert log["parser_id"] == "docling"


def test_create_accepts_dict_dsl(pol_module, captured_log, monkeypatch, caplog):
    """A pipeline DSL stored as a dict (peewee JSONField round-trip)
    must not crash create() and must still resolve the pipeline's
    Parser choice. Before the dict-input fix, ``json.loads(dict)``
    raised TypeError, the DSL was dropped, and the WARNING fired
    spuriously on every PARSE task."""
    document = _make_document(suffix="pdf", parser_id="DeepDOC")
    user_pipeline = _make_user_pipeline()

    dsl_dict = {
        "components": {
            "parser-node": {
                "obj": {
                    "component_name": "Parser",
                    "params": {"setups": {"pdf": {"parse_method": "docling"}}},
                },
            },
        },
        "path": [],
    }

    with (
        caplog.at_level(pol_module.logging.WARNING, logger="root"),
        patch.object(pol_module.DocumentService, "get_by_id", return_value=(True, document)),
        patch.object(pol_module.UserCanvasService, "get_by_id", return_value=(True, user_pipeline)),
    ):
        pol_module.PipelineOperationLogService.create(
            document_id="doc-1",
            pipeline_id="pipe-1",
            task_type=pol_module.PipelineTaskType.PARSE,
            task_id="task-1",
            referred_document_id="doc-1",
            dsl=dsl_dict,
        )

    log = captured_log["log"]
    assert log["parser_id"] == "docling", "dict DSL must still resolve the pipeline's parser"
    # The "Pipeline DSL is missing or malformed" WARNING must NOT fire
    # for a valid dict input.
    assert not any("Pipeline DSL is missing or malformed" in rec.message for rec in caplog.records)


def test_create_falls_back_to_empty_dsl_for_malformed_dsl(pol_module, captured_log, monkeypatch, caplog):
    """Malformed DSL must not crash create() — it falls back to {} for
    the dsl column, the WARNING fires (per CodeRabbit review), and the
    parser_id falls back to document.parser_id."""
    document = _make_document(suffix="pdf", parser_id="DeepDOC")
    user_pipeline = _make_user_pipeline()

    with (
        caplog.at_level(pol_module.logging.WARNING, logger="root"),
        patch.object(pol_module.DocumentService, "get_by_id", return_value=(True, document)),
        patch.object(pol_module.UserCanvasService, "get_by_id", return_value=(True, user_pipeline)),
    ):
        pol_module.PipelineOperationLogService.create(
            document_id="doc-1",
            pipeline_id="pipe-1",
            task_type=pol_module.PipelineTaskType.PARSE,
            task_id="task-1",
            referred_document_id="doc-1",
            dsl="not-json",
        )

    log = captured_log["log"]
    assert log["dsl"] == {}
    assert log["parser_id"] == "DeepDOC"
    assert any("Pipeline DSL is missing or malformed" in rec.message for rec in caplog.records)


def test_create_demotes_debug_log_when_dsl_has_no_parser(pol_module, captured_log, monkeypatch, caplog):
    """Per the CodeRabbit review follow-up: when the pipeline DSL is valid
    but has no Parser component (a common case for pipelines that don't
    need text extraction), the message must NOT spam the warning
    channel. Only the malformed-DSL warning is at WARNING; this one
    is at DEBUG."""
    document = _make_document(suffix="pdf", parser_id="DeepDOC")
    user_pipeline = _make_user_pipeline()

    dsl = json.dumps({"components": {"some-other-node": {"obj": {"component_name": "Begin"}}}, "path": []})

    with (
        caplog.at_level(pol_module.logging.DEBUG, logger="root"),
        patch.object(pol_module.DocumentService, "get_by_id", return_value=(True, document)),
        patch.object(pol_module.UserCanvasService, "get_by_id", return_value=(True, user_pipeline)),
    ):
        pol_module.PipelineOperationLogService.create(
            document_id="doc-1",
            pipeline_id="pipe-1",
            task_type=pol_module.PipelineTaskType.PARSE,
            task_id="task-1",
            referred_document_id="doc-1",
            dsl=dsl,
        )

    assert not any(rec.levelno >= pol_module.logging.WARNING and "Could not resolve pipeline parser from DSL" in rec.message for rec in caplog.records)
    log = captured_log["log"]
    assert log["parser_id"] == "DeepDOC"
