import re
from pathlib import Path

import pytest
from playwright.sync_api import expect

from test.playwright.helpers._auth_helpers import ensure_authed
from test.playwright.helpers.flow_steps import flow_params, require
from test.playwright.helpers._next_apps_helpers import (
    RESULT_TIMEOUT_MS,
    _fill_and_save_create_modal,
    _goto_home,
    _nav_click,
    _open_create_from_list,
    _unique_name,
    _wait_for_url_regex,
)


def _visible_testids(page, limit: int = 80):
    try:
        return page.evaluate(
            """
            (limit) => {
              const elements = Array.from(document.querySelectorAll('[data-testid]'));
              const visible = elements.filter((el) => {
                const style = window.getComputedStyle(el);
                if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') {
                  return false;
                }
                const rect = el.getBoundingClientRect();
                return rect.width > 0 && rect.height > 0;
              });
              const values = Array.from(
                new Set(
                  visible.map((el) => el.getAttribute('data-testid')).filter(Boolean),
                ),
              );
              values.sort();
              return values.slice(0, limit);
            }
            """,
            limit,
        )
    except Exception as exc:
        return [f"<testid_dump_failed: {exc}>"]


def _raise_with_diagnostics(page, message: str, snap=None, snap_name: str = "") -> None:
    testids = _visible_testids(page)
    if snap is not None and snap_name:
        try:
            snap(snap_name)
        except Exception:
            pass
    details = f"{message} url={page.url} testids={testids}"
    print(details, flush=True)
    raise AssertionError(details)


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


