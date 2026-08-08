import pytest

from api.utils import web_utils


class _FailingSMTP:
    def __init__(self, fail_at, quit_fails=False):
        self.events = []
        self.fail_at = fail_at
        self.quit_fails = quit_fails

    async def connect(self):
        self.events.append("connect")

    async def login(self, _username, _password):
        self.events.append("login")
        if self.fail_at == "login":
            raise RuntimeError("login failed")

    async def send_message(self, _message):
        self.events.append("send")
        if self.fail_at == "send":
            raise RuntimeError("send failed")

    async def quit(self):
        self.events.append("quit")
        if self.quit_fails:
            raise RuntimeError("quit failed")


@pytest.mark.parametrize(
    ("fail_at", "expected_events"),
    [
        ("login", ["connect", "login", "quit"]),
        ("send", ["connect", "login", "send", "quit"]),
    ],
)
@pytest.mark.asyncio
async def test_send_email_closes_smtp_after_failure(
    monkeypatch,
    fail_at,
    expected_events,
):
    smtp = _FailingSMTP(fail_at)

    async def render_template(_template, **_context):
        return "body"

    monkeypatch.setattr(web_utils.aiosmtplib, "SMTP", lambda **_kwargs: smtp)
    monkeypatch.setattr(web_utils, "render_template_string", render_template)
    monkeypatch.setattr(web_utils, "EMAIL_TEMPLATES", {"test": "template"})
    monkeypatch.setattr(
        web_utils.settings,
        "MAIL_DEFAULT_SENDER",
        ("RAGFlow", "sender@example.com"),
    )
    monkeypatch.setattr(web_utils.settings, "MAIL_USERNAME", "user")
    monkeypatch.setattr(web_utils.settings, "MAIL_PASSWORD", "password")

    with pytest.raises(RuntimeError, match=f"{fail_at} failed"):
        await web_utils.send_email_html(
            "recipient@example.com",
            "subject",
            "test",
        )

    assert smtp.events == expected_events


@pytest.mark.parametrize("fail_at", ["login", "send"])
@pytest.mark.asyncio
async def test_send_email_preserves_operation_error_when_quit_fails(
    monkeypatch,
    fail_at,
):
    smtp = _FailingSMTP(fail_at, quit_fails=True)

    async def render_template(_template, **_context):
        return "body"

    monkeypatch.setattr(web_utils.aiosmtplib, "SMTP", lambda **_kwargs: smtp)
    monkeypatch.setattr(web_utils, "render_template_string", render_template)
    monkeypatch.setattr(web_utils, "EMAIL_TEMPLATES", {"test": "template"})
    monkeypatch.setattr(
        web_utils.settings,
        "MAIL_DEFAULT_SENDER",
        ("RAGFlow", "sender@example.com"),
    )
    monkeypatch.setattr(web_utils.settings, "MAIL_USERNAME", "user")
    monkeypatch.setattr(web_utils.settings, "MAIL_PASSWORD", "password")

    with pytest.raises(RuntimeError, match=f"{fail_at} failed"):
        await web_utils.send_email_html(
            "recipient@example.com",
            "subject",
            "test",
        )

    assert smtp.events[-1] == "quit"


@pytest.mark.asyncio
async def test_send_email_surfaces_quit_failure_after_success(monkeypatch):
    smtp = _FailingSMTP(None, quit_fails=True)

    async def render_template(_template, **_context):
        return "body"

    monkeypatch.setattr(web_utils.aiosmtplib, "SMTP", lambda **_kwargs: smtp)
    monkeypatch.setattr(web_utils, "render_template_string", render_template)
    monkeypatch.setattr(web_utils, "EMAIL_TEMPLATES", {"test": "template"})
    monkeypatch.setattr(
        web_utils.settings,
        "MAIL_DEFAULT_SENDER",
        ("RAGFlow", "sender@example.com"),
    )
    monkeypatch.setattr(web_utils.settings, "MAIL_USERNAME", "user")
    monkeypatch.setattr(web_utils.settings, "MAIL_PASSWORD", "password")

    with pytest.raises(RuntimeError, match="quit failed"):
        await web_utils.send_email_html(
            "recipient@example.com",
            "subject",
            "test",
        )
