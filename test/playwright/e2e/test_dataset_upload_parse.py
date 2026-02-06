import json
import re
import time
from pathlib import Path
from urllib.parse import urljoin

import pytest
from playwright.sync_api import expect

from test.playwright.helpers.flow_steps import flow_params, require
from test.playwright.helpers.auth_selectors import EMAIL_INPUT, PASSWORD_INPUT, SUBMIT_BUTTON
from test.playwright.helpers.auth_waits import wait_for_login_complete
from test.playwright.helpers.response_capture import capture_response
from test.playwright.helpers.datasets import (
    delete_uploaded_file,
    ensure_parse_on,
    ensure_upload_modal_open,
    open_create_dataset_modal,
    select_chunking_method_general,
    upload_file,
    wait_for_dataset_detail,
    wait_for_dataset_detail_ready,
    wait_for_success_dot,
)

RESULT_TIMEOUT_MS = 15000


def step_01_login(
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
    email, password = seeded_user_credentials

    repo_root = Path(__file__).resolve().parents[3]
    file_paths = [
        repo_root / "test/benchmark/test_docs/Doc1.pdf",
        repo_root / "test/benchmark/test_docs/Doc2.pdf",
        repo_root / "test/benchmark/test_docs/Doc3.pdf",
    ]
    for path in file_paths:
        if not path.is_file():
            pytest.fail(f"Missing upload fixture: {path}")
    flow_state["file_paths"] = [str(path) for path in file_paths]
    flow_state["filenames"] = [path.name for path in file_paths]

    with step("open login page"):
        flow_page.goto(login_url, wait_until="domcontentloaded")

    form, _ = active_auth_context()
    email_input = form.locator(EMAIL_INPUT)
    password_input = form.locator(PASSWORD_INPUT)
    with step("fill credentials"):
        expect(email_input).to_have_count(1)
        expect(password_input).to_have_count(1)
        email_input.fill(email)
        password_input.fill(password)
        password_input.blur()

    with step("submit login"):
        submit_button = form.locator(SUBMIT_BUTTON)
        expect(submit_button).to_have_count(1)
        auth_click(submit_button, "submit_login")

    with step("wait for login"):
        wait_for_login_complete(flow_page, timeout_ms=RESULT_TIMEOUT_MS)
    flow_state["logged_in"] = True
    snap("login_complete")


def step_02_open_datasets(
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
    with step("open datasets"):
        page.goto(urljoin(base_url.rstrip("/") + "/", "/"), wait_until="domcontentloaded")
        nav_button = page.locator("button", has_text=re.compile(r"^Dataset$", re.I))
        if nav_button.count() > 0:
            nav_button.first.click()
        else:
            page.goto(
                urljoin(base_url.rstrip("/") + "/", "/datasets"),
                wait_until="domcontentloaded",
            )
    snap("datasets_open")


def step_03_create_dataset(
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
    with step("open create dataset modal"):
        modal = open_create_dataset_modal(page, expect, RESULT_TIMEOUT_MS)
    snap("dataset_modal_open")

    dataset_name = f"qa-dataset-{int(time.time() * 1000)}"
    with step("fill dataset form"):
        name_input = modal.locator("input[placeholder='Please input name.']").first
        expect(name_input).to_be_visible()
        name_input.fill(dataset_name)

        try:
            select_chunking_method_general(page, expect, modal, RESULT_TIMEOUT_MS)
        except Exception:
            snap("failure_dataset_create")
            raise

        save_button = None
        if hasattr(modal, "get_by_role"):
            save_button = modal.get_by_role("button", name=re.compile(r"^save$", re.I))
        if save_button is None or save_button.count() == 0:
            save_button = modal.locator("button", has_text=re.compile(r"^save$", re.I)).first
        expect(save_button).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        save_button.click()
        expect(modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
        wait_for_dataset_detail(page, timeout_ms=RESULT_TIMEOUT_MS)
        wait_for_dataset_detail_ready(page, expect, timeout_ms=RESULT_TIMEOUT_MS)
    flow_state["dataset_name"] = dataset_name
    snap("dataset_created")
    snap("dataset_detail_ready")


def step_04_upload_files(
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
    require(flow_state, "dataset_name", "file_paths")
    page = flow_page
    file_paths = [Path(path) for path in flow_state["file_paths"]]
    filenames = flow_state.get("filenames") or [path.name for path in file_paths]
    flow_state["filenames"] = filenames

    for idx, file_path in enumerate(file_paths):
        filename = file_path.name
        with step(f"open upload modal for {filename}"):
            upload_modal = ensure_upload_modal_open(
                page, expect, auth_click, timeout_ms=RESULT_TIMEOUT_MS
            )
        if idx == 0:
            snap("upload_modal_open")

        with step(f"enable parse on creation for {filename}"):
            ensure_parse_on(upload_modal, expect)
        if idx == 0:
            snap("parse_toggle_on")

        with step(f"upload file {filename}"):
            upload_file(page, expect, upload_modal, str(file_path), RESULT_TIMEOUT_MS)
            expect(upload_modal.locator(f"text={filename}")).to_be_visible(
                timeout=RESULT_TIMEOUT_MS
            )

        with step(f"submit upload {filename}"):
            save_button = upload_modal.locator(
                "button", has_text=re.compile("save", re.I)
            ).first

            def trigger():
                save_button.click()

            capture_response(
                page,
                trigger,
                lambda resp: resp.request.method == "POST"
                and "/v1/document/upload" in resp.url,
            )
            expect(upload_modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
        snap(f"upload_{filename}_submitted")

        row = page.locator(
            f"[data-testid='document-row'][data-doc-name={json.dumps(filename)}]"
        )
        expect(row).to_be_visible(timeout=RESULT_TIMEOUT_MS)

    flow_state["uploads_done"] = True


def step_05_wait_parse_success(
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
    require(flow_state, "uploads_done", "filenames")
    page = flow_page
    for filename in flow_state["filenames"]:
        with step(f"wait for parse success {filename}"):
            wait_for_success_dot(page, expect, filename, timeout_ms=RESULT_TIMEOUT_MS)
        snap(f"parse_{filename}_success")
    flow_state["parse_complete"] = True


def step_06_delete_one_file(
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
    require(flow_state, "parse_complete", "filenames")
    page = flow_page
    delete_filename = "Doc3.pdf"
    with step(f"delete uploaded file {delete_filename}"):
        delete_uploaded_file(page, expect, delete_filename, timeout_ms=RESULT_TIMEOUT_MS)
    snap("file_deleted_doc3")
    expect(
        page.locator(
            f"[data-testid='document-row'][data-doc-name={json.dumps('Doc1.pdf')}]"
        )
    ).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    expect(
        page.locator(
            f"[data-testid='document-row'][data-doc-name={json.dumps('Doc2.pdf')}]"
        )
    ).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    snap("success")


STEPS = [
    ("01_login", step_01_login),
    ("02_open_datasets", step_02_open_datasets),
    ("03_create_dataset", step_03_create_dataset),
    ("04_upload_files", step_04_upload_files),
    ("05_wait_parse_success", step_05_wait_parse_success),
    ("06_delete_one_file", step_06_delete_one_file),
]


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(STEPS))
def test_dataset_upload_parse_and_delete_flow(
    step_fn,
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
    step_fn(
        flow_page,
        flow_state,
        base_url,
        login_url,
        active_auth_context,
        step,
        snap,
        auth_click,
        seeded_user_credentials,
    )
