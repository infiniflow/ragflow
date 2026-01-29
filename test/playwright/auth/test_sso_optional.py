import re

import pytest


@pytest.mark.p1
@pytest.mark.auth
def test_sso_optional(login_url, page, active_auth_context, step):
    with step("open login page"):
        page.goto(login_url, wait_until="domcontentloaded")

    form, _ = active_auth_context()
    sso_buttons = form.locator("button:has-text('Sign in with')")
    if sso_buttons.count() == 0:
        pytest.skip("No SSO providers rendered on the login page")

    with step("initiate SSO navigation"):
        clicked = False
        for handle in sso_buttons.element_handles():
            if handle.is_visible() and handle.is_enabled():
                handle.click()
                clicked = True
                break
        if not clicked:
            pytest.skip("SSO buttons were present but not interactable")

        page.wait_for_url(re.compile(r".*/v1/user/login/"), timeout=5000)
