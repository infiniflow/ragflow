import json
import os
import re
import time
from pathlib import Path
from urllib.parse import urljoin

import pytest
from playwright.sync_api import TimeoutError as PlaywrightTimeoutError
from playwright.sync_api import expect

RESULT_TIMEOUT_MS = 15000


def _env_bool(name: str) -> bool:
    value = os.getenv(name)
    if not value:
        return False
    return value.strip().lower() in {"1", "true", "yes", "on"}


def _debug(msg: str) -> None:
    if _env_bool("PW_DEBUG_DUMP"):
        print(msg, flush=True)


def _capture_response(page, trigger, predicate, timeout_ms: int = RESULT_TIMEOUT_MS):
    if hasattr(page, "expect_response"):
        with page.expect_response(predicate, timeout=timeout_ms) as response_info:
            trigger()
        return response_info.value
    if hasattr(page, "expect_event"):
        with page.expect_event("response", predicate=predicate, timeout=timeout_ms) as response_info:
            trigger()
        return response_info.value
    if hasattr(page, "wait_for_event"):
        trigger()
        return page.wait_for_event("response", predicate=predicate, timeout=timeout_ms)
    raise RuntimeError("Playwright Page lacks expect_response/expect_event/wait_for_event.")


def _wait_for_login_complete(page, timeout_ms: int = RESULT_TIMEOUT_MS) -> None:
    wait_js = """
        () => {
          const path = window.location.pathname || '';
          if (path.includes('/login')) return false;
          const token = localStorage.getItem('Token');
          const auth = localStorage.getItem('Authorization');
          return Boolean((token && token.length) || (auth && auth.length));
        }
        """
    page.wait_for_function(wait_js, timeout=timeout_ms)


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
        if _env_bool("PW_DEBUG_DUMP"):
            url = page.url
            button_count = page.locator("button, [role='button']").count()
            body_text = page.evaluate(
                "(() => (document.body && document.body.innerText) || '')()"
            )
            _debug(
                f"[dataset] detail_ready_failed url={url} button_count={button_count}"
            )
            _debug(f"[dataset] body_text_snippet={body_text[:200]!r}")
        raise


def _upload_file(page, dialog, file_path: str) -> None:
    dropzone = dialog.locator("text=Drag and drop your file here to upload").first
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
    name_json = json.dumps(file_name)
    wait_js = f"""
        () => {{
          const name = {name_json};
          const rows = Array.from(document.querySelectorAll('tbody tr'));
          const row = rows.find((r) => r.innerText && r.innerText.includes(name));
          if (!row) return false;
          const dot = row.querySelector('span.size-1.inline-block.rounded');
          if (!dot) return false;
          const style = window.getComputedStyle(dot);
          const color = (style.backgroundColor || '').replace(/\\s+/g, '').toLowerCase();
          const token = (window.getComputedStyle(document.documentElement).getPropertyValue('--state-success') || '').trim();
          const normalizedToken = token.replace(/\\s+/g, '');
          const candidates = new Set();
          if (normalizedToken) {{
            candidates.add('rgb(' + normalizedToken + ')');
            candidates.add('rgba(' + normalizedToken + ')');
          }}
          candidates.add('rgb(59,160,92)');
          candidates.add('rgba(59,160,92,1)');
          return candidates.has(color);
        }}
        """
    page.wait_for_function(wait_js, timeout=timeout_ms)


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
    _debug(f"[dataset] clickable_candidates={total} visible_sample={lines}")


