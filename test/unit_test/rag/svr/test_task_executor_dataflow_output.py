import ast
from pathlib import Path

import pytest


@pytest.fixture
def normalize_dataflow_output():
    repo_root = Path(__file__).resolve().parents[4]
    source = (repo_root / "rag" / "svr" / "task_executor.py").read_text(encoding="utf-8")
    tree = ast.parse(source)
    helper = next(node for node in tree.body if isinstance(node, ast.FunctionDef) and node.name == "_normalize_dataflow_output")
    namespace = {"copy": __import__("copy")}
    exec(compile(ast.Module(body=[helper], type_ignores=[]), "task_executor.py", "exec"), namespace)
    return namespace["_normalize_dataflow_output"]


@pytest.mark.parametrize("output_type", ["markdown", "text", "html"])
def test_scalar_dataflow_output_keeps_text_as_string(normalize_dataflow_output, output_type):
    chunks, normalized_type = normalize_dataflow_output({output_type: "hello"})

    assert normalized_type == output_type
    assert chunks == [{"text": "hello"}]
    assert isinstance(chunks[0]["text"], str)


@pytest.mark.parametrize("output_type", ["markdown", "text", "html"])
def test_empty_scalar_dataflow_output_has_no_chunks(normalize_dataflow_output, output_type):
    assert normalize_dataflow_output({output_type: ""}) == ([], output_type)
