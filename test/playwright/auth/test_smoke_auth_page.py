import pytest

from test.playwright.helpers.flow_steps import flow_params, require


def step_01_open_login(flow_page, flow_state, smoke_login_url, step):
    page = flow_page
    with step("navigate to login page"):
        response = page.goto(smoke_login_url, wait_until="domcontentloaded")
    flow_state["smoke_opened"] = True
    flow_state["smoke_response"] = response


def step_02_validate_page(flow_page, flow_state, smoke_login_url, step):
    require(flow_state, "smoke_opened")
    page = flow_page
    response = flow_state.get("smoke_response")
    content = page.content()
    content_type = ""
    status = None
    if response is not None:
        status = response.status
        content_type = response.headers.get("content-type", "")

    content_head = content.lstrip()[:200]
    looks_json = content_head.startswith("{") or content_head.startswith("[")
    is_html = "text/html" in content_type.lower() or "<html" in content.lower()

    if response is not None and status is not None and status >= 400:
        raise AssertionError(_format_diag(page, response, "HTTP error status"))

    if looks_json or not is_html:
        raise AssertionError(_format_diag(page, response, "Non-HTML response"))

    root_count = page.locator("#root").count()
    input_count = page.locator("input").count()
    logo_count = page.locator("img[alt='logo']").count()
    if root_count + input_count + logo_count == 0:
        raise AssertionError(
            _format_diag(page, response, "No SPA root, inputs, or logo found")
        )


STEPS = [
    ("01_open_login", step_01_open_login),
    ("02_validate_page", step_02_validate_page),
]


@pytest.mark.smoke
@pytest.mark.p0
@pytest.mark.auth
@pytest.mark.parametrize("step_fn", flow_params(STEPS))
def test_auth_page_smoke_flow(step_fn, flow_page, flow_state, smoke_login_url, step):
    step_fn(flow_page, flow_state, smoke_login_url, step)


def _format_diag(page, response, reason: str) -> str:
    status = response.status if response is not None else "<no response>"
    content_type = ""
    if response is not None:
        content_type = response.headers.get("content-type", "")
    url = page.url
    title = page.title()
    snippet = page.content().strip().replace("\n", " ")[:500]
    return (
        f"{reason}. url={url} title={title} status={status} "
        f"content_type={content_type} snippet={snippet}"
    )