def _get_upload_modal(page):
    modal = page.locator("[role='dialog']").filter(
        has=page.locator("text=/drag and drop your file here to upload/i")
    )
    if modal.count() == 0:
        modal = page.locator("[role='dialog']").filter(has_text=re.compile(r"upload", re.I))
    return modal


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
    parse_label = upload_modal.locator(
        "label", has_text=re.compile("parse on creation", re.I)
    ).first
    expect(parse_label).to_be_visible()
    parse_row = parse_label.locator("xpath=..")
    parse_switch = parse_row.locator("[role='switch'], button").first
    state = parse_switch.get_attribute("aria-checked")
    if state is None:
        state = parse_switch.get_attribute("data-state")
    if state in ("true", "checked"):
        return
    parse_switch.click()
    if parse_switch.get_attribute("aria-checked") is not None:
        expect(parse_switch).to_have_attribute("aria-checked", "true")
    else:
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
        if _env_bool("PW_DEBUG_DUMP"):
            _debug("[dataset] upload_trigger_not_found initial scan")
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
        if _env_bool("PW_DEBUG_DUMP"):
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
            _debug(f"[dataset] upload_item_missing has_upload_text={has_upload_text}")
            _debug(f"[dataset] visible_button_texts={button_texts}")
        raise AssertionError(
            "Upload file popover item not found after clicking Add trigger."
        )

    try:
        page.wait_for_load_state("domcontentloaded", timeout=timeout_ms)
    except Exception:
        pass

    upload_modal = page.locator("[role='dialog']").filter(
        has=page.locator("text=/drag and drop your file here to upload/i")
    )
    if upload_modal.count() == 0:
        upload_modal = page.locator("[role='dialog']").filter(
            has_text=re.compile(r"upload", re.I)
        )
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
        if _env_bool("PW_DEBUG_DUMP"):
            modal_text = modal.inner_text()
            button_count = modal.locator("button").count()
            label_count = modal.locator(
                "text=/please select a chunking method\\./i"
            ).count()
            _debug(
                "[dataset] chunking_trigger_missing "
                f"button_count={button_count} label_count={label_count} "
                f"trigger_locator_count={trigger_locator.count()} "
                f"trigger_handle_found={trigger_handle is not None}"
            )
            _debug(f"[dataset] modal_text_snippet={modal_text[:300]!r}")
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
    if option.count() == 0 and _env_bool("PW_DEBUG_DUMP"):
        try:
            listbox_text = listbox.inner_text()
        except Exception:
            listbox_text = ""
        span_count = listbox.locator(
            "span", has_text=re.compile(r"^General$", re.I)
        ).count()
        _debug(
            "[dataset] general_option_missing "
            f"listbox_count={listbox.count()} span_count={span_count}"
        )
        _debug(f"[dataset] listbox_text_snippet={listbox_text[:300]!r}")
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
        if _env_bool("PW_DEBUG_DUMP"):
            url = page.url
            body_text = page.evaluate(
                "(() => (document.body && document.body.innerText) || '')()"
            )
            lines = body_text.splitlines()
            snippet = "\n".join(lines[:20])[:500]
            _debug(f"[dataset] entrypoint_wait_timeout url={url} snippet={snippet!r}")
        raise

    empty_text = page.locator("text=/no dataset created yet/i").first
    if empty_text.count() > 0:
        _debug("[dataset] using empty-state entrypoint")
        expect(empty_text).to_be_visible(timeout=5000)
        element_handle = empty_text.element_handle()
        if element_handle is None:
            _debug("[dataset] empty-state text element handle not available")
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
            _debug("[dataset] empty-state clickable ancestor not found")
            _dump_clickable_candidates(page)
            raise AssertionError("No clickable ancestor found for empty dataset state.")
        element.click()
    else:
        _debug("[dataset] using create button entrypoint")
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
            if _env_bool("PW_DEBUG_DUMP"):
                url = page.url
                body_text = page.evaluate(
                    "(() => (document.body && document.body.innerText) || '')()"
                )
                lines = body_text.splitlines()
                snippet = "\n".join(lines[:20])[:500]
                _debug(f"[dataset] entrypoint_not_found url={url} snippet={snippet!r}")
                _dump_clickable_candidates(page)
            raise AssertionError("No dataset entrypoint found after readiness wait.")
        _debug(f"[dataset] create_button_count={create_btn.count()}")
        try:
            expect(create_btn).to_be_visible(timeout=5000)
        except AssertionError:
            if _env_bool("PW_DEBUG_DUMP"):
                url = page.url
                body_text = page.evaluate(
                    "(() => (document.body && document.body.innerText) || '')()"
                )
                lines = body_text.splitlines()
                snippet = "\n".join(lines[:20])[:500]
                _debug(f"[dataset] entrypoint_not_found url={url} snippet={snippet!r}")
            raise
        create_btn.click()

    modal = page.locator("[role='dialog']").filter(has_text=re.compile("create dataset", re.I))
    expect(modal).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    return modal


