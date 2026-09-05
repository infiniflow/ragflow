"""Browser regression for centrally managed datasets.

The suite uses a real Chromium page and a deterministic API stub. It proves
that assigned datasets remain readable while every dataset mutation and the
configuration surface stay unavailable to regular users.
"""

import json
import re
from urllib.parse import parse_qs, urlparse

import pytest
from playwright.sync_api import expect


DATASET_ID = "managed-kb-1"
DOCUMENT_ID = "managed-doc-1"


def _envelope(data, *, code=0, message="", **extra):
    return {"code": code, "data": data, "message": message, **extra}


def _install_session(page, *, is_superuser=False):
    user_info = json.dumps(
        {
            "id": "dataset-user",
            "email": "dataset-user@example.test",
            "nickname": "Dataset User",
            "is_superuser": is_superuser,
        }
    )
    page.add_init_script(
        f"""
        (() => {{
          const userInfo = {user_info};
          localStorage.setItem('Authorization', 'Bearer dataset-browser-test');
          localStorage.setItem('token', 'dataset-browser-test');
          localStorage.setItem('userInfo', JSON.stringify(userInfo));
          localStorage.setItem('lng', 'ru');
        }})()
        """
    )


def _dataset():
    return {
        "id": DATASET_ID,
        "name": "Central Dataset",
        "nickname": "Central Admin",
        "description": "Read-only assigned dataset",
        "avatar": "",
        "chunk_count": 3,
        "chunk_method": "naive",
        "create_date": "2026-09-05",
        "create_time": 1_788_540_000,
        "created_by": "central-admin",
        "document_count": 1,
        "embedding_model": "embedding-central@default@OpenAI",
        "graphrag_task_finish_at": "",
        "graphrag_task_id": None,
        "language": "English",
        "mindmap_task_finish_at": None,
        "mindmap_task_id": None,
        "pagerank": 0,
        "parser_config": {
            "auto_keywords": 0,
            "auto_questions": 0,
            "children_delimiter": "\n",
            "chunk_token_num": 128,
            "delimiter": "\n",
            "graphrag": {"entity_types": [], "method": "light", "use_graphrag": False},
            "html4excel": False,
            "image_context_size": 0,
            "layout_recognize": "DeepDOC",
            "llm_id": "chat-primary@default@OpenAI",
            "parent_child": {"children_delimiter": "\n", "use_parent_child": False},
            "raptor": {
                "max_cluster": 64,
                "max_token": 256,
                "prompt": "",
                "random_seed": 0,
                "threshold": 0.1,
                "use_raptor": False,
            },
            "table_context_size": 0,
            "topn_tags": 3,
        },
        "permission": "me",
        "pipeline_id": "",
        "raptor_task_finish_at": "",
        "raptor_task_id": "",
        "similarity_threshold": 0.2,
        "size": 1024,
        "status": "1",
        "tenant_avatar": "",
        "tenant_embd_id": 0,
        "tenant_id": "central-admin",
        "token_num": 50,
        "update_date": "2026-09-05",
        "update_time": 1_788_540_100,
        "vector_similarity_weight": 0.3,
        "connectors": [],
    }


def _document():
    return {
        "id": DOCUMENT_ID,
        "dataset_id": DATASET_ID,
        "name": "managed-policy.pdf",
        "create_date": "2026-09-05",
        "create_time": 1_788_540_000,
        "created_by": "central-admin",
        "nickname": "Central Admin",
        "location": "managed-policy.pdf",
        "parser_config": {},
        "pipeline_id": "",
        "pipeline_name": "",
        "process_duration": 0,
        "progress": 1,
        "progress_msg": "",
        "run": "DONE",
        "size": 1024,
        "source_type": "local",
        "status": "1",
        "suffix": "pdf",
        "thumbnail": "",
        "token_num": 50,
        "type": "pdf",
        "update_date": "2026-09-05",
        "update_time": 1_788_540_100,
        "meta_fields": {},
        "chunk_method": "naive",
        "chunk_count": 3,
    }


