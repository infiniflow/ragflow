import json
import re
from urllib.parse import urljoin
import os
import pytest
from playwright.sync_api import TimeoutError as PlaywrightTimeoutError
from playwright.sync_api import expect

from test.playwright.helpers.flow_steps import flow_params, require
from helpers.auth_waits import wait_for_login_complete
from helpers.debug_utils import debug
from helpers.response_capture import capture_response

RESULT_TIMEOUT_MS = 15000


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


def _open_user_settings(page, base_url: str) -> None:
    entrypoint = page.locator("[data-testid='settings-entrypoint']")
    if entrypoint.count() > 0:
        entrypoint.first.click()
        _wait_for_path_prefix(page, "/user-setting", timeout_ms=5000)
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
            _wait_for_path_prefix(page, "/user-setting", timeout_ms=5000)
            return
        except PlaywrightTimeoutError:
            continue
        except Exception as exc:
            debug(f"[model-providers] settings click failed: {exc}")

    fallback_url = urljoin(base_url.rstrip("/") + "/", "/user-setting")
    page.goto(fallback_url, wait_until="domcontentloaded")
    _wait_for_path_prefix(page, "/user-setting")


def _needs_selection(combobox, option_text: str) -> bool:
    current_text = combobox.inner_text().strip()
    return option_text not in current_text


def _click_with_retry(page, locator_factory, attempts: int = 3) -> None:
    last_exc = None
    for _ in range(attempts):
        option = locator_factory()
        try:
            expect(option).to_be_attached(timeout=RESULT_TIMEOUT_MS)
            expect(option).to_be_visible(timeout=RESULT_TIMEOUT_MS)
            option.scroll_into_view_if_needed()
            option.click()
            return
        except Exception as exc:
            last_exc = exc
            page.wait_for_timeout(100)
    raise AssertionError(f"Click failed after {attempts} attempts: {last_exc}")


def _select_cmdk_option_by_value_prefix(
    page,
    combobox,
    value_prefix: str,
    option_text: str,
    list_testid: str,
    fallback_to_first: bool = False,
) -> tuple[str, str | None]:
    combobox.click()
    page.wait_for_selector(
        f"[data-testid='{list_testid}']:visible [data-testid='combobox-option']",
        timeout=RESULT_TIMEOUT_MS,
    )
    options_container = page.locator(
        f"[data-testid='{list_testid}']:visible"
    )
    expect(options_container).to_have_count(1, timeout=RESULT_TIMEOUT_MS)
    options_container = options_container.first

    escaped_prefix = value_prefix.replace("'", "\\'")
    value_selector = (
        f"[data-testid='combobox-option'][data-value^='{escaped_prefix}']"
    )
    option_pattern = re.compile(rf"\\b{re.escape(option_text)}\\b", re.I)

    def option_locator():
        by_value = options_container.locator(value_selector)
        if by_value.count() > 0:
            return by_value.first
        return options_container.locator("[data-testid='combobox-option']").filter(
            has_text=option_pattern
        ).first

    option = option_locator()
    if option.count() == 0:
        options = options_container.locator("[data-testid='combobox-option']")
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
            _click_with_retry(page, lambda: first_option, attempts=3)
            if selected_text:
                expect(combobox).to_contain_text(
                    selected_text, timeout=RESULT_TIMEOUT_MS
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
            f"options_count={options.count()}\n"
            f"options:\n{dump_text}"
        )

    _click_with_retry(page, option_locator, attempts=3)
    expect(combobox).to_contain_text(option_text, timeout=RESULT_TIMEOUT_MS)
    return option_text, option.get_attribute("data-value")


def _select_default_model(
    page,
    combobox,
    value_prefix: str,
    option_text: str,
    list_testid: str,
    fallback_to_first: bool = False,
) -> tuple[str, str | None]:
    if not _needs_selection(combobox, option_text):
        try:
            current_text = combobox.inner_text().strip()
        except Exception:
            current_text = option_text
        return current_text, None

    selected = ("", None)

    def trigger():
        nonlocal selected
        selected = _select_cmdk_option_by_value_prefix(
            page,
            combobox,
            value_prefix,
            option_text,
            list_testid,
            fallback_to_first=fallback_to_first,
        )

    try:
        capture_response(
            page,
            trigger,
            lambda resp: resp.request.method == "POST"
            and "/v1/user/set_tenant_info" in resp.url,
        )
    except PlaywrightTimeoutError:
        pass

    expected_text = selected[0] or option_text
    expect(combobox).to_contain_text(expected_text, timeout=RESULT_TIMEOUT_MS)
    return selected


