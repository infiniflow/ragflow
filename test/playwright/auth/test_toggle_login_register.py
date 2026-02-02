import pytest
from playwright.sync_api import expect


@pytest.mark.p1
@pytest.mark.auth
def test_toggle_login_register(login_url, page, active_auth_context, step):
    with step("open login page"):
        page.goto(login_url, wait_until="domcontentloaded")

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

    with step("switch back to login"):
        toggle_back = card.locator("[data-testid='auth-toggle-login']")
        expect(toggle_back).to_have_count(1)
        toggle_back.click()

    form, _ = active_auth_context()
    nickname_input = form.locator("[data-testid='auth-nickname']")
    expect(nickname_input).to_have_count(0)
