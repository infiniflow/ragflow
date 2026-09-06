"""Check tag API contracts using the original Python functions and fake I/O.

Run from the repository root: python internal/service/dataset/testdata/python_tags_contract.py
This checks source behavior without importing the full Python server or services.
"""

import ast
import asyncio
import json
import sys
from pathlib import Path
from types import ModuleType, SimpleNamespace


def load_functions(path, names, namespace):
    tree = ast.parse(Path(path).read_text(encoding="utf-8"), filename=path)
    selected = [node for node in tree.body if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)) and node.name in names]
    assert {node.name for node in selected} == set(names)
    for node in selected:
        node.decorator_list = []
    # Execute selected repository functions with fake I/O instead of importing the full server.
    exec(compile(ast.Module(body=selected, type_ignores=[]), path, "exec"), namespace)  # noqa: S102


def main():
    kb_id = "123e4567e89b12d3a456426614174000"
    kb = SimpleNamespace(tenant_id="user-1", doc_num=0)
    tags = [("finance", 2), ("urgent", 1)]
    updates = []
    queried = []

    def all_tags(tenant_id, ids):
        queried.append((tenant_id, ids))
        return tags

    search = ModuleType("rag.nlp.search")
    search.index_name = lambda tenant_id: "ragflow_" + tenant_id
    nlp = ModuleType("rag.nlp")
    nlp.search = search
    sys.modules["rag"] = ModuleType("rag")
    sys.modules["rag.nlp"] = nlp
    sys.modules["rag.nlp.search"] = search
    namespace = {
        "KnowledgebaseService": SimpleNamespace(accessible=lambda *_: True, get_by_id=lambda _: (True, kb)),
        "UserTenantService": SimpleNamespace(get_tenants_by_user_id=lambda _: [{"tenant_id": "user-1"}]),
        "settings": SimpleNamespace(
            retriever=SimpleNamespace(all_tags=all_tags),
            docStoreConn=SimpleNamespace(update=lambda *args: updates.append(args) or True),
        ),
    }
    load_functions("api/apps/services/dataset_api_service.py", ["list_tags", "aggregate_tags", "rename_tag"], namespace)
    ok, listed = namespace["list_tags"](kb_id, "user-1")
    assert ok and json.loads(json.dumps(listed)) == [["finance", 2], ["urgent", 1]]
    queried.clear()
    ok, aggregated = namespace["aggregate_tags"]([kb_id], "user-1")
    assert ok and queried == [("user-1", [kb_id])]
    assert aggregated == [{"value": "finance", "count": 2}, {"value": "urgent", "count": 1}]
    ok, renamed = namespace["rename_tag"](kb_id, "user-1", "old-tag ", " new-tag ")
    assert ok and renamed == {"from": "old-tag ", "to": " new-tag "}
    condition, change, index, dataset = updates[0]
    assert condition == {"tag_kwd": "old-tag ", "kb_id": [kb_id]}
    assert change == {"remove": {"tag_kwd": "old-tag"}, "add": {"tag_kwd": " new-tag "}}
    assert index == "ragflow_user-1" and dataset == kb_id

    namespace["KnowledgebaseService"].accessible = lambda dataset_id, _: dataset_id == kb_id
    assert namespace["aggregate_tags"]([" "], "user-1") == (False, "No authorization for dataset ' '")

    ret_code = SimpleNamespace(SUCCESS=0, DATA_ERROR=102, ARGUMENT_ERROR=101)
    response = {"RetCode": ret_code, "_safe_jsonify": lambda value: value}
    load_functions("api/utils/api_utils.py", ["get_result", "get_error_data_result", "get_error_argument_result"], response)
    load_functions("api/utils/pagination_utils.py", ["validate_rest_api_ids"], response)
    response["REST_API_MAX_IDS"] = 100
    response["validate_rest_api_ids"]([kb_id] * 100, "dataset_ids")
    try:
        response["validate_rest_api_ids"]([kb_id] * 101, "dataset_ids")
    except ValueError as exc:
        assert str(exc) == "dataset_ids must contain at most 100 IDs"
    else:
        raise AssertionError("101 IDs must fail")
    assert response["get_result"](data=listed) == {"code": 0, "data": listed}
    assert response["get_error_data_result"](message="Internal server error") == {"code": 102, "message": "Internal server error"}

    def failing_service(*_):
        raise RuntimeError("private backend address")

    routes = dict(response)
    routes.update(
        {
            "dataset_api_service": SimpleNamespace(list_tags=failing_service, aggregate_tags=failing_service, rename_tag=failing_service),
            "logging": SimpleNamespace(exception=lambda *_: None),
            "request": SimpleNamespace(args={"dataset_ids": kb_id}),
        }
    )
    load_functions("api/apps/restful_apis/dataset_api.py", ["list_tags", "aggregate_tags", "rename_tag"], routes)
    expected_error = {"code": 102, "message": "Internal server error"}
    assert routes["list_tags"]("user-1", kb_id) == expected_error
    assert routes["aggregate_tags"]("user-1") == expected_error

    async def get_json():
        return {"from_tag": "old", "to_tag": "new"}

    routes["request"].get_json = get_json
    assert asyncio.run(routes["rename_tag"]("user-1", kb_id)) == expected_error
    print(json.dumps({"list_tags": listed, "aggregate_tags_zero_doc_num": aggregated, "rename_tag": renamed, "error": expected_error, "max_ids": 100}, indent=2))


if __name__ == "__main__":
    main()
