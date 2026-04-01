import re
import time
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
    expected_path_map = {
        "nav-chat": "/chats",
        "nav-search": "/searches",
        "nav-agent": "/agents",
    }
    expected_path = expected_path_map.get(testid)

    def _ensure_expected_path():
        if not expected_path:
            return
        if expected_path in page.url:
            return
        try:
            page.wait_for_url(
                re.compile(rf".*{re.escape(expected_path)}(?:[/?#].*)?$"),
                wait_until="domcontentloaded",
                timeout=5000,
            )
        except Exception:
            page.goto(expected_path, wait_until="domcontentloaded")

    locator = page.locator(f"[data-testid='{testid}']")
    if locator.count() > 0:
        expect(locator.first).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        locator.first.click()
        _ensure_expected_path()
        return

    nav_text_map = {
        "nav-chat": "chat",
        "nav-search": "search",
        "nav-agent": "agent",
    }
    label = nav_text_map.get(testid)
    if label:
        pattern = re.compile(rf"^{re.escape(label)}$", re.I)
        fallback = page.get_by_role("button", name=pattern)
        if fallback.count() == 0:
            top_nav = page.locator("[data-testid='top-nav']")
            if top_nav.count() > 0:
                fallback = top_nav.first.get_by_text(pattern)
            else:
                fallback = page.get_by_text(pattern)
        if fallback.count() == 0:
            fallback = page.locator("button, [role='button'], a, span, div").filter(
                has_text=pattern
            )
        expect(fallback.first).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        fallback.first.click()
        _ensure_expected_path()
        return

    expect(locator).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    locator.click()
    _ensure_expected_path()


def _open_create_from_list(
    page,
    empty_testid: str,
    create_btn_testid: str,
    modal_testid: str = "rename-modal",
):
    empty = page.locator(f"[data-testid='{empty_testid}']")
    if empty.count() > 0 and empty.first.is_visible():
        empty.first.click()
    else:
        create_btn = page.locator(f"[data-testid='{create_btn_testid}']")
        if create_btn.count() > 0:
            expect(create_btn.first).to_be_visible(timeout=RESULT_TIMEOUT_MS)
            create_btn.first.click()
        else:
            create_text_map = {
                "create-chat": r"create\s+chat",
                "create-search": r"create\s+search",
                "create-agent": r"create\s+agent",
            }
            pattern = create_text_map.get(create_btn_testid)
            clicked = False
            if pattern:
                fallback_btn = page.get_by_role(
                    "button", name=re.compile(pattern, re.I)
                )
                if fallback_btn.count() > 0 and fallback_btn.first.is_visible():
                    fallback_btn.first.click()
                    clicked = True

            if not clicked:
                empty_text_map = {
                    "chats-empty-create": r"no chat app created yet",
                    "search-empty-create": r"no search app created yet",
                    "agents-empty-create": r"no agent",
                }
                empty_pattern = empty_text_map.get(empty_testid)
                if empty_pattern:
                    empty_state = page.locator("div, section, article").filter(
                        has_text=re.compile(empty_pattern, re.I)
                    )
                    if empty_state.count() > 0 and empty_state.first.is_visible():
                        empty_state.first.click()
                        clicked = True

            if not clicked:
                fallback_card = page.locator(
                    ".border-dashed, [class*='border-dashed']"
                ).first
                expect(fallback_card).to_be_visible(timeout=RESULT_TIMEOUT_MS)
                fallback_card.click()
    if modal_testid == "agent-create-modal":
        menu = page.locator("[data-testid='agent-create-menu']")
        if menu.count() > 0 and menu.first.is_visible():
            create_blank = menu.locator("text=/create from blank/i")
            if create_blank.count() > 0 and create_blank.first.is_visible():
                create_blank.first.click()
            else:
                first_item = menu.locator("[role='menuitem']").first
                expect(first_item).to_be_visible(timeout=RESULT_TIMEOUT_MS)
                first_item.click()
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


