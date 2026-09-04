import ast
import logging
from pathlib import Path
from types import SimpleNamespace

import pytest


@pytest.fixture
def record_terminal_pipeline_log():
    repo_root = Path(__file__).resolve().parents[4]
    source = (repo_root / "rag" / "svr" / "task_executor.py").read_text(encoding="utf-8")
    tree = ast.parse(source)
    helper = next(
        node
        for node in tree.body
        if isinstance(node, ast.FunctionDef) and node.name == "_record_terminal_pipeline_log_best_effort"
    )
    namespace = {
        "PipelineOperationLogService": SimpleNamespace(create=lambda **_kwargs: "saved"),
        "PipelineTaskType": SimpleNamespace(PARSE="parse"),
        "get_recording_context": lambda: SimpleNamespace(save_func_return_value=lambda *_args: None),
        "logging": logging,
    }
    exec(compile(ast.Module(body=[helper], type_ignores=[]), "task_executor.py", "exec"), namespace)
    return namespace["_record_terminal_pipeline_log_best_effort"], namespace


def test_terminal_operation_log_failure_is_best_effort(record_terminal_pipeline_log):
    record_log, namespace = record_terminal_pipeline_log
    saved = []
    logged = []
    namespace["get_recording_context"] = lambda: SimpleNamespace(save_func_return_value=lambda *args: saved.append(args))
    namespace["logging"] = SimpleNamespace(exception=lambda message: logged.append(message))
    namespace["PipelineOperationLogService"] = SimpleNamespace(create=lambda **_kwargs: (_ for _ in ()).throw(RuntimeError("db unavailable")))

    record_log("doc-1", "pipeline-1", SimpleNamespace())

    assert saved == []
    assert logged == ["Failed to persist terminal dataflow operation log"]


def test_terminal_operation_log_success_is_recorded(record_terminal_pipeline_log):
    record_log, namespace = record_terminal_pipeline_log
    saved = []
    namespace["get_recording_context"] = lambda: SimpleNamespace(save_func_return_value=lambda *args: saved.append(args))
    namespace["PipelineOperationLogService"] = SimpleNamespace(create=lambda **_kwargs: "created")

    record_log("doc-1", "pipeline-1", SimpleNamespace())

    assert saved == [("PipelineOperationLogService.create", "created")]
