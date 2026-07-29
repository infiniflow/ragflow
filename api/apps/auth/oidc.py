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

import jwt
from common.http_client import sync_request
from .oauth import OAuthClient


# Asymmetric signing algorithms safe to accept for OIDC ID tokens.
# Symmetric HMAC algorithms (HS*) are intentionally excluded — when the
# verification key is the asymmetric public key fetched from the provider's
# JWKS (as it is for every OIDC ID token), accepting HS256 lets an attacker
# forge tokens by HMAC-signing them with the public key bytes
# (RSA/HMAC algorithm-confusion attack, CWE-347). "none" is excluded for the
# obvious reason that it disables signature verification entirely.
_ALLOWED_OIDC_SIGNING_ALGS = frozenset(
    {
        "RS256",
        "RS384",
        "RS512",
        "ES256",
        "ES384",
        "ES512",
        "PS256",
        "PS384",
        "PS512",
        "EdDSA",
    }
)

# OIDC Core 1.0 § 2 makes RS256 the spec-default ``id_token_signing_alg``,
# so this is the safe fallback when a provider's discovery document does not
# advertise ``id_token_signing_alg_values_supported`` (or advertises only
# algorithms outside the safe allowlist).
_DEFAULT_OIDC_SIGNING_ALGS = ("RS256",)


def _resolve_id_token_signing_algs(metadata):
    """Return the algorithms to pass to ``jwt.decode(..., algorithms=...)``.

    Intersects the provider-advertised
    ``id_token_signing_alg_values_supported`` with
    :data:`_ALLOWED_OIDC_SIGNING_ALGS`. Falls back to
    :data:`_DEFAULT_OIDC_SIGNING_ALGS` when the provider does not advertise
    the field or advertises only algorithms outside the safe allowlist —
    crucially, the fallback is to RS256, **never** to whatever the JWT
    header claims at verification time.
    """
    advertised = metadata.get("id_token_signing_alg_values_supported") or []
    if not isinstance(advertised, (list, tuple)):
        advertised = []
    safe = [a for a in advertised if isinstance(a, str) and a in _ALLOWED_OIDC_SIGNING_ALGS]
    return safe or list(_DEFAULT_OIDC_SIGNING_ALGS)


class OIDCClient(OAuthClient):
    def __init__(self, config):
        """
        Initialize the OIDCClient with the provider's configuration.
        Use `issuer` as the single source of truth for configuration discovery.
        """
        self.issuer = config.get("issuer")
        if not self.issuer:
            raise ValueError("Missing issuer in configuration.")

        oidc_metadata = self._load_oidc_metadata(self.issuer)
        config.update(
            {
                "issuer": oidc_metadata["issuer"],
                "jwks_uri": oidc_metadata["jwks_uri"],
                "authorization_url": oidc_metadata["authorization_endpoint"],
                "token_url": oidc_metadata["token_endpoint"],
                "userinfo_url": oidc_metadata["userinfo_endpoint"],
            }
        )

        super().__init__(config)
        self.issuer = config["issuer"]
        self.jwks_uri = config["jwks_uri"]
        # Pin the accepted ID-token signing algorithms at construction time
        # from a trusted source (provider metadata + safe allowlist) so the
        # JWT verification step in :meth:`parse_id_token` cannot be tricked
        # by attacker-controlled JWT headers (CWE-345 / CWE-347).
        self.id_token_signing_algs = _resolve_id_token_signing_algs(oidc_metadata)

    @staticmethod
    def _load_oidc_metadata(issuer):
        """
        Load OIDC metadata from `/.well-known/openid-configuration`.
        """
        try:
            metadata_url = f"{issuer}/.well-known/openid-configuration"
            response = sync_request("GET", metadata_url, timeout=7)
            response.raise_for_status()
            return response.json()
        except Exception as e:
            raise ValueError(f"Failed to fetch OIDC metadata: {e}")

    def parse_id_token(self, id_token):
        """
        Parse and validate OIDC ID Token (JWT format) with signature verification.

        The accepted signing algorithms come from ``self.id_token_signing_algs``
        (pinned at construction time from the provider's discovery metadata,
        intersected with :data:`_ALLOWED_OIDC_SIGNING_ALGS`). We deliberately
        do **not** read the algorithm from the unverified JWT header — doing
        so would let an attacker bypass signature verification by setting
        ``"alg": "none"`` or pull off the classic RSA / HMAC algorithm
        confusion by setting ``"alg": "HS256"`` and signing with the public
        key fetched from the provider's JWKS (CWE-345 / CWE-347).
        """
        try:
            # Use PyJWT's PyJWKClient to fetch JWKS and find signing key.
            # The client reads the ``kid`` from the JWT header internally to
            # look up the key — that's fine: ``kid`` is not a security
            # decision, the signature still proves which key was used.
            jwks_cli = jwt.PyJWKClient(self.jwks_uri)
            signing_key = jwks_cli.get_signing_key_from_jwt(id_token).key

            # Decode and verify signature against the pinned allowlist.
            decoded_token = jwt.decode(
                id_token,
                key=signing_key,
                algorithms=list(self.id_token_signing_algs),
                audience=str(self.client_id),
                issuer=self.issuer,
            )
            return decoded_token
        except Exception as e:
            raise ValueError(f"Error parsing ID Token: {e}")

    def fetch_user_info(self, access_token, id_token=None, **kwargs):
        """
        Fetch user info.
        """
        user_info = {}
        if id_token:
            user_info = self.parse_id_token(id_token)
        user_info.update(super().fetch_user_info(access_token).to_dict())
        return self.normalize_user_info(user_info)

    async def async_fetch_user_info(self, access_token, id_token=None, **kwargs):
        user_info = {}
        if id_token:
            user_info = self.parse_id_token(id_token)
        user_info.update((await super().async_fetch_user_info(access_token)).to_dict())
        return self.normalize_user_info(user_info)

    def normalize_user_info(self, user_info):
        return super().normalize_user_info(user_info)
