import json
import re
import time
from pathlib import Path
from urllib.parse import urljoin

import pytest
from playwright.sync_api import TimeoutError as PlaywrightTimeoutError
from playwright.sync_api import expect

from test.playwright.helpers.flow_steps import flow_params, require
from test.playwright.helpers.auth_selectors import EMAIL_INPUT, PASSWORD_INPUT, SUBMIT_BUTTON
from test.playwright.helpers.auth_waits import wait_for_login_complete
from test.playwright.helpers.debug_utils import debug
from test.playwright.helpers.env_utils import env_bool
from test.playwright.helpers.response_capture import capture_response

RESULT_TIMEOUT_MS = 15000


def _wait_for_dataset_detail(page, timeout_ms: int = RESULT_TIMEOUT_MS) -> None:
    wait_js = """
        () => {
          const path = window.location.pathname || '';
          return /^\\/datasets\\/.+/.test(path) || /^\\/dataset\\/dataset\\/.+/.test(path);
        }
        """
    page.wait_for_function(wait_js, timeout=timeout_ms)


def _wait_for_dataset_detail_ready(page, timeout_ms: int = RESULT_TIMEOUT_MS) -> None:
    _wait_for_dataset_detail(page, timeout_ms=timeout_ms)
    try:
        page.wait_for_load_state("networkidle", timeout=timeout_ms)
    except Exception:
        try:
            page.wait_for_load_state("domcontentloaded", timeout=timeout_ms)
        except Exception:
            pass

    heading = page.locator("[role='heading']").first
    main = page.locator("[role='main']").first
    if main.count() > 0:
        anchor = main.locator("text=/\\b(add|upload|file|document)\\b/i").first
    else:
        anchor = page.locator("text=/\\b(add|upload|file|document)\\b/i").first
    try:
        if heading.count() > 0:
            expect(heading).to_be_visible(timeout=timeout_ms)
            return
        if main.count() > 0:
            expect(main).to_be_visible(timeout=timeout_ms)
            return
        expect(anchor).to_be_visible(timeout=timeout_ms)
    except AssertionError:
        if env_bool("PW_DEBUG_DUMP"):
            url = page.url
            button_count = page.locator("button, [role='button']").count()
            body_text = page.evaluate(
                "(() => (document.body && document.body.innerText) || '')()"
            )
            debug(
                f"[dataset] detail_ready_failed url={url} button_count={button_count}"
            )
            debug(f"[dataset] body_text_snippet={body_text[:200]!r}")
        raise


def _upload_file(page, dialog, file_path: str) -> None:
    dropzone = dialog.locator("[data-testid='dataset-upload-dropzone']").first
    expect(dropzone).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    if hasattr(page, "expect_file_chooser"):
        with page.expect_file_chooser() as chooser_info:
            dropzone.click()
        chooser_info.value.set_files(file_path)
        return
    input_locator = dialog.locator("input[type='file']")
    if input_locator.count() == 0:
        raise AssertionError("File chooser not available and no input[type='file'] found.")
    input_locator.first.set_input_files(file_path)


def _wait_for_success_dot(page, file_name: str, timeout_ms: int = RESULT_TIMEOUT_MS) -> None:
    name_selector = f"[data-doc-name={json.dumps(file_name)}]"
    row = page.locator(f"[data-testid='document-row']{name_selector}")
    expect(row).to_be_visible(timeout=timeout_ms)
    status = row.locator("[data-testid='document-parse-status']")
    expect(status).to_have_attribute("data-state", "success", timeout=timeout_ms)


def _dump_clickable_candidates(page) -> None:
    candidates = page.locator("button, [role='button'], a")
    total = candidates.count()
    lines = []
    limit = min(total, 10)
    for idx in range(limit):
        item = candidates.nth(idx)
        try:
            if not item.is_visible():
                continue
            text = item.inner_text().strip().replace("\n", " ")
        except Exception:
            continue
        if text:
            lines.append(text[:80])
    debug(f"[dataset] clickable_candidates={total} visible_sample={lines}")


def _get_upload_modal(page):
    return page.locator("[data-testid='dataset-upload-modal']")


def _ensure_upload_modal_open(page, auth_click, timeout_ms: int = RESULT_TIMEOUT_MS):
    modal = _get_upload_modal(page)
    if modal.count() > 0:
        try:
            expect(modal).to_be_visible(timeout=timeout_ms)
            return modal
        except AssertionError:
            pass
    return _open_upload_modal_from_dataset_detail(page, auth_click, timeout_ms=timeout_ms)


