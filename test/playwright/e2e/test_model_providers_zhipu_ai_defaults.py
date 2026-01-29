import json
import os
import re
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


def _wait_for_path_prefix(page, prefix: str, timeout_ms: int = RESULT_TIMEOUT_MS) -> None:
    prefix_json = json.dumps(prefix)
    wait_js = f"""
        () => {{
          const prefix = {prefix_json};
          const path = window.location.pathname || '';
          return path.startsWith(prefix);
        }}
        """
    page.wait_for_function(wait_js, timeout=timeout_ms)


def _safe_close_modal(modal) -> None:
    try:
        api_input = modal.locator("input").first
        if api_input.count() > 0:
            api_input.fill("")
    except Exception as exc:
        _debug(f"[model-providers] failed to clear api input: {exc}")
    try:
        cancel_button = modal.locator("button", has_text=re.compile("cancel", re.I))
        if cancel_button.count() > 0:
            cancel_button.first.click()
            return
    except Exception as exc:
        _debug(f"[model-providers] cancel modal click failed: {exc}")
    try:
        close_button = modal.locator("button", has=modal.locator("svg")).first
        if close_button.count() > 0:
            close_button.click()
    except Exception as exc:
        _debug(f"[model-providers] close modal click failed: {exc}")


def _open_user_settings(page, base_url: str) -> None:
    header = page.locator("section").filter(has=page.locator("img[alt='logo']")).first
    candidates = [
        page.locator("a[href='/user-setting']"),
        page.locator("text=User settings"),
        header.locator("img:not([alt='logo'])"),
    ]

    for candidate in candidates:
        _debug(f"[model-providers] settings candidate count={candidate.count()}")
        if candidate.count() == 0:
            continue
        try:
            candidate.first.click()
            _wait_for_path_prefix(page, "/user-setting", timeout_ms=5000)
            return
        except PlaywrightTimeoutError:
            continue
        except Exception as exc:
            _debug(f"[model-providers] settings click failed: {exc}")

    fallback_url = urljoin(base_url.rstrip("/") + "/", "/user-setting")
    page.goto(fallback_url, wait_until="domcontentloaded")
    _wait_for_path_prefix(page, "/user-setting")


def _needs_selection(combobox, option_text: str) -> bool:
    current_text = combobox.inner_text().strip()
    return option_text not in current_text


def _select_with_search(page, combobox, search_text: str, option_text: str) -> None:
    combobox.click()
    search_inputs = page.locator("input[placeholder='Search...'], input[placeholder='Search…']")
    if search_inputs.count() > 0:
        search_inputs.last.fill(search_text)
    wait_js = """
        () => {
          const selectors = ['[role="listbox"]', '[cmdk-list]', '[data-state="open"]'];
          return selectors.some((sel) => Array.from(document.querySelectorAll(sel)).some((el) => {
            const rect = el.getBoundingClientRect();
            return rect.width > 0 && rect.height > 0;
          }));
        }
        """
    page.wait_for_function(wait_js, timeout=RESULT_TIMEOUT_MS)

    last_exc = None
    option_pattern = re.compile(re.escape(option_text), re.I)
    for _ in range(5):
        try:
            if hasattr(page, "get_by_role"):
                option = page.get_by_role("option", name=option_pattern).first
                if option.count() == 0:
                    option = page.locator("[cmdk-item], [role='option']").filter(
                        has_text=option_pattern
                    ).first
            else:
                option = page.locator("[cmdk-item], [role='option']").filter(
                    has_text=option_pattern
                ).first
            expect(option).to_be_attached(timeout=RESULT_TIMEOUT_MS)
            expect(option).to_be_visible(timeout=RESULT_TIMEOUT_MS)
            try:
                option.click()
            except Exception as exc:
                last_exc = exc
                try:
                    option.scroll_into_view_if_needed()
                except Exception as scroll_exc:
                    last_exc = scroll_exc
                try:
                    option.click(trial=True)
                    option.click()
                except Exception as click_exc:
                    last_exc = click_exc
                    option.click(force=True)
            expect(combobox).to_contain_text(option_text, timeout=RESULT_TIMEOUT_MS)
            return
        except Exception as exc:
            last_exc = exc
    raise AssertionError(f"Failed to select option {option_text!r}: {last_exc}")


