import re
from pathlib import Path

import pytest
from playwright.sync_api import expect

from test.playwright.helpers._auth_helpers import ensure_authed
from test.playwright.helpers._next_apps_helpers import (
    RESULT_TIMEOUT_MS,
    _fill_and_save_create_modal,
    _goto_home,
    _nav_click,
    _open_create_from_list,
    _unique_name,
    _wait_for_url_regex,
)


def _set_import_file(modal, file_path: str) -> None:
    upload_target = modal.locator("[data-testid='agent-import-file']").first
    if upload_target.count() == 0:
        raise AssertionError("agent-import-file not found in import modal.")
    tag_name = upload_target.evaluate("el => el.tagName.toLowerCase()")
    if tag_name == "input" and upload_target.get_attribute("type") == "file":
        upload_target.set_input_files(file_path)
        return
    file_input = modal.locator("input[type='file']").first
    if file_input.count() == 0:
        raise AssertionError("No file input found in agent import modal.")
    file_input.set_input_files(file_path)


@pytest.mark.p1
@pytest.mark.auth
def test_agent_create_then_import_json_then_run_and_wait_idle(
    base_url,
    login_url,
    page,
    active_auth_context,
    step,
    snap,
    auth_click,
):
    repo_root = Path(__file__).resolve().parents[3]
    dv_path = repo_root / "test/benchmark/test_docs/dv.json"
    if not dv_path.is_file():
        pytest.fail(f"Missing agent import fixture: {dv_path}")

    with step("ensure logged in"):
        ensure_authed(page, login_url, active_auth_context, auth_click)

    with step("open agent list"):
        _goto_home(page, base_url)
        _nav_click(page, "nav-agent")
        expect(page.locator("[data-testid='agents-list']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    snap("agent_list_open")

    first_name = _unique_name("qa-agent")
    with step("create first agent"):
        _open_create_from_list(page, "agents-empty-create", "create-agent")
        _fill_and_save_create_modal(page, first_name)
        expect(page.locator("[data-testid='agents-list']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
        expect(page.locator("[data-testid='agent-card']").first).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    snap("agent_first_created")

    second_name = _unique_name("qa-agent-import")
    with step("import agent json"):
        create_button = page.locator("[data-testid='create-agent']")
        expect(create_button).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        create_button.click()
        menu = page.locator("[data-testid='agent-create-menu']")
        expect(menu).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        menu.locator("[data-testid='agent-import-json']").click()

        modal = page.locator("[data-testid='agent-import-modal']")
        expect(modal).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        snap("agent_import_modal")

        _set_import_file(modal, str(dv_path))
        name_input = modal.locator("[data-testid='app-name-input']")
        expect(name_input).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        name_input.fill(second_name)
        save_button = modal.locator("[data-testid='app-save']")
        expect(save_button).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        save_button.click()
        expect(modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
    snap("agent_second_created")

    with step("open imported agent"):
        card = page.locator(
            "[data-testid='agent-card']",
            has=page.locator(
                "[data-testid='agent-name']", has_text=re.compile(second_name)
            ),
        ).first
        expect(card).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        auth_click(card, "open_agent")
        _wait_for_url_regex(page, r"/agent/")
        expect(page.locator("[data-testid='agent-detail']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    snap("agent_detail_open")

    with step("run agent"):
        run_button = page.locator("[data-testid='agent-run']")
        expect(run_button).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        run_button.click()
        run_chat = page.locator("[data-testid='agent-run-chat']")
        expect(run_chat).to_be_visible(timeout=RESULT_TIMEOUT_MS)
    snap("agent_run_started")

    with step("send agent chat"):
        textarea = page.locator("[data-testid='chat-textarea']")
        expect(textarea).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        textarea.fill("say hello")
        textarea.press("Enter")
        idle_marker = page.locator("[data-testid='agent-run-idle']")
        expect(idle_marker).to_be_visible(timeout=60000)
    snap("agent_run_idle_restored")
