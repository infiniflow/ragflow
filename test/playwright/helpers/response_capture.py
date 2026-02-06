
try:
    from test.playwright.helpers._auth_helpers import RESULT_TIMEOUT_MS as DEFAULT_TIMEOUT_MS
except Exception:
    # Fallback for standalone usage when helper constants are unavailable.
    DEFAULT_TIMEOUT_MS = 30_000


def capture_response(page, trigger, predicate, timeout_ms: int = DEFAULT_TIMEOUT_MS):
    if hasattr(page, "expect_response"):
        with page.expect_response(predicate, timeout=timeout_ms) as response_info:
            trigger()
        return response_info.value
    if hasattr(page, "expect_event"):
        with page.expect_event(
            "response", predicate=predicate, timeout=timeout_ms
        ) as response_info:
            trigger()
        return response_info.value
    if hasattr(page, "wait_for_event"):
        trigger()
        return page.wait_for_event("response", predicate=predicate, timeout=timeout_ms)
    raise RuntimeError("Playwright Page lacks expect_response/expect_event/wait_for_event.")


def capture_response_json(
    page, trigger, predicate, timeout_ms: int = DEFAULT_TIMEOUT_MS
) -> dict:
    response = capture_response(page, trigger, predicate, timeout_ms)
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