@pytest.mark.p1
@pytest.mark.auth
def test_add_zhipu_ai_set_defaults_persist(
    base_url,
    login_url,
    page,
    active_auth_context,
    step,
    snap,
    auth_click,
):
    api_key = os.getenv("ZHIPU_AI_API_KEY")
    if not api_key:
        pytest.skip("ZHIPU_AI_API_KEY not set; skipping model providers test.")

    email = os.getenv("SEEDED_USER_EMAIL")
    password = os.getenv("SEEDED_USER_PASSWORD")
    if not email or not password:
        pytest.skip("SEEDED_USER_EMAIL/SEEDED_USER_PASSWORD not set.")

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

    snap("home_loaded")

    with step("open settings"):
        _open_user_settings(page, base_url)
    snap("settings_opened")

    with step("open model providers"):
        model_nav = page.locator("button", has_text="Model providers")
        expect(model_nav).to_have_count(1)
        model_nav.first.click()
        expect(page.locator("text=Set default models")).to_be_visible()
    snap("model_providers_open")

    with step("filter providers"):
        search_input = page.locator("input[placeholder='Search'], input[placeholder='search']")
        expect(search_input).to_have_count(1)
        search_input.first.fill("zhipu")
        available_section = page.locator("text=Available models").first.locator("xpath=..")
        provider = available_section.locator("text=ZHIPU-AI").first
        if provider.count() == 0:
            added_section = page.locator("text=Added models").first.locator("xpath=..")
            if added_section.locator("text=ZHIPU-AI").count() == 0:
                raise AssertionError("ZHIPU-AI provider not found in available or added models.")
        else:
            expect(provider).to_be_visible()
    snap("provider_filtered")

    with step("add ZHIPU-AI api key"):
        if provider.count() > 0:
            provider.click()
        else:
            added_section = page.locator("text=Added models").first.locator("xpath=..")
            card = added_section.locator(
                "div",
                has=page.locator("text=ZHIPU-AI"),
            ).filter(has=page.locator("button", has_text=re.compile("API-?Key", re.I))).first
            api_key_button = card.locator("button", has_text=re.compile("API-?Key", re.I)).first
            expect(api_key_button).to_be_visible()
            api_key_button.click()
        modal = page.locator("[role='dialog']").filter(
            has=page.locator("text=API-Key")
        )
        expect(modal).to_be_visible()
        api_input = modal.locator("input").first
        save_button = modal.locator("button", has_text=re.compile("save", re.I)).first
        try:
            def trigger():
                api_input.fill(api_key)
                save_button.click()

            _capture_response(
                page,
                trigger,
                lambda resp: resp.request.method == "POST" and "/v1/llm/set_api_key" in resp.url,
            )
            expect(modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
        except Exception:
            _safe_close_modal(modal)
            raise

    with step("confirm added model"):
        added_section = page.locator("text=Added models").first.locator("xpath=..")
        expect(added_section).to_be_visible()
        expect(added_section.locator("text=ZHIPU-AI")).to_be_visible()
    snap("provider_saved")

    with step("set default models"):
        llm_row = page.locator("label", has_text="LLM").first.locator("xpath=..")
        llm_combo = llm_row.locator("button[role='combobox']").first

        if _needs_selection(llm_combo, "glm-4-flash"):
            def llm_trigger():
                _select_with_search(page, llm_combo, "glm-4-flash", "glm-4-flash")

            _capture_response(
                page,
                llm_trigger,
                lambda resp: resp.request.method == "POST" and "/v1/user/set_tenant_info" in resp.url,
            )

        emb_row = page.locator("label", has_text="Embedding").first.locator("xpath=..")
        emb_combo = emb_row.locator("button[role='combobox']").first

        if _needs_selection(emb_combo, "embedding-2"):
            def emb_trigger():
                _select_with_search(page, emb_combo, "embedding-2", "embedding-2")

            _capture_response(
                page,
                emb_trigger,
                lambda resp: resp.request.method == "POST" and "/v1/user/set_tenant_info" in resp.url,
            )

    snap("defaults_selected")

    with step("reload and verify defaults"):
        page.reload(wait_until="domcontentloaded")
        expect(page.locator("text=Set default models")).to_be_visible()
        llm_combo = page.locator("label", has_text="LLM").first.locator("xpath=..").locator("button[role='combobox']").first
        emb_combo = page.locator("label", has_text="Embedding").first.locator("xpath=..").locator("button[role='combobox']").first
        expect(llm_combo).to_contain_text("glm-4-flash")
        expect(emb_combo).to_contain_text("embedding-2")
        added_section = page.locator("text=Added models").first.locator("xpath=..")
        expect(added_section.locator("text=ZHIPU-AI")).to_be_visible()
    snap("defaults_persisted")
    snap("success")
