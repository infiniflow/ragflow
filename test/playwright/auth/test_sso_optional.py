import re

import pytest

from test.playwright.helpers.flow_steps import flow_params, require


def step_01_open_login(flow_page, flow_state, login_url, active_auth_context, step, snap):
    with step("open login page"):
        flow_page.goto(login_url, wait_until="domcontentloaded")
    flow_state["login_opened"] = True
    snap("open")


def step_02_initiate_sso(flow_page, flow_state, login_url, active_auth_context, step, snap):
    require(flow_state, "login_opened")
    page = flow_page
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
    flow_state["sso_clicked"] = True
    snap("sso_clicked")


STEPS = [
    ("01_open_login", step_01_open_login),
    ("02_initiate_sso", step_02_initiate_sso),
]


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(STEPS))
def test_sso_optional_flow(
    step_fn, flow_page, flow_state, login_url, active_auth_context, step, snap
):
    step_fn(flow_page, flow_state, login_url, active_auth_context, step, snap)
