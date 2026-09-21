import ast
import asyncio
from pathlib import Path

import pytest


def _load_route(top_n):
    path = Path("api/apps/restful_apis/dataset_api.py")
    tree = ast.parse(path.read_text())
    route = next(node for node in tree.body if isinstance(node, ast.AsyncFunctionDef) and node.name == "get_wiki_graph")
    route.decorator_list = []
    module = ast.Module(body=[route], type_ignores=[])
    ast.fix_missing_locations(module)

    class Args:
        def get(self, key, default=None):
            return {"top_n": top_n}.get(key, default)

    class Service:
        async def get_wiki_graph(self, *_args, **_kwargs):
            raise AssertionError("invalid top_n must not reach the service")

    def result(**kwargs):
        return kwargs

    namespace = {
        "request": type("Request", (), {"args": Args()})(),
        "dataset_api_service": Service(),
        "get_result": result,
        "get_error_data_result": result,
        "RetCode": type("RetCode", (), {"SERVER_ERROR": 500}),
        "logging": __import__("logging"),
    }
    exec(compile(module, str(path), "exec"), namespace)
    return namespace["get_wiki_graph"]


@pytest.mark.p2
@pytest.mark.parametrize("value", ["not-a-number", "1.5"])
def test_get_wiki_graph_rejects_non_integer_top_n(value):
    response = asyncio.run(_load_route(value)("tenant", "dataset"))
    assert response["message"] == "top_n must be an integer"
