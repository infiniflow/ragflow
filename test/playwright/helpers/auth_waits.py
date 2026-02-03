
from playwright.sync_api import TimeoutError as PlaywrightTimeoutError

from test.playwright.helpers.auth_selectors import (
    AUTH_ACTIVE_FORM,
    AUTH_FORM,
    EMAIL_INPUT,
    PASSWORD_INPUT,
    SUBMIT_BUTTON,
)

try:
    from test.playwright.helpers._next_apps_helpers import (
        RESULT_TIMEOUT_MS as DEFAULT_TIMEOUT_MS,
    )
except Exception:
    DEFAULT_TIMEOUT_MS = 15000


def wait_for_login_complete(page, timeout_ms: int | None = None) -> None:
    if timeout_ms is None:
        timeout_ms = DEFAULT_TIMEOUT_MS
    wait_js = """
        () => {
          const path = window.location.pathname || '';
          if (path.includes('/login')) return false;
          const token = localStorage.getItem('Token');
          const auth = localStorage.getItem('Authorization');
          return Boolean((token && token.length) || (auth && auth.length));
        }
        """
    try:
        page.wait_for_function(wait_js, timeout=timeout_ms)
    except PlaywrightTimeoutError as exc:
        url = page.url
        testids = []
        try:
            testids = page.evaluate(
                """
                () => Array.from(document.querySelectorAll('[data-testid]'))
                  .map((el) => el.getAttribute('data-testid'))
                  .filter((val) => val && /auth/i.test(val))
                  .slice(0, 30)
                """
            )
        except Exception:
            testids = []
        raise AssertionError(
            f"Login did not complete within {timeout_ms}ms. url={url} auth_testids={testids}"
        ) from exc


def get_active_auth_form(page):
    form = page.locator(AUTH_ACTIVE_FORM)
    if form.count() == 0:
        form = page.locator(AUTH_FORM)
    return form


def wait_for_auth_ui_ready(page, timeout_ms: int | None = None) -> None:
    if timeout_ms is None:
        timeout_ms = DEFAULT_TIMEOUT_MS
    form = get_active_auth_form(page)
    try:
        from playwright.sync_api import expect

        expect(form).to_have_count(1, timeout=timeout_ms)
        email_input = form.locator(EMAIL_INPUT)
        password_input = form.locator(PASSWORD_INPUT)
        submit_button = form.locator(SUBMIT_BUTTON)
        expect(email_input).to_have_count(1, timeout=timeout_ms)
        expect(password_input).to_have_count(1, timeout=timeout_ms)
        expect(submit_button).to_have_count(1, timeout=timeout_ms)
        expect(submit_button).to_be_visible(timeout=timeout_ms)
        expect(submit_button).to_be_enabled(timeout=timeout_ms)
    except Exception as exc:
        raise AssertionError(
            f"Auth UI not ready within {timeout_ms}ms. url={page.url}"
        ) from exc
