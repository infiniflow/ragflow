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
"""Test cases for env_flag in common.settings."""

import pytest

from common.misc_utils import env_flag


class TestEnvFlag:
    def test_unset_keeps_the_documented_default(self, monkeypatch):
        monkeypatch.delenv("SOME_FLAG", raising=False)
        assert env_flag("SOME_FLAG", True) is True
        assert env_flag("SOME_FLAG", False) is False

    @pytest.mark.parametrize("raw", ["1", "true", "TRUE", "True", "yes", "YES", "on", "ON", " true "])
    def test_truthy_vocabulary(self, monkeypatch, raw):
        monkeypatch.setenv("SOME_FLAG", raw)
        assert env_flag("SOME_FLAG", False) is True

    @pytest.mark.parametrize("raw", ["0", "false", "FALSE", "no", "NO", " false ", ""])
    def test_falsy_vocabulary(self, monkeypatch, raw):
        monkeypatch.setenv("SOME_FLAG", raw)
        assert env_flag("SOME_FLAG", True) is False

    @pytest.mark.parametrize("raw", ["off", "OFF", "disabled", "none", "n", "0.0"])
    def test_an_unrecognized_value_turns_a_default_on_flag_off(self, monkeypatch, raw):
        """The reason this helper exists.

        A deny-list parse (`value not in ("0", "false", "no")`) reads every one of
        these as enabled, so an administrator who writes `off` gets the opposite of
        what they asked for. For a switch that decides whether a stranger arriving
        over OAuth is provisioned an account, that is the wrong direction to guess in.
        """
        monkeypatch.setenv("SOME_FLAG", raw)
        assert env_flag("SOME_FLAG", True) is False

    def test_surrounding_whitespace_is_not_a_different_value(self, monkeypatch):
        monkeypatch.setenv("SOME_FLAG", "  false\n")
        assert env_flag("SOME_FLAG", True) is False


class TestTimeoutAssertionFlag:
    """`ENABLE_TIMEOUT_ASSERTION` is documented as enabling *or disabling* the
    parsing-task timeout, but every read was a bare presence check, so writing
    `false` armed a 280-second timeout instead of the effectively unbounded one.
    """

    @pytest.mark.parametrize("raw", ["false", "0", "no", "off", " false "])
    def test_a_disabled_value_disables_it(self, monkeypatch, raw):
        monkeypatch.setenv("ENABLE_TIMEOUT_ASSERTION", raw)
        assert env_flag("ENABLE_TIMEOUT_ASSERTION", False) is False

    @pytest.mark.parametrize("raw", ["1", "true", "yes", "on"])
    def test_an_enabled_value_still_enables_it(self, monkeypatch, raw):
        monkeypatch.setenv("ENABLE_TIMEOUT_ASSERTION", raw)
        assert env_flag("ENABLE_TIMEOUT_ASSERTION", False) is True

    def test_unset_leaves_it_off(self, monkeypatch):
        monkeypatch.delenv("ENABLE_TIMEOUT_ASSERTION", raising=False)
        assert env_flag("ENABLE_TIMEOUT_ASSERTION", False) is False
