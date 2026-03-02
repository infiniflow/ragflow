import json
import os

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


def _is_already_registered(toast_text: str) -> bool:
    text = (toast_text or "").lower()
    return "already" in text and ("register" in text or "registered" in text)


def _wait_for_auth_not_loading(page, timeout_ms: int = 5000) -> None:
    auth_status_selector = json.dumps(AUTH_STATUS)
    page.wait_for_function(
        """
        () => {
          const status = document.querySelector(%s);
          if (!status) return true;
          return status.getAttribute('data-state') !== 'loading';
        }
        """ % auth_status_selector,
        timeout=timeout_ms,
    )


def step_01_open_login(
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_debug_dump,
    auth_click,
    reg_email,
    reg_email_generator,
    reg_password,
    reg_nickname,
    reg_email_unique,
):
    page = flow_page
    with step("open login page"):
        page.goto(login_url, wait_until="domcontentloaded")
    flow_state["login_opened"] = True
    snap("open")


def step_02_switch_to_register(
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_debug_dump,
    auth_click,
    reg_email,
    reg_email_generator,
    reg_password,
    reg_nickname,
    reg_email_unique,
):
    require(flow_state, "login_opened")
    form, card = active_auth_context()
    toggle_button = card.locator(REGISTER_TAB)
    if toggle_button.count() == 0:
        flow_state["register_toggle_available"] = False
        pytest.skip("Register toggle not present; registerEnabled may be disabled.")

    with step("switch to register"):
        expect(toggle_button).to_have_count(1)
        toggle_button.click()
    flow_state["register_toggle_available"] = True
    snap("toggled_register")


def step_03_submit_registration(
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_debug_dump,
    auth_click,
    reg_email,
    reg_email_generator,
    reg_password,
    reg_nickname,
    reg_email_unique,
):
    require(flow_state, "login_opened", "register_toggle_available")
    page = flow_page
    form, _ = active_auth_context()
    nickname_input = form.locator(NICKNAME_INPUT)
    if nickname_input.count() == 0:
        pytest.skip("Register form not active; cannot submit registration.")

    email_input = form.locator(EMAIL_INPUT)
    password_input = form.locator(PASSWORD_INPUT)

    current_email = reg_email
    with step("fill registration form"):
        expect(email_input).to_have_count(1)
        expect(password_input).to_have_count(1)
        nickname_input.fill(reg_nickname)
        email_input.fill(current_email)
        password_input.fill(reg_password)
        expect(password_input).to_have_attribute("type", "password")
        password_input.blur()
    snap("filled")

    retried = False
    while True:
        with step("submit registration and wait for response"):
            form, _ = active_auth_context()
            submit_button = form.locator(SUBMIT_BUTTON)
            expect(submit_button).to_have_count(1)
            if not retried:
                snap("before_submit_click")
                auth_debug_dump("before_submit_click", submit_button)

            try:
                response_info = capture_response_json(
                    page,
                    lambda: (
                        auth_click(
                            submit_button,
                            "submit_register_retry" if retried else "submit_register",
                        ),
                        snap("retry_submitted" if retried else "submitted"),
                    ),
                    lambda resp: resp.request.method == "POST"
                    and "/v1/user/register" in resp.url,
                    timeout_ms=RESULT_TIMEOUT_MS,
                )
            except PlaywrightTimeoutError as exc:
                snap("failure")
                raise AssertionError(
                    f"Register response not received in time. url={page.url} email={current_email}"
                ) from exc

        _debug_register_response(page, response_info)

        if response_info.get("code") == 0:
            snap("registered_success_response")
            form, _ = active_auth_context()
            nickname_input = form.locator(NICKNAME_INPUT)
            expect(nickname_input).to_have_count(0, timeout=RESULT_TIMEOUT_MS)
            break

        snap("registered_error_response")
        message_text = response_info.get("message", "") or ""
        if _is_already_registered(message_text) and not retried:
            retried = True
            with step("retry registration with new email"):
                _wait_for_auth_not_loading(page)
                form, _ = active_auth_context()
                email_input = form.locator(EMAIL_INPUT)
                expect(email_input).to_have_count(1)
                current_email = reg_email_generator(force_unique=True)
                email_input.fill(current_email)
                snap("retry_filled")
            continue

        snap("failure")
        raise AssertionError(
            "Registration error detected. "
            f"url={response_info.get('__url__')} status={response_info.get('__status__')} "
            f"code={response_info.get('code')} message={response_info.get('message')} "
            f"email={current_email}"
        )

    snap("success")
    flow_state["register_complete"] = True
    flow_state["registered_email"] = current_email
    print(f"REGISTERED_EMAIL={current_email}", flush=True)


STEPS = [
    ("01_open_login", step_01_open_login),
    ("02_switch_to_register", step_02_switch_to_register),
    ("03_submit_registration", step_03_submit_registration),
]


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(STEPS))
def test_register_success_optional_flow(
    step_fn,
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_debug_dump,
    auth_click,
    reg_email,
    reg_email_generator,
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
        auth_debug_dump,
        auth_click,
        reg_email,
        reg_email_generator,
        reg_password,
        reg_nickname,
        reg_email_unique,
    )
