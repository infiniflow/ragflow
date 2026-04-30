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

import logging

from .oauth import UserInfo


class LDAPAuthError(Exception):
    pass


class LDAPClient:
    """
    LDAP authentication client.

    Two bind strategies are supported:
      * Direct bind: provide ``bind_dn_template`` such as
        ``"uid={username},ou=people,dc=example,dc=com"``. The client formats
        the template with the user-supplied username and binds with the
        user-supplied password.
      * Search-then-bind: provide ``bind_user_dn`` + ``bind_user_password``
        (service account) plus ``user_search_base`` and ``user_search_filter``.
        The client first binds as the service account, locates the user
        entry, then re-binds with the user's DN and password.
    """

    DEFAULT_CONNECT_TIMEOUT = 5
    DEFAULT_RECEIVE_TIMEOUT = 10

    def __init__(self, config):
        self.config = config
        self.channel_id = config.get("channel") or config.get("display_name") or "ldap"
        self.host = config["host"]
        self.port = int(config.get("port", 389))
        self.use_ssl = bool(config.get("use_ssl", False))
        self.use_tls = bool(config.get("use_tls", False))

        # TLS hardening: validate certificates by default. Operators can
        # disable validation explicitly (e.g. self-signed test directories).
        self.tls_validate = bool(config.get("tls_validate", True))
        self.ca_certs_file = config.get("ca_certs_file")

        self.connect_timeout = int(config.get("connect_timeout", self.DEFAULT_CONNECT_TIMEOUT))
        self.receive_timeout = int(config.get("receive_timeout", self.DEFAULT_RECEIVE_TIMEOUT))

        self.bind_dn_template = config.get("bind_dn_template")
        self.bind_user_dn = config.get("bind_user_dn")
        self.bind_user_password = config.get("bind_user_password")
        self.user_search_base = config.get("user_search_base")
        self.user_search_filter = config.get("user_search_filter", "(uid={username})")

        self.email_attr = config.get("email_attr", "mail")
        self.nickname_attr = config.get("nickname_attr", "cn")
        self.username_attr = config.get("username_attr", "uid")

        # When True, refuse to authenticate users whose directory entry has
        # no email attribute. When False, fall back to a channel-namespaced
        # synthetic address so that distinct directories cannot collide on
        # one application account.
        self.require_email = bool(config.get("require_email", False))

        if not self.bind_dn_template and not self.bind_user_dn:
            raise ValueError("LDAP config must define either 'bind_dn_template' or 'bind_user_dn' for search-then-bind.")

    def _server(self):
        import ssl

        from ldap3 import Server, Tls

        tls = None
        if self.use_tls or self.use_ssl:
            validate = ssl.CERT_REQUIRED if self.tls_validate else ssl.CERT_NONE
            tls = Tls(validate=validate, ca_certs_file=self.ca_certs_file)

        return Server(
            host=self.host,
            port=self.port,
            use_ssl=self.use_ssl,
            tls=tls,
            get_info="NONE",
            connect_timeout=self.connect_timeout,
        )

    def authenticate(self, username, password):
        """
        Validate the user against the configured LDAP directory.

        Returns a :class:`UserInfo` on success. Raises :class:`LDAPAuthError`
        on invalid credentials or directory misconfiguration.
        """
        from ldap3.core.exceptions import LDAPException

        if not username or not password:
            raise LDAPAuthError("Username and password are required.")

        server = self._server()

        try:
            user_dn, attrs = self._resolve_user(server, username, password)
        except LDAPAuthError:
            raise
        except LDAPException:
            logging.exception("LDAP directory error")
            raise LDAPAuthError("LDAP directory error.")

        email = self._first(attrs.get(self.email_attr))
        nickname = self._first(attrs.get(self.nickname_attr)) or username
        login_name = self._first(attrs.get(self.username_attr)) or username

        if not email:
            if self.require_email:
                raise LDAPAuthError("Directory entry has no email attribute and require_email is enabled.")
            # Channel-namespaced fallback so two directories using the same
            # uid do not collapse onto a single application user.
            email = f"{login_name}@{self.channel_id}.ldap.local"

        return UserInfo(
            email=email,
            username=login_name,
            nickname=nickname,
            avatar_url=None,
        )

    def _open(self, server, user, password):
        """Build a Connection with the project-wide timeout and TLS policy."""
        from ldap3 import Connection, SIMPLE

        return Connection(
            server,
            user=user,
            password=password,
            authentication=SIMPLE,
            auto_bind=False,
            receive_timeout=self.receive_timeout,
        )

    def _bind(self, conn):
        """Bind the connection, performing StartTLS first when configured."""
        if self.use_tls and not self.use_ssl:
            if not conn.start_tls():
                raise LDAPAuthError("StartTLS negotiation failed.")
        return conn.bind()

    def _resolve_user(self, server, username, password):
        attrs_wanted = [self.email_attr, self.nickname_attr, self.username_attr]

        if self.bind_dn_template:
            user_dn = self.bind_dn_template.format(username=self._escape_rdn(username))
            conn = self._open(server, user_dn, password)
            if not self._bind(conn):
                raise LDAPAuthError("Invalid credentials.")
            try:
                if self.user_search_base:
                    conn.search(
                        search_base=self.user_search_base,
                        search_filter=f"({self.username_attr}={self._escape_filter(username)})",
                        attributes=attrs_wanted,
                    )
                    entry = conn.entries[0] if conn.entries else None
                    attrs = self._entry_attrs(entry, attrs_wanted)
                else:
                    attrs = {}
            finally:
                conn.unbind()
            return user_dn, attrs

        # search-then-bind
        service_conn = self._open(server, self.bind_user_dn, self.bind_user_password)
        if not self._bind(service_conn):
            raise LDAPAuthError("LDAP service account bind failed.")
        try:
            search_filter = self.user_search_filter.format(username=self._escape_filter(username))
            service_conn.search(
                search_base=self.user_search_base,
                search_filter=search_filter,
                attributes=attrs_wanted,
            )
            if not service_conn.entries:
                raise LDAPAuthError("User not found in directory.")
            entry = service_conn.entries[0]
            user_dn = entry.entry_dn
            attrs = self._entry_attrs(entry, attrs_wanted)
        finally:
            service_conn.unbind()

        user_conn = self._open(server, user_dn, password)
        if not self._bind(user_conn):
            raise LDAPAuthError("Invalid credentials.")
        user_conn.unbind()
        return user_dn, attrs

    @staticmethod
    def _escape_filter(value):
        # Escape per RFC 4515 to prevent LDAP filter injection.
        replacements = {
            "\\": r"\5c",
            "*": r"\2a",
            "(": r"\28",
            ")": r"\29",
            "\x00": r"\00",
        }
        for ch, rep in replacements.items():
            value = value.replace(ch, rep)
        return value

    @staticmethod
    def _escape_rdn(value):
        # RFC 4514 RDN-value escaping for safe interpolation into a DN.
        from ldap3.utils.dn import escape_rdn

        return escape_rdn(value)

    @staticmethod
    def _first(value):
        if value is None:
            return None
        if isinstance(value, (list, tuple)):
            return value[0] if value else None
        return value

    @staticmethod
    def _entry_attrs(entry, wanted):
        if entry is None:
            return {}
        result = {}
        for name in wanted:
            try:
                attr = entry[name]
                values = attr.values if hasattr(attr, "values") else attr
                result[name] = values
            except Exception:
                continue
        return result
