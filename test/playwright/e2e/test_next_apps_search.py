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


@pytest.mark.p1
@pytest.mark.auth
def test_search_create_select_dataset_and_results_nav_appears(
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

    with step("open search list"):
        _goto_home(page, base_url)
        _nav_click(page, "nav-search")
        expect(page.locator("[data-testid='search-list']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    snap("search_list_open")

    with step("open create search modal"):
        _open_create_from_list(page, "search-empty-create", "create-search")
    snap("search_create_modal")

    search_name = _unique_name("qa-search")
    with step("create search app"):
        _fill_and_save_create_modal(page, search_name)
        _wait_for_url_or_testid(page, r"/next-search/", "search-detail")
        expect(page.locator("[data-testid='search-detail']")).to_be_visible(
            timeout=RESULT_TIMEOUT_MS
        )
    snap("search_created")

    with step("select dataset"):
        search_input = page.locator(
            "input[placeholder*='How can I help you today']"
        ).first
        _select_first_dataset_and_save(
            page,
            timeout_ms=RESULT_TIMEOUT_MS,
            post_save_ready_locator=search_input,
        )
    snap("search_dataset_saved")

    with step("run search query"):
        expect(search_input).to_be_visible(timeout=RESULT_TIMEOUT_MS)
        search_input.fill("ragflow")
        search_input.press("Enter")
        _wait_for_results_navigation(page, timeout_ms=RESULT_TIMEOUT_MS)
    snap("search_results_nav")
