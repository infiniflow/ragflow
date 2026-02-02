import os
from urllib.parse import urlparse

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


def _wait_for_login_outcome(page, post_login_path: str | None, timeout_ms: int = RESULT_TIMEOUT_MS):
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
          const authStatus = document.querySelector('[data-testid="auth-status"]');
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
        """,
        post_login_path,
        timeout=timeout_ms,
    )


@pytest.mark.p0
@pytest.mark.auth
def test_register_then_login_flow(
    login_url,
    page,
    active_auth_context,
    step,
    snap,
    auth_click,
    reg_email,
    reg_password,
    reg_nickname,
    reg_email_unique,
):
    if not reg_email_unique:
        pytest.skip("Set REG_EMAIL_UNIQUE=1 for deterministic register→login flow.")

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
    snap("register_toggled")

    email_input = form.locator(
        "input[data-testid='auth-email'], [data-testid='auth-email'] input"
    )
    password_input = form.locator(
        "input[data-testid='auth-password'], [data-testid='auth-password'] input"
    )

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
        submit_button = form.locator(
            "button[data-testid='auth-submit'], [data-testid='auth-submit'] button, [data-testid='auth-submit']"
        )
        expect(submit_button).to_have_count(1)
        try:
            response_info = _capture_register_response(
                page,
                lambda: (
                    auth_click(submit_button, "submit_register"),
                    snap("register_submitted"),
                ),
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
    nickname_input = form.locator("[data-testid='auth-nickname']")
    expect(nickname_input).to_have_count(0, timeout=RESULT_TIMEOUT_MS)
    snap("register_success")
    print(f"REGISTERED_EMAIL={reg_email}", flush=True)

    with step("fill login form"):
        form, _ = active_auth_context()
        email_input = form.locator(
            "input[data-testid='auth-email'], [data-testid='auth-email'] input"
        )
        password_input = form.locator(
            "input[data-testid='auth-password'], [data-testid='auth-password'] input"
        )
        expect(email_input).to_have_count(1)
        expect(password_input).to_have_count(1)
        email_input.fill(reg_email)
        password_input.fill(reg_password)
        expect(password_input).to_have_attribute("type", "password")
        password_input.blur()
    snap("login_filled")

    with step("submit login"):
        submit_button = form.locator(
            "button[data-testid='auth-submit'], [data-testid='auth-submit'] button, [data-testid='auth-submit']"
        )
        expect(submit_button).to_have_count(1)
        auth_click(submit_button, "submit_login")
    snap("login_submitted")

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
        raise AssertionError(
            f"Login error detected. url={page.url}"
        )

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
