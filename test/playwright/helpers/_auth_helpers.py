import os

import pytest
from playwright.sync_api import expect

RESULT_TIMEOUT_MS = 15000


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


def ensure_authed(
    page,
    login_url: str,
    active_auth_context,
    auth_click,
    timeout_ms: int = RESULT_TIMEOUT_MS,
) -> None:
    email = os.getenv("SEEDED_USER_EMAIL")
    password = os.getenv("SEEDED_USER_PASSWORD")
    if not email or not password:
        pytest.skip("SEEDED_USER_EMAIL/SEEDED_USER_PASSWORD not set.")

    token_wait_js = """
        () => {
          const token = localStorage.getItem('Token');
          const auth = localStorage.getItem('Authorization');
          return Boolean((token && token.length) || (auth && auth.length));
        }
        """

    try:
        if "/login" not in page.url:
            if page.locator("input[autocomplete='email']").count() == 0:
                try:
                    page.wait_for_function(token_wait_js, timeout=2000)
                    return
                except Exception:
                    pass
    except Exception:
        pass

    page.goto(login_url, wait_until="domcontentloaded")

    form, _ = active_auth_context()
    email_input = form.locator("input[autocomplete='email']")
    password_input = form.locator("input[type='password']")
    expect(email_input).to_have_count(1)
    expect(password_input).to_have_count(1)
    email_input.fill(email)
    password_input.fill(password)
    password_input.blur()

    submit_button = form.locator("button[type='submit']")
    expect(submit_button).to_have_count(1)
    auth_click(submit_button, "submit_login")

    _wait_for_login_complete(page, timeout_ms=timeout_ms)
    expect(page.locator("form:visible input[autocomplete='email']")).to_have_count(
        0, timeout=timeout_ms
    )