def _ensure_parse_on(upload_modal) -> None:
    parse_switch = upload_modal.locator("[data-testid='parse-on-creation-toggle']").first
    expect(parse_switch).to_be_visible()
    state = parse_switch.get_attribute("data-state")
    if state == "checked":
        return
    parse_switch.click()
    expect(parse_switch).to_have_attribute("data-state", "checked")


def _open_upload_modal_from_dataset_detail(page, auth_click, timeout_ms: int = RESULT_TIMEOUT_MS):
    _wait_for_dataset_detail_ready(page, timeout_ms=timeout_ms)
    page.wait_for_selector("button", timeout=timeout_ms)

    if hasattr(page, "get_by_role"):
        tab_locator = page.get_by_role(
            "tab", name=re.compile(r"^(files|documents|file)$", re.I)
        )
        if tab_locator.count() > 0:
            tab = tab_locator.first
            try:
                if tab.is_visible():
                    tab.click()
                    page.wait_for_timeout(250)
            except Exception:
                pass

    candidate_names = re.compile(
        r"(upload file|upload|add file|add document|add|new)", re.I
    )
    trigger_locator = None
    if hasattr(page, "get_by_role"):
        trigger_locator = page.get_by_role("button", name=candidate_names)
    if trigger_locator is None or trigger_locator.count() == 0:
        trigger_locator = page.locator("[role='button'], button, a").filter(
            has_text=candidate_names
        )

    trigger = None
    if trigger_locator.count() > 0:
        limit = min(trigger_locator.count(), 5)
        for idx in range(limit):
            candidate = trigger_locator.nth(idx)
            try:
                if candidate.is_visible():
                    trigger = candidate
                    break
            except Exception:
                continue

    if trigger is None:
        aria_candidates = page.locator(
            "button[aria-label], button[title], [role='button'][aria-label], [role='button'][title]"
        )
        limit = min(aria_candidates.count(), 10)
        for idx in range(limit):
            candidate = aria_candidates.nth(idx)
            try:
                if not candidate.is_visible():
                    continue
                aria_label = candidate.get_attribute("aria-label") or ""
                title = candidate.get_attribute("title") or ""
                if candidate_names.search(aria_label) or candidate_names.search(title):
                    trigger = candidate
                    break
            except Exception:
                continue

    if trigger is None:
        if env_bool("PW_DEBUG_DUMP"):
            debug("[dataset] upload_trigger_not_found initial scan")
        button_dump = []
        buttons = page.locator("button")
        total = buttons.count()
        limit = min(total, 20)
        for idx in range(limit):
            item = buttons.nth(idx)
            try:
                if not item.is_visible():
                    continue
            except Exception:
                continue
            try:
                text = item.inner_text().strip()
            except Exception as exc:
                text = f"<text-error:{exc}>"
            try:
                aria_label = item.get_attribute("aria-label")
            except Exception as exc:
                aria_label = f"<aria-error:{exc}>"
            try:
                title = item.get_attribute("title")
            except Exception as exc:
                title = f"<title-error:{exc}>"
            button_dump.append(
                {"text": text, "aria_label": aria_label, "title": title}
            )
        raise AssertionError(
            "Upload entrypoint not found on dataset detail page. "
            f"visible_buttons={button_dump}"
        )

    try:
        if trigger.evaluate("el => el.tagName.toLowerCase() === 'button'"):
            auth_click(trigger, "open_upload")
        else:
            trigger.click()
    except Exception:
        trigger.click()

    def _click_upload_file_popover_item() -> bool:
        locators = [
            page.locator("[role='menuitem']").filter(
                has_text=re.compile(r"^upload file$", re.I)
            ),
            page.locator("[role='option']").filter(
                has_text=re.compile(r"^upload file$", re.I)
            ),
            page.locator("div, span, li").filter(
                has_text=re.compile(r"^upload file$", re.I)
            ),
        ]
        for locator in locators:
            if locator.count() == 0:
                continue
            limit = min(locator.count(), 5)
            for idx in range(limit):
                candidate = locator.nth(idx)
                try:
                    if candidate.is_visible():
                        candidate.click()
                        return True
                except Exception:
                    continue
        return False

    clicked_item = _click_upload_file_popover_item()
    if not clicked_item:
        if env_bool("PW_DEBUG_DUMP"):
            try:
                button_texts = page.evaluate(
                    """
                    () => Array.from(document.querySelectorAll('button,[role="button"],a'))
                      .filter((el) => {
                        const rect = el.getBoundingClientRect();
                        return rect.width > 0 && rect.height > 0;
                      })
                      .map((el) => (el.innerText || '').trim())
                      .filter(Boolean)
                      .slice(0, 20)
                    """
                )
            except Exception:
                button_texts = []
            has_upload_text = page.locator("text=/upload file/i").count() > 0
            debug(f"[dataset] upload_item_missing has_upload_text={has_upload_text}")
            debug(f"[dataset] visible_button_texts={button_texts}")
        raise AssertionError(
            "Upload file popover item not found after clicking Add trigger."
        )

    try:
        page.wait_for_load_state("domcontentloaded", timeout=timeout_ms)
    except Exception:
        pass

    upload_modal = page.locator("[data-testid='dataset-upload-modal']")
    expect(upload_modal).to_be_visible(timeout=timeout_ms)
    return upload_modal


