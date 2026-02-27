import json
import re
from urllib.parse import urljoin

from playwright.sync_api import TimeoutError as PlaywrightTimeoutError

from test.playwright.helpers.debug_utils import debug
from test.playwright.helpers.response_capture import capture_response


def wait_for_path_prefix(page, prefix: str, timeout_ms: int) -> None:
    """Wait until the URL path starts with the provided prefix."""
    prefix_json = json.dumps(prefix)
    wait_js = f"""
        () => {{
          const prefix = {prefix_json};
          const path = window.location.pathname || '';
          return path.startsWith(prefix);
        }}
        """
    page.wait_for_function(wait_js, timeout=timeout_ms)


def safe_close_modal(modal) -> None:
    """Best-effort close for API key modal."""
    try:
        api_input = modal.locator("input").first
        if api_input.count() > 0:
            api_input.fill("")
    except Exception as exc:
        debug(f"[model-providers] failed to clear api input: {exc}")
    try:
        cancel_button = modal.locator("button", has_text=re.compile("cancel", re.I))
        if cancel_button.count() > 0:
            cancel_button.first.click()
            return
    except Exception as exc:
        debug(f"[model-providers] cancel modal click failed: {exc}")
    try:
        close_button = modal.locator("button", has=modal.locator("svg")).first
        if close_button.count() > 0:
            close_button.click()
    except Exception as exc:
        debug(f"[model-providers] close modal click failed: {exc}")


def open_user_settings(page, base_url: str) -> None:
    """Navigate to the user settings page with fallback paths."""
    entrypoint = page.locator("[data-testid='settings-entrypoint']")
    if entrypoint.count() > 0:
        entrypoint.first.click()
        wait_for_path_prefix(page, "/user-setting", timeout_ms=5000)
        return

    header = page.locator("section").filter(has=page.locator("img[alt='logo']")).first
    candidates = [
        page.locator("a[href='/user-setting']"),
        page.locator("text=User settings"),
        header.locator("img:not([alt='logo'])"),
    ]

    for candidate in candidates:
        debug(f"[model-providers] settings candidate count={candidate.count()}")
        if candidate.count() == 0:
            continue
        try:
            candidate.first.click()
            wait_for_path_prefix(page, "/user-setting", timeout_ms=5000)
            return
        except PlaywrightTimeoutError:
            continue
        except Exception as exc:
            debug(f"[model-providers] settings click failed: {exc}")

    fallback_url = urljoin(base_url.rstrip("/") + "/", "/user-setting")
    page.goto(fallback_url, wait_until="domcontentloaded")
    wait_for_path_prefix(page, "/user-setting", timeout_ms=5000)


def needs_selection(combobox, option_text: str) -> bool:
    """Return True when the combobox does not already show the option text."""
    current_text = combobox.inner_text().strip()
    return option_text not in current_text


def click_with_retry(page, expect, locator_factory, attempts: int, timeout_ms: int) -> None:
    """Click a locator with retries and visibility checks."""
    last_exc = None
    for _ in range(attempts):
        option = locator_factory()
        try:
            expect(option).to_be_attached(timeout=timeout_ms)
            expect(option).to_be_visible(timeout=timeout_ms)
            option.scroll_into_view_if_needed()
            option.click()
            return
        except Exception as exc:
            last_exc = exc
            page.wait_for_timeout(100)
    raise AssertionError(f"Click failed after {attempts} attempts: {last_exc}")


