import base64
import json
import re
import time
from pathlib import Path
from urllib.parse import urljoin

import pytest
from playwright.sync_api import expect

from test.playwright.helpers._auth_helpers import ensure_authed
from test.playwright.helpers.flow_steps import flow_params, require
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


def make_test_png(path: Path) -> Path:
    png_b64 = (
        "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8"
        "/w8AAgMBAp6X6QAAAABJRU5ErkJggg=="
    )
    path.write_bytes(base64.b64decode(png_b64))
    return path


def extract_dataset_id_from_url(url: str) -> str:
    match = re.search(r"/(?:datasets|dataset/dataset)/([^/?#]+)", url or "")
    if not match:
        raise AssertionError(f"Unable to parse dataset id from url={url!r}")
    return match.group(1)


def set_switch_state(page, test_id: str, desired_checked: bool) -> None:
    switch = page.get_by_test_id(test_id).first
    expect(switch).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    switch.scroll_into_view_if_needed()
    current_checked = (switch.get_attribute("data-state") or "") == "checked"
    if current_checked == desired_checked:
        return
    switch.click()
    expect(switch).to_have_attribute(
        "data-state",
        "checked" if desired_checked else "unchecked",
        timeout=RESULT_TIMEOUT_MS,
    )


def set_number_input(page, test_id: str, value: str | int | float) -> None:
    number_input = page.get_by_test_id(test_id).first
    expect(number_input).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    number_input.scroll_into_view_if_needed()
    number_input.click()
    try:
        number_input.press("Control+a")
    except Exception:
        pass
    number_input.fill(str(value))
    try:
        number_input.press("Tab")
    except Exception:
        pass


def select_combobox_option(
    page,
    trigger_test_id: str,
    preferred_text: str | None = None,
) -> str:
    trigger = page.get_by_test_id(trigger_test_id).first
    expect(trigger).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    trigger.scroll_into_view_if_needed()
    current_text = ""
    try:
        current_text = trigger.inner_text().strip()
    except Exception:
        current_text = ""
    trigger.click()

    options = page.get_by_test_id("combobox-option")
    expect(options.first).to_be_visible(timeout=RESULT_TIMEOUT_MS)

    def click_option(option) -> None:
        option.scroll_into_view_if_needed()
        try:
            option.click()
        except Exception:
            page.wait_for_timeout(120)
            option.scroll_into_view_if_needed()
            option.click(force=True)

    if preferred_text:
        preferred_option = options.filter(
            has_text=re.compile(rf"^{re.escape(preferred_text)}$", re.I)
        )
        if preferred_option.count() > 0:
            click_option(preferred_option.first)
            return preferred_text

    selected_text = ""
    option_count = options.count()
    for idx in range(option_count):
        option = options.nth(idx)
        try:
            if not option.is_visible():
                continue
        except Exception:
            continue
        text = option.inner_text().strip()
        if not text:
            continue
        if current_text and text.lower() == current_text.lower() and option_count > 1:
            continue
        click_option(option)
        selected_text = text
        break

    if not selected_text:
        fallback = options.first
        selected_text = fallback.inner_text().strip()
        click_option(fallback)
    return selected_text


def select_ragflow_option(
    page,
    trigger_test_id: str,
    preferred_text: str | None = None,
) -> str:
    trigger = page.get_by_test_id(trigger_test_id).first
    expect(trigger).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    trigger.scroll_into_view_if_needed()
    current_text = ""
    try:
        current_text = trigger.inner_text().strip()
    except Exception:
        current_text = ""
    trigger.click()

    options = page.locator("[role='option']")
    expect(options.first).to_be_visible(timeout=RESULT_TIMEOUT_MS)

    if preferred_text:
        preferred_option = options.filter(
            has_text=re.compile(rf"^{re.escape(preferred_text)}$", re.I)
        )
        if preferred_option.count() > 0:
            preferred_option.first.click()
            return preferred_text

    selected_text = ""
    option_count = options.count()
    for idx in range(option_count):
        option = options.nth(idx)
        try:
            if not option.is_visible():
                continue
        except Exception:
            continue
        text = option.inner_text().strip()
        if not text:
            continue
        if current_text and text.lower() == current_text.lower() and option_count > 1:
            continue
        option.click()
        selected_text = text
        break

    if not selected_text:
        fallback = options.first
        selected_text = fallback.inner_text().strip()
        fallback.click()
    return selected_text