def step_01_open_login(
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
    api_key = os.getenv("ZHIPU_AI_API_KEY")
    if not api_key:
        pytest.skip("ZHIPU_AI_API_KEY not set; skipping model providers test.")

    email, password = seeded_user_credentials

    flow_state["api_key"] = api_key
    flow_state["email"] = email
    flow_state["password"] = password

    with step("open login page"):
        flow_page.goto(login_url, wait_until="domcontentloaded")
    flow_state["login_opened"] = True
    snap("login_opened")


def step_02_login(
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
    require(flow_state, "login_opened", "email", "password")
    page = flow_page
    form, _ = active_auth_context()
    email_input = form.locator("input[data-testid='auth-email'], [data-testid='auth-email'] input")
    password_input = form.locator(
        "input[data-testid='auth-password'], [data-testid='auth-password'] input"
    )
    with step("fill credentials"):
        expect(email_input).to_have_count(1)
        expect(password_input).to_have_count(1)
        email_input.fill(flow_state["email"])
        password_input.fill(flow_state["password"])
        password_input.blur()

    with step("submit login"):
        submit_button = form.locator(
            "button[data-testid='auth-submit'], [data-testid='auth-submit'] button, [data-testid='auth-submit']"
        )
        expect(submit_button).to_have_count(1)
        auth_click(submit_button, "submit_login")

    with step("wait for login"):
        wait_for_login_complete(page, timeout_ms=RESULT_TIMEOUT_MS)

    flow_state["logged_in"] = True
    snap("home_loaded")


def step_03_open_settings(
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
    with step("open settings"):
        _open_user_settings(page, base_url)
    flow_state["settings_open"] = True
    snap("settings_opened")


def step_04_open_model_providers(
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
    require(flow_state, "settings_open")
    page = flow_page
    with step("open model providers"):
        model_nav = page.locator("[data-testid='settings-nav-model-providers']")
        expect(model_nav).to_have_count(1)
        model_nav.first.click()
        expect(page.locator("text=Set default models")).to_be_visible()
    flow_state["model_providers_open"] = True
    snap("model_providers_open")


def step_05_filter_zhipu(
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
    require(flow_state, "model_providers_open")
    page = flow_page
    with step("filter providers"):
        search_input = page.locator("[data-testid='model-providers-search']")
        expect(search_input).to_have_count(1)
        search_input.first.fill("zhipu")
        available_section = page.locator("[data-testid='available-models-section']")
        provider = available_section.locator(
            "[data-testid='available-model-card'][data-provider='ZHIPU-AI']"
        ).first
        if provider.count() == 0:
            added_section = page.locator("[data-testid='added-models-section']")
            if (
                added_section.locator(
                    "[data-testid='added-model-card'][data-provider='ZHIPU-AI']"
                ).count()
                == 0
            ):
                raise AssertionError("ZHIPU-AI provider not found in available or added models.")
        else:
            expect(provider).to_be_visible()
    flow_state["provider_filtered"] = True
    snap("provider_filtered")


def step_06_add_api_key(
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
    require(flow_state, "provider_filtered", "api_key")
    page = flow_page
    available_section = page.locator("[data-testid='available-models-section']")
    provider = available_section.locator(
        "[data-testid='available-model-card'][data-provider='ZHIPU-AI']"
    ).first

    with step("add ZHIPU-AI api key"):
        if provider.count() > 0:
            provider.click()
        else:
            added_section = page.locator("[data-testid='added-models-section']")
            card = added_section.locator(
                "[data-testid='added-model-card'][data-provider='ZHIPU-AI']"
            ).first
            api_key_button = card.locator("button", has_text=re.compile("API-?Key", re.I)).first
            expect(api_key_button).to_be_visible()
            api_key_button.click()
        modal = page.locator("[data-testid='apikey-modal']")
        expect(modal).to_be_visible()
        api_input = modal.locator("[data-testid='apikey-input']").first
        save_button = modal.locator("[data-testid='apikey-save']").first
        try:
            def trigger():
                api_input.fill(flow_state["api_key"])
                save_button.click()

            capture_response(
                page,
                trigger,
                lambda resp: resp.request.method == "POST" and "/v1/llm/set_api_key" in resp.url,
            )
            expect(modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
        except Exception:
            _safe_close_modal(modal)
            raise

    with step("confirm added model"):
        added_section = page.locator("[data-testid='added-models-section']")
        expect(added_section).to_be_visible()
        expect(
            added_section.locator(
                "[data-testid='added-model-card'][data-provider='ZHIPU-AI']"
            )
        ).to_be_visible()
    flow_state["provider_added"] = True
    snap("provider_saved")


def step_07_set_defaults(
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
    require(flow_state, "provider_added")
    page = flow_page
    with step("set default models"):
        llm_combo = page.locator("[data-testid='default-llm-combobox']").first
        emb_combo = page.locator("[data-testid='default-embedding-combobox']").first

        _select_default_model(
            page,
            llm_combo,
            "glm-4-flash",
            "glm-4-flash",
            list_testid="default-llm-options",
        )
        selected_emb_text, _ = _select_default_model(
            page,
            emb_combo,
            "embedding-2",
            "embedding-2",
            list_testid="default-embedding-options",
            fallback_to_first=True,
        )
    flow_state["selected_emb_text"] = selected_emb_text
    flow_state["defaults_set"] = True
    snap("defaults_selected")


def step_08_verify_persist(
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
    require(flow_state, "defaults_set")
    page = flow_page
    with step("reload and verify defaults"):
        page.reload(wait_until="domcontentloaded")
        expect(page.locator("text=Set default models")).to_be_visible()
        llm_combo = page.locator("[data-testid='default-llm-combobox']").first
        emb_combo = page.locator("[data-testid='default-embedding-combobox']").first
        expect(llm_combo).to_contain_text("glm-4-flash")
        expect(emb_combo).to_contain_text(flow_state.get("selected_emb_text") or "embedding-2")
        added_section = page.locator("[data-testid='added-models-section']")
        expect(
            added_section.locator(
                "[data-testid='added-model-card'][data-provider='ZHIPU-AI']"
            )
        ).to_be_visible()
    snap("defaults_persisted")
    snap("success")


STEPS = [
    ("01_open_login", step_01_open_login),
    ("02_login", step_02_login),
    ("03_open_settings", step_03_open_settings),
    ("04_open_model_providers", step_04_open_model_providers),
    ("05_filter_zhipu", step_05_filter_zhipu),
    ("06_add_api_key", step_06_add_api_key),
    ("07_set_defaults", step_07_set_defaults),
    ("08_verify_persist", step_08_verify_persist),
]


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(STEPS))
def test_add_zhipu_ai_set_defaults_persist_flow(
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
