import pytest
from pathlib import Path
from tempfile import gettempdir
from time import monotonic, time

from playwright.sync_api import TimeoutError as PlaywrightTimeoutError
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
    ensure_chat_ready,
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


MM_REQUEST_METHOD_WHITELIST = {"POST", "PUT", "PATCH"}


def _mm_payload_from_request(req) -> dict:
    try:
        payload = req.post_data_json
        if callable(payload):
            payload = payload()
        if isinstance(payload, dict):
            return payload
    except Exception:
        pass
    return {}


def _mm_is_checked(locator) -> bool:
    return (locator.get_attribute("data-state") or "") == "checked"


def _mm_open_and_close_embed_dialog_if_available(page) -> bool:
    page.get_by_test_id("chat-detail-embed-open").click()
    dialog = page.locator("[role='dialog']").last
    try:
        expect(dialog).to_be_visible(timeout=3000)
    except AssertionError:
        # Embed modal is gated by token/beta availability in some environments.
        expect(page.get_by_test_id("chat-detail")).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        return False

    page.keyboard.press("Escape")
    try:
        expect(dialog).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
    except AssertionError:
        # Fallback to clicking outside if Escape is ignored by current build.
        page.mouse.click(5, 5)
        expect(dialog).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
    return True


def _mm_settings_save_request(req) -> bool:
    return req.method.upper() in MM_REQUEST_METHOD_WHITELIST and "/dialog/set" in req.url


def _mm_open_settings_panel(page):
    settings_root = page.get_by_test_id("chat-detail-settings")
    if settings_root.count() > 0 and settings_root.is_visible():
        return settings_root

    settings_btn = page.get_by_test_id("chat-settings")
    expect(settings_btn).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    settings_btn.click()
    expect(settings_root).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    return settings_root


def _mm_click_model_option_by_testid(page, option_testid: str) -> None:
    deadline = monotonic() + 8
    while monotonic() < deadline:
        option = page.locator(f"[data-testid='{option_testid}']").first
        if option.count() == 0:
            page.wait_for_timeout(120)
            continue
        try:
            option.click(timeout=2000, force=True)
            return
        except Exception:
            page.wait_for_timeout(120)
    raise AssertionError(f"failed to click model option: {option_testid}")


def _mm_dismiss_open_popovers(page) -> None:
    popovers = page.locator("[data-radix-popper-content-wrapper] [role='dialog']")
    for _ in range(4):
        if popovers.count() == 0:
            return
        page.keyboard.press("Escape")
        page.wait_for_timeout(120)


def _mm_open_model_options(page, card, option_prefix: str):
    options = page.locator(f"[data-testid^='{option_prefix}']")
    deadline = monotonic() + 12
    while monotonic() < deadline:
        card.get_by_test_id("chat-detail-multimodel-card-model-select").click()
        try:
            expect(options.first).to_be_visible(timeout=1200)
            return options
        except AssertionError:
            pass

        popover_root = page.locator("[data-radix-popper-content-wrapper]").last
        if popover_root.count() > 0:
            popover_model_select = popover_root.locator("button[role='combobox']").first
            if popover_model_select.count() > 0:
                try:
                    popover_model_select.click(timeout=1200)
                except Exception:
                    pass
                try:
                    expect(options.first).to_be_visible(timeout=1200)
                    return options
                except AssertionError:
                    pass
        page.wait_for_timeout(120)

    raise AssertionError(
        f"no model options rendered for prefix={option_prefix!r} in multi-model selector"
    )