@pytest.mark.p1
@pytest.mark.auth
def test_dataset_upload_parse_and_delete(
    base_url,
    login_url,
    page,
    active_auth_context,
    step,
    snap,
    auth_click,
):
    email = os.getenv("SEEDED_USER_EMAIL")
    password = os.getenv("SEEDED_USER_PASSWORD")
    if not email or not password:
        pytest.skip("SEEDED_USER_EMAIL/SEEDED_USER_PASSWORD not set.")

    repo_root = Path(__file__).resolve().parents[3]
    file_paths = [
        repo_root / "test/benchmark/test_docs/Doc1.pdf",
        repo_root / "test/benchmark/test_docs/Doc2.pdf",
        repo_root / "test/benchmark/test_docs/Doc3.pdf",
    ]
    for path in file_paths:
        if not path.is_file():
            pytest.fail(f"Missing upload fixture: {path}")

    with step("open login page"):
        page.goto(login_url, wait_until="domcontentloaded")

    form, _ = active_auth_context()
    email_input = form.locator("input[autocomplete='email']")
    password_input = form.locator("input[type='password']")
    with step("fill credentials"):
        expect(email_input).to_have_count(1)
        expect(password_input).to_have_count(1)
        email_input.fill(email)
        password_input.fill(password)
        password_input.blur()

    with step("submit login"):
        submit_button = form.locator("button[type='submit']")
        expect(submit_button).to_have_count(1)
        auth_click(submit_button, "submit_login")

    with step("wait for login"):
        _wait_for_login_complete(page)

    with step("open datasets"):
        page.goto(urljoin(base_url.rstrip("/") + "/", "/"), wait_until="domcontentloaded")
        nav_button = page.locator("button", has_text=re.compile(r"^Dataset$", re.I))
        if nav_button.count() > 0:
            nav_button.first.click()
        else:
            page.goto(urljoin(base_url.rstrip("/") + "/", "/datasets"), wait_until="domcontentloaded")
    snap("datasets_open")

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
    snap("dataset_created")
    snap("dataset_detail_ready")

    filenames = [path.name for path in file_paths]
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

            _capture_response(
                page,
                trigger,
                lambda resp: resp.request.method == "POST"
                and "/v1/document/upload" in resp.url,
            )
            expect(upload_modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
        snap(f"upload_{filename}_submitted")

        row = page.locator("tbody tr", has_text=filename).first
        expect(row).to_be_visible(timeout=RESULT_TIMEOUT_MS)

        with step(f"wait for parse success {filename}"):
            _wait_for_success_dot(page, filename, timeout_ms=RESULT_TIMEOUT_MS)
            dot_button = row.locator(
                "button", has=row.locator("span.size-1.inline-block.rounded")
            ).first
            if dot_button.count() > 0:
                dot_button.click()
                dialog = page.locator("[role='dialog']").filter(
                    has_text=re.compile("status", re.I)
                )
                dialog_visible = True
                try:
                    expect(dialog).to_be_visible(timeout=5000)
                except PlaywrightTimeoutError:
                    dialog_visible = False
                if dialog_visible:
                    if dialog.locator("text=Success").count() > 0:
                        expect(dialog.locator("text=Success")).to_be_visible()
                    close_button = dialog.locator(
                        "button", has_text=re.compile("close", re.I)
                    )
                    if close_button.count() > 0:
                        close_button.first.click()
        snap(f"parse_{filename}_success")

    delete_filename = "Doc3.pdf"
    with step(f"delete uploaded file {delete_filename}"):
        row = page.locator("tbody tr", has_text=delete_filename).first
        expect(row).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        row.hover()
        delete_button = row.locator("td").last.locator("button").last
        delete_button.click()
        confirm = page.locator("[role='alertdialog']")
        expect(confirm).to_be_visible()
        confirm.locator("button", has_text=re.compile("^delete$", re.I)).first.click()
        expect(row).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
    snap("file_deleted_doc3")
    expect(page.locator("tbody tr", has_text="Doc1.pdf").first).to_be_visible(
        timeout=RESULT_TIMEOUT_MS
    )
    expect(page.locator("tbody tr", has_text="Doc2.pdf").first).to_be_visible(
        timeout=RESULT_TIMEOUT_MS
    )
    snap("success")