def _search_query_input(page):
    candidates = [
        page.locator("[data-testid='search-query-input']"),
        page.locator("input[placeholder*='How can I help you today']"),
        page.locator("input[placeholder*='help you today']"),
    ]
    for candidate in candidates:
        if candidate.count() > 0:
            return candidate.first
    return page.locator("input[type='text']").first


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

    def _find_dataset_combobox(search_scope):
        combo = search_scope.locator(f"[data-testid='{combobox_testid}']")
        if combo.count() > 0:
            return combo
        combo = search_scope.locator("[role='combobox']").filter(
            has_text=re.compile(r"select|dataset|please", re.I)
        )
        if combo.count() > 0:
            return combo
        return search_scope.locator("[role='combobox']")

    combobox = _find_dataset_combobox(scope_root)
    if combobox.count() == 0:
        settings_candidates = [
            scope_root.locator("button:has(svg.lucide-settings)"),
            scope_root.locator("button:has(svg[class*='settings'])"),
            scope_root.locator("[data-testid='chat-settings']"),
            scope_root.locator("[data-testid='search-settings']"),
            scope_root.locator("button", has_text=re.compile(r"search settings", re.I)),
            scope_root.locator("button", has=scope_root.locator("svg.lucide-settings")),
            page.locator("button:has(svg.lucide-settings)"),
            page.locator("button", has_text=re.compile(r"search settings", re.I)),
        ]
        for settings_button in settings_candidates:
            if settings_button.count() == 0:
                continue
            if not settings_button.first.is_visible():
                continue
            settings_button.first.click()
            break

        settings_dialog = page.locator("[role='dialog']").filter(
            has_text=re.compile(r"settings", re.I)
        )
        if settings_dialog.count() > 0 and settings_dialog.first.is_visible():
            scope_root = settings_dialog.first
        combobox = _find_dataset_combobox(scope_root)

    combobox = combobox.first
    expect(combobox).to_be_visible(timeout=timeout_ms)
    combo_text = ""
    try:
        combo_text = combobox.inner_text()
    except Exception:
        combo_text = ""
    if combo_text and not re.search(r"please\s+select|select", combo_text, re.I):
        return

    save_button = scope_root.locator(f"[data-testid='{save_testid}']")
    if save_button.count() == 0:
        save_button = scope_root.get_by_role(
            "button", name=re.compile(r"^save$", re.I)
        )
    if save_button.count() == 0:
        save_button = scope_root.locator(
            "button[type='submit']", has_text=re.compile(r"^save$", re.I)
        ).first
    save_button = save_button.first
    expect(save_button).to_be_visible(timeout=timeout_ms)

    def _open_dataset_options():
        last_list_text = ""
        for _ in range(10):
            candidates = [
                page.locator("[data-testid='datasets-options']:visible"),
                page.locator("[role='listbox']:visible"),
                page.locator("[cmdk-list]:visible"),
            ]
            for candidate in candidates:
                if candidate.count() > 0:
                    options_root = candidate.first
                    expect(options_root).to_be_visible(timeout=timeout_ms)
                    return options_root, last_list_text

            combobox.click()
            page.wait_for_timeout(120)

            list_locator = page.locator("[data-testid='datasets-options']").first
            if list_locator.count() > 0:
                try:
                    last_list_text = list_locator.inner_text() or ""
                except Exception:
                    last_list_text = ""
        raise AssertionError(
            "Dataset option popover did not open. "
            f"combobox_testid={combobox_testid!r} last_list_text={last_list_text[:200]!r}"
        )

    def _pick_first_dataset_option(options_root) -> bool:
        search_input = options_root.locator("[cmdk-input], input[placeholder*='Search']").first
        if search_input.count() > 0:
            try:
                search_input.fill("")
                search_input.focus()
            except Exception:
                pass
            page.wait_for_timeout(100)

        selectors = [
            "[data-testid^='datasets-option-']:not([aria-disabled='true']):not([data-disabled='true'])",
            "[role='option']:not([aria-disabled='true']):not([data-disabled='true'])",
            "[cmdk-item]:not([aria-disabled='true']):not([data-disabled='true'])",
        ]
        for selector in selectors:
            candidates = options_root.locator(selector)
            if candidates.count() == 0:
                continue
            limit = min(candidates.count(), 20)
            for idx in range(limit):
                candidate = candidates.nth(idx)
                try:
                    if not candidate.is_visible():
                        continue
                    text = (candidate.inner_text() or "").strip().lower()
                except Exception:
                    continue
                if (
                    not text
                    or "no results found" in text
                    or text == "close"
                    or text == "clear"
                ):
                    continue
                for _ in range(3):
                    try:
                        candidate.click(timeout=2000)
                        return True
                    except Exception:
                        try:
                            candidate.click(timeout=2000, force=True)
                            return True
                        except Exception:
                            page.wait_for_timeout(100)
                break

        try:
            if search_input.count() > 0:
                search_input.focus()
            else:
                combobox.focus()
            page.keyboard.press("ArrowDown")
            page.keyboard.press("Enter")
            return True
        except Exception:
            return False

    def _parse_request_payload(req) -> dict:
        try:
            payload = req.post_data_json
            if callable(payload):
                payload = payload()
            if isinstance(payload, dict):
                return payload
        except Exception:
            pass
        return {}

    def _has_selected_kb_ids(payload: dict) -> bool:
        if save_testid == "search-settings-save":
            search_config = payload.get("search_config", {})
            kb_ids = search_config.get("kb_ids")
            if not isinstance(kb_ids, list):
                kb_ids = payload.get("kb_ids")
            return isinstance(kb_ids, list) and len(kb_ids) > 0
        kb_ids = payload.get("kb_ids")
        return isinstance(kb_ids, list) and len(kb_ids) > 0

    response_url_pattern = (
        "/dialog/set" if save_testid == "chat-settings-save" else "/api/v1/searches/"
    )
    last_payload = {}
    last_combobox_text = ""
    last_list_text = ""
    for attempt in range(5):
        options, last_list_text = _open_dataset_options()
        clicked = _pick_first_dataset_option(options)
        if not clicked:
            raise AssertionError(
                "Failed to select dataset option after retries. "
                f"list_text={last_list_text[:200]!r}"
            )

        page.wait_for_timeout(120)
        try:
            page.keyboard.press("Escape")
        except Exception:
            pass

        response = None
        try:
            response = capture_response(
                page,
                lambda: save_button.click(),
                lambda resp: response_url_pattern in resp.url
                and resp.request.method in ("POST", "PUT", "PATCH"),
                timeout_ms=response_timeout_ms,
            )
        except Exception:
            try:
                save_button.click()
            except Exception:
                pass

        payload = {}
        if response is not None:
            payload = _parse_request_payload(response.request)
        last_payload = payload
        if _has_selected_kb_ids(payload):
            if post_save_ready_locator is not None:
                expect(post_save_ready_locator).to_be_visible(timeout=timeout_ms)
            else:
                page.wait_for_timeout(250)
            return

        try:
            last_combobox_text = (combobox.inner_text() or "").strip()
        except Exception:
            last_combobox_text = ""
        page.wait_for_timeout(200 * (attempt + 1))

    raise AssertionError(
        "Dataset selection did not persist in save payload. "
        f"save_testid={save_testid!r} payload={last_payload!r} "
        f"combobox_text={last_combobox_text!r} list_text={last_list_text[:200]!r}"
    )


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
            # Some UI builds remove the stream-status marker when generation finishes.
            expect(page.locator("[data-testid='chat-stream-status']")).to_have_count(
                0, timeout=timeout_ms
            )
            return
        except Exception:
            pass
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
