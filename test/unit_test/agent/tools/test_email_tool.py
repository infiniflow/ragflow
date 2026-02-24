#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
from types import SimpleNamespace

from agent.tools.email import Email, EmailParam


class _DummySMTP:
    last_instance = None

    def __init__(self, host, port):
        self.host = host
        self.port = port
        self.starttls_called = False
        self.login_args = None
        type(self).last_instance = self

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, tb):
        return False

    def ehlo(self):
        pass

    def starttls(self, context=None):
        self.starttls_called = True

    def login(self, username, password):
        self.login_args = (username, password)

    def send_message(self, msg, from_addr, recipients):
        return None

    def sendmail(self, from_addr, recipients, msg):
        return None


def _build_tool():
    param = EmailParam()
    param.smtp_server = "smtp.example.com"
    param.smtp_port = 587
    param.email = "sender@example.com"
    param.smtp_username = ""
    param.password = "secret"
    param.sender_name = "Sender"
    param.check()

    tool = Email.__new__(Email)
    tool._param = param
    tool._canvas = SimpleNamespace(is_canceled=lambda: False, task_id="unit-test")
    return tool


def test_email_login_uses_smtp_username_when_provided(monkeypatch):
    tool = _build_tool()
    tool._param.smtp_username = "SES_SMTP_USERNAME"

    monkeypatch.setattr("agent.tools.email.smtplib.SMTP", _DummySMTP)

    ok = tool._invoke(
        to_email="to@example.com",
        subject="SMTP username test",
        content="<p>Hello</p>",
    )

    smtp = _DummySMTP.last_instance
    assert ok is True
    assert smtp.starttls_called is True
    assert smtp.login_args == ("SES_SMTP_USERNAME", "secret")


def test_email_login_falls_back_to_sender_email(monkeypatch):
    tool = _build_tool()

    monkeypatch.setattr("agent.tools.email.smtplib.SMTP", _DummySMTP)

    ok = tool._invoke(
        to_email="to@example.com",
        subject="SMTP fallback test",
        content="<p>Hello</p>",
    )

    smtp = _DummySMTP.last_instance
    assert ok is True
    assert smtp.login_args == ("sender@example.com", "secret")