def _select_chunking_method_general(page, modal) -> None:
    trigger_locator = modal.locator(
        "button",
        has=modal.locator(
            "span", has_text=re.compile(r"please select a chunking method\\.", re.I)
        ),
    ).first
    trigger_handle = None
    if trigger_locator.count() == 0:
        label = modal.locator("text=/please select a chunking method\\./i").first
        if label.count() > 0:
            element_handle = label.element_handle()
            if element_handle is not None:
                handle = page.evaluate_handle("(el) => el.closest('button')", element_handle)
                trigger_handle = handle.as_element()
        if trigger_handle is None:
            trigger_locator = modal.locator(
                "button", has_text=re.compile(r"please select a chunking method\\.", re.I)
            ).first

    if trigger_locator.count() == 0 and trigger_handle is None:
        if env_bool("PW_DEBUG_DUMP"):
            modal_text = modal.inner_text()
            button_count = modal.locator("button").count()
            label_count = modal.locator(
                "text=/please select a chunking method\\./i"
            ).count()
            debug(
                "[dataset] chunking_trigger_missing "
                f"button_count={button_count} label_count={label_count} "
                f"trigger_locator_count={trigger_locator.count()} "
                f"trigger_handle_found={trigger_handle is not None}"
            )
            debug(f"[dataset] modal_text_snippet={modal_text[:300]!r}")
        raise AssertionError("Chunking method dropdown trigger not found.")

    trigger_for_assert = None
    if trigger_locator.count() > 0:
        expect(trigger_locator).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        trigger_locator.click()
        trigger_for_assert = trigger_locator
    elif trigger_handle is not None:
        trigger_handle.click()
    listbox = page.locator("[role='listbox']:visible").last
    if listbox.count() == 0:
        listbox = page.locator("[cmdk-list]:visible").last
    if listbox.count() == 0:
        listbox = page.locator("[data-state='open']:visible").last
    if listbox.count() == 0:
        listbox = page.locator("body").locator("div:visible").last

    option = listbox.locator("span", has_text=re.compile(r"^General$", re.I)).first
    if option.count() == 0:
        option = listbox.locator(
            "div", has=page.locator("span", has_text=re.compile(r"^General$", re.I))
        ).first
    if option.count() == 0 and env_bool("PW_DEBUG_DUMP"):
        try:
            listbox_text = listbox.inner_text()
        except Exception:
            listbox_text = ""
        span_count = listbox.locator(
            "span", has_text=re.compile(r"^General$", re.I)
        ).count()
        debug(
            "[dataset] general_option_missing "
            f"listbox_count={listbox.count()} span_count={span_count}"
        )
        debug(f"[dataset] listbox_text_snippet={listbox_text[:300]!r}")
    expect(option).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    option.click()
    if trigger_for_assert is not None:
        expect(trigger_for_assert).to_contain_text(re.compile(r"General", re.I))


