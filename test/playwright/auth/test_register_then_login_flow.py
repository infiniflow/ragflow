import json
import os
from urllib.parse import urlparse

import pytest
from playwright.sync_api import TimeoutError as PlaywrightTimeoutError
from playwright.sync_api import expect

from test.playwright.helpers.auth_selectors import (
    AUTH_STATUS,
    EMAIL_INPUT,
    NICKNAME_INPUT,
    PASSWORD_INPUT,
    REGISTER_TAB,
    SUBMIT_BUTTON,
)
from test.playwright.helpers.flow_steps import flow_params, require
from test.playwright.helpers.response_capture import capture_response_json

RESULT_TIMEOUT_MS = 15000


def _debug_register_response(page, response_info: dict) -> None:
    if not os.getenv("PW_DEBUG_DUMP"):
        return
    message = response_info.get("message")
    if isinstance(message, str) and len(message) > 300:
        message = message[:300]
    print(
        "[auth-debug] register_response "
        f"url={response_info.get('__url__')} status={response_info.get('__status__')} "
        f"code={response_info.get('code')} message={message}",
        flush=True,
    )
    try:
        sonner = page.locator("[data-sonner-toast]")
        if sonner.count() > 0:
            html = sonner.first.evaluate("el => el.outerHTML.slice(0, 300)")
            print(f"[auth-debug] sonner_toast={html}", flush=True)
    except Exception as exc:
        print(f"[auth-debug] sonner_toast_dump_failed: {exc}", flush=True)


def _wait_for_login_outcome(
    page, post_login_path: str | None, timeout_ms: int = RESULT_TIMEOUT_MS
):
    auth_status_selector = json.dumps(AUTH_STATUS)
    return page.wait_for_function(
        """
        (postLoginPath) => {
          const isVisible = (el) => {
            if (!el) return false;
            const style = window.getComputedStyle(el);
            if (style && (style.visibility === 'hidden' || style.display === 'none')) {
              return false;
            }
            const rect = el.getBoundingClientRect();
            return rect.width > 0 && rect.height > 0;
          };
          const authStatus = document.querySelector(%s);
          const statusState = authStatus ? authStatus.getAttribute('data-state') : '';
          if (statusState === 'error') return { state: 'error' };
          if (statusState === 'success') return { state: 'success' };

          const path = window.location.pathname || '';
          const successByUrl = postLoginPath
            ? path.startsWith(postLoginPath)
            : !path.includes('/login');
          const successMarker = document.querySelector(
            "a[href*='github.com/infiniflow/ragflow'], a[href*='discord.com/invite']"
          );
          if (successByUrl || successMarker) return { state: 'success' };
          return false;
        }
        """ % auth_status_selector,
        post_login_path,
        timeout=timeout_ms,
    )


def step_01_open_login(
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    reg_email,
    reg_password,
    reg_nickname,
    reg_email_unique,
):
    with step("open login page"):
        flow_page.goto(login_url, wait_until="domcontentloaded")
    flow_state["login_opened"] = True
    snap("open")


def step_02_switch_to_register(
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    reg_email,
    reg_password,
    reg_nickname,
    reg_email_unique,
):
    require(flow_state, "login_opened")
    if not reg_email_unique:
        flow_state["reg_email_unique"] = False
        pytest.skip("Set REG_EMAIL_UNIQUE=1 for deterministic register→login flow.")
    flow_state["reg_email_unique"] = True
    form, card = active_auth_context()
    toggle_button = card.locator(REGISTER_TAB)
    if toggle_button.count() == 0:
        flow_state["register_toggle_available"] = False
        pytest.skip("Register toggle not present; registerEnabled may be disabled.")

    with step("switch to register"):
        expect(toggle_button).to_have_count(1)
        toggle_button.click()
    flow_state["register_toggle_available"] = True
    snap("register_toggled")


