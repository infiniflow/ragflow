import pytest
from playwright.sync_api import expect


@pytest.mark.p1
@pytest.mark.auth
def test_validation_presence(login_url, page, active_auth_context, step, auth_click):
    with step("open login page"):
        page.goto(login_url, wait_until="domcontentloaded")

    form, _ = active_auth_context()
    expect(
        form.locator(
            "input[data-testid='auth-email'], [data-testid='auth-email'] input"
        )
    ).to_have_count(1)

    with step("submit empty login form"):
        submit_button = form.locator(
            "button[data-testid='auth-submit'], [data-testid='auth-submit'] button, [data-testid='auth-submit']"
        )
        expect(submit_button).to_have_count(1)
        auth_click(submit_button, "submit_validation")

    invalid_inputs = form.locator("input[aria-invalid='true']")
    error_messages = form.locator("p[id$='-form-item-message']")

    try:
        expect(invalid_inputs).not_to_have_count(0, timeout=2000)
        return
    except AssertionError:
        pass

    try:
        expect(error_messages).not_to_have_count(0, timeout=1000)
        return
    except AssertionError:
        pass

    raise AssertionError(
        "No validation feedback detected after submitting an empty login form. "
        "Expected aria-invalid inputs or visible error containers. "
        "See artifacts for DOM evidence."
    )