def select_cmdk_option_by_value_prefix(
    page,
    expect,
    combobox,
    value_prefix: str,
    option_text: str,
    list_testid: str,
    fallback_to_first: bool,
    timeout_ms: int,
) -> tuple[str, str | None]:
    """Select a cmdk option by value prefix or option text."""
    combobox.click()

    controls_id = combobox.get_attribute("aria-controls")
    options_container = None
    option_selector = (
        "[data-testid='combobox-option'], [role='option'], [cmdk-item], [data-value]"
    )

    if controls_id:
        controls_selector = f"[id={json.dumps(controls_id)}]:visible"
        scoped = page.locator(controls_selector)
        if scoped.count() > 0:
            options_container = scoped.first

    if options_container is None and list_testid:
        legacy_container = page.locator(f"[data-testid='{list_testid}']:visible")
        if legacy_container.count() > 0:
            options_container = legacy_container.first

    escaped_prefix = value_prefix.replace("'", "\\'")
    value_selector = f"[data-value^='{escaped_prefix}']"
    option_pattern = re.compile(rf"\b{re.escape(option_text)}\b", re.I)

    def options_locator():
        if options_container is not None:
            return options_container.locator(option_selector)
        return page.locator(option_selector)

    def option_locator():
        by_value = (
            options_container.locator(value_selector)
            if options_container is not None
            else page.locator(f"{value_selector}:visible")
        )
        if by_value.count() > 0:
            return by_value.first
        return options_locator().filter(has_text=option_pattern).first

    expect(options_locator().first).to_be_visible(timeout=timeout_ms)

    option = option_locator()
    if option.count() == 0:
        options = options_locator()
        if fallback_to_first and options.count() > 0:
            first_option = options.first
            selected_text = ""
            selected_value = None
            try:
                selected_text = first_option.inner_text().strip()
            except Exception:
                selected_text = ""
            try:
                selected_value = first_option.get_attribute("data-value")
            except Exception:
                selected_value = None
            click_with_retry(page, expect, lambda: first_option, attempts=3, timeout_ms=timeout_ms)
            if selected_text:
                expect(combobox).to_contain_text(
                    selected_text, timeout=timeout_ms
                )
            try:
                expect(combobox).to_have_attribute(
                    "aria-expanded", "false", timeout=timeout_ms
                )
            except AssertionError:
                page.keyboard.press("Escape")
                expect(combobox).to_have_attribute(
                    "aria-expanded", "false", timeout=timeout_ms
                )
            return selected_text or option_text, selected_value
        dump = []
        count = min(options.count(), 30)
        for i in range(count):
            item = options.nth(i)
            try:
                text = item.inner_text().strip()
            except Exception as exc:
                text = f"<text-error:{exc}>"
            try:
                data_value = item.get_attribute("data-value")
            except Exception as exc:
                data_value = f"<value-error:{exc}>"
            dump.append(f"{i + 1:02d}. text={text!r} data-value={data_value!r}")
        dump_text = "\n".join(dump)
        raise AssertionError(
            "No matching cmdk option found. "
            f"value_prefix={value_prefix!r} option_text={option_text!r} "
            f"list_testid={list_testid!r} aria_controls={controls_id!r} "
            f"options_count={options.count()}\n"
            f"options:\n{dump_text}"
        )

    selected_text = option_text
    try:
        selected_text = option.inner_text().strip() or option_text
    except Exception:
        selected_text = option_text
    selected_value = option.get_attribute("data-value")
    click_with_retry(page, expect, option_locator, attempts=3, timeout_ms=timeout_ms)
    expect(combobox).to_contain_text(selected_text, timeout=timeout_ms)
    try:
        expect(combobox).to_have_attribute("aria-expanded", "false", timeout=timeout_ms)
    except AssertionError:
        page.keyboard.press("Escape")
        expect(combobox).to_have_attribute("aria-expanded", "false", timeout=timeout_ms)
    return selected_text, selected_value


def select_default_model(
    page,
    expect,
    combobox,
    value_prefix: str,
    option_text: str,
    list_testid: str,
    fallback_to_first: bool,
    timeout_ms: int,
) -> tuple[str, str | None]:
    """Select and persist a default model."""
    if not needs_selection(combobox, option_text):
        try:
            current_text = combobox.inner_text().strip()
        except Exception:
            current_text = option_text
        return current_text, None

    selected = ("", None)

    def trigger():
        nonlocal selected
        selected = select_cmdk_option_by_value_prefix(
            page,
            expect,
            combobox,
            value_prefix,
            option_text,
            list_testid,
            fallback_to_first=fallback_to_first,
            timeout_ms=timeout_ms,
        )

    try:
        capture_response(
            page,
            trigger,
            lambda resp: resp.request.method == "POST"
            and "/v1/user/set_tenant_info" in resp.url,
        )
    except PlaywrightTimeoutError:
        if not selected[0]:
            raise

    expected_text = selected[0] or option_text
    expect(combobox).to_contain_text(expected_text, timeout=timeout_ms)
    return selected