def step_03_register_user(
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    reg_email,
    reg_password,
    reg_nickname,
    reg_email_unique,
):
    require(flow_state, "login_opened", "register_toggle_available", "reg_email_unique")
    page = flow_page
    form, _ = active_auth_context()
    nickname_input = form.locator(NICKNAME_INPUT)
    expect(nickname_input).to_have_count(1)
    expect(nickname_input).to_be_visible()

    email_input = form.locator(EMAIL_INPUT)
    password_input = form.locator(PASSWORD_INPUT)

    with step("fill registration form"):
        expect(email_input).to_have_count(1)
        expect(password_input).to_have_count(1)
        nickname_input.fill(reg_nickname)
        email_input.fill(reg_email)
        password_input.fill(reg_password)
        expect(password_input).to_have_attribute("type", "password")
        password_input.blur()
    snap("register_filled")

    with step("submit registration and wait for response"):
        submit_button = form.locator(SUBMIT_BUTTON)
        expect(submit_button).to_have_count(1)
        try:
            response_info = capture_response_json(
                page,
                lambda: (
                    auth_click(submit_button, "submit_register"),
                    snap("register_submitted"),
                ),
                lambda resp: resp.request.method == "POST"
                and "/v1/user/register" in resp.url,
                timeout_ms=RESULT_TIMEOUT_MS,
            )
        except PlaywrightTimeoutError as exc:
            snap("register_failure")
            raise AssertionError(
                f"Register response not received in time. url={page.url}"
            ) from exc

    _debug_register_response(page, response_info)

    if response_info.get("code") != 0:
        snap("register_error_response")
        snap("register_failure")
        raise AssertionError(
            "Registration error detected. "
            f"url={response_info.get('__url__')} status={response_info.get('__status__')} "
            f"code={response_info.get('code')} message={response_info.get('message')}"
        )

    snap("register_success_response")
    form, _ = active_auth_context()
    nickname_input = form.locator(NICKNAME_INPUT)
    expect(nickname_input).to_have_count(0, timeout=RESULT_TIMEOUT_MS)
    snap("register_success")
    flow_state["registered_email"] = reg_email
    flow_state["registered_password"] = reg_password
    flow_state["register_complete"] = True
    print(f"REGISTERED_EMAIL={reg_email}", flush=True)


def step_04_login_user(
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    reg_email,
    reg_password,
    reg_nickname,
    reg_email_unique,
):
    require(flow_state, "register_complete", "registered_email", "registered_password")
    form, _ = active_auth_context()
    with step("fill login form"):
        email_input = form.locator(EMAIL_INPUT)
        password_input = form.locator(PASSWORD_INPUT)
        expect(email_input).to_have_count(1)
        expect(password_input).to_have_count(1)
        email_input.fill(flow_state["registered_email"])
        password_input.fill(flow_state["registered_password"])
        expect(password_input).to_have_attribute("type", "password")
        password_input.blur()
    snap("login_filled")

    with step("submit login"):
        submit_button = form.locator(SUBMIT_BUTTON)
        expect(submit_button).to_have_count(1)
        auth_click(submit_button, "submit_login")
    snap("login_submitted")


def step_05_verify_login(
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    reg_email,
    reg_password,
    reg_nickname,
    reg_email_unique,
):
    require(flow_state, "register_complete")
    page = flow_page
    post_login_path = os.getenv("POST_LOGIN_PATH")

    with step("wait for login outcome"):
        try:
            login_result = _wait_for_login_outcome(page, post_login_path)
        except PlaywrightTimeoutError as exc:
            snap("login_failure")
            raise AssertionError(
                f"Login result did not resolve in time. url={page.url}"
            ) from exc

    login_outcome = login_result.json_value()
    if login_outcome.get("state") == "error":
        snap("login_error")
        snap("login_failure")
        raise AssertionError(f"Login error detected. url={page.url}")

    path = urlparse(page.url).path
    if post_login_path:
        if not path.startswith(post_login_path):
            snap("login_failure")
            raise AssertionError(
                f"Post-login path mismatch. expected_prefix={post_login_path} url={page.url}"
            )
    elif "/login" in path:
        snap("login_failure")
        raise AssertionError(f"URL still on login after submit. url={page.url}")

    snap("login_success")


STEPS = [
    ("01_open_login", step_01_open_login),
    ("02_switch_to_register", step_02_switch_to_register),
    ("03_register_user", step_03_register_user),
    ("04_login_user", step_04_login_user),
    ("05_verify_login", step_05_verify_login),
]


@pytest.mark.p0
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(STEPS))
def test_register_then_login_flow(
    step_fn,
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    reg_email,
    reg_password,
    reg_nickname,
    reg_email_unique,
):
    step_fn(
        flow_page,
        flow_state,
        login_url,
        active_auth_context,
        step,
        snap,
        auth_click,
        reg_email,
        reg_password,
        reg_nickname,
        reg_email_unique,
    )
