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
    _select_first_dataset_and_save,
    _unique_name,
    _wait_for_url_or_testid,
)


def _wait_for_results_navigation(page, timeout_ms: int = RESULT_TIMEOUT_MS) -> None:
    wait_js = """
        () => {
          const top = document.querySelector("[data-testid='top-nav']");
          const navs = Array.from(document.querySelectorAll('[role="navigation"]'));
          return navs.some((nav) => !top || !top.contains(nav));
        }
        """
    page.wait_for_function(wait_js, timeout=timeout_ms)
    index = page.evaluate(
        """
        () => {
          const top = document.querySelector("[data-testid='top-nav']");
          const navs = Array.from(document.querySelectorAll('[role="navigation"]'));
          for (let i = 0; i < navs.length; i += 1) {
            if (!top || !top.contains(navs[i])) return i;
          }
          return -1;
        }
        """
    )
    navs = page.locator("[role='navigation']")
    target = navs.first if index < 0 else navs.nth(index)
    expect(target).to_be_visible(timeout=timeout_ms)


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


def step_02_open_search_list(
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
    with step("open search list"):
        _goto_home(page, base_url)
        _nav_click(page, "nav-search")
        expect(page.locator("[data-testid='search-list']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    snap("search_list_open")


def step_03_open_create_modal(
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
    with step("open create search modal"):
        _open_create_from_list(page, "search-empty-create", "create-search")
    flow_state["search_modal_open"] = True
    snap("search_create_modal")


def step_04_create_search(
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
    require(flow_state, "search_modal_open")
    page = flow_page
    search_name = _unique_name("qa-search")
    flow_state["search_name"] = search_name
    with step("create search app"):
        _fill_and_save_create_modal(page, search_name)
        _wait_for_url_or_testid(page, r"/next-search/", "search-detail")
        expect(page.locator("[data-testid='search-detail']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    flow_state["search_created"] = True
    snap("search_created")


def step_05_select_dataset(
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
    require(flow_state, "search_created")
    page = flow_page
    with step("select dataset"):
        search_input = page.locator(
            "input[placeholder*='How can I help you today']"
        ).first
        _select_first_dataset_and_save(
            page,
            timeout_ms=RESULT_TIMEOUT_MS,
            post_save_ready_locator=search_input,
        )
        flow_state["search_input_ready"] = True
    snap("search_dataset_saved")


def step_06_run_query(
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
    require(flow_state, "search_input_ready")
    page = flow_page
    search_input = page.locator("input[placeholder*='How can I help you today']").first
    with step("run search query"):
        expect(search_input).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        search_input.fill("ragflow")
        search_input.press("Enter")
        _wait_for_results_navigation(page, timeout_ms=RESULT_TIMEOUT_MS)
    snap("search_results_nav")


STEPS = [
    ("01_ensure_authed", step_01_ensure_authed),
    ("02_open_search_list", step_02_open_search_list),
    ("03_open_create_modal", step_03_open_create_modal),
    ("04_create_search", step_04_create_search),
    ("05_select_dataset", step_05_select_dataset),
    ("06_run_query", step_06_run_query),
]


@pytest.mark.p1
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(STEPS))
def test_search_create_select_dataset_and_results_nav_appears_flow(
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
