import pytest
from playwright.sync_api import expect

from test.playwright.helpers.flow_context import FlowContext
from test.playwright.helpers._auth_helpers import ensure_authed
from test.playwright.helpers.flow_steps import flow_params, require
from test.playwright.helpers._next_apps_helpers import (
    RESULT_TIMEOUT_MS,
    _fill_and_save_create_modal,
    _goto_home,
    _nav_click,
    _open_create_from_list,
    _select_first_dataset_and_save,
    _send_chat_and_wait_done,
    _unique_name,
    _wait_for_url_or_testid,
)


def step_01_ensure_authed(ctx: FlowContext, step, snap):
    with step("ensure logged in"):
        ensure_authed(
            ctx.page,
            ctx.login_url,
            ctx.active_auth_context,
            ctx.auth_click,
            seeded_user_credentials=ctx.seeded_user_credentials,
        )
    ctx.state["logged_in"] = True
    snap("authed")


def step_02_open_chat_list(ctx: FlowContext, step, snap):
    require(ctx.state, "logged_in")
    page = ctx.page
    with step("open chat list"):
        _goto_home(page, ctx.base_url)
        _nav_click(page, "nav-chat")
        expect(page.locator("[data-testid='chats-list']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    snap("chat_list_open")


def step_03_open_create_modal(ctx: FlowContext, step, snap):
    require(ctx.state, "logged_in")
    page = ctx.page
    with step("open create chat modal"):
        _open_create_from_list(page, "chats-empty-create", "create-chat")
    ctx.state["chat_modal_open"] = True
    snap("chat_create_modal")


def step_04_create_chat(ctx: FlowContext, step, snap):
    require(ctx.state, "chat_modal_open")
    page = ctx.page
    chat_name = _unique_name("qa-chat")
    ctx.state["chat_name"] = chat_name
    with step("create chat app"):
        _fill_and_save_create_modal(page, chat_name)
        chat_detail = page.locator("[data-testid='chat-detail']")
        try:
            _wait_for_url_or_testid(page, r"/next-chat/", "chat-detail", timeout_ms=5000)
        except AssertionError:
            list_root = page.locator("[data-testid='chats-list']")
            expect(list_root).to_be_visible(timeout=RESULT_TIMEOUT_MS)
            card = list_root.locator(f"text={chat_name}").first
            expect(card).to_be_visible(timeout=RESULT_TIMEOUT_MS)
            card.click()
        expect(chat_detail).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    ctx.state["chat_created"] = True
    snap("chat_created")


def step_05_select_dataset(ctx: FlowContext, step, snap):
    require(ctx.state, "chat_created")
    page = ctx.page
    with step("select dataset"):
        _select_first_dataset_and_save(page, timeout_ms=RESULT_TIMEOUT_MS)
    ctx.state["chat_dataset_selected"] = True
    snap("chat_dataset_saved")


def step_06_ask_question(ctx: FlowContext, step, snap):
    require(ctx.state, "chat_dataset_selected")
    page = ctx.page
    with step("ask question"):
        _send_chat_and_wait_done(page, "what is ragflow", timeout_ms=60000)
    snap("chat_stream_done")


STEPS = [
    ("01_ensure_authed", step_01_ensure_authed),
    ("02_open_chat_list", step_02_open_chat_list),
    ("03_open_create_modal", step_03_open_create_modal),
    ("04_create_chat", step_04_create_chat),
    ("05_select_dataset", step_05_select_dataset),
    ("06_ask_question", step_06_ask_question),
]


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(STEPS))
def test_chat_create_select_dataset_and_receive_answer_flow(
    step_fn,
    flow_page,
    flow_state,
    base_url,
    login_url,
    active_auth_context,
    step,
    snap,
    auth_click,
    seeded_user_credentials,
):
    ctx = FlowContext(
        page=flow_page,
        state=flow_state,
        base_url=base_url,
        login_url=login_url,
        active_auth_context=active_auth_context,
        auth_click=auth_click,
        seeded_user_credentials=seeded_user_credentials,
    )
    step_fn(ctx, step, snap)