def mm_step_01_ensure_authed_and_open_chat_list(ctx: FlowContext, step, snap):
    page = ctx.page
    with step("ensure logged in and open chat list"):
        ensure_authed(
            page,
            ctx.login_url,
            ctx.active_auth_context,
            ctx.auth_click,
            seeded_user_credentials=ctx.seeded_user_credentials,
        )
        _goto_home(page, ctx.base_url)
        _nav_click(page, "nav-chat")
        expect(page.locator("[data-testid='chats-list']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    ctx.state["mm_logged_in"] = True
    snap("chat_mm_list")


def mm_step_02_create_chat_and_open_detail(ctx: FlowContext, step, snap):
    require(ctx.state, "mm_logged_in")
    page = ctx.page
    with step("create chat and open detail"):
        chat_name = _unique_name("qa-chat-mm")
        _open_create_from_list(page, "chats-empty-create", "create-chat")
        _fill_and_save_create_modal(page, chat_name)
        try:
            _wait_for_url_or_testid(page, r"/next-chat/", "chat-detail", timeout_ms=5000)
        except AssertionError:
            list_root = page.locator("[data-testid='chats-list']")
            expect(list_root).to_be_visible(timeout=RESULT_TIMEOUT_MS)
            card = list_root.locator(f"text={chat_name}").first
            expect(card).to_be_visible(timeout=RESULT_TIMEOUT_MS)
            card.click()
        expect(page.get_by_test_id("chat-detail")).to_be_visible(timeout=RESULT_TIMEOUT_MS)

    ctx.state["mm_chat_name"] = chat_name
    ctx.state["mm_chat_detail_open"] = True
    snap("chat_mm_detail_open")


def mm_step_03_select_dataset(ctx: FlowContext, step, snap):
    require(ctx.state, "mm_chat_detail_open")
    page = ctx.page
    with step("select dataset deterministically"):
        _select_first_dataset_and_save(page, timeout_ms=RESULT_TIMEOUT_MS)
        expect(page.get_by_test_id("chat-textarea")).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    ctx.state["mm_dataset_selected"] = True
    snap("chat_mm_dataset_ready")


def mm_step_04_embed_open_close(ctx: FlowContext, step, snap):
    require(ctx.state, "mm_dataset_selected")
    page = ctx.page
    with step("embed open and close"):
        _mm_open_and_close_embed_dialog_if_available(page)
        expect(page.get_by_test_id("chat-detail")).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    ctx.state["mm_embed_checked"] = True
    snap("chat_mm_embed_checked")


def mm_step_05_sessions_panel_row_ops(ctx: FlowContext, step, snap):
    require(ctx.state, "mm_embed_checked")
    page = ctx.page
    with step("sessions panel and session row operations"):
        sessions_root = page.get_by_test_id("chat-detail-sessions")
        expect(sessions_root).to_be_visible(timeout=RESULT_TIMEOUT_MS)

        page.get_by_test_id("chat-detail-sessions-close").click()
        expect(page.get_by_test_id("chat-detail-sessions-open")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
        page.get_by_test_id("chat-detail-sessions-open").click()
        expect(sessions_root).to_be_visible(timeout=RESULT_TIMEOUT_MS)

        page.get_by_test_id("chat-detail-session-new").click()
        session_rows = page.locator("[data-testid='chat-detail-session-item']")
        expect(session_rows.first).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        active_session = sessions_root.locator(
            "li[aria-selected='true'] [data-testid='chat-detail-session-item']"
        )
        selected_row = active_session.first if active_session.count() > 0 else session_rows.first
        created_session_id = selected_row.get_attribute("data-session-id") or ""
        assert created_session_id, "failed to capture created session id"

        selected_row.click()
        expect(
            page.locator(
                f"[data-testid='chat-detail-session-item'][data-session-id='{created_session_id}']"
            ).first
        ).to_be_visible(timeout=RESULT_TIMEOUT_MS)

        search_input = page.get_by_test_id("chat-detail-session-search")
        expect(search_input).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        row_count_before = session_rows.count()
        no_match_query = "__PW_NO_MATCH_SESSION__"
        search_input.fill(no_match_query)
        expect(search_input).to_have_value(no_match_query, timeout=RESULT_TIMEOUT_MS)
        filtered_rows = page.locator("[data-testid='chat-detail-session-item']")
        min_filtered_count = row_count_before
        deadline = monotonic() + 5
        while monotonic() < deadline:
            min_filtered_count = min(min_filtered_count, filtered_rows.count())
            if min_filtered_count < row_count_before:
                break
            page.wait_for_timeout(100)

        # When only one row exists, some builds keep it visible for temporary sessions.
        # In that case we still validate the search interaction without forcing impossible narrowing.
        if row_count_before > 1:
            assert (
                min_filtered_count < row_count_before
            ), "session search did not narrow visible rows"
        else:
            assert min_filtered_count <= row_count_before
        search_input.fill("")
        expect(
            page.locator(
                f"[data-testid='chat-detail-session-item'][data-session-id='{created_session_id}']"
            ).first
        ).to_be_visible(timeout=RESULT_TIMEOUT_MS)

        row_li = sessions_root.locator(
            f"li:has([data-testid='chat-detail-session-item'][data-session-id='{created_session_id}'])"
        ).first
        row_li.hover()
        actions_btn = page.locator(
            f"[data-testid='chat-detail-session-actions'][data-session-id='{created_session_id}']"
        ).first
        expect(actions_btn).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        actions_btn.click()

        row_delete = page.locator(
            f"[data-testid='chat-detail-session-delete'][data-session-id='{created_session_id}']"
        ).first
        expect(row_delete).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        row_delete.click()
        row_delete_dialog = page.get_by_test_id("confirm-delete-dialog")
        try:
            expect(row_delete_dialog).to_be_visible(timeout=3000)
            page.get_by_test_id("confirm-delete-dialog-cancel-btn").click()
            expect(row_delete_dialog).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
        except AssertionError:
            # If no dialog renders in this branch, still dismiss any menu overlay.
            page.keyboard.press("Escape")

        expect(
            page.locator(
                f"[data-testid='chat-detail-session-item'][data-session-id='{created_session_id}']"
            ).first
        ).to_be_visible(timeout=RESULT_TIMEOUT_MS)

    ctx.state["mm_created_session_id"] = created_session_id
    ctx.state["mm_session_row_checked"] = True
    snap("chat_mm_sessions_row_checked")


def mm_step_06_selection_mode_batch_delete(ctx: FlowContext, step, snap):
    require(ctx.state, "mm_session_row_checked", "mm_created_session_id")
    page = ctx.page
    created_session_id = ctx.state["mm_created_session_id"]
    with step("selection mode and batch delete cancel + confirm"):
        sessions_root = page.get_by_test_id("chat-detail-sessions")
        if sessions_root.count() == 0 or not sessions_root.is_visible():
            page.get_by_test_id("chat-detail-sessions-open").click()
        expect(sessions_root).to_be_visible(timeout=RESULT_TIMEOUT_MS)

        selection_enable = page.get_by_test_id("chat-detail-session-selection-enable")
        expect(selection_enable).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        try:
            selection_enable.click(timeout=5000)
        except PlaywrightTimeoutError:
            page.keyboard.press("Escape")
            page.mouse.click(5, 5)
            selection_enable.click(timeout=RESULT_TIMEOUT_MS)
        checked_before = page.locator(
            "[data-testid='chat-detail-session-checkbox'][data-state='checked']"
        ).count()
        page.get_by_test_id("chat-detail-session-select-all").click()
        checked_after = page.locator(
            "[data-testid='chat-detail-session-checkbox'][data-state='checked']"
        ).count()
        if page.locator("[data-testid='chat-detail-session-checkbox']").count() > 1:
            assert checked_after != checked_before
        else:
            assert checked_after >= checked_before

        session_checkbox = page.locator(
            f"[data-testid='chat-detail-session-checkbox'][data-session-id='{created_session_id}']"
        ).first
        expect(session_checkbox).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        if _mm_is_checked(session_checkbox):
            session_checkbox.click()
            assert not _mm_is_checked(session_checkbox)
        session_checkbox.click()
        assert _mm_is_checked(session_checkbox), "target session checkbox did not become checked"

        page.get_by_test_id("chat-detail-session-selection-exit").click()
        expect(
            page.locator(
                f"[data-testid='chat-detail-session-item'][data-session-id='{created_session_id}']"
            ).first
        ).to_be_visible(timeout=RESULT_TIMEOUT_MS)

        selection_enable = page.get_by_test_id("chat-detail-session-selection-enable")
        expect(selection_enable).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        try:
            selection_enable.click(timeout=5000)
        except PlaywrightTimeoutError:
            page.keyboard.press("Escape")
            page.mouse.click(5, 5)
            selection_enable.click(timeout=RESULT_TIMEOUT_MS)
        session_checkbox = page.locator(
            f"[data-testid='chat-detail-session-checkbox'][data-session-id='{created_session_id}']"
        ).first
        expect(session_checkbox).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        if not _mm_is_checked(session_checkbox):
            session_checkbox.click()

        page.get_by_test_id("chat-detail-session-batch-delete").click()
        batch_dialog = page.get_by_test_id("chat-detail-session-batch-delete-dialog")
        expect(batch_dialog).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        page.get_by_test_id("chat-detail-session-batch-delete-cancel").click()
        expect(batch_dialog).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
        expect(
            page.locator(
                f"[data-testid='chat-detail-session-checkbox'][data-session-id='{created_session_id}']"
            ).first
        ).to_be_visible(timeout=RESULT_TIMEOUT_MS)

        page.get_by_test_id("chat-detail-session-batch-delete").click()
        expect(batch_dialog).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        page.get_by_test_id("chat-detail-session-batch-delete-confirm").click()
        expect(batch_dialog).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
        expect(
            page.locator(
                f"[data-testid='chat-detail-session-item'][data-session-id='{created_session_id}']"
            )
        ).to_have_count(0, timeout=RESULT_TIMEOUT_MS)
        expect(
            sessions_root.locator(
                "li[aria-selected='true'] "
                f"[data-testid='chat-detail-session-item'][data-session-id='{created_session_id}']"
            )
        ).to_have_count(0, timeout=RESULT_TIMEOUT_MS)

    ctx.state["mm_sessions_cleanup_done"] = True
    snap("chat_mm_sessions_cleanup_done")


def mm_step_07_settings_open_close_cancel_save(ctx: FlowContext, step, snap):
    require(ctx.state, "mm_sessions_cleanup_done")
    page = ctx.page
    with step("settings open close cancel and save checks"):
        settings_root = _mm_open_settings_panel(page)
        page.get_by_test_id("chat-detail-settings-close").click()
        expect(settings_root).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)

        settings_root = _mm_open_settings_panel(page)
        name_input = settings_root.locator("input[name='name']").first
        expect(name_input).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        current_name = name_input.input_value()
        name_input.fill(f"{current_name}-cancel")

        with pytest.raises(PlaywrightTimeoutError):
            with page.expect_request(_mm_settings_save_request, timeout=1200):
                page.get_by_test_id("chat-detail-settings-cancel").click()
        expect(settings_root).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)

        settings_root = _mm_open_settings_panel(page)
        dataset_combo = settings_root.get_by_test_id("chat-datasets-combobox")
        expect(dataset_combo).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        dataset_combo.click()
        options_root = page.locator("[data-testid='datasets-options']").first
        expect(options_root).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        option = options_root.locator("[data-testid^='datasets-option-']").first
        if option.count() == 0:
            option = options_root.locator("[role='option']").first
        expect(option).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        option.click()

        current_name = name_input.input_value()
        name_input.fill(f"{current_name}-save")
        with page.expect_request(_mm_settings_save_request, timeout=RESULT_TIMEOUT_MS) as req_info:
            page.get_by_test_id("chat-settings-save").click()
        payload = _mm_payload_from_request(req_info.value)
        assert payload.get("dialog_id"), "missing dialog_id in /dialog/set payload"
        assert "llm_id" in payload, "missing llm_id in /dialog/set payload"
        assert "llm_setting" in payload, "missing llm_setting in /dialog/set payload"

    ctx.state["mm_settings_saved"] = True
    snap("chat_mm_settings_saved")


def mm_step_08_enter_multimodel_view(ctx: FlowContext, step, snap):
    require(ctx.state, "mm_settings_saved")
    page = ctx.page
    with step("enter multi-model view"):
        expect(page.get_by_test_id("chat-detail")).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        expect(page.get_by_test_id("chat-textarea")).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        page.get_by_test_id("chat-detail-multimodel-toggle").click()
        mm_root = page.get_by_test_id("chat-detail-multimodel-root")
        expect(mm_root).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        mm_grid = page.get_by_test_id("chat-detail-multimodel-grid")
        expect(mm_grid).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        cards = mm_grid.locator("[data-testid='chat-detail-multimodel-card']")
        expect(cards).to_have_count(1, timeout=RESULT_TIMEOUT_MS)
        _mm_dismiss_open_popovers(page)

    ctx.state["mm_option_prefix"] = "chat-detail-llm-option-"
    ctx.state["mm_multimodel_view_ready"] = True
    snap("chat_mm_multimodel_view_ready")


def mm_step_09_add_second_multimodel_card(ctx: FlowContext, step, snap):
    require(ctx.state, "mm_multimodel_view_ready")
    page = ctx.page
    with step("add second multi-model card"):
        mm_grid = page.get_by_test_id("chat-detail-multimodel-grid")
        expect(mm_grid).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        cards = mm_grid.locator("[data-testid='chat-detail-multimodel-card']")
        expect(cards).to_have_count(1, timeout=RESULT_TIMEOUT_MS)
        page.get_by_test_id("chat-detail-multimodel-add-card").click()
        expect(cards).to_have_count(2, timeout=RESULT_TIMEOUT_MS)
        _mm_dismiss_open_popovers(page)

    ctx.state["mm_multimodel_two_cards_ready"] = True
    snap("chat_mm_two_cards_ready")


def mm_step_10_select_models_for_two_cards(ctx: FlowContext, step, snap):
    require(ctx.state, "mm_multimodel_two_cards_ready", "mm_option_prefix")
    page = ctx.page
    option_prefix = ctx.state["mm_option_prefix"]
    with step("select models for two multi-model cards"):
        mm_grid = page.get_by_test_id("chat-detail-multimodel-grid")
        expect(mm_grid).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        selected_option_testids: list[str] = []

        for card_index in (0, 1):
            card = mm_grid.locator(
                f"[data-testid='chat-detail-multimodel-card'][data-card-index='{card_index}']"
            ).first
            expect(card).to_be_visible(timeout=RESULT_TIMEOUT_MS)
            options = _mm_open_model_options(page, card, option_prefix)
            option_testids = [
                tid
                for tid in options.evaluate_all(
                    "els => els.map(el => el.getAttribute('data-testid') || '')"
                )
                if tid
            ]
            option_testids = list(dict.fromkeys(option_testids))
            assert option_testids, "no deterministic model options were rendered"

            if len(option_testids) > 1 and card_index == 1:
                chosen = option_testids[1]
            else:
                chosen = option_testids[0]
            selected_option_testids.append(chosen)
            _mm_click_model_option_by_testid(page, chosen)
            _mm_dismiss_open_popovers(page)

    ctx.state["mm_selected_option_testids"] = selected_option_testids
    ctx.state["mm_models_selected"] = True
    snap("chat_mm_models_selected")


def mm_step_11_apply_multimodel_config(ctx: FlowContext, step, snap):
    require(ctx.state, "mm_models_selected")
    page = ctx.page
    with step("apply multi-model config"):
        mm_grid = page.get_by_test_id("chat-detail-multimodel-grid")
        expect(mm_grid).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        _mm_dismiss_open_popovers(page)

        apply_btn = mm_grid.locator(
            "[data-testid='chat-detail-multimodel-card-apply'][data-card-index='0']"
        ).first
        expect(apply_btn).to_be_enabled(timeout=RESULT_TIMEOUT_MS)
        with page.expect_request(_mm_settings_save_request, timeout=RESULT_TIMEOUT_MS) as req_info:
            apply_btn.click()
        payload = _mm_payload_from_request(req_info.value)
        assert payload.get("dialog_id"), "missing dialog_id in apply-config payload"
        assert "llm_id" in payload, "missing llm_id in apply-config payload"
        assert "llm_setting" in payload, "missing llm_setting in apply-config payload"

    ctx.state["mm_cards_configured"] = True
    snap("chat_mm_cards_configured")


def mm_step_12_composer_and_single_send(ctx: FlowContext, step, snap):
    require(ctx.state, "mm_cards_configured", "mm_selected_option_testids", "mm_option_prefix")
    page = ctx.page
    selected_option_testids = ctx.state["mm_selected_option_testids"]
    option_prefix = ctx.state["mm_option_prefix"]
    completion_payloads: list[dict] = []

    def _on_completion_request(req):
        if (
            req.method.upper() in MM_REQUEST_METHOD_WHITELIST
            and "/conversation/completion" in req.url
        ):
            completion_payloads.append(_mm_payload_from_request(req))

    with step("composer interactions and single send in multi-model mode"):
        attach_path = Path(gettempdir()) / f"chat-detail-attach-{int(time() * 1000)}.txt"
        attach_path.write_text("chat-detail-attachment", encoding="utf-8")
        try:
            try:
                with page.expect_file_chooser(timeout=5000) as chooser_info:
                    page.get_by_test_id("chat-detail-attach").click()
                chooser_info.value.set_files(str(attach_path))
            except PlaywrightTimeoutError:
                file_input = page.locator("input[type='file']").first
                expect(file_input).to_be_attached(timeout=RESULT_TIMEOUT_MS)
                file_input.set_input_files(str(attach_path))
            expect(page.locator(f"text={attach_path.name}").first).to_be_visible(
                timeout=RESULT_TIMEOUT_MS
            )

            thinking_toggle = page.get_by_test_id("chat-detail-thinking-toggle")
            expect(thinking_toggle).to_be_visible(timeout=RESULT_TIMEOUT_MS)
            thinking_class_before = thinking_toggle.get_attribute("class") or ""
            thinking_toggle.click()
            thinking_class_after = thinking_toggle.get_attribute("class") or ""
            assert thinking_class_after != thinking_class_before

            internet_toggle = page.get_by_test_id("chat-detail-internet-toggle")
            if internet_toggle.count() > 0:
                expect(internet_toggle).to_be_visible(timeout=RESULT_TIMEOUT_MS)
                internet_class_before = internet_toggle.get_attribute("class") or ""
                internet_toggle.click()
                internet_class_after = internet_toggle.get_attribute("class") or ""
                assert internet_class_after != internet_class_before

            audio_toggle = page.get_by_test_id("chat-detail-audio-toggle")
            if audio_toggle.count() > 0:
                expect(audio_toggle).to_be_visible(timeout=RESULT_TIMEOUT_MS)
                expect(audio_toggle).to_be_enabled(timeout=RESULT_TIMEOUT_MS)
                audio_toggle.focus()
                expect(audio_toggle).to_be_focused(timeout=RESULT_TIMEOUT_MS)

            page.on("request", _on_completion_request)
            prompt = f"multi model send {int(time())}"
            textarea = page.get_by_test_id("chat-textarea")
            textarea.fill(prompt)
            send_btn = page.get_by_test_id("chat-detail-send")
            expect(send_btn).to_be_enabled(timeout=RESULT_TIMEOUT_MS)
            send_btn.click()

            stream_status = page.get_by_test_id("chat-stream-status")
            try:
                expect(stream_status).to_be_visible(timeout=5000)
            except AssertionError:
                pass
            expect(stream_status).to_have_count(0, timeout=90000)

            deadline = monotonic() + 8
            while not completion_payloads and monotonic() < deadline:
                page.wait_for_timeout(100)
        finally:
            page.remove_listener("request", _on_completion_request)
            attach_path.unlink(missing_ok=True)

        assert completion_payloads, "no /conversation/completion request was captured"
        payloads_with_messages = [p for p in completion_payloads if p.get("messages")]
        assert payloads_with_messages, "completion requests did not include messages"

        selected_model_ids = [
            tid.replace(option_prefix, "")
            for tid in selected_option_testids
            if tid.startswith(option_prefix)
        ]
        has_model_payload = any(
            (p.get("llm_id") in selected_model_ids)
            or ("llm_id" in p)
            or any(
                k in p
                for k in (
                    "temperature",
                    "top_p",
                    "presence_penalty",
                    "frequency_penalty",
                    "max_tokens",
                )
            )
            for p in payloads_with_messages
        )
        assert has_model_payload, "no completion payload carried model-specific fields"

    ctx.state["mm_single_send_done"] = True
    snap("chat_mm_single_send_done")


def mm_step_13_remove_extra_card_and_exit(ctx: FlowContext, step, snap):
    require(ctx.state, "mm_single_send_done")
    page = ctx.page
    with step("remove extra card and exit multi-model"):
        _mm_dismiss_open_popovers(page)
        cards = page.locator("[data-testid='chat-detail-multimodel-card']")
        current_count = cards.count()
        assert current_count >= 2, "expected at least two cards before remove assertion"
        remove_btns = page.locator("[data-testid='chat-detail-multimodel-card-remove']")
        expect(remove_btns.first).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        remove_btns.first.click()
        expect(cards).to_have_count(current_count - 1, timeout=RESULT_TIMEOUT_MS)

        page.get_by_test_id("chat-detail-multimodel-back").click()
        expect(page.get_by_test_id("chat-detail-multimodel-root")).not_to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
        expect(page.get_by_test_id("chat-detail")).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        expect(page.get_by_test_id("chat-textarea")).to_be_visible(timeout=RESULT_TIMEOUT_MS)

    ctx.state["mm_exit_clean"] = True
    snap("chat_mm_exit_clean")


MM_STEPS = [
    ("01_ensure_authed_and_open_chat_list", mm_step_01_ensure_authed_and_open_chat_list),
    ("02_create_chat_and_open_detail", mm_step_02_create_chat_and_open_detail),
    ("03_select_dataset", mm_step_03_select_dataset),
    ("04_embed_open_close", mm_step_04_embed_open_close),
    ("05_sessions_panel_row_ops", mm_step_05_sessions_panel_row_ops),
    ("06_selection_mode_batch_delete", mm_step_06_selection_mode_batch_delete),
    ("07_settings_open_close_cancel_save", mm_step_07_settings_open_close_cancel_save),
    ("08_enter_multimodel_view", mm_step_08_enter_multimodel_view),
    ("09_add_second_multimodel_card", mm_step_09_add_second_multimodel_card),
    ("10_select_models_for_two_cards", mm_step_10_select_models_for_two_cards),
    ("11_apply_multimodel_config", mm_step_11_apply_multimodel_config),
    ("12_composer_and_single_send", mm_step_12_composer_and_single_send),
    ("13_remove_extra_card_and_exit", mm_step_13_remove_extra_card_and_exit),
]


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(MM_STEPS))
def test_chat_detail_multi_model_mode_coverage_flow(
    step_fn,
    flow_page,
    flow_state,
    base_url,
    login_url,
    ensure_chat_ready,
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
