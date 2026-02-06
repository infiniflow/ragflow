
from playwright.sync_api import TimeoutError as PlaywrightTimeoutError

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