def _open_create_dataset_modal(page):
    wait_js = """
        () => {
          const txt = (document.body && document.body.innerText || '').toLowerCase();
          if (txt.includes('no dataset created yet')) return true;
          return Array.from(document.querySelectorAll('button')).some((b) =>
            (b.innerText || '').toLowerCase().includes('create dataset')
          );
        }
        """
    try:
        page.wait_for_function(wait_js, timeout=RESULT_TIMEOUT_MS)
    except PlaywrightTimeoutError:
        if env_bool("PW_DEBUG_DUMP"):
            url = page.url
            body_text = page.evaluate(
                "(() => (document.body && document.body.innerText) || '')()"
            )
            lines = body_text.splitlines()
            snippet = "\n".join(lines[:20])[:500]
            debug(f"[dataset] entrypoint_wait_timeout url={url} snippet={snippet!r}")
        raise

    empty_text = page.locator("text=/no dataset created yet/i").first
    if empty_text.count() > 0:
        debug("[dataset] using empty-state entrypoint")
        expect(empty_text).to_be_visible(timeout=5000)
        element_handle = empty_text.element_handle()
        if element_handle is None:
            debug("[dataset] empty-state text element handle not available")
            _dump_clickable_candidates(page)
            raise AssertionError("Empty-state text element not available for click.")
        handle = page.evaluate_handle(
            """
            (el) => {
              const closest = el.closest('button, a, [role="button"]');
              if (closest) return closest;
              let node = el;
              for (let i = 0; i < 6 && node; i += 1) {
                if (node.nodeType !== Node.ELEMENT_NODE) {
                  node = node.parentElement;
                  continue;
                }
                const element = node;
                const hasOnClick = typeof element.onclick === 'function' || element.hasAttribute('onclick');
                const tabIndex = element.getAttribute('tabindex');
                const hasTab = tabIndex === '0';
                const cursor = window.getComputedStyle(element).cursor;
                if (hasOnClick || hasTab || cursor === 'pointer') {
                  return element;
                }
                node = element.parentElement;
              }
              return null;
            }
            """,
            element_handle,
        )
        element = handle.as_element()
        if element is None:
            debug("[dataset] empty-state clickable ancestor not found")
            _dump_clickable_candidates(page)
            raise AssertionError("No clickable ancestor found for empty dataset state.")
        element.click()
    else:
        debug("[dataset] using create button entrypoint")
        create_btn = None
        if hasattr(page, "get_by_role"):
            create_btn = page.get_by_role(
                "button", name=re.compile(r"create dataset", re.I)
            )
        if create_btn is None or create_btn.count() == 0:
            create_btn = page.locator(
                "button", has_text=re.compile(r"create dataset", re.I)
            ).first
        if create_btn.count() == 0:
            if env_bool("PW_DEBUG_DUMP"):
                url = page.url
                body_text = page.evaluate(
                    "(() => (document.body && document.body.innerText) || '')()"
                )
                lines = body_text.splitlines()
                snippet = "\n".join(lines[:20])[:500]
                debug(f"[dataset] entrypoint_not_found url={url} snippet={snippet!r}")
                _dump_clickable_candidates(page)
            raise AssertionError("No dataset entrypoint found after readiness wait.")
        debug(f"[dataset] create_button_count={create_btn.count()}")
        try:
            expect(create_btn).to_be_visible(timeout=5000)
        except AssertionError:
            if env_bool("PW_DEBUG_DUMP"):
                url = page.url
                body_text = page.evaluate(
                    "(() => (document.body && document.body.innerText) || '')()"
                )
                lines = body_text.splitlines()
                snippet = "\n".join(lines[:20])[:500]
                debug(f"[dataset] entrypoint_not_found url={url} snippet={snippet!r}")
            raise
        create_btn.click()

    modal = page.locator("[role='dialog']").filter(has_text=re.compile("create dataset", re.I))
    expect(modal).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    return modal


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
        modal = _open_create_dataset_modal(page)
    snap("dataset_modal_open")

    dataset_name = f"qa-dataset-{int(time.time() * 1000)}"
    with step("fill dataset form"):
        name_input = modal.locator("input[placeholder='Please input name.']").first
        expect(name_input).to_be_visible()
        name_input.fill(dataset_name)

        try:
            _select_chunking_method_general(page, modal)
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
        _wait_for_dataset_detail(page)
        _wait_for_dataset_detail_ready(page)
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
            upload_modal = _ensure_upload_modal_open(page, auth_click)
        if idx == 0:
            snap("upload_modal_open")

        with step(f"enable parse on creation for {filename}"):
            _ensure_parse_on(upload_modal)
        if idx == 0:
            snap("parse_toggle_on")

        with step(f"upload file {filename}"):
            _upload_file(page, upload_modal, str(file_path))
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
            _wait_for_success_dot(page, filename, timeout_ms=RESULT_TIMEOUT_MS)
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
        row = page.locator(
            f"[data-testid='document-row'][data-doc-name={json.dumps(delete_filename)}]"
        )
        expect(row).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        delete_button = row.locator("[data-testid='document-delete']")
        expect(delete_button).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        delete_button.click()
        confirm = page.locator("[role='alertdialog']")
        expect(confirm).to_be_visible()
        confirm.locator("button", has_text=re.compile("^delete$", re.I)).first.click()
        expect(row).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
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
