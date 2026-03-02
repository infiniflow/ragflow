import re
import os
import pytest
from playwright.sync_api import expect

from test.playwright.helpers.flow_steps import flow_params, require
from test.playwright.helpers.auth_selectors import EMAIL_INPUT, PASSWORD_INPUT, SUBMIT_BUTTON
from test.playwright.helpers.auth_waits import wait_for_login_complete
from test.playwright.helpers.response_capture import capture_response
from test.playwright.helpers.model_providers import (
    open_user_settings,
    safe_close_modal,
    select_default_model,
)

RESULT_TIMEOUT_MS = 15000


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
    email_input = form.locator(EMAIL_INPUT)
    password_input = form.locator(PASSWORD_INPUT)
    with step("fill credentials"):
        expect(email_input).to_have_count(1)
        expect(password_input).to_have_count(1)
        email_input.fill(flow_state["email"])
        password_input.fill(flow_state["password"])
        password_input.blur()

    with step("submit login"):
        submit_button = form.locator(SUBMIT_BUTTON)
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
        open_user_settings(page, base_url)
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
            safe_close_modal(modal)
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

        select_default_model(
            page,
            expect,
            llm_combo,
            "glm-4-flash",
            "glm-4-flash",
            list_testid="default-llm-options",
            fallback_to_first=False,
            timeout_ms=RESULT_TIMEOUT_MS,
        )
        selected_emb_text, _ = select_default_model(
            page,
            expect,
            emb_combo,
            "embedding-2",
            "embedding-2",
            list_testid="default-embedding-options",
            fallback_to_first=True,
            timeout_ms=RESULT_TIMEOUT_MS,
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
