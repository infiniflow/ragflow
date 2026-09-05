import pytest
from playwright.sync_api import expect
from urllib.parse import urljoin, urlparse

from test.playwright.helpers._auth_helpers import ensure_authed
from test.playwright.helpers.flow_steps import require
from test.playwright.helpers._next_apps_helpers import (
    RESULT_TIMEOUT_MS,
    _fill_and_save_create_modal,
    _goto_home,
    _nav_click,
    _open_create_from_list,
    _search_query_input,
    _select_first_dataset_and_save,
    _unique_name,
    _wait_for_url_or_testid,
)


def _assert_search_result(response, document_id: str, snippet: str) -> None:
    assert response.status == 200, f"Retrieval HTTP {response.status}"
    payload = response.json()
    assert payload.get("code") == 0, f"Retrieval failed: {payload}"
    data = payload.get("data") or {}
    assert data.get("total", 0) > 0, "Seeded query returned no results"
    assert any(
        chunk.get("doc_id") == document_id and snippet in chunk.get("content_with_weight", "")
        for chunk in data.get("chunks", [])
    ), "Retrieval did not return the seeded document and content"


def _is_search_response(response, query: str, dataset_id: str) -> bool:
    request = response.request
    if request.method != "POST" or urlparse(response.url).path != "/api/v1/datasets/search":
        return False
    payload = request.post_data_json
    return isinstance(payload, dict) and payload.get("question") == query and dataset_id in payload.get("dataset_ids", [])


def _api_data(response):
    assert response.ok, f"Search fixture HTTP {response.status}"
    payload = response.json()
    assert payload.get("code") == 0, f"Search fixture failed: {payload}"
    return payload.get("data")


def _seed_search_document(page, base_url, state):
    token = page.evaluate("localStorage.getItem('Authorization') || localStorage.getItem('Token')")
    assert token, "Missing search fixture authorization"
    headers = {"Authorization": token}
    state["seed_headers"] = headers
    dataset = state["dataset"]
    url = urljoin(base_url, f"/api/v1/datasets/{dataset['kb_id']}/documents")
    name = _unique_name("retrieval-proof") + ".txt"
    document = _api_data(page.request.post(url + "?type=empty", headers=headers, data={"name": name}))
    state["seed_document_id"] = document["id"]
    state["seed_document_name"] = name
    snippet = "RAGFlow retrieval proof: the violet observatory stores seven copper telescopes."
    state["seed_snippet"] = snippet
    _api_data(page.request.post(
        url + f"/{document['id']}/chunks", headers=headers,
        data={"content": snippet, "important_keywords": ["violet observatory", "copper telescopes"]},
    ))


def step_01_ensure_authed(
    flow_page,
    flow_state,
    base_url,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
):
    with step("ensure logged in"):
        ensure_authed(
            flow_page,
            login_url,
            active_auth_context,
            auth_click,
            seeded_user_credentials=seeded_user_credentials,
        )
    flow_state["logged_in"] = True
    _seed_search_document(flow_page, base_url, flow_state)
    snap("authed")


def step_02_open_search_list(
    flow_page,
    flow_state,
    base_url,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
):
    require(flow_state, "logged_in")
    page = flow_page
    with step("open search list"):
        _goto_home(page, base_url)
        _nav_click(page, "nav-search")
        expect(page.locator("[data-testid='search-list']")).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    snap("search_list_open")


def step_03_open_create_modal(
    flow_page,
    flow_state,
    base_url,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
):
    require(flow_state, "logged_in")
    page = flow_page
    with step("open create search modal"):
        _open_create_from_list(page, "search-empty-create", "create-search")
    flow_state["search_modal_open"] = True
    snap("search_create_modal")


def step_04_create_search(
    flow_page,
    flow_state,
    base_url,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
):
    require(flow_state, "search_modal_open")
    page = flow_page
    search_name = _unique_name("qa-search")
    flow_state["search_name"] = search_name
    with step("create search app"):
        _fill_and_save_create_modal(page, search_name)
        _wait_for_url_or_testid(page, r"/next-search/", "search-detail")
        expect(page.locator("[data-testid='search-detail']")).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    flow_state["search_created"] = True
    snap("search_created")


def step_05_select_dataset(
    flow_page,
    flow_state,
    base_url,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
):
    require(flow_state, "search_created")
    page = flow_page
    with step("select dataset"):
        search_input = _search_query_input(page)
        _select_first_dataset_and_save(
            page,
            timeout_ms=RESULT_TIMEOUT_MS,
            post_save_ready_locator=search_input,
            dataset_id=flow_state["dataset"]["kb_id"],
            dataset_name=flow_state["dataset"]["kb_name"],
        )
        flow_state["search_input_ready"] = True
    snap("search_dataset_saved")


def step_06_run_query(
    flow_page,
    flow_state,
    base_url,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
):
    require(flow_state, "search_input_ready")
    page = flow_page
    search_input = _search_query_input(page)
    with step("run search query"):
        expect(search_input).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        query = "violet observatory copper telescopes"
        search_input.fill(query)
        with page.expect_response(
            lambda response: _is_search_response(response, query, flow_state["dataset"]["kb_id"]),
            timeout=60000,
        ) as result:
            search_input.press("Enter")
        _assert_search_result(result.value, flow_state["seed_document_id"], flow_state["seed_snippet"])
        root = page.get_by_test_id("search-detail")
        expect(root.get_by_text(flow_state["seed_document_name"], exact=True)).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        expect(root.get_by_text(flow_state["seed_snippet"], exact=True)).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    snap("seeded_search_result")


STEPS = [
    ("01_ensure_authed", step_01_ensure_authed),
    ("02_open_search_list", step_02_open_search_list),
    ("03_open_create_modal", step_03_open_create_modal),
    ("04_create_search", step_04_create_search),
    ("05_select_dataset", step_05_select_dataset),
    ("06_run_query", step_06_run_query),
]


@pytest.mark.p1
@pytest.mark.auth
def test_search_create_select_dataset_and_results_appear_flow(
    flow_page,
    flow_state,
    base_url,
    login_url,
    ensure_dataset_ready,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
):
    flow_state["dataset"] = ensure_dataset_ready
    try:
        for _, step_fn in STEPS:
            step_fn(
                flow_page, flow_state, base_url, login_url, active_auth_context,
                step, snap, auth_click, seeded_user_credentials,
            )
    finally:
        if flow_state.get("seed_document_id"):
            documents_url = urljoin(base_url, f"/api/v1/datasets/{ensure_dataset_ready['kb_id']}/documents")
            _api_data(flow_page.request.delete(
                documents_url,
                headers=flow_state["seed_headers"], data={"ids": [flow_state["seed_document_id"]]},
            ))
            remaining = _api_data(flow_page.request.get(
                documents_url, headers=flow_state["seed_headers"],
                params={"ids": flow_state["seed_document_id"]},
            ))
            assert remaining["docs"] == [], "Seeded search document was not removed"
