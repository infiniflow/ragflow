"""Real UI upload and real worker/index/query proof on the named disposable stack."""

import json
import os
from pathlib import Path
import re
import secrets
import time
from urllib.parse import urlparse
from urllib.request import Request, urlopen

from playwright.sync_api import expect

from test.playwright.helpers.datasets import ensure_parse_on, wait_for_success_dot
from test.playwright.helpers._next_apps_helpers import (
    _fill_and_save_create_modal,
    _open_create_from_list,
    _search_query_input,
    _select_first_dataset_and_save,
    _send_chat_and_wait_done,
)
from test.playwright.e2e.test_next_apps_search import _assert_search_result, _is_search_response
from test.playwright.conftest import _rsa_encrypt_password

BASE = os.environ.get("RAGFLOW_BASE_URL", "")


def api(path, method="GET", data=None):
    assert BASE == "http://127.0.0.1:19382", "Only the disposable T1 stack is accepted"
    with urlopen(
        Request(BASE + path, method=method, headers={"Authorization": os.environ["QA_LIVE_TOKEN"], "Content-Type": "application/json"}, data=json.dumps(data).encode() if data is not None else None),
        timeout=180,
    ) as response:
        payload = json.load(response)
    assert payload.get("code") == 0, f"{method} {path}: code={payload.get('code')}"
    return payload.get("data")