def get_request_json_payload(response) -> dict:
    payload = None
    request = response.request
    try:
        post_data_json = request.post_data_json
        payload = post_data_json() if callable(post_data_json) else post_data_json
    except Exception:
        payload = None

    if payload is None:
        try:
            post_data = request.post_data
            raw = post_data() if callable(post_data) else post_data
            if raw:
                payload = json.loads(raw)
        except Exception:
            payload = None

    if not isinstance(payload, dict):
        raise AssertionError(f"Expected JSON object payload for /v1/kb/update, got={payload!r}")
    return payload


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
    tmp_path,
    ensure_dataset_ready,
):
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
        ensure_authed(
            flow_page,
            login_url,
            active_auth_context,
            auth_click,
            seeded_user_credentials=seeded_user_credentials,
            timeout_ms=RESULT_TIMEOUT_MS,
        )
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
    tmp_path,
    ensure_dataset_ready,
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
    tmp_path,
    ensure_dataset_ready,
):
    require(flow_state, "logged_in")
    page = flow_page
    with step("open create dataset modal"):
        try:
            modal = open_create_dataset_modal(page, expect, RESULT_TIMEOUT_MS)
        except AssertionError:
            fallback_id = (ensure_dataset_ready or {}).get("kb_id")
            fallback_name = (ensure_dataset_ready or {}).get("kb_name")
            if not fallback_id or not fallback_name:
                raise
            page.goto(
                urljoin(base_url.rstrip("/") + "/", f"/dataset/dataset/{fallback_id}"),
                wait_until="domcontentloaded",
            )
            wait_for_dataset_detail_ready(page, expect, timeout_ms=RESULT_TIMEOUT_MS * 2)
            flow_state["dataset_name"] = fallback_name
            flow_state["dataset_id"] = fallback_id
            snap("dataset_created")
            snap("dataset_detail_ready")
            return
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
        created_kb_id = None

        def trigger():
            save_button.click()

        create_response = capture_response(
            page,
            trigger,
            lambda resp: resp.request.method == "POST" and "/v1/kb/create" in resp.url,
            timeout_ms=RESULT_TIMEOUT_MS * 2,
        )
        try:
            create_payload = create_response.json()
        except Exception:
            create_payload = {}
        if isinstance(create_payload, dict):
            data = create_payload.get("data") or {}
            if isinstance(data, dict):
                created_kb_id = data.get("id") or data.get("kb_id")

        expect(modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
        try:
            wait_for_dataset_detail(page, timeout_ms=RESULT_TIMEOUT_MS * 2)
        except Exception:
            if created_kb_id:
                page.goto(
                    urljoin(
                        base_url.rstrip("/") + "/", f"/dataset/dataset/{created_kb_id}"
                    ),
                    wait_until="domcontentloaded",
                )
            else:
                raise
        wait_for_dataset_detail_ready(page, expect, timeout_ms=RESULT_TIMEOUT_MS * 2)
    dataset_id = extract_dataset_id_from_url(page.url)
    flow_state["dataset_name"] = dataset_name
    flow_state["dataset_id"] = dataset_id
    snap("dataset_created")
    snap("dataset_detail_ready")


def step_04_set_dataset_settings(
    flow_page,
    flow_state,
    base_url,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
    tmp_path,
    ensure_dataset_ready,
):
    require(flow_state, "dataset_name", "dataset_id")
    page = flow_page
    dataset_id = flow_state["dataset_id"]
    dataset_name = flow_state["dataset_name"]
    metadata_field_key = "auto_meta_field"

    with step("open dataset settings page"):
        page.goto(
            urljoin(
                base_url.rstrip("/") + "/", f"/dataset/dataset-setting/{dataset_id}"
            ),
            wait_until="domcontentloaded",
        )
        expect(page.get_by_test_id("ds-settings-basic-name-input")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
        expect(page.get_by_test_id("ds-settings-page-save-btn")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    snap("dataset_settings_open")

    with step("fill base settings"):
        page.get_by_test_id("ds-settings-basic-name-input").fill(
            f"{dataset_name}-cfg"
        )
        select_combobox_option(
            page, "ds-settings-basic-language-select", preferred_text="English"
        )

        avatar_path = make_test_png(tmp_path / "avatar-test.png")
        page.get_by_test_id("ds-settings-basic-avatar-upload").set_input_files(
            str(avatar_path)
        )
        crop_modal = page.get_by_test_id("ds-settings-basic-avatar-crop-modal")
        expect(crop_modal).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        page.get_by_test_id("ds-settings-basic-avatar-crop-confirm-btn").click()
        expect(crop_modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)

        page.get_by_test_id("ds-settings-basic-description-input").fill(
            "Dataset setting playwright description"
        )
        try:
            select_combobox_option(page, "ds-settings-basic-permissions-select")
        except Exception:
            page.keyboard.press("Escape")

        embedding_trigger = page.get_by_test_id(
            "ds-settings-basic-embedding-model-select"
        ).first
        expect(embedding_trigger).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        if not embedding_trigger.is_disabled():
            try:
                select_combobox_option(page, "ds-settings-basic-embedding-model-select")
            except Exception:
                page.keyboard.press("Escape")

    with step("fill parser and metadata settings"):
        set_number_input(page, "ds-settings-parser-page-rank-input", 12)
        select_combobox_option(
            page, "ds-settings-parser-pdf-parser-select", preferred_text="Plain Text"
        )
        set_number_input(page, "ds-settings-parser-recommended-chunk-size-input", 640)
        set_switch_state(page, "ds-settings-parser-child-chunk-switch", True)
        expect(
            page.get_by_test_id("ds-settings-parser-child-chunk-delimiter-input")
        ).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        set_switch_state(page, "ds-settings-parser-page-index-switch", True)
        set_number_input(page, "ds-settings-parser-image-table-context-window-input", 16)
        set_switch_state(page, "ds-settings-metadata-switch", True)

        page.get_by_test_id("ds-settings-metadata-open-modal-btn").click()
        metadata_modal = page.get_by_test_id("ds-settings-metadata-modal")
        expect(metadata_modal).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        page.get_by_test_id("ds-settings-metadata-add-btn").click()

        nested_modal = page.get_by_test_id("ds-settings-metadata-add-modal")
        expect(nested_modal).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        field_input = nested_modal.locator("input[name='field']")
        if field_input.count() == 0:
            field_input = nested_modal.locator("input")
        expect(field_input.first).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        field_input.first.fill(metadata_field_key)
        description_input = nested_modal.locator("textarea")
        if description_input.count() > 0:
            description_input.first.fill("auto metadata field from playwright")
        confirm_btn = page.get_by_test_id("ds-settings-metadata-add-modal-confirm-btn")
        confirm_btn.click()
        try:
            expect(nested_modal).not_to_be_visible(timeout=3000)
        except AssertionError:
            retry_field_input = nested_modal.locator("input[name='field']")
            if retry_field_input.count() > 0:
                retry_field_input.first.fill("auto_meta_field_retry")
            confirm_btn.click()
            expect(nested_modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
        snap("dataset_settings_metadata_modal")

        page.get_by_test_id("ds-settings-metadata-modal-save-btn").click()
        expect(metadata_modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)

        overlap_slider = page.get_by_test_id(
            "ds-settings-parser-overlapped-percent-slider"
        ).first
        expect(overlap_slider).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        overlap_slider.focus()
        overlap_slider.press("ArrowRight")
        set_number_input(page, "ds-settings-parser-auto-keyword-input", 3)
        set_number_input(page, "ds-settings-parser-auto-question-input", 2)
        set_switch_state(page, "ds-settings-parser-excel-to-html-switch", True)

    with step("fill graph and raptor settings"):
        page.get_by_test_id("ds-settings-graph-entity-types-add-btn").click()
        entity_input = page.get_by_test_id("ds-settings-graph-entity-types-input").first
        expect(entity_input).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        entity_input.fill("playwright_entity")
        entity_input.press("Enter")
        select_ragflow_option(
            page, "ds-settings-graph-method-select", preferred_text="General"
        )
        set_switch_state(page, "ds-settings-graph-entity-resolution-switch", True)
        set_switch_state(page, "ds-settings-graph-community-reports-switch", True)

        raptor_scope_dataset = page.get_by_role(
            "radio", name=re.compile(r"^Dataset$", re.I)
        ).first
        raptor_scope_dataset.check(force=True)
        expect(raptor_scope_dataset).to_be_checked(timeout=RESULT_TIMEOUT_MS)
        page.get_by_test_id("ds-settings-raptor-prompt-textarea").fill(
            "Playwright prompt for dataset settings"
        )
        set_number_input(page, "ds-settings-raptor-max-token-input", 300)
        set_number_input(page, "ds-settings-raptor-threshold-input", 0.3)
        set_number_input(page, "ds-settings-raptor-max-cluster-input", 128)
        set_number_input(page, "ds-settings-raptor-seed-input", 1234)
        seed_input = page.get_by_test_id("ds-settings-raptor-seed-input").first
        seed_before_randomize = seed_input.input_value()
        page.get_by_test_id("ds-settings-raptor-seed-randomize-btn").click()
        page.wait_for_function(
            """([testId, previous]) => {
              const node = document.querySelector(`[data-testid="${testId}"]`);
              return !!node && String(node.value) !== String(previous);
            }""",
            arg=["ds-settings-raptor-seed-input", seed_before_randomize],
            timeout=RESULT_TIMEOUT_MS,
        )

    with step("save dataset settings and assert update payload"):
        try:
            expect(page.locator("[data-sonner-toast]")).to_have_count(0, timeout=8000)
        except AssertionError:
            pass
        save_btn = page.get_by_test_id("ds-settings-page-save-btn").first
        expect(save_btn).to_be_visible(timeout=RESULT_TIMEOUT_MS)

        def trigger():
            save_btn.click()

        response = capture_response(
            page,
            trigger,
            lambda resp: resp.request.method == "POST" and "/v1/kb/update" in resp.url,
            timeout_ms=RESULT_TIMEOUT_MS * 2,
        )
        assert 200 <= response.status < 400, f"Unexpected /v1/kb/update status={response.status}"
        response_payload = response.json()
        if isinstance(response_payload, dict):
            assert response_payload.get("code") == 0, (
                f"/v1/kb/update response code={response_payload.get('code')} "
                f"message={response_payload.get('message')}"
            )

        payload = get_request_json_payload(response)
        assert payload.get("kb_id") == dataset_id, (
            f"Expected kb_id={dataset_id!r}, got {payload.get('kb_id')!r}"
        )
        for key in ("name", "language", "parser_config"):
            assert key in payload, f"Expected key {key!r} in /v1/kb/update payload"
        parser_config = payload.get("parser_config") or {}
        assert (
            parser_config.get("image_table_context_window")
            == parser_config.get("image_context_size")
            == parser_config.get("table_context_size")
        ), "Expected image/table context window transform keys to be aligned"
        expect(page.locator("[data-sonner-toast]").first).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )

    with step("return to dataset detail for upload"):
        page.goto(
            urljoin(base_url.rstrip("/") + "/", f"/dataset/dataset/{dataset_id}"),
            wait_until="domcontentloaded",
        )
        wait_for_dataset_detail_ready(page, expect, timeout_ms=RESULT_TIMEOUT_MS)

    flow_state["dataset_settings_done"] = True
    flow_state["settings_update_payload"] = payload
    snap("dataset_settings_saved")


def step_05_upload_files(
    flow_page,
    flow_state,
    base_url,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
    tmp_path,
    ensure_dataset_ready,
):
    require(flow_state, "dataset_name", "dataset_settings_done", "file_paths")
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
                and "/v1/datasets/.*/documents" in resp.url,
            )
            expect(upload_modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
        snap(f"upload_{filename}_submitted")

        row = page.locator(
            f"[data-testid='document-row'][data-doc-name={json.dumps(filename)}]"
        )
        expect(row).to_be_visible(timeout=RESULT_TIMEOUT_MS)

    flow_state["uploads_done"] = True


def step_06_wait_parse_success(
    flow_page,
    flow_state,
    base_url,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
    tmp_path,
    ensure_dataset_ready,
):
    require(flow_state, "uploads_done", "filenames")
    page = flow_page
    parse_timeout_ms = RESULT_TIMEOUT_MS * 8
    for filename in flow_state["filenames"]:
        with step(f"wait for parse success {filename}"):
            wait_for_success_dot(page, expect, filename, timeout_ms=parse_timeout_ms)
        snap(f"parse_{filename}_success")
    flow_state["parse_complete"] = True


def step_07_delete_one_file(
    flow_page,
    flow_state,
    base_url,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
    tmp_path,
    ensure_dataset_ready,
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
    ("04_set_dataset_settings", step_04_set_dataset_settings),
    ("05_upload_files", step_05_upload_files),
    ("06_wait_parse_success", step_06_wait_parse_success),
    ("07_delete_one_file", step_07_delete_one_file),
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
    ensure_model_provider_configured,
    ensure_dataset_ready,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
    tmp_path,
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
        tmp_path,
        ensure_dataset_ready,
    )