class ManagedDatasetsApiStub:
    def __init__(self, *, is_superuser=False):
        self.is_superuser = is_superuser
        self.requests = []
        self.mutations = []

    def __call__(self, route):
        request = route.request
        parsed = urlparse(request.url)
        path = parsed.path.rstrip("/") or "/"
        query = parse_qs(parsed.query)
        self.requests.append((request.method, path, query))
        if request.method not in {"GET", "HEAD", "OPTIONS"}:
            self.mutations.append((request.method, path))

        if path == "/api/v1/system/config":
            route.fulfill(
                json=_envelope(
                    {
                        "registerEnabled": 0,
                        "disablePasswordLogin": False,
                        "visibleSections": ["home", "dataset", "chat"],
                    }
                )
            )
            return
        if path == "/api/v1/auth/login/channels":
            route.fulfill(json=_envelope([]))
            return
        if path == "/api/v1/system/version":
            route.fulfill(json=_envelope("dataset-browser-test"))
            return
        if path == "/api/v1/users/me":
            route.fulfill(
                json=_envelope(
                    {
                        "id": "dataset-user",
                        "email": "dataset-user@example.test",
                        "nickname": "Dataset User",
                        "language": "ru",
                        "avatar": None,
                        "is_superuser": self.is_superuser,
                    }
                )
            )
            return
        if path == "/api/v1/users/me/eva-credentials":
            route.fulfill(json=_envelope({"items": []}))
            return
        if path == "/api/v1/tenants":
            route.fulfill(json=_envelope([]))
            return
        if path == "/api/v1/datasets":
            route.fulfill(json=_envelope([_dataset()], total_datasets=1))
            return
        if path == f"/api/v1/datasets/{DATASET_ID}":
            route.fulfill(json=_envelope(_dataset()))
            return
        if path == f"/api/v1/datasets/{DATASET_ID}/graph":
            route.fulfill(json=_envelope({"graph": {}, "mind_map": {}}))
            return
        if path == f"/api/v1/datasets/{DATASET_ID}/documents":
            if query.get("type") == ["filter"]:
                route.fulfill(json=_envelope({"filter": {"run_status": {}, "suffix": {}, "metadata": {}}}))
            else:
                route.fulfill(json=_envelope({"docs": [_document()], "total": 1}))
            return

        route.fulfill(json=_envelope({}))


def _open(page, base_url, stub, path):
    _install_session(page, is_superuser=stub.is_superuser)
    page.route("**/api/v1/**", stub)
    page.goto(f"{base_url.rstrip('/')}{path}")


@pytest.mark.p1
@pytest.mark.auth
def test_regular_user_dataset_catalog_is_read_only_even_with_create_query(
    page,
    base_url,
):
    stub = ManagedDatasetsApiStub()
    _open(page, base_url, stub, "/datasets?isCreate=true")

    expect(page.get_by_test_id("dataset-card-managed-kb-1")).to_be_visible()
    expect(page.get_by_text("Central Dataset", exact=True)).to_be_visible()
    expect(page.get_by_test_id("datasets-create")).to_have_count(0)
    expect(page.get_by_test_id("dataset-actions-managed-kb-1")).to_have_count(0)
    assert stub.mutations == []


@pytest.mark.p1
@pytest.mark.auth
def test_regular_user_dataset_documents_are_visible_but_not_mutable(page, base_url):
    stub = ManagedDatasetsApiStub()
    _open(page, base_url, stub, f"/dataset/files/{DATASET_ID}")

    expect(page.get_by_test_id("dataset-documents")).to_be_visible()
    expect(page.get_by_test_id("document-row")).to_have_attribute("data-doc-name", "managed-policy.pdf")
    expect(page.get_by_test_id("dataset-nav-configuration")).to_have_count(0)
    expect(page.get_by_test_id("dataset-add-file")).to_have_count(0)
    expect(page.get_by_role("checkbox")).to_have_count(0)
    for switch in page.get_by_role("switch").all():
        expect(switch).to_be_disabled()
    assert stub.mutations == []


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize(
    "path",
    [
        f"/dataset/configuration/{DATASET_ID}",
        f"/dataset/configuration/{DATASET_ID}/",
    ],
)
def test_regular_user_direct_dataset_configuration_redirects_to_files(
    page,
    base_url,
    path,
):
    stub = ManagedDatasetsApiStub()
    _open(page, base_url, stub, path)

    expect(page).to_have_url(
        re.compile(rf"/dataset/files/{DATASET_ID}/?$"),
        timeout=15000,
    )
    expect(page.get_by_test_id("dataset-documents")).to_be_visible()
    assert stub.mutations == []


@pytest.mark.p1
@pytest.mark.auth
def test_superuser_keeps_dataset_management_controls(page, base_url):
    stub = ManagedDatasetsApiStub(is_superuser=True)
    _open(page, base_url, stub, "/datasets")

    expect(page.get_by_test_id("datasets-create")).to_be_visible()
    expect(page.get_by_test_id("dataset-actions-managed-kb-1")).to_be_attached()

    page.goto(f"{base_url.rstrip('/')}/dataset/files/{DATASET_ID}")
    expect(page.get_by_test_id("dataset-add-file")).to_be_visible()
    expect(page.get_by_test_id("dataset-nav-configuration")).to_be_visible()
    assert stub.mutations == []
