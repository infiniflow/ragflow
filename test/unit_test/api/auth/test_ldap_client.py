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

from api.apps.auth.ldap import LDAPAuthError, LDAPClient


def _direct_bind_config(**overrides):
    base = {
        "host": "ldap.example.com",
        "port": 389,
        "channel": "corp",
        "bind_dn_template": "uid={username},ou=people,dc=example,dc=com",
    }
    base.update(overrides)
    return base


def _search_bind_config(**overrides):
    base = {
        "host": "ldap.example.com",
        "port": 389,
        "channel": "corp",
        "bind_user_dn": "cn=admin,dc=example,dc=com",
        "bind_user_password": "service",
        "user_search_base": "ou=people,dc=example,dc=com",
        "user_search_filter": "(uid={username})",
    }
    base.update(overrides)
    return base


class TestLDAPClientConfig:
    def test_requires_a_bind_strategy(self):
        with pytest.raises(ValueError):
            LDAPClient({"host": "ldap.example.com"})

    def test_search_then_bind_requires_user_search_base(self):
        cfg = _search_bind_config()
        cfg.pop("user_search_base")
        with pytest.raises(ValueError, match="user_search_base"):
            LDAPClient(cfg)

    def test_search_then_bind_rejects_empty_service_password(self):
        # An empty service-account password would let ldap3 fall back to
        # an anonymous bind, silently bypassing service-account auth.
        cfg = _search_bind_config()
        cfg["bind_user_password"] = ""
        with pytest.raises(ValueError, match="bind_user_password"):
            LDAPClient(cfg)

    def test_rejects_both_bind_strategies(self):
        cfg = _search_bind_config()
        cfg["bind_dn_template"] = "uid={username},dc=x"
        with pytest.raises(ValueError, match="not both"):
            LDAPClient(cfg)

    def test_channel_id_is_sanitised_for_synthetic_email(self):
        cfg = _direct_bind_config()
        cfg["channel"] = "Corp LDAP / Internal"
        cli = LDAPClient(cfg)
        assert cli.channel_id == "Corp-LDAP-Internal"

    def test_filter_escape_neutralizes_metacharacters(self):
        cli = LDAPClient(_direct_bind_config())
        assert cli._escape_filter("a*b(c)") == r"a\2ab\28c\29"
        assert cli._escape_filter("d\\e") == r"d\5ce"

    def test_rdn_escape_uses_ldap3_helper(self):
        cli = LDAPClient(_direct_bind_config())
        # Comma is a DN separator and must be escaped to live inside an RDN.
        assert cli._escape_rdn("a,b") == "a\\,b"


class TestDirectBind:
    def test_authenticate_returns_user_info_on_success(self):
        cli = LDAPClient(_direct_bind_config())

        bind_conn = MagicMock()
        bind_conn.bind.return_value = True
        bind_conn.entries = []

        with patch.object(cli, "_open", return_value=bind_conn) as opener, patch.object(cli, "_server"):
            info = cli.authenticate("alice", "secret")

        # _open was called with the formatted DN.
        first_call = opener.call_args_list[0]
        assert first_call.args[1] == "uid=alice,ou=people,dc=example,dc=com"
        assert first_call.args[2] == "secret"
        bind_conn.bind.assert_called_once()

        # Synthetic email is namespaced by channel, not directory host.
        assert info.email == "alice@corp.ldap.local"
        assert info.username == "alice"

    def test_authenticate_raises_on_invalid_credentials(self):
        cli = LDAPClient(_direct_bind_config())

        bind_conn = MagicMock()
        bind_conn.bind.return_value = False

        with patch.object(cli, "_open", return_value=bind_conn), patch.object(cli, "_server"):
            with pytest.raises(LDAPAuthError):
                cli.authenticate("alice", "wrong")

    def test_authenticate_rejects_empty_credentials(self):
        cli = LDAPClient(_direct_bind_config())
        with pytest.raises(LDAPAuthError):
            cli.authenticate("", "")

    def test_require_email_blocks_users_without_mail(self):
        cli = LDAPClient(_direct_bind_config(require_email=True))

        bind_conn = MagicMock()
        bind_conn.bind.return_value = True
        bind_conn.entries = []

        with patch.object(cli, "_open", return_value=bind_conn), patch.object(cli, "_server"):
            with pytest.raises(LDAPAuthError):
                cli.authenticate("alice", "secret")


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

        with patch.object(cli, "_open", side_effect=[service_conn, user_conn]) as opener, patch.object(cli, "_server"):
            info = cli.authenticate("bob", "userpw")

        assert opener.call_count == 2
        # Service account first, then user DN.
        first_call = opener.call_args_list[0]
        second_call = opener.call_args_list[1]
        assert first_call.args[1] == "cn=admin,dc=example,dc=com"
        assert second_call.args[1] == "uid=bob,ou=people,dc=example,dc=com"
        assert second_call.args[2] == "userpw"

        # Search filter was injected with the escaped username.
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

        with patch.object(cli, "_open", return_value=service_conn), patch.object(cli, "_server"):
            with pytest.raises(LDAPAuthError):
                cli.authenticate("ghost", "pw")

    def test_authenticate_rejects_ambiguous_search_match(self):
        cli = LDAPClient(_search_bind_config())

        service_conn = MagicMock()
        service_conn.bind.return_value = True
        # Two entries match the same login — must refuse rather than bind one
        # principal and return another's attributes.
        service_conn.entries = [MagicMock(), MagicMock()]

        with patch.object(cli, "_open", return_value=service_conn), patch.object(cli, "_server"):
            with pytest.raises(LDAPAuthError, match="Ambiguous"):
                cli.authenticate("dup", "pw")


class TestDirectBindPostSearch:
    def test_rejects_ambiguous_post_bind_search(self):
        # bind_dn_template + user_search_base path. The post-bind search must
        # also refuse multiple matches.
        cfg = _direct_bind_config(user_search_base="ou=people,dc=example,dc=com")
        cli = LDAPClient(cfg)

        bind_conn = MagicMock()
        bind_conn.bind.return_value = True
        bind_conn.entries = [MagicMock(), MagicMock()]

        with patch.object(cli, "_open", return_value=bind_conn), patch.object(cli, "_server"):
            with pytest.raises(LDAPAuthError, match="Ambiguous"):
                cli.authenticate("alice", "secret")


class TestStartTLS:
    def test_use_tls_opens_socket_then_start_tls_then_bind(self):
        cli = LDAPClient(_direct_bind_config(use_tls=True))

        conn = MagicMock()
        conn.start_tls.return_value = True
        conn.bind.return_value = True
        conn.entries = []

        with patch.object(cli, "_open", return_value=conn), patch.object(cli, "_server"):
            cli.authenticate("alice", "secret")

        # Order matters: ldap3.start_tls() requires an open socket, and bind
        # must come after the channel has been upgraded.
        manager = MagicMock()
        manager.attach_mock(conn.open, "open")
        manager.attach_mock(conn.start_tls, "start_tls")
        manager.attach_mock(conn.bind, "bind")
        conn.open.assert_called_once()
        conn.start_tls.assert_called_once()
        conn.bind.assert_called_once()

    def test_failed_start_tls_aborts_login(self):
        cli = LDAPClient(_direct_bind_config(use_tls=True))

        conn = MagicMock()
        conn.start_tls.return_value = False

        with patch.object(cli, "_open", return_value=conn), patch.object(cli, "_server"):
            with pytest.raises(LDAPAuthError):
                cli.authenticate("alice", "secret")

        conn.bind.assert_not_called()