def test_real_upload_parse_search_chat_and_cleanup(playwright, tmp_path):
    assert os.environ.get("QA_DISPOSABLE_PROJECT", "").startswith("ragflow-t1-live-20260906-b-"), "Disposable live runner required"
    proof = {}
    with urlopen(
        Request(
            BASE + "/api/v1/auth/login",
            method="POST",
            headers={"Content-Type": "application/json"},
            data=json.dumps({"email": os.environ["SEEDED_USER_EMAIL"], "password": _rsa_encrypt_password(os.environ["SEEDED_USER_PASSWORD"])}).encode(),
        ),
        timeout=60,
    ) as logged_in:
        assert json.load(logged_in).get("code") == 0
        os.environ["QA_LIVE_TOKEN"] = logged_in.headers["Authorization"]
    suffix = secrets.token_hex(4)
    name = "regression-probe-" + suffix
    snippet = "RAGFlow synthetic proof: the violet observatory stores seven copper telescopes."
    file = tmp_path / (name + ".txt")
    file.write_text(snippet, encoding="utf-8")
    dataset = api(
        "/api/v1/datasets",
        "POST",
        {"name": name, "chunk_method": "naive", "parse_type": 1, "embedding_model": "bge-m3:latest@QA@Ollama", "parser_config": {"chunk_token_num": 256, "auto_keywords": 0, "auto_questions": 0}},
    )
    dataset_id = dataset.get("id") or dataset.get("kb_id")
    assert dataset_id
    docs_path = f"/api/v1/datasets/{dataset_id}/documents"
    chat_id = search_id = None
    browser = playwright.chromium.launch(headless=True)
    context = browser.new_context(viewport={"width": 1920, "height": 1080}, locale="en-US")
    page = context.new_page()
    page.set_default_timeout(30000)
    page.add_init_script("localStorage.setItem('lng', 'en')")
    errors = []
    page.on("pageerror", lambda error: errors.append(str(error)))
    try:
        page.goto(BASE + "/login")
        form = page.locator("form[data-active='true']")
        form.locator("[data-testid='auth-email']").fill(os.environ["SEEDED_USER_EMAIL"])
        form.locator("[data-testid='auth-password']").fill(os.environ["SEEDED_USER_PASSWORD"])
        form.locator("[data-testid='auth-submit']").click()
        expect(page).not_to_have_url(re.compile("/login"))
        os.environ["QA_LIVE_TOKEN"] = page.evaluate("localStorage.getItem('Authorization')")
        page.goto(BASE + f"/dataset/files/{dataset_id}")
        expect(page.get_by_test_id("dataset-documents")).to_be_visible()
        page.get_by_test_id("dataset-add-file").click()
        page.get_by_role("menuitem", name=re.compile("^upload file$", re.I)).click()
        modal = page.get_by_test_id("dataset-upload-modal")
        expect(modal).to_be_visible()
        ensure_parse_on(modal, expect)
        modal.locator("input[type=file]:not([webkitdirectory])").set_input_files(str(file))
        with page.expect_response(lambda response: response.request.method == "POST" and urlparse(response.url).path == docs_path) as uploaded:
            modal.get_by_role("button", name=re.compile("^save$", re.I)).click()
        assert uploaded.value.status == 200
        payload = uploaded.value.json()
        assert payload.get("code") == 0 and len(payload["data"]) == 1
        doc_id = payload["data"][0]["id"]
        expect(modal).not_to_be_visible()
        proof["uploaded_via_ui"] = True
        deadline = time.monotonic() + 240
        while True:
            state = api(docs_path + "?ids=" + doc_id)
            doc = next(row for row in state["docs"] if row["id"] == doc_id)
            if doc.get("run") == "DONE":
                break
            assert doc.get("run") != "FAIL", f"Parsing failed: {doc.get('progress_msg')}"
            assert time.monotonic() < deadline, f"Parser timeout: state={doc.get('run')} progress={doc.get('progress_msg')}"
            time.sleep(2)
        assert doc.get("chunk_count", doc.get("chunk_num", 0)) > 0
        wait_for_success_dot(page, expect, file.name, timeout_ms=30000)
        proof["parsed_by_real_worker"] = {"chunks": doc.get("chunk_count", doc.get("chunk_num")), "document_id": doc_id}
        found = api("/api/v1/datasets/search", "POST", {"dataset_ids": [dataset_id], "question": "violet observatory copper telescopes"})
        assert any(row.get("doc_id") == doc_id and snippet in row.get("content_with_weight", "") for row in found["chunks"])
        proof["real_retrieval_matches_uploaded_document"] = True
        page.goto(BASE + "/searches")
        expect(page.get_by_test_id("search-list")).to_be_visible()
        _open_create_from_list(page, "search-empty-create", "create-search")
        with page.expect_response(lambda r: r.request.method == "POST" and urlparse(r.url).path == "/api/v1/searches") as created:
            _fill_and_save_create_modal(page, name + "-search")
        assert created.value.status == 200 and created.value.json().get("code") == 0
        search_id = created.value.json()["data"]["search_id"]
        page.goto(BASE + "/search/" + search_id)
        expect(page.get_by_test_id("search-detail")).to_be_visible()
        _select_first_dataset_and_save(page, dataset_id=dataset_id, dataset_name=name)
        query = "violet observatory copper telescopes"
        search_input = _search_query_input(page)
        search_input.fill(query)
        with page.expect_response(lambda r: _is_search_response(r, query, dataset_id), timeout=60000) as searched:
            search_input.press("Enter")
        _assert_search_result(searched.value, doc_id, snippet)
        expect(page.get_by_test_id("search-detail").get_by_text(file.name, exact=True)).to_be_visible()
        proof["browser_search_returns_real_parsed_content"] = True
        page.goto(BASE + "/chats")
        expect(page.get_by_test_id("chats-list")).to_be_visible()
        _open_create_from_list(page, "chats-empty-create", "create-chat")
        with page.expect_response(lambda r: r.request.method == "POST" and urlparse(r.url).path == "/api/v1/chats") as created:
            _fill_and_save_create_modal(page, name + "-chat")
        assert created.value.status == 200 and created.value.json().get("code") == 0
        chat_id = created.value.json()["data"]["id"]
        page.goto(BASE + "/chat/" + chat_id)
        expect(page.get_by_test_id("chat-detail")).to_be_visible()
        _select_first_dataset_and_save(page, dataset_id=dataset_id, dataset_name=name)
        question = "According to the provided document, how many copper telescopes does the violet observatory store?"
        with page.expect_response(lambda r: r.request.method == "POST" and urlparse(r.url).path == "/api/v1/chat/completions", timeout=180000) as answered:
            _send_chat_and_wait_done(page, question, timeout_ms=180000)
        answers = [json.loads(line[5:]).get("data") for line in answered.value.text().splitlines() if line.startswith("data:")]
        text = " ".join(str(item.get("answer", "")) for item in answers if isinstance(item, dict))
        assert re.search(r"\b(seven|7)\b", text, re.I), "Real answer does not contain the fixture fact"
        proof["browser_chat_answers_fixture_fact"] = True
        assert not errors, f"Unhandled page exceptions: {len(errors)}"
        page.screenshot(path=str(Path(os.environ["QA_LIVE_EVIDENCE"]) / "synthetic-chat.png"), full_page=True)
    finally:
        context.close()
        browser.close()
        if search_id:
            api("/api/v1/searches/" + search_id, "DELETE")
        if chat_id:
            api("/api/v1/chats/" + chat_id, "DELETE")
        api("/api/v1/datasets", "DELETE", {"ids": [dataset_id]})
        remaining = api("/api/v1/datasets?page=1&page_size=100")
        assert isinstance(remaining, list) and len(remaining) < 100 and all(item["id"] != dataset_id for item in remaining), "Synthetic dataset remains after deletion"
        proof["dataset_cleanup_verified"] = True
        (Path(os.environ["QA_LIVE_EVIDENCE"]) / "synthetic-proof.json").write_text(json.dumps(proof, indent=2), encoding="utf-8")
