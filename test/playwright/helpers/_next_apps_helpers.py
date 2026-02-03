import re
import time
from typing import Callable
from urllib.parse import urljoin

from playwright.sync_api import expect

from test.playwright.helpers.response_capture import capture_response

RESULT_TIMEOUT_MS = 15000


def _unique_name(prefix: str) -> str:
    return f"{prefix}-{int(time.time() * 1000)}"


def _assert_not_on_login(page) -> None:
    if "/login" in page.url or page.locator("input[autocomplete='email']").count() > 0:
        raise AssertionError(
            "Expected authenticated session; landed on /login. "
            "Ensure ensure_authed(...) was called and credentials are set."
        )


def _goto_home(page, base_url: str) -> None:
    page.goto(urljoin(base_url.rstrip("/") + "/", "/"), wait_until="domcontentloaded")
    _assert_not_on_login(page)


def _nav_click(page, testid: str) -> None:
    locator = page.locator(f"[data-testid='{testid}']")
    expect(locator).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    locator.click()


def _open_create_from_list(
    page,
    empty_testid: str,
    create_btn_testid: str,
    modal_testid: str = "rename-modal",
):
    empty = page.locator(f"[data-testid='{empty_testid}']")
    if empty.count() > 0 and empty.is_visible():
        empty.click()
    else:
        create_btn = page.locator(f"[data-testid='{create_btn_testid}']")
        expect(create_btn).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        create_btn.click()
    modal = page.locator(f"[data-testid='{modal_testid}']")
    expect(modal).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    return modal


def _fill_and_save_create_modal(
    page,
    name: str,
    modal_testid: str = "rename-modal",
    name_input_testid: str = "rename-name-input",
    save_testid: str = "rename-save",
) -> None:
    modal = page.locator(f"[data-testid='{modal_testid}']")
    expect(modal).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    name_input = modal.locator(f"[data-testid='{name_input_testid}']")
    expect(name_input).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    name_input.fill(name)
    save_button = modal.locator(f"[data-testid='{save_testid}']")
    expect(save_button).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    save_button.click()
    expect(modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)


def _select_first_dataset_and_save(
    page,
    timeout_ms: int = RESULT_TIMEOUT_MS,
    response_timeout_ms: int = 30000,
    post_save_ready_locator=None,
) -> None:
    chat_root = page.locator("[data-testid='chat-detail']")
    search_root = page.locator("[data-testid='search-detail']")
    scope_root = None
    combobox_testid = None
    save_testid = None
    try:
        if chat_root.count() > 0 and chat_root.is_visible():
            scope_root = chat_root
            combobox_testid = "chat-datasets-combobox"
            save_testid = "chat-settings-save"
    except Exception:
        pass
    if scope_root is None:
        try:
            if search_root.count() > 0 and search_root.is_visible():
                scope_root = search_root
                combobox_testid = "search-datasets-combobox"
                save_testid = "search-settings-save"
        except Exception:
            pass
    if scope_root is None:
        scope_root = page
        combobox_testid = "search-datasets-combobox"
        save_testid = "search-settings-save"

    combobox = scope_root.locator(f"[data-testid='{combobox_testid}']")
    expect(combobox).to_have_count(1, timeout=timeout_ms)
    expect(combobox).to_be_visible(timeout=timeout_ms)
    combo_text = ""
    try:
        combo_text = combobox.inner_text()
    except Exception:
        combo_text = ""
    if "please select" not in combo_text.lower():
        return

    combobox.click()

    options = page.locator("[data-testid='datasets-options']")
    expect(options).to_be_visible(timeout=timeout_ms)
    try:
        expect(options).to_have_count(1, timeout=timeout_ms)
        options = options.first
    except AssertionError:
        pass

    option = options.locator("[data-testid='datasets-option-0']")
    if option.count() == 0:
        option = options.locator("[data-testid^='datasets-option-']").first
    if option.count() == 0:
        option = options.locator("[data-testid='datasets-option']").first
    expect(option).to_be_visible(timeout=timeout_ms)
    option.click()

    save_button = scope_root.locator(f"[data-testid='{save_testid}']")
    if save_button.count() == 0:
        save_button = scope_root.locator(
            "button[type='submit']", has_text=re.compile(r"^save$", re.I)
        ).first
    else:
        expect(save_button).to_have_count(1, timeout=timeout_ms)

    def trigger():
        save_button.click()

    try:
        capture_response(
            page,
            trigger,
            lambda resp: "/v1/dialog/set" in resp.url
            and resp.request.method in ("POST", "PUT", "PATCH"),
            timeout_ms=response_timeout_ms,
        )
    except Exception:
        pass
    try:
        expect(options).not_to_be_visible(timeout=timeout_ms)
    except AssertionError:
        pass
    if post_save_ready_locator is not None:
        expect(post_save_ready_locator).to_be_visible(timeout=timeout_ms)
    else:
        page.wait_for_timeout(250)


