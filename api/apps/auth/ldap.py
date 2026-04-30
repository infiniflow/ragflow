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

    def __init__(self, config):
        self.config = config
        self.host = config["host"]
        self.port = int(config.get("port", 389))
        self.use_ssl = bool(config.get("use_ssl", False))
        self.use_tls = bool(config.get("use_tls", False))

        self.bind_dn_template = config.get("bind_dn_template")
        self.bind_user_dn = config.get("bind_user_dn")
        self.bind_user_password = config.get("bind_user_password")
        self.user_search_base = config.get("user_search_base")
        self.user_search_filter = config.get("user_search_filter", "(uid={username})")

        self.email_attr = config.get("email_attr", "mail")
        self.nickname_attr = config.get("nickname_attr", "cn")
        self.username_attr = config.get("username_attr", "uid")

        if not self.bind_dn_template and not self.bind_user_dn:
            raise ValueError("LDAP config must define either 'bind_dn_template' or 'bind_user_dn' for search-then-bind.")

    def _server(self):
        from ldap3 import Server, Tls
        import ssl

        tls = None
        if self.use_tls or self.use_ssl:
            tls = Tls(validate=ssl.CERT_NONE)
        return Server(
            host=self.host,
            port=self.port,
            use_ssl=self.use_ssl,
            tls=tls,
            get_info="ALL",
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
        except LDAPException as e:
            logging.exception("LDAP directory error")
            raise LDAPAuthError(f"LDAP error: {e}")

        email = self._first(attrs.get(self.email_attr))
        nickname = self._first(attrs.get(self.nickname_attr)) or username
        login_name = self._first(attrs.get(self.username_attr)) or username

        if not email:
            # Some directories don't expose mail; fall back to a synthetic one
            # so that the existing user model (which keys on email) still works.
            email = f"{login_name}@{self.host}"

        return UserInfo(
            email=email,
            username=login_name,
            nickname=nickname,
            avatar_url=None,
        )

    def _resolve_user(self, server, username, password):
        from ldap3 import Connection, SIMPLE

        attrs_wanted = [self.email_attr, self.nickname_attr, self.username_attr]

        if self.bind_dn_template:
            user_dn = self.bind_dn_template.format(username=self._escape(username))
            conn = Connection(
                server,
                user=user_dn,
                password=password,
                authentication=SIMPLE,
                auto_bind=False,
            )
            if not conn.bind():
                raise LDAPAuthError("Invalid credentials.")
            try:
                if self.user_search_base:
                    conn.search(
                        search_base=self.user_search_base,
                        search_filter=f"(distinguishedName={user_dn})" if "dc=" not in user_dn.lower() else f"({self.username_attr}={self._escape(username)})",
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
        service_conn = Connection(
            server,
            user=self.bind_user_dn,
            password=self.bind_user_password,
            authentication=SIMPLE,
            auto_bind=False,
        )
        if not service_conn.bind():
            raise LDAPAuthError("LDAP service account bind failed.")
        try:
            search_filter = self.user_search_filter.format(username=self._escape(username))
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

        user_conn = Connection(
            server,
            user=user_dn,
            password=password,
            authentication=SIMPLE,
            auto_bind=False,
        )
        if not user_conn.bind():
            raise LDAPAuthError("Invalid credentials.")
        user_conn.unbind()
        return user_dn, attrs

    @staticmethod
    def _escape(value):
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
