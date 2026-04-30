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

from unittest.mock import MagicMock, patch

import pytest

pytest.importorskip("ldap3")

from api.apps.auth.ldap import LDAPAuthError, LDAPClient  # noqa: E402


def _direct_bind_config():
    return {
        "host": "ldap.example.com",
        "port": 389,
        "bind_dn_template": "uid={username},ou=people,dc=example,dc=com",
    }


def _search_bind_config():
    return {
        "host": "ldap.example.com",
        "port": 389,
        "bind_user_dn": "cn=admin,dc=example,dc=com",
        "bind_user_password": "service",
        "user_search_base": "ou=people,dc=example,dc=com",
        "user_search_filter": "(uid={username})",
    }


class TestLDAPClientConfig:
    def test_requires_a_bind_strategy(self):
        with pytest.raises(ValueError):
            LDAPClient({"host": "ldap.example.com"})

    def test_escape_neutralizes_filter_metacharacters(self):
        cli = LDAPClient(_direct_bind_config())
        assert cli._escape("a*b(c)") == r"a\2ab\28c\29"
        assert cli._escape("d\\e") == r"d\5ce"


class TestDirectBind:
    def test_authenticate_returns_user_info_on_success(self):
        cli = LDAPClient(_direct_bind_config())

        bind_conn = MagicMock()
        bind_conn.bind.return_value = True
        bind_conn.entries = []

        with patch("ldap3.Connection", return_value=bind_conn) as conn_cls, patch(
            "ldap3.Server"
        ):
            info = cli.authenticate("alice", "secret")

        # Connection was constructed with the formatted DN.
        kwargs = conn_cls.call_args.kwargs
        assert kwargs["user"] == "uid=alice,ou=people,dc=example,dc=com"
        assert kwargs["password"] == "secret"
        bind_conn.bind.assert_called_once()

        # Synthetic email when directory does not return a mail attribute.
        assert info.email.endswith("@ldap.example.com")
        assert info.username == "alice"

    def test_authenticate_raises_on_invalid_credentials(self):
        cli = LDAPClient(_direct_bind_config())

        bind_conn = MagicMock()
        bind_conn.bind.return_value = False

        with patch("ldap3.Connection", return_value=bind_conn), patch("ldap3.Server"):
            with pytest.raises(LDAPAuthError):
                cli.authenticate("alice", "wrong")

    def test_authenticate_rejects_empty_credentials(self):
        cli = LDAPClient(_direct_bind_config())
        with pytest.raises(LDAPAuthError):
            cli.authenticate("", "")


class TestSearchThenBind:
    def test_authenticate_locates_user_then_rebinds(self):
        cli = LDAPClient(_search_bind_config())

        service_conn = MagicMock()
        service_conn.bind.return_value = True
        entry = MagicMock()
        entry.entry_dn = "uid=bob,ou=people,dc=example,dc=com"
        mail_attr = MagicMock()
        mail_attr.values = ["bob@example.com"]
        cn_attr = MagicMock()
        cn_attr.values = ["Bob Builder"]
        uid_attr = MagicMock()
        uid_attr.values = ["bob"]
        entry.__getitem__.side_effect = lambda name: {
            "mail": mail_attr,
            "cn": cn_attr,
            "uid": uid_attr,
        }[name]
        service_conn.entries = [entry]

        user_conn = MagicMock()
        user_conn.bind.return_value = True

        with patch("ldap3.Connection", side_effect=[service_conn, user_conn]) as conn_cls, patch(
            "ldap3.Server"
        ):
            info = cli.authenticate("bob", "userpw")

        assert conn_cls.call_count == 2
        # Service account first, then user DN.
        first_call = conn_cls.call_args_list[0].kwargs
        second_call = conn_cls.call_args_list[1].kwargs
        assert first_call["user"] == "cn=admin,dc=example,dc=com"
        assert second_call["user"] == "uid=bob,ou=people,dc=example,dc=com"
        assert second_call["password"] == "userpw"

        # Search filter was injected with escaped username.
        service_conn.search.assert_called_once()
        search_kwargs = service_conn.search.call_args.kwargs
        assert search_kwargs["search_filter"] == "(uid=bob)"

        assert info.email == "bob@example.com"
        assert info.nickname == "Bob Builder"
        assert info.username == "bob"

    def test_authenticate_raises_when_user_not_found(self):
        cli = LDAPClient(_search_bind_config())

        service_conn = MagicMock()
        service_conn.bind.return_value = True
        service_conn.entries = []

        with patch("ldap3.Connection", return_value=service_conn), patch("ldap3.Server"):
            with pytest.raises(LDAPAuthError):
                cli.authenticate("ghost", "pw")