def _send_chat_and_wait_done(
    page, text: str, timeout_ms: int = 60000
) -> None:
    textarea = page.locator("[data-testid='chat-textarea']")
    expect(textarea).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    tag_name = ""
    contenteditable = None
    try:
        tag_name = textarea.evaluate("el => el.tagName")
    except Exception:
        tag_name = ""
    try:
        contenteditable = textarea.get_attribute("contenteditable")
    except Exception:
        contenteditable = None

    is_input = tag_name in ("INPUT", "TEXTAREA")
    is_editable = is_input or contenteditable == "true"
    if not is_editable:
        raise AssertionError(
            "chat-textarea is not an editable element. "
            f"url={page.url} tag={tag_name!r} contenteditable={contenteditable!r}"
        )

    textarea.fill(text)
    typed_value = ""
    try:
        if is_input:
            typed_value = textarea.input_value()
        else:
            typed_value = textarea.inner_text()
    except Exception:
        typed_value = ""

    if text not in (typed_value or ""):
        textarea.click()
        page.keyboard.press("Control+A")
        page.keyboard.type(text)
        try:
            if is_input:
                typed_value = textarea.input_value()
            else:
                typed_value = textarea.inner_text()
        except Exception:
            typed_value = ""
        if text not in (typed_value or ""):
            raise AssertionError(
                "Failed to type prompt into chat-textarea. "
                f"url={page.url} tag={tag_name!r} contenteditable={contenteditable!r} "
                f"typed_value={typed_value!r}"
            )

    composer = textarea.locator("xpath=ancestor::form[1]")
    if composer.count() == 0:
        composer = textarea.locator("xpath=ancestor::div[1]")
    send_button = None
    if composer.count() > 0:
        if hasattr(composer, "get_by_role"):
            send_button = composer.get_by_role(
                "button", name=re.compile(r"send message", re.I)
            )
        if send_button is None or send_button.count() == 0:
            send_button = composer.locator(
                "button", has_text=re.compile(r"send message", re.I)
            )
    if send_button is not None and send_button.count() > 0:
        send_button.first.click()
        send_used = True
    else:
        textarea.press("Enter")
        send_used = False

    status_marker = page.locator("[data-testid='chat-stream-status']").first
    try:
        expect(status_marker).to_have_attribute(
            "data-status", "idle", timeout=timeout_ms
        )
    except Exception as exc:
        try:
            marker_count = page.locator("[data-testid='chat-stream-status']").count()
        except Exception:
            marker_count = -1
        try:
            status_value = status_marker.get_attribute("data-status")
        except Exception:
            status_value = None
        raise AssertionError(
            "Chat stream status marker not idle within timeout. "
            f"url={page.url} marker_count={marker_count} status={status_value!r} "
            f"tag={tag_name!r} contenteditable={contenteditable!r} "
            f"typed_value={typed_value!r} send_button_used={send_used}"
        ) from exc


def _wait_for_url_regex(page, pattern: str, timeout_ms: int = RESULT_TIMEOUT_MS) -> None:
    regex = re.compile(pattern)
    page.wait_for_url(regex, wait_until="commit", timeout=timeout_ms)


def _wait_for_url_or_testid(
    page, url_regex: str, testid: str, timeout_ms: int = RESULT_TIMEOUT_MS
) -> str:
    end_time = time.time() + (timeout_ms / 1000)
    regex = re.compile(url_regex)
    locator = page.locator(f"[data-testid='{testid}']")
    while time.time() < end_time:
        try:
            if regex.search(page.url):
                return "url"
        except Exception:
            pass
        try:
            if locator.count() > 0 and locator.is_visible():
                return "testid"
        except Exception:
            pass
        page.wait_for_timeout(100)
    raise AssertionError(
        f"Timed out waiting for url {url_regex!r} or testid {testid!r}. url={page.url}"
    )
