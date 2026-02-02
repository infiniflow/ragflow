import os

import pytest
from playwright.sync_api import TimeoutError as PlaywrightTimeoutError
from playwright.sync_api import expect

RESULT_TIMEOUT_MS = 15000


def _capture_register_response(page, trigger, timeout_ms: int = RESULT_TIMEOUT_MS) -> dict:
    def predicate(resp):
        return resp.request.method == "POST" and "/v1/user/register" in resp.url

    if hasattr(page, "expect_response"):
        with page.expect_response(predicate, timeout=timeout_ms) as response_info:
            trigger()
        response = response_info.value
    elif hasattr(page, "expect_event"):
        with page.expect_event("response", predicate=predicate, timeout=timeout_ms) as response_info:
            trigger()
        response = response_info.value
    elif hasattr(page, "wait_for_event"):
        trigger()
        response = page.wait_for_event("response", predicate=predicate, timeout=timeout_ms)
    else:
        raise RuntimeError(
            "Playwright Page lacks expect_response/expect_event/wait_for_event."
        )

    info: dict = {"__url__": response.url, "__status__": response.status}
    try:
        data = response.json()
        if isinstance(data, dict):
            info.update(data)
        else:
            info["__parse_error__"] = "non-dict response body"
    except Exception as exc:
        info["__parse_error__"] = str(exc)
    return info


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
    page.wait_for_function(
        """
        () => {
          const status = document.querySelector('[data-testid="auth-status"]');
          if (!status) return true;
          return status.getAttribute('data-state') !== 'loading';
        }
        """,
        timeout=timeout_ms,
    )


@pytest.mark.p1
@pytest.mark.auth
def test_register_success_optional(
    login_url,
    page,
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
    with step("open login page"):
        page.goto(login_url, wait_until="domcontentloaded")
    snap("open")

    form, card = active_auth_context()
    toggle_button = card.locator("[data-testid='auth-toggle-register']")
    if toggle_button.count() == 0:
        pytest.skip("Register toggle not present; registerEnabled may be disabled.")

    with step("switch to register"):
        expect(toggle_button).to_have_count(1)
        toggle_button.click()

    form, _ = active_auth_context()
    nickname_input = form.locator("[data-testid='auth-nickname']")
    expect(nickname_input).to_have_count(1)
    expect(nickname_input).to_be_visible()
    snap("toggled_register")

    email_input = form.locator(
        "input[data-testid='auth-email'], [data-testid='auth-email'] input"
    )
    password_input = form.locator(
        "input[data-testid='auth-password'], [data-testid='auth-password'] input"
    )

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
            submit_button = form.locator(
                "button[data-testid='auth-submit'], [data-testid='auth-submit'] button, [data-testid='auth-submit']"
            )
            expect(submit_button).to_have_count(1)
            if not retried:
                snap("before_submit_click")
                auth_debug_dump("before_submit_click", submit_button)

            try:
                response_info = _capture_register_response(
                    page,
                    lambda: (
                        auth_click(
                            submit_button,
                            "submit_register_retry" if retried else "submit_register",
                        ),
                        snap("retry_submitted" if retried else "submitted"),
                    ),
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
            nickname_input = form.locator("[data-testid='auth-nickname']")
            expect(nickname_input).to_have_count(0, timeout=RESULT_TIMEOUT_MS)
            break

        snap("registered_error_response")
        message_text = response_info.get("message", "") or ""
        if _is_already_registered(message_text) and not retried:
            retried = True
            with step("retry registration with new email"):
                _wait_for_auth_not_loading(page)
                form, _ = active_auth_context()
                email_input = form.locator(
                    "input[data-testid='auth-email'], [data-testid='auth-email'] input"
                )
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
    print(f"REGISTERED_EMAIL={current_email}", flush=True)
