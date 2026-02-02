import pytest
from playwright.sync_api import expect

from test.playwright.helpers._auth_helpers import ensure_authed
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


@pytest.mark.p1
@pytest.mark.auth
def test_chat_create_select_dataset_and_receive_answer(
    base_url,
    login_url,
    page,
    active_auth_context,
    step,
    snap,
    auth_click,
):
    with step("ensure logged in"):
        ensure_authed(page, login_url, active_auth_context, auth_click)

    with step("open chat list"):
        _goto_home(page, base_url)
        _nav_click(page, "nav-chat")
        expect(page.locator("[data-testid='chats-list']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    snap("chat_list_open")

    with step("open create chat modal"):
        _open_create_from_list(page, "chats-empty-create", "create-chat")
    snap("chat_create_modal")

    chat_name = _unique_name("qa-chat")
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
    snap("chat_created")

    with step("select dataset"):
        _select_first_dataset_and_save(page, timeout_ms=RESULT_TIMEOUT_MS)
    snap("chat_dataset_saved")

    with step("ask question"):
        _send_chat_and_wait_done(page, "what is ragflow", timeout_ms=60000)
    snap("chat_stream_done")
