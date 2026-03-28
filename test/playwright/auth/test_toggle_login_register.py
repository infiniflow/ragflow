import pytest
from playwright.sync_api import expect

from test.playwright.helpers.auth_selectors import LOGIN_TAB, NICKNAME_INPUT, REGISTER_TAB
from test.playwright.helpers.flow_steps import flow_params, require


def step_01_open_login(flow_page, flow_state, login_url, active_auth_context, step, snap):
    page = flow_page
    with step("open login page"):
        page.goto(login_url, wait_until="domcontentloaded")
    flow_state["login_opened"] = True
    snap("open")


def step_02_switch_to_register(
    flow_page, flow_state, login_url, active_auth_context, step, snap
):
    require(flow_state, "login_opened")
    form, card = active_auth_context()
    toggle_button = card.locator(REGISTER_TAB)
    if toggle_button.count() == 0:
        flow_state["register_toggle_available"] = False
        pytest.skip("Register toggle not present; registerEnabled may be disabled.")
    flow_state["register_toggle_available"] = True
    with step("switch to register"):
        expect(toggle_button).to_have_count(1)
        toggle_button.click()
    snap("toggled_register")


def step_03_assert_register_visible(
    flow_page, flow_state, login_url, active_auth_context, step, snap
):
    require(flow_state, "login_opened", "register_toggle_available")
    form, _ = active_auth_context()
    nickname_input = form.locator(NICKNAME_INPUT)
    expect(nickname_input).to_have_count(1)
    expect(nickname_input).to_be_visible()
    snap("register_visible")


def step_04_switch_back_to_login(
    flow_page, flow_state, login_url, active_auth_context, step, snap
):
    require(flow_state, "login_opened", "register_toggle_available")
    form, card = active_auth_context()
    toggle_back = card.locator(LOGIN_TAB)
    expect(toggle_back).to_have_count(1)
    toggle_back.click()
    flow_state["login_toggled_back"] = True
    snap("toggled_login")


def step_05_assert_login_visible(
    flow_page, flow_state, login_url, active_auth_context, step, snap
):
    require(flow_state, "login_opened", "login_toggled_back")
    form, _ = active_auth_context()
    nickname_input = form.locator(NICKNAME_INPUT)
    expect(nickname_input).to_have_count(0)
    snap("login_visible")


STEPS = [
    ("01_open_login", step_01_open_login),
    ("02_switch_to_register", step_02_switch_to_register),
    ("03_assert_register_visible", step_03_assert_register_visible),
    ("04_switch_back_to_login", step_04_switch_back_to_login),
    ("05_assert_login_visible", step_05_assert_login_visible),
]


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(STEPS))
def test_toggle_login_register_flow(
    step_fn, flow_page, flow_state, login_url, active_auth_context, step, snap
):
    step_fn(flow_page, flow_state, login_url, active_auth_context, step, snap)
