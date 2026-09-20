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

import importlib.util
import sys
import types
from pathlib import Path

import pytest


def _load_service(monkeypatch):
    class FakeDB:
        @staticmethod
        def connection_context():
            return lambda function: function

    peewee = types.ModuleType("peewee")
    peewee.Tuple = lambda *args: None
    peewee.fn = types.SimpleNamespace()
    monkeypatch.setitem(sys.modules, "peewee", peewee)

    module_names = [
        "api",
        "api.db",
        "api.db.db_models",
        "api.db.services",
        "api.db.services.canvas_service",
        "api.db.services.common_service",
        "api.db.services.document_service",
        "api.db.services.knowledgebase_service",
        "api.db.services.pipeline_dsl_version_service",
        "api.db.services.task_service",
        "common",
        "common.constants",
        "common.misc_utils",
        "common.time_utils",
    ]
    for name in module_names:
        monkeypatch.setitem(sys.modules, name, types.ModuleType(name))

    db_models = sys.modules["api.db.db_models"]
    db_models.DB = FakeDB()
    db_models.Document = object
    db_models.PipelineDSLVersion = object
    db_models.PipelineOperationLog = object
    sys.modules["api.db"].VALID_PIPELINE_TASK_TYPES = set()
    sys.modules["api.db.services.canvas_service"].UserCanvasService = object
    sys.modules["api.db.services.common_service"].CommonService = object
    sys.modules["api.db.services.document_service"].DocumentService = object
    sys.modules["api.db.services.knowledgebase_service"].KnowledgebaseService = object
    sys.modules["api.db.services.pipeline_dsl_version_service"].PipelineDSLVersionService = object
    sys.modules["api.db.services.task_service"].GRAPH_RAPTOR_FAKE_DOC_ID = "fake"
    sys.modules["api.db.services.task_service"].TaskService = object

    class PipelineTaskType:
        GRAPH_RAG = "graph"
        RAPTOR = "raptor"
        MINDMAP = "mindmap"
        ARTIFACT = "artifact"
        SKILL = "skill"
        STRUCTURE_GRAPH = "structure_graph"
        STRUCTURE_MINDMAP = "structure_mindmap"
        TIMELINE = "timeline"
        SESSION_GRAPH = "session_graph"
        SESSION_ESSENCE = "session_essence"
        STRUCTURE = "structure"

    class TaskStatus:
        CANCEL = types.SimpleNamespace(value="cancel")
        DONE = types.SimpleNamespace(value="done")
        FAIL = types.SimpleNamespace(value="fail")

    sys.modules["common.constants"].PipelineTaskType = PipelineTaskType
    sys.modules["common.constants"].TaskStatus = TaskStatus
    sys.modules["common.misc_utils"].get_uuid = lambda: "uuid"
    sys.modules["common.time_utils"].current_timestamp = lambda: 1
    sys.modules["common.time_utils"].datetime_format = lambda value: value

    service_path = Path(__file__).resolve().parents[5] / "api" / "db" / "services" / "pipeline_operation_log_service.py"
    spec = importlib.util.spec_from_file_location("_pipeline_operation_log_service_test", service_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


@pytest.fixture
def pol_module(monkeypatch):
    return _load_service(monkeypatch)


@pytest.mark.parametrize(
    ("pipeline_id", "parser_id", "expected"),
    [
        ("pipeline-1", "general", "pipeline-1"),
        (None, "general", "builtin:general"),
        (None, "", None),
    ],
)
def test_pipeline_dsl_id(pol_module, pipeline_id, parser_id, expected):
    assert pol_module._pipeline_dsl_id(pipeline_id, parser_id) == expected


def test_sanitize_pipeline_dsl_removes_runtime_state_without_mutating_input(pol_module):
    dsl = {
        "task_id": "run-1",
        "components": {
            "Tokenizer:0": {
                "obj": {
                    "params": {
                        "mode": "static",
                        "outputs": {
                            "chunks": {
                                "value": [
                                    {
                                        "text": "content",
                                        "q_1024_vec": [0.1, 0.2],
                                        "metadata": {"q_3_vec": [1, 2, 3], "source": "test"},
                                    }
                                ]
                            },
                            "embedding_token_consumption": {"value": 12},
                        },
                    }
                }
            }
        },
        "path": ["Tokenizer:0"],
    }

    result = pol_module._sanitize_pipeline_dsl(dsl)

    params = result["components"]["Tokenizer:0"]["obj"]["params"]
    assert "task_id" not in result
    assert "outputs" not in params
    assert params["mode"] == "static"
    assert result["path"] == ["Tokenizer:0"]
    assert dsl["task_id"] == "run-1"
    assert "outputs" in dsl["components"]["Tokenizer:0"]["obj"]["params"]


def test_resolve_dsl_references_preserves_legacy_embedded_dsl(monkeypatch, pol_module):
    service = pol_module.PipelineOperationLogService
    monkeypatch.setattr(
        service,
        "_load_dsl_versions",
        classmethod(lambda cls, references: {}),
    )
    logs = [{"id": "legacy", "dsl_id": None, "dsl_version": None, "dsl": {"legacy": True}}]

    resolved = service._resolve_dsl_references(logs)

    assert resolved == [{"id": "legacy", "dsl": {"legacy": True}}]


def test_resolve_dsl_references_uses_exact_stored_versions(monkeypatch, pol_module):
    service = pol_module.PipelineOperationLogService
    versions = {
        ("pipeline-1", 1): {"revision": 1},
        ("pipeline-1", 2): {"revision": 2},
    }
    seen = []

    def load_versions(cls, references):
        seen.append(references)
        return {reference: versions[reference] for reference in references}

    monkeypatch.setattr(
        service,
        "_load_dsl_versions",
        classmethod(load_versions),
    )
    logs = [
        {
            "id": "log-1",
            "dsl_id": "pipeline-1",
            "dsl_version": 1,
            "dsl": {"stale": True},
        },
        {
            "id": "log-2",
            "dsl_id": "pipeline-1",
            "dsl_version": 2,
            "dsl": {},
        },
    ]

    resolved = service._resolve_dsl_references(logs)

    assert seen == [{("pipeline-1", 1), ("pipeline-1", 2)}]
    assert resolved == [
        {"id": "log-1", "dsl": {"revision": 1}},
        {"id": "log-2", "dsl": {"revision": 2}},
    ]


def test_resolve_dsl_references_isolates_incomplete_reference(pol_module):
    service = pol_module.PipelineOperationLogService
    logs = [
        {"id": "legacy", "dsl_id": None, "dsl_version": None, "dsl": {"legacy": True}},
        {"id": "broken", "dsl_id": "pipeline-1", "dsl_version": None, "dsl": {"stale": True}},
    ]

    resolved = service._resolve_dsl_references(logs)

    assert resolved[0] == {"id": "legacy", "dsl": {"legacy": True}}
    assert resolved[1]["dsl"] is None
    assert "incomplete DSL reference" in resolved[1]["dsl_resolution_error"]

    with pytest.raises(RuntimeError, match="incomplete DSL reference"):
        service._resolve_dsl_references(
            [{"id": "broken", "dsl_id": "pipeline-1", "dsl_version": None, "dsl": {}}],
            strict=True,
        )


def test_resolve_dsl_references_isolates_missing_version(monkeypatch, pol_module):
    service = pol_module.PipelineOperationLogService
    monkeypatch.setattr(
        service,
        "_load_dsl_versions",
        classmethod(lambda cls, references: {}),
    )
    logs = [{"id": "broken", "dsl_id": "pipeline-1", "dsl_version": 3, "dsl": {"stale": True}}]

    resolved = service._resolve_dsl_references(logs)

    assert resolved[0]["dsl"] is None
    assert "pipeline-1" in resolved[0]["dsl_resolution_error"]
    assert "@3" in resolved[0]["dsl_resolution_error"]

    with pytest.raises(RuntimeError, match="pipeline-1.*@3.*not found"):
        service._resolve_dsl_references(
            [{"id": "broken", "dsl_id": "pipeline-1", "dsl_version": 3, "dsl": {}}],
            strict=True,
        )
