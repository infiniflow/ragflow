import pytest
from playwright.sync_api import expect

from test.playwright.helpers.auth_selectors import EMAIL_INPUT, SUBMIT_BUTTON
from test.playwright.helpers.flow_steps import flow_params, require


def step_01_open_login(
    flow_page, flow_state, login_url, active_auth_context, step, snap, auth_click
):
    page = flow_page
    with step("open login page"):
        page.goto(login_url, wait_until="domcontentloaded")
    flow_state["login_opened"] = True
    snap("open")


def step_02_submit_empty(
    flow_page, flow_state, login_url, active_auth_context, step, snap, auth_click
):
    require(flow_state, "login_opened")
    form, _ = active_auth_context()
    expect(form.locator(EMAIL_INPUT)).to_have_count(1)

    with step("submit empty login form"):
        submit_button = form.locator(SUBMIT_BUTTON)
        expect(submit_button).to_have_count(1)
        auth_click(submit_button, "submit_validation")
    flow_state["submitted_empty"] = True
    snap("submitted_empty")


def step_03_assert_validation(
    flow_page, flow_state, login_url, active_auth_context, step, snap, auth_click
):
    require(flow_state, "login_opened", "submitted_empty")
    form, _ = active_auth_context()
    invalid_inputs = form.locator("input[aria-invalid='true']")
    error_messages = form.locator("p[id$='-form-item-message']")

    try:
        expect(invalid_inputs).not_to_have_count(0, timeout=2000)
        snap("validation_visible")
        return
    except AssertionError:
        pass

    try:
        expect(error_messages).not_to_have_count(0, timeout=1000)
        snap("validation_visible")
        return
    except AssertionError:
        pass

    raise AssertionError(
        "No validation feedback detected after submitting an empty login form. "
        "Expected aria-invalid inputs or visible error containers. "
        "See artifacts for DOM evidence."
    )


STEPS = [
    ("01_open_login", step_01_open_login),
    ("02_submit_empty", step_02_submit_empty),
    ("03_assert_validation", step_03_assert_validation),
]


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(STEPS))
def test_validation_presence_flow(
    step_fn, flow_page, flow_state, login_url, active_auth_context, step, snap, auth_click
):
    step_fn(flow_page, flow_state, login_url, active_auth_context, step, snap, auth_click)