def step_01_ensure_authed(
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
    repo_root = Path(__file__).resolve().parents[3]
    dv_path = repo_root / "test/benchmark/test_docs/dv.json"
    if not dv_path.is_file():
        pytest.fail(f"Missing agent import fixture: {dv_path}")
    flow_state["dv_path"] = str(dv_path)

    with step("ensure logged in"):
        ensure_authed(
            flow_page,
            login_url,
            active_auth_context,
            auth_click,
            seeded_user_credentials=seeded_user_credentials,
        )
    flow_state["logged_in"] = True
    snap("authed")


def step_02_open_agent_list(
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
    require(flow_state, "logged_in")
    page = flow_page
    with step("open agent list"):
        _goto_home(page, base_url)
        _nav_click(page, "nav-agent")
        expect(page.locator("[data-testid='agents-list']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    snap("agent_list_open")


def step_03_create_first_agent(
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
    require(flow_state, "logged_in")
    page = flow_page
    first_name = _unique_name("qa-agent")
    flow_state["first_agent_name"] = first_name
    with step("create first agent"):
        _open_create_from_list(
            page,
            "agents-empty-create",
            "create-agent",
            modal_testid="agent-create-modal",
        )
        _fill_and_save_create_modal(
            page,
            first_name,
            modal_testid="agent-create-modal",
            name_input_testid="agent-name-input",
            save_testid="agent-save",
        )
        expect(page.locator("[data-testid='agents-list']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
        expect(page.locator("[data-testid='agent-card']").first).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    flow_state["first_agent_created"] = True
    snap("agent_first_created")


def step_04_import_agent(
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
    require(flow_state, "first_agent_created", "dv_path")
    page = flow_page
    second_name = _unique_name("qa-agent-import")
    flow_state["second_agent_name"] = second_name
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

        _set_import_file(modal, flow_state["dv_path"])
        name_input = modal.locator("[data-testid='agent-name-input']")
        expect(name_input).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        name_input.fill(second_name)
        save_button = modal.locator("[data-testid='agent-import-save']")
        expect(save_button).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        save_button.click()
        expect(modal).not_to_be_visible(timeout=RESULT_TIMEOUT_MS)
    flow_state["second_agent_created"] = True
    snap("agent_second_created")


def step_05_open_imported_agent(
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
    require(flow_state, "second_agent_created", "second_agent_name")
    page = flow_page
    with step("open imported agent"):
        card = page.locator(
            "[data-testid='agent-card']",
            has=page.locator(
                "[data-testid='agent-name']", has_text=re.compile(flow_state["second_agent_name"])
            ),
        ).first
        expect(card).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        auth_click(card, "open_agent")
        _wait_for_url_regex(page, r"/agent/")
        expect(page.locator("[data-testid='agent-detail']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    flow_state["agent_detail_open"] = True
    snap("agent_detail_open")


def step_06_run_agent(
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
    require(flow_state, "agent_detail_open")
    page = flow_page
    with step("run agent"):
        import os

        run_ui_timeout_ms = int(os.getenv("PW_AGENT_RUN_UI_TIMEOUT_MS", "60000"))
        run_root = page.locator("[data-testid='agent-run']")
        run_ui_selector = (
            "[data-testid='agent-run-chat'], "
            "[data-testid='chat-textarea'], "
            "[data-testid='agent-run-idle']"
        )
        run_ui_locator = page.locator(run_ui_selector)

        try:
            if run_ui_locator.count() > 0 and run_ui_locator.first.is_visible():
                flow_state["agent_running"] = True
                snap("agent_run_already_open")
                return
        except Exception:
            pass

        if run_root.count() == 0:
            run_button = page.get_by_role("button", name=re.compile(r"^run$", re.I))
        else:
            run_button = run_root
        expect(run_button).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        try:
            auth_click(run_button, "agent_run")
        except Exception:
            page.wait_for_timeout(500)
            auth_click(run_button, "agent_run_retry")

        try:
            run_ui_locator.first.wait_for(state="visible", timeout=run_ui_timeout_ms)
        except Exception:
            _raise_with_diagnostics(
                page,
                "Agent run UI did not open after clicking Run.",
                snap=snap,
                snap_name="agent_run_missing",
            )

        flow_state["agent_running"] = True
        snap("agent_run_started")
        return


def step_07_send_chat(
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
    require(flow_state, "agent_running")
    page = flow_page
    with step("send agent chat"):
        dataset_combobox = page.locator("[data-testid='chat-datasets-combobox']")
        if dataset_combobox.count() > 0:
            try:
                if dataset_combobox.is_visible():
                    dataset_combobox.click()
                    options = page.locator("[data-testid='datasets-options']")
                    expect(options).to_be_visible(timeout=RESULT_TIMEOUT_MS)
                    option = page.locator("[data-testid='datasets-option-0']")
                    if option.count() == 0:
                        option = page.locator("[data-testid^='datasets-option-']").first
                    if option.count() > 0 and option.is_visible():
                        try:
                            flow_state["dataset_label"] = option.inner_text()
                        except Exception:
                            flow_state["dataset_label"] = ""
                        option.click()
                        flow_state["dataset_selected"] = True
            except Exception:
                pass

        textarea = page.locator("[data-testid='chat-textarea']")
        idle_marker = page.locator("[data-testid='agent-run-idle']")
        try:
            expect(textarea).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        except AssertionError:
            _raise_with_diagnostics(
                page,
                "Chat textarea not visible in agent run UI.",
                snap=snap,
                snap_name="agent_run_chat_missing",
            )

        textarea.fill("say hello")
        textarea.press("Enter")
        try:
            expect(idle_marker).to_be_visible(timeout=60000)
        except AssertionError:
            # Older UI builds do not expose agent-run-idle; fallback to assistant reply.
            agent_chat = page.locator("[data-testid='agent-run-chat']")
            assistant_reply = agent_chat.locator(
                "text=/how can i assist|hello/i"
            ).first
            try:
                expect(assistant_reply).to_be_visible(timeout=60000)
            except AssertionError:
                _raise_with_diagnostics(
                    page,
                    "Agent run chat did not return to idle state after sending message.",
                    snap=snap,
                    snap_name="agent_run_idle_missing",
                )
    snap("agent_run_idle_restored")


STEPS = [
    ("01_ensure_authed", step_01_ensure_authed),
    ("02_open_agent_list", step_02_open_agent_list),
    ("03_create_first_agent", step_03_create_first_agent),
    ("04_import_agent", step_04_import_agent),
    ("05_open_imported_agent", step_05_open_imported_agent),
    ("06_run_agent", step_06_run_agent),
    ("07_send_chat", step_07_send_chat),
]


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(STEPS))
def test_agent_create_then_import_json_then_run_and_wait_idle_flow(
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
    step_fn(
        flow_page,
        flow_state,
        base_url,
        login_url,
        active_auth_context,
        step,
        snap,
        auth_click,
        seeded_user_credentials,
    )
