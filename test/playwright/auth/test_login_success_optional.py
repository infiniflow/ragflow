import json
import os
from urllib.parse import urlparse

import pytest
from playwright.sync_api import TimeoutError as PlaywrightTimeoutError
from playwright.sync_api import expect

from test.playwright.helpers.auth_selectors import (
    AUTH_ACTIVE_FORM,
    AUTH_STATUS,
    EMAIL_INPUT,
    PASSWORD_INPUT,
    SUBMIT_BUTTON,
)
from test.playwright.helpers.auth_waits import wait_for_login_complete
from test.playwright.helpers.env_utils import env_bool
from test.playwright.helpers.flow_steps import flow_params, require

DEMO_EMAIL = "qa@infiniflow.com"
DEMO_PASSWORD = "123"


def _resolve_creds():
    if env_bool("DEMO_CREDS"):
        return DEMO_EMAIL, DEMO_PASSWORD, "demo"
    email = os.getenv("SEEDED_USER_EMAIL")
    password = os.getenv("SEEDED_USER_PASSWORD")
    if not email or not password:
        return None
    return email, password, "env"


def _debug_login_state(page, label: str) -> None:
    if not env_bool("PW_DEBUG_DUMP"):
        return
    try:
        title = page.title()
    except Exception as exc:
        title = f"<title_error:{exc}>"
    try:
        storage_flags = page.evaluate(
            """
            () => Array.from(document.querySelectorAll('[data-testid]'))
              .map((el) => el.getAttribute('data-testid'))
              .filter((val) => val && /auth/i.test(val))
              .slice(0, 30)
            """
        )
    except Exception as exc:
        storage_flags = {"error": str(exc)}
    print(
        f"[auth-debug] label={label} url={page.url} title={title} storage={storage_flags}",
        flush=True,
    )


def step_01_open_login(
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
):
    _ = seeded_user_credentials
    creds = _resolve_creds()
    if not creds:
        pytest.skip("SEEDED_USER_EMAIL/SEEDED_USER_PASSWORD not set and DEMO_CREDS=1 not enabled")
    seeded_email, seeded_password, source = creds
    if source == "env":
        lowered = seeded_email.lower()
        example_domain = "infiniflow.io"
        if lowered.endswith(f"@{example_domain}"):
            raise AssertionError(
                "SEEDED_USER_EMAIL must be a real account (not *@example.com). "
                "Set valid credentials or use DEMO_CREDS=1 for demo mode."
            )
    print(f"[AUTH] using email: {seeded_email} (source={source})", flush=True)
    flow_state["seeded_email"] = seeded_email
    flow_state["seeded_password"] = seeded_password
    flow_state["login_opened"] = True

    with step("open login page"):
        flow_page.goto(login_url, wait_until="domcontentloaded")
    snap("open")


def step_02_submit_login(
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
):
    require(flow_state, "login_opened", "seeded_email", "seeded_password")
    form, _ = active_auth_context()
    email_input = form.locator(EMAIL_INPUT)
    password_input = form.locator(PASSWORD_INPUT)

    with step("fill credentials"):
        expect(email_input).to_have_count(1)
        expect(password_input).to_have_count(1)
        email_input.fill(flow_state["seeded_email"])
        password_input.fill(flow_state["seeded_password"])
        expect(password_input).to_have_attribute("type", "password")
        password_input.blur()
    snap("filled")

    with step("submit login"):
        submit_button = form.locator(SUBMIT_BUTTON)
        expect(submit_button).to_have_count(1)
        auth_click(submit_button, "submit_login")
    flow_state["login_submitted"] = True
    snap("submitted")


def step_03_verify_login(
    flow_page,
    flow_state,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
):
    require(flow_state, "login_submitted")
    page = flow_page
    post_login_path = os.getenv("POST_LOGIN_PATH")
    post_login_path_js = json.dumps(post_login_path)
    auth_status_selector = json.dumps(AUTH_STATUS)
    wait_js = """
        () => {{
          const postLoginPath = {post_login_path};
          const isVisible = (el) => {{
            if (!el) return false;
            const style = window.getComputedStyle(el);
            if (style && (style.visibility === 'hidden' || style.display === 'none')) {{
              return false;
            }}
            const rect = el.getBoundingClientRect();
            return rect.width > 0 && rect.height > 0;
          }};
          const path = window.location.pathname || '';
          const successByUrl = postLoginPath
            ? path.startsWith(postLoginPath)
            : !path.includes('/login');
          const successMarker = document.querySelector(
            "a[href*='github.com/infiniflow/ragflow'], a[href*='discord.com/invite']"
          );
          const authStatus = document.querySelector({auth_status_selector});
          const statusState = authStatus ? authStatus.getAttribute('data-state') : '';
          if (statusState === 'error') return {{ state: 'error' }};
          if (statusState === 'success') return {{ state: 'success' }};
          if (successByUrl || successMarker) return {{ state: 'success' }};
          return false;
        }}
        """.format(
            post_login_path=post_login_path_js,
            auth_status_selector=auth_status_selector,
        )

    with step("wait for success or error"):
        try:
            result = page.wait_for_function(
                wait_js,
                timeout=15000,
            )
        except PlaywrightTimeoutError as exc:
            snap("failure")
            _debug_login_state(page, "wait_for_outcome_timeout")
            raise AssertionError(
                f"Login result did not resolve in time. url={page.url}"
            ) from exc

    with step("verify authenticated UI marker"):
        outcome = result.json_value()
        if outcome.get("state") == "error":
            snap("error")
            snap("failure")
            _debug_login_state(page, "login_error")
            raise AssertionError(
                "Login error detected. "
                f"url={page.url}"
            )
        path = urlparse(page.url).path
        if post_login_path:
            if not path.startswith(post_login_path):
                snap("failure")
                _debug_login_state(page, "post_login_path_mismatch")
                raise AssertionError(
                    f"Post-login path mismatch. expected_prefix={post_login_path} url={page.url}"
                )
        elif "/login" in path:
            snap("failure")
            _debug_login_state(page, "still_on_login_path")
            raise AssertionError(f"URL still on login after submit. url={page.url}")

    with step("verify auth tokens and login form hidden"):
        wait_for_login_complete(page, timeout_ms=15000)
        try:
            expect(page.locator(AUTH_ACTIVE_FORM)).to_have_count(0, timeout=15000)
        except AssertionError as exc:
            snap("failure")
            _debug_login_state(page, "login_form_still_visible")
            raise AssertionError(
                f"Login form still visible after login. url={page.url}"
            ) from exc
    snap("success")


STEPS = [
    ("01_open_login", step_01_open_login),
    ("02_submit_login", step_02_submit_login),
    ("03_verify_login", step_03_verify_login),
]


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(STEPS))
def test_login_success_optional_flow(
    step_fn,
    flow_page,
    flow_state,
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
        login_url,
        active_auth_context,
        step,
        snap,
        auth_click,
        seeded_user_credentials,
    )
