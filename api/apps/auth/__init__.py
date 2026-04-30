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

from .github import GithubOAuthClient
from .ldap import LDAPClient
from .oauth import OAuthClient
from .oidc import OIDCClient

CLIENT_TYPES = {
    "oauth2": OAuthClient,
    "oidc": OIDCClient,
    "github": GithubOAuthClient,
    "ldap": LDAPClient,
}


def infer_channel_type(config):
    """Resolve the auth channel type for a single OAUTH_CONFIG entry.

    Order: explicit ``type`` field > OIDC discovery (``issuer``) >
    LDAP heuristic (``host`` + a bind option) > generic ``oauth2``.
    """
    declared = str(config.get("type", "")).lower()
    if declared:
        return declared
    if config.get("issuer"):
        return "oidc"
    if config.get("host") and (config.get("bind_dn_template") or config.get("bind_user_dn")):
        return "ldap"
    return "oauth2"


def get_auth_client(config):
    """Instantiate the auth client matching ``config``'s channel type.

    Raises ``ValueError`` for unknown types.
    """
    channel_type = infer_channel_type(config)
    client_class = CLIENT_TYPES.get(channel_type)
    if not client_class:
        raise ValueError(f"Unsupported type: {channel_type}")

    return client_class(config)
