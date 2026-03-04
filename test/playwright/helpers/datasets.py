import json
import re

from playwright.sync_api import TimeoutError as PlaywrightTimeoutError

from test.playwright.helpers.debug_utils import debug
from test.playwright.helpers.env_utils import env_bool


def wait_for_dataset_detail(page, timeout_ms: int) -> None:
    """Wait for dataset detail path to appear in the URL."""
    wait_js = """
        () => {
          const path = window.location.pathname || '';
          return /^\\/datasets\\/.+/.test(path) || /^\\/dataset\\/dataset\\/.+/.test(path);
        }
        """
    page.wait_for_function(wait_js, timeout=timeout_ms)


def wait_for_dataset_detail_ready(page, expect, timeout_ms: int) -> None:
    """Wait for dataset detail UI to become ready/visible."""
    wait_for_dataset_detail(page, timeout_ms=timeout_ms)
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


def upload_file(page, expect, dialog, file_path: str, timeout_ms: int) -> None:
    """Upload a file from the dataset upload modal."""
    dropzone = dialog.locator("[data-testid='dataset-upload-dropzone']").first
    expect(dropzone).to_be_visible(timeout=timeout_ms)
    if hasattr(page, "expect_file_chooser"):
        with page.expect_file_chooser() as chooser_info:
            dropzone.click()
        chooser_info.value.set_files(file_path)
        return
    input_locator = dialog.locator("input[type='file']")
    if input_locator.count() == 0:
        raise AssertionError("File chooser not available and no input[type='file'] found.")
    input_locator.first.set_input_files(file_path)


def wait_for_success_dot(page, expect, file_name: str, timeout_ms: int) -> None:
    """Wait for the parse success dot to show for a file row."""
    name_selector = f"[data-doc-name={json.dumps(file_name)}]"
    row = page.locator(f"[data-testid='document-row']{name_selector}")
    expect(row).to_be_visible(timeout=timeout_ms)
    status = row.locator("[data-testid='document-parse-status']")
    expect(status).to_have_attribute("data-state", "success", timeout=timeout_ms)


def dump_clickable_candidates(page) -> None:
    """Dump a short list of visible clickable UI candidates for debugging."""
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


def get_upload_modal(page):
    """Return the dataset upload modal locator."""
    return page.locator("[data-testid='dataset-upload-modal']")


def ensure_upload_modal_open(page, expect, auth_click, timeout_ms: int):
    """Ensure the dataset upload modal is visible, opening it if needed."""
    modal = get_upload_modal(page)
    if modal.count() > 0:
        try:
            expect(modal).to_be_visible(timeout=timeout_ms)
            return modal
        except AssertionError:
            pass
    return open_upload_modal_from_dataset_detail(
        page, expect, auth_click, timeout_ms=timeout_ms
    )


def ensure_parse_on(upload_modal, expect) -> None:
    """Enable parse-on-creation toggle in the upload modal."""
    parse_switch = upload_modal.locator("[data-testid='parse-on-creation-toggle']").first
    expect(parse_switch).to_be_visible()
    state = parse_switch.get_attribute("data-state")
    if state == "checked":
        return
    parse_switch.click()
    expect(parse_switch).to_have_attribute("data-state", "checked")


def open_upload_modal_from_dataset_detail(page, expect, auth_click, timeout_ms: int):
    """Open the upload modal from dataset detail view."""
    wait_for_dataset_detail_ready(page, expect, timeout_ms=timeout_ms)
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


def select_chunking_method_general(page, expect, modal, timeout_ms: int) -> None:
    """Select the General chunking method inside the dataset modal."""
    trigger_locator = modal.locator(
        "button",
        has=modal.locator(
            "span", has_text=re.compile(r"please select a chunking method\\.", re.I)
        ),
    ).first
    if trigger_locator.count() == 0:
        label = modal.locator("text=/please select a chunking method\\./i").first
        if label.count() > 0:
            trigger_locator = label.locator("xpath=ancestor::button[1]").first
        if trigger_locator.count() == 0:
            trigger_locator = modal.locator(
                "button",
                has_text=re.compile(r"please select a chunking method\\.", re.I),
            ).first

    if trigger_locator.count() == 0:
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
                "trigger_handle_found=False"
            )
            debug(f"[dataset] modal_text_snippet={modal_text[:300]!r}")
        raise AssertionError("Chunking method dropdown trigger not found.")

    trigger_for_assert = trigger_locator
    expect(trigger_locator).to_be_visible(timeout=timeout_ms)
    try:
        trigger_locator.click()
    except Exception:
        trigger_locator.click(force=True)
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
    expect(option).to_be_visible(timeout=timeout_ms)
    option.click()
    if trigger_for_assert is not None:
        try:
            expect(trigger_for_assert).to_contain_text(
                re.compile(r"General", re.I), timeout=timeout_ms
            )
        except AssertionError:
            # Trigger can rerender after selection; verify selected label in modal instead.
            expect(modal).to_contain_text(re.compile(r"General", re.I), timeout=timeout_ms)


def open_create_dataset_modal(page, expect, timeout_ms: int):
    """Open the create dataset modal from the datasets page."""
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
        page.wait_for_function(wait_js, timeout=timeout_ms)
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
            dump_clickable_candidates(page)
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
            dump_clickable_candidates(page)
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
                dump_clickable_candidates(page)
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
    expect(modal).to_be_visible(timeout=timeout_ms)
    return modal


def delete_uploaded_file(page, expect, filename: str, timeout_ms: int) -> None:
    """Delete a document row by filename and confirm the modal."""
    row = page.locator(
        f"[data-testid='document-row'][data-doc-name={json.dumps(filename)}]"
    )
    expect(row).to_be_visible(timeout=timeout_ms)
    delete_button = row.locator("[data-testid='document-delete']")
    expect(delete_button).to_be_visible(timeout=timeout_ms)
    delete_button.click()
    confirm = page.locator("[role='alertdialog']")
    expect(confirm).to_be_visible()
    confirm_delete = confirm.locator(
        "button", has_text=re.compile("^delete$", re.I)
    ).first
    expect(confirm_delete).to_be_visible(timeout=timeout_ms)
    try:
        confirm_delete.click(timeout=timeout_ms)
    except Exception:
        # The confirm button can rerender during open/animation; reacquire and force.
        confirm_delete = confirm.locator(
            "button", has_text=re.compile("^delete$", re.I)
        ).first
        confirm_delete.click(timeout=timeout_ms, force=True)
    expect(row).not_to_be_visible(timeout=timeout_ms)
