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

"""Google OAuth env switches must read `off` as off.

`GOOGLE_OAUTH_OPEN_BROWSER` and `GOOGLE_OAUTH_ALLOW_CONSOLE_FALLBACK` were parsed
with `... .lower() != "false"`, a deny-list that reads every value it does not
recognize as enabled. These tests call the real flow function and assert on the
keyword it hands to `run_local_server` / whether it falls back to `run_console`,
so they fail against the deny-list parse rather than merely restating it.
"""

import json
import sys
import types

import pytest

from common.data_source.config import DocumentSource
from common.data_source.google_util import oauth_flow

TIMEOUT_OFF = "0"  # GOOGLE_OAUTH_FLOW_TIMEOUT_SECS: take the synchronous path


class _FlowProbe:
    """Records what the flow function actually asks the OAuth library to do."""

    def __init__(self, fail_with: Exception | None = None):
        self.open_browser: object = "not-called"
        self.console_fallback_used = False
        self._fail_with = fail_with

    def make_module(self) -> tuple[types.ModuleType, types.ModuleType]:
        probe = self

        class _Credentials:
            def to_json(self):
                return json.dumps({"token": "tok", "refresh_token": "ref"})

        class _Flow:
            def run_local_server(self, **kwargs):
                probe.open_browser = kwargs.get("open_browser", "missing")
                if probe._fail_with is not None:
                    raise probe._fail_with
                return _Credentials()

            def run_console(self):
                probe.console_fallback_used = True
                return _Credentials()

        class InstalledAppFlow:
            @staticmethod
            def from_client_config(_client_config, scopes=None):
                return _Flow()

        flow_mod = types.ModuleType("google_auth_oauthlib.flow")
        flow_mod.InstalledAppFlow = InstalledAppFlow
        pkg_mod = types.ModuleType("google_auth_oauthlib")
        pkg_mod.flow = flow_mod
        return flow_mod, pkg_mod


def _install_probe(monkeypatch, probe: _FlowProbe):
    flow_mod, pkg_mod = probe.make_module()
    monkeypatch.setitem(sys.modules, "google_auth_oauthlib", pkg_mod)
    monkeypatch.setitem(sys.modules, "google_auth_oauthlib.flow", flow_mod)
    monkeypatch.setenv("GOOGLE_OAUTH_FLOW_TIMEOUT_SECS", TIMEOUT_OFF)


def _run_flow(monkeypatch, probe: _FlowProbe):
    _install_probe(monkeypatch, probe)
    oauth_flow._run_local_server_flow({"installed": {"client_id": "x", "client_secret": "y"}}, DocumentSource.GOOGLE_DRIVE)


@pytest.mark.parametrize("raw", ["false", "0", "no", "off", "OFF", " off ", "disabled", ""])
def test_a_disabled_value_does_not_open_a_browser(monkeypatch, raw):
    """The deny-list parse read all of these as `open a browser`."""
    monkeypatch.setenv("GOOGLE_OAUTH_OPEN_BROWSER", raw)
    probe = _FlowProbe()
    _run_flow(monkeypatch, probe)
    assert probe.open_browser is False


@pytest.mark.parametrize("raw", ["true", "1", "yes", "on", "TRUE"])
def test_an_enabled_value_still_opens_a_browser(monkeypatch, raw):
    monkeypatch.setenv("GOOGLE_OAUTH_OPEN_BROWSER", raw)
    probe = _FlowProbe()
    _run_flow(monkeypatch, probe)
    assert probe.open_browser is True


def test_unset_opens_a_browser_as_documented(monkeypatch):
    monkeypatch.delenv("GOOGLE_OAUTH_OPEN_BROWSER", raising=False)
    probe = _FlowProbe()
    _run_flow(monkeypatch, probe)
    assert probe.open_browser is True


@pytest.mark.parametrize("raw", ["false", "0", "no", "off", ""])
def test_a_disabled_console_fallback_value_does_not_fall_back(monkeypatch, raw):
    monkeypatch.setenv("GOOGLE_OAUTH_ALLOW_CONSOLE_FALLBACK", raw)
    probe = _FlowProbe(fail_with=OSError("no local port"))
    _install_probe(monkeypatch, probe)
    with pytest.raises(OSError):
        oauth_flow._run_local_server_flow({"installed": {"client_id": "x", "client_secret": "y"}}, DocumentSource.GOOGLE_DRIVE)
    assert probe.console_fallback_used is False


@pytest.mark.parametrize("raw", ["true", "1", "yes"])
def test_an_enabled_console_fallback_value_still_falls_back(monkeypatch, raw):
    monkeypatch.setenv("GOOGLE_OAUTH_ALLOW_CONSOLE_FALLBACK", raw)
    probe = _FlowProbe(fail_with=OSError("no local port"))
    _install_probe(monkeypatch, probe)
    oauth_flow._run_local_server_flow({"installed": {"client_id": "x", "client_secret": "y"}}, DocumentSource.GOOGLE_DRIVE)
    assert probe.console_fallback_used is True
