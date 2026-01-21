import pytest


@pytest.mark.smoke
@pytest.mark.p0
@pytest.mark.auth
def test_auth_page_smoke(smoke_login_url, page, step):
    with step("navigate to login page"):
        response = page.goto(smoke_login_url, wait_until="domcontentloaded")

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
