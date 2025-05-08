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
import requests
from .oauth import OAuthClient


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
        config.update({
            'issuer': oidc_metadata['issuer'],
            'jwks_uri': oidc_metadata['jwks_uri'], 
            'authorization_url': oidc_metadata['authorization_endpoint'],
            'token_url': oidc_metadata['token_endpoint'],
            'userinfo_url': oidc_metadata['userinfo_endpoint']
        })

        super().__init__(config)
        self.issuer = config['issuer']
        self.jwks_uri = config['jwks_uri']


    def _load_oidc_metadata(self, issuer):
        """
        Load OIDC metadata from `/.well-known/openid-configuration`.
        """
        try:
            metadata_url = f"{issuer}/.well-known/openid-configuration"
            response = requests.get(metadata_url, timeout=7)
            response.raise_for_status()
            return response.json()
        except requests.exceptions.RequestException as e:
            raise ValueError(f"Failed to fetch OIDC metadata: {e}")


    def parse_id_token(self, id_token):
        """
        Parse and validate OIDC ID Token (JWT format) with signature verification.
        """
        try:
            # Decode JWT header without verifying signature
            headers = jwt.get_unverified_header(id_token)
            
            # OIDC usually uses `RS256` for signing
            alg = headers.get("alg", "RS256")

            # Use PyJWT's PyJWKClient to fetch JWKS and find signing key
            jwks_url = f"{self.issuer}/.well-known/jwks.json"
            jwks_cli = jwt.PyJWKClient(jwks_url)
            signing_key = jwks_cli.get_signing_key_from_jwt(id_token).key

            # Decode and verify signature
            decoded_token = jwt.decode(
                id_token,
                key=signing_key,
                algorithms=[alg],  
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


    def normalize_user_info(self, user_info):
        return super().normalize_user_info(user_info)
