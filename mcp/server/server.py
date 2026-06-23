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

import hashlib
import json
import logging
import random
import secrets
import time
from collections import OrderedDict
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from functools import wraps
from typing import Any
from urllib.parse import urlencode

import click
import httpx
import mcp.types as types
from mcp.server.auth.provider import (
    AccessToken,
    AuthorizationCode,
    AuthorizationParams,
    AuthorizeError,
    OAuthAuthorizationServerProvider,
    RefreshToken,
)
from mcp.server.auth.routes import create_auth_routes
from mcp.server.auth.settings import ClientRegistrationOptions, RevocationOptions
from mcp.server.lowlevel import Server
from mcp.shared.auth import OAuthClientInformationFull, OAuthToken
from pydantic import AnyHttpUrl
from starlette.applications import Starlette
from starlette.middleware import Middleware
from starlette.responses import JSONResponse, Response
from starlette.routing import Mount, Route
from enum import StrEnum


class LaunchMode(StrEnum):
    SELF_HOST = "self-host"
    HOST = "host"
    OAUTH = "oauth"


class Transport(StrEnum):
    SSE = "sse"
    STEAMABLE_HTTP = "streamable-http"


BASE_URL = "http://127.0.0.1:9380"
HOST = "127.0.0.1"
PORT = "9382"
HOST_API_KEY = ""
MODE = ""
TRANSPORT_SSE_ENABLED = True
TRANSPORT_STREAMABLE_HTTP_ENABLED = True
JSON_RESPONSE = True

# OAuth / OIDC delegation config (used only when MODE == LaunchMode.OAUTH)
OAUTH_ISSUER_URL = ""          # Public URL of *this* MCP server, e.g. https://mcp.example.com
OIDC_ISSUER = ""               # External OIDC provider base URL, e.g. https://accounts.google.com
OIDC_CLIENT_ID = ""            # Client ID registered with the OIDC provider
OIDC_CLIENT_SECRET = ""        # Client secret registered with the OIDC provider
OIDC_SCOPES = "openid email"   # Space-separated scopes to request from the OIDC provider


class RAGFlowConnector:
    _MAX_DATASET_CACHE = 32
    _CACHE_TTL = 300
    # Keep in sync with api.utils.pagination_utils.REST_API_MAX_PAGE_SIZE.
    _REST_API_MAX_PAGE_SIZE = 100

    _dataset_metadata_cache: OrderedDict[str, tuple[dict, float | int]] = OrderedDict()  # "dataset_id" -> (metadata, expiry_ts)
    _document_metadata_cache: OrderedDict[str, tuple[list[tuple[str, dict]], float | int]] = OrderedDict()  # "dataset_id" -> ([(document_id, doc_metadata)], expiry_ts)

    def __init__(self, base_url: str, version="v1"):
        self.base_url = base_url
        self.version = version
        self.api_url = f"{self.base_url}/api/{self.version}"
        self._async_client = None

    async def _get_client(self):
        if self._async_client is None:
            self._async_client = httpx.AsyncClient(timeout=httpx.Timeout(60.0))
        return self._async_client

    async def close(self):
        if self._async_client is not None:
            await self._async_client.aclose()
            self._async_client = None

    async def _post(self, path, json=None, stream=False, files=None, api_key: str = ""):
        if not api_key:
            return None
        client = await self._get_client()
        res = await client.post(url=self.api_url + path, json=json, headers={"Authorization": f"Bearer {api_key}"})
        return res

    async def _get(self, path, params=None, api_key: str = ""):
        if not api_key:
            return None
        client = await self._get_client()
        res = await client.get(url=self.api_url + path, params=params, headers={"Authorization": f"Bearer {api_key}"})
        return res

    def _is_cache_valid(self, ts):
        return time.time() < ts

    def _get_expiry_timestamp(self):
        offset = random.randint(-30, 30)
        return time.time() + self._CACHE_TTL + offset

    def _get_cached_dataset_metadata(self, dataset_id):
        entry = self._dataset_metadata_cache.get(dataset_id)
        if entry:
            data, ts = entry
            if self._is_cache_valid(ts):
                self._dataset_metadata_cache.move_to_end(dataset_id)
                return data
        return None

    def _set_cached_dataset_metadata(self, dataset_id, metadata):
        self._dataset_metadata_cache[dataset_id] = (metadata, self._get_expiry_timestamp())
        self._dataset_metadata_cache.move_to_end(dataset_id)
        if len(self._dataset_metadata_cache) > self._MAX_DATASET_CACHE:
            self._dataset_metadata_cache.popitem(last=False)

    def _get_cached_document_metadata_by_dataset(self, dataset_id):
        entry = self._document_metadata_cache.get(dataset_id)
        if entry:
            data_list, ts = entry
            if self._is_cache_valid(ts):
                self._document_metadata_cache.move_to_end(dataset_id)
                return {doc_id: doc_meta for doc_id, doc_meta in data_list}
        return None

    def _set_cached_document_metadata_by_dataset(self, dataset_id, doc_id_meta_list):
        self._document_metadata_cache[dataset_id] = (doc_id_meta_list, self._get_expiry_timestamp())
        self._document_metadata_cache.move_to_end(dataset_id)

    async def _fetch_datasets_page(
        self,
        *,
        api_key: str,
        page: int,
        page_size: int,
        orderby: str = "create_time",
        desc: bool = True,
        id: str | None = None,
        name: str | None = None,
    ):
        """Fetch one structured page of accessible datasets from the backend API."""
        params = {"page": page, "page_size": page_size, "orderby": orderby, "desc": desc}
        if id:
            params["id"] = id
        if name:
            params["name"] = name

        res = await self._get("/datasets", params, api_key=api_key)
        if not res or res.status_code != 200:
            error_message = None
            if res is not None:
                try:
                    error_message = res.json().get("message")
                except Exception:
                    error_message = None
            raise Exception([types.TextContent(type="text", text=error_message or "Cannot process this operation.")])

        res_json = res.json()
        if res_json.get("code") != 0:
            raise Exception([types.TextContent(type="text", text=res_json.get("message", "Cannot process this operation."))])

        return res_json

    async def _fetch_all_datasets(
        self,
        *,
        api_key: str,
        orderby: str = "create_time",
        desc: bool = True,
        id: str | None = None,
        name: str | None = None,
    ):
        """Fetch all accessible datasets without exceeding the REST API page-size limit."""
        datasets = []
        page = 1

        while True:
            logging.debug("fetching all /datasets page=%s page_size=%s", page, self._REST_API_MAX_PAGE_SIZE)
            res_json = await self._fetch_datasets_page(
                api_key=api_key,
                page=page,
                page_size=self._REST_API_MAX_PAGE_SIZE,
                orderby=orderby,
                desc=desc,
                id=id,
                name=name,
            )
            page_datasets = res_json.get("data", [])
            logging.debug("received %s datasets from page=%s", len(page_datasets), page)
            if not page_datasets:
                break

            datasets.extend(page_datasets)
            total = res_json.get("total")
            if total is not None and len(datasets) >= total:
                break

            page += 1

        return datasets

    async def list_datasets(self, *, api_key: str, page: int = 1, page_size: int = -1, orderby: str = "create_time", desc: bool = True, id: str | None = None, name: str | None = None):
        """Return accessible datasets as newline-delimited JSON for MCP tool descriptions."""
        if page_size == -1:
            datasets = await self._fetch_all_datasets(api_key=api_key, orderby=orderby, desc=desc, id=id, name=name)
        else:
            page_size = min(page_size, self._REST_API_MAX_PAGE_SIZE)
            res_json = await self._fetch_datasets_page(api_key=api_key, page=page, page_size=page_size, orderby=orderby, desc=desc, id=id, name=name)
            datasets = res_json["data"]

        result_list = []
        for data in datasets:
            d = {"description": data["description"], "id": data["id"]}
            result_list.append(json.dumps(d, ensure_ascii=False))
        return "\n".join(result_list)

    async def resolve_dataset_ids(self, *, api_key: str):
        """Resolve all accessible dataset IDs for MCP retrieval fallback."""
        logging.info("Resolving accessible dataset IDs for MCP retrieval")
        try:
            datasets = await self._fetch_all_datasets(api_key=api_key)
        except Exception as exc:
            logging.warning("resolve_dataset_ids failed to fetch /datasets error=%s", exc)
            raise

        dataset_ids = [data["id"] for data in datasets if data.get("id")]
        resolved = list(dict.fromkeys(dataset_ids))
        logging.info("resolve_dataset_ids resolved %s accessible dataset IDs", len(resolved))
        return resolved

    async def retrieval(
        self,
        *,
        api_key: str,
        dataset_ids,
        document_ids=None,
        question="",
        page=1,
        page_size=30,
        similarity_threshold=0.2,
        vector_similarity_weight=0.3,
        top_k=1024,
        rerank_id: str | None = None,
        keyword: bool = False,
        force_refresh: bool = False,
    ):
        if document_ids is None:
            document_ids = []

        if not dataset_ids:
            logging.info("MCP retrieval omitted dataset_ids; resolving accessible datasets")
            dataset_ids = await self.resolve_dataset_ids(api_key=api_key)
            if not dataset_ids:
                logging.info("MCP retrieval found no accessible datasets for current user")
                raise Exception([types.TextContent(type="text", text="No accessible datasets found.")])

        data_json = {
            "page": page,
            "page_size": page_size,
            "similarity_threshold": similarity_threshold,
            "vector_similarity_weight": vector_similarity_weight,
            "top_k": top_k,
            "rerank_id": rerank_id,
            "keyword": keyword,
            "question": question,
            "dataset_ids": dataset_ids,
            "document_ids": document_ids,
        }
        # Send a POST request to the backend service (using requests library as an example, actual implementation may vary)
        res = await self._post("/retrieval", json=data_json, api_key=api_key)
        if not res or res.status_code != 200:
            raise Exception([types.TextContent(type="text", text="Cannot process this operation.")])

        res = res.json()
        if res.get("code") == 0:
            data = res["data"]
            chunks = []

            # Cache document metadata and dataset information
            document_cache, dataset_cache = await self._get_document_metadata_cache(dataset_ids, api_key=api_key, force_refresh=force_refresh)

            # Process chunks with enhanced field mapping including per-chunk metadata
            for chunk_data in data.get("chunks", []):
                enhanced_chunk = self._map_chunk_fields(chunk_data, dataset_cache, document_cache)
                chunks.append(enhanced_chunk)

            # Build structured response (no longer need response-level document_metadata)
            response = {
                "chunks": chunks,
                "pagination": {
                    "page": data.get("page", page),
                    "page_size": data.get("page_size", page_size),
                    "total_chunks": data.get("total", len(chunks)),
                    "total_pages": (data.get("total", len(chunks)) + page_size - 1) // page_size,
                },
                "query_info": {
                    "question": question,
                    "similarity_threshold": similarity_threshold,
                    "vector_weight": vector_similarity_weight,
                    "keyword_search": keyword,
                    "dataset_count": len(dataset_ids),
                },
            }

            return [types.TextContent(type="text", text=json.dumps(response, ensure_ascii=False))]

        raise Exception([types.TextContent(type="text", text=res.get("message"))])

    async def _get_document_metadata_cache(self, dataset_ids, *, api_key: str, force_refresh=False):
        """Cache document metadata for all documents in the specified datasets"""
        document_cache = {}
        dataset_cache = {}

        try:
            for dataset_id in dataset_ids:
                dataset_meta = None if force_refresh else self._get_cached_dataset_metadata(dataset_id)
                if not dataset_meta:
                    # First get dataset info for name
                    dataset_res = await self._get("/datasets", {"id": dataset_id, "page_size": 1}, api_key=api_key)
                    if dataset_res and dataset_res.status_code == 200:
                        dataset_data = dataset_res.json()
                        if dataset_data.get("code") == 0 and dataset_data.get("data"):
                            dataset_info = dataset_data["data"][0]
                            dataset_meta = {"name": dataset_info.get("name", "Unknown"), "description": dataset_info.get("description", "")}
                            self._set_cached_dataset_metadata(dataset_id, dataset_meta)
                if dataset_meta:
                    dataset_cache[dataset_id] = dataset_meta

                docs = None if force_refresh else self._get_cached_document_metadata_by_dataset(dataset_id)
                if docs is None:
                    page = 1
                    page_size = 30
                    doc_id_meta_list = []
                    docs = {}
                    while page:
                        docs_res = await self._get(f"/datasets/{dataset_id}/documents?page={page}", api_key=api_key)
                        if not docs_res:
                            break
                        docs_data = docs_res.json()
                        if docs_data.get("code") == 0 and docs_data.get("data", {}).get("docs"):
                            for doc in docs_data["data"]["docs"]:
                                doc_id = doc.get("id")
                                if not doc_id:
                                    continue
                                doc_meta = {
                                    "document_id": doc_id,
                                    "name": doc.get("name", ""),
                                    "location": doc.get("location", ""),
                                    "type": doc.get("type", ""),
                                    "size": doc.get("size"),
                                    "chunk_count": doc.get("chunk_count"),
                                    "create_date": doc.get("create_date", ""),
                                    "update_date": doc.get("update_date", ""),
                                    "token_count": doc.get("token_count"),
                                    "thumbnail": doc.get("thumbnail", ""),
                                    "dataset_id": doc.get("dataset_id", dataset_id),
                                    "meta_fields": doc.get("meta_fields", {}),
                                }
                                doc_id_meta_list.append((doc_id, doc_meta))
                                docs[doc_id] = doc_meta

                            page += 1
                            if docs_data.get("data", {}).get("total", 0) - page * page_size <= 0:
                                page = None

                        self._set_cached_document_metadata_by_dataset(dataset_id, doc_id_meta_list)
                if docs:
                    document_cache.update(docs)

        except Exception as e:
            # Gracefully handle metadata cache failures
            logging.error(f"Problem building the document metadata cache: {str(e)}")
            pass

        return document_cache, dataset_cache

    def _map_chunk_fields(self, chunk_data, dataset_cache, document_cache):
        """Preserve all original API fields and add per-chunk document metadata"""
        # Start with ALL raw data from API (preserve everything like original version)
        mapped = dict(chunk_data)

        # Add dataset name enhancement
        dataset_id = chunk_data.get("dataset_id") or chunk_data.get("kb_id")
        if dataset_id and dataset_id in dataset_cache:
            mapped["dataset_name"] = dataset_cache[dataset_id]["name"]
        else:
            mapped["dataset_name"] = "Unknown"

        # Add document name convenience field
        mapped["document_name"] = chunk_data.get("document_keyword", "")

        # Add per-chunk document metadata
        document_id = chunk_data.get("document_id")
        if document_id and document_id in document_cache:
            mapped["document_metadata"] = document_cache[document_id]

        return mapped


class _OIDCAuthCode(AuthorizationCode):
    """AuthorizationCode extended to carry the PKCE verifier state used mid-flow."""

    oidc_state: str  # opaque value we pass to the OIDC provider to bind callbacks


class _OIDCAccessToken(AccessToken):
    """AccessToken extended to carry the resolved RAGFlow API key."""

    ragflow_api_key: str


class OIDCDelegatingProvider(OAuthAuthorizationServerProvider):
    """
    OAuth 2.1 authorization server that delegates identity to an external OIDC provider.

    Flow:
      1. MCP client → /authorize  →  redirect to OIDC provider
      2. OIDC provider → /oauth/callback  →  exchange code, resolve API key
      3. /oauth/callback → redirect to MCP client redirect_uri with our own auth code
      4. MCP client → /token  →  exchange our auth code for our access token
      5. MCP tool call uses access token; server maps it back to the RAGFlow API key
    """

    _ACCESS_TOKEN_TTL = 3600   # 1 hour
    _AUTH_CODE_TTL = 300       # 5 minutes

    def __init__(self, oidc_issuer: str, oidc_client_id: str, oidc_client_secret: str, oidc_scopes: str, oauth_issuer_url: str):
        self._oidc_issuer = oidc_issuer.rstrip("/")
        self._oidc_client_id = oidc_client_id
        self._oidc_client_secret = oidc_client_secret
        self._oidc_scopes = oidc_scopes
        self._oauth_issuer_url = oauth_issuer_url.rstrip("/")

        # In-memory stores (sufficient for a single-process deployment; swap for
        # Redis-backed stores in a horizontally-scaled deployment).
        self._clients: dict[str, OAuthClientInformationFull] = {}
        self._auth_codes: dict[str, _OIDCAuthCode] = {}
        self._access_tokens: dict[str, _OIDCAccessToken] = {}
        self._refresh_tokens: dict[str, RefreshToken] = {}

        # Maps MCP auth code → resolved RAGFlow API key (consumed once during token exchange)
        self._auth_code_to_api_key: dict[str, str] = {}

        # Pending OIDC state → (mcp_client_id, mcp_authorization_params)
        # Keyed by the `state` we forward to the OIDC provider.
        self._pending_oidc: dict[str, tuple[str, AuthorizationParams]] = {}

        self._http: httpx.AsyncClient | None = None

    async def _get_http(self) -> httpx.AsyncClient:
        if self._http is None:
            self._http = httpx.AsyncClient(timeout=httpx.Timeout(30.0))
        return self._http

    async def close(self):
        if self._http is not None:
            await self._http.aclose()
            self._http = None

    # ------------------------------------------------------------------
    # OIDC provider discovery
    # ------------------------------------------------------------------

    async def _discover_oidc(self) -> dict:
        """Fetch OIDC discovery document from the provider."""
        http = await self._get_http()
        url = f"{self._oidc_issuer}/.well-known/openid-configuration"
        resp = await http.get(url)
        resp.raise_for_status()
        return resp.json()

    # ------------------------------------------------------------------
    # OAuthAuthorizationServerProvider protocol
    # ------------------------------------------------------------------

    async def get_client(self, client_id: str) -> OAuthClientInformationFull | None:
        return self._clients.get(client_id)

    async def register_client(self, client_info: OAuthClientInformationFull) -> None:
        self._clients[client_info.client_id] = client_info
        logging.info("oauth: registered dynamic client %s", client_info.client_id)

    async def authorize(self, client: OAuthClientInformationFull, params: AuthorizationParams) -> str:
        """
        Build the OIDC authorization URL and stash enough state to reconstruct the
        MCP authorization code after the callback.
        """
        try:
            discovery = await self._discover_oidc()
        except Exception as exc:
            logging.error("oauth: OIDC discovery failed: %s", exc)
            raise AuthorizeError(error="server_error", error_description="OIDC provider discovery failed") from exc

        authorization_endpoint = discovery.get("authorization_endpoint")
        if not authorization_endpoint:
            raise AuthorizeError(error="server_error", error_description="OIDC provider has no authorization_endpoint")

        oidc_state = secrets.token_urlsafe(32)
        self._pending_oidc[oidc_state] = (client.client_id, params)

        callback_url = f"{self._oauth_issuer_url}/oauth/callback"
        qs = urlencode({
            "response_type": "code",
            "client_id": self._oidc_client_id,
            "redirect_uri": callback_url,
            "scope": self._oidc_scopes,
            "state": oidc_state,
        })
        redirect = f"{authorization_endpoint}?{qs}"
        logging.debug("oauth: redirecting to OIDC provider: %s", redirect)
        return redirect

    async def handle_oidc_callback(self, oidc_code: str, oidc_state: str) -> tuple[str, str, str]:
        """
        Called by the /oauth/callback route.

        Exchanges the OIDC code for tokens, resolves the RAGFlow API key from the
        user's identity, mints our own authorization code, and returns
        (redirect_uri, mcp_code, state_part) so the route can redirect the MCP client.
        """
        entry = self._pending_oidc.pop(oidc_state, None)
        if entry is None:
            raise ValueError(f"Unknown or expired OIDC state: {oidc_state!r}")

        mcp_client_id, mcp_params = entry
        client = self._clients.get(mcp_client_id)
        if client is None:
            raise ValueError(f"Unknown MCP client_id after OIDC callback: {mcp_client_id!r}")

        # Exchange OIDC code for tokens
        try:
            discovery = await self._discover_oidc()
        except Exception as exc:
            raise RuntimeError("OIDC discovery failed during callback") from exc

        token_endpoint = discovery.get("token_endpoint")
        if not token_endpoint:
            raise RuntimeError("OIDC provider has no token_endpoint")

        callback_url = f"{self._oauth_issuer_url}/oauth/callback"
        http = await self._get_http()
        token_resp = await http.post(
            token_endpoint,
            data={
                "grant_type": "authorization_code",
                "code": oidc_code,
                "redirect_uri": callback_url,
                "client_id": self._oidc_client_id,
                "client_secret": self._oidc_client_secret,
            },
        )
        if token_resp.status_code != 200:
            raise RuntimeError(f"OIDC token exchange failed: {token_resp.status_code} {token_resp.text}")

        oidc_tokens = token_resp.json()
        id_token = oidc_tokens.get("id_token")
        oidc_access_token = oidc_tokens.get("access_token")

        # Resolve the user's email / subject from the UserInfo endpoint
        userinfo_endpoint = discovery.get("userinfo_endpoint")
        email = None
        if userinfo_endpoint and oidc_access_token:
            try:
                ui_resp = await http.get(userinfo_endpoint, headers={"Authorization": f"Bearer {oidc_access_token}"})
                if ui_resp.status_code == 200:
                    ui = ui_resp.json()
                    email = ui.get("email") or ui.get("sub")
            except Exception as exc:
                logging.warning("oauth: userinfo fetch failed: %s", exc)

        if not email:
            # Fall back to decoding the id_token subject without verification
            # (verification is left to the IdP; we trust the code exchange result)
            if id_token:
                try:
                    import base64 as _b64
                    payload_b64 = id_token.split(".")[1]
                    padding = 4 - len(payload_b64) % 4
                    payload = json.loads(_b64.urlsafe_b64decode(payload_b64 + "=" * padding))
                    email = payload.get("email") or payload.get("sub")
                except Exception as exc:
                    logging.warning("oauth: id_token decode failed: %s", exc)

        if not email:
            raise RuntimeError("Could not determine user identity from OIDC response")

        # Resolve the RAGFlow API key for this user identity
        ragflow_api_key = await self._resolve_ragflow_api_key(email)

        # Mint our own MCP authorization code
        mcp_code = secrets.token_urlsafe(32)
        auth_code = _OIDCAuthCode(
            code=mcp_code,
            scopes=mcp_params.scopes or [],
            expires_at=time.time() + self._AUTH_CODE_TTL,
            client_id=mcp_client_id,
            code_challenge=mcp_params.code_challenge,
            redirect_uri=mcp_params.redirect_uri,
            redirect_uri_provided_explicitly=mcp_params.redirect_uri_provided_explicitly,
            resource=mcp_params.resource,
            oidc_state=oidc_state,
        )
        self._auth_codes[mcp_code] = auth_code
        self._auth_code_to_api_key[mcp_code] = ragflow_api_key
        logging.info("oauth: minted MCP auth code for user sha256:%s (client %s)", _obfuscate_email(email), mcp_client_id)

        redirect_uri = str(mcp_params.redirect_uri)
        state_part = f"&state={mcp_params.state}" if mcp_params.state else ""
        return redirect_uri, mcp_code, state_part

    async def _resolve_ragflow_api_key(self, email: str) -> str:
        """
        Map an OIDC-authenticated email to a RAGFlow API key.

        Strategy: call /api/v1/user/setting with the email as a lookup. If no
        matching API key exists, raise so the caller surfaces a 401.
        Operators can pre-provision API keys in RAGFlow for each SSO user.
        """
        http = await self._get_http()
        try:
            resp = await http.get(
                f"{BASE_URL}/api/v1/user/oauth_api_key",
                params={"email": email},
                headers={"Authorization": f"Bearer {HOST_API_KEY}"} if HOST_API_KEY else {},
            )
            if resp.status_code == 200:
                data = resp.json()
                if data.get("code") == 0:
                    api_key = data.get("data", {}).get("api_key")
                    if api_key:
                        logging.debug("oauth: resolved API key for user sha256:%s", _obfuscate_email(email))
                        return api_key
        except Exception as exc:
            logging.warning("oauth: API key resolution request failed for sha256:%s: %s", _obfuscate_email(email), exc)

        # Fallback: if a global HOST_API_KEY is configured, use it (useful for
        # single-org deployments where all SSO users share one tenant).
        if HOST_API_KEY:
            logging.debug("oauth: no per-user API key found for sha256:%s; using HOST_API_KEY", _obfuscate_email(email))
            return HOST_API_KEY

        raise RuntimeError(f"No RAGFlow API key found for SSO user: {email}")

    async def load_authorization_code(self, client: OAuthClientInformationFull, authorization_code: str) -> _OIDCAuthCode | None:
        code = self._auth_codes.get(authorization_code)
        if code is None:
            return None
        if time.time() > code.expires_at:
            del self._auth_codes[authorization_code]
            return None
        if code.client_id != client.client_id:
            return None
        return code

    async def exchange_authorization_code(self, client: OAuthClientInformationFull, authorization_code: _OIDCAuthCode) -> OAuthToken:
        del self._auth_codes[authorization_code.code]

        # Re-resolve API key from the auth code's stored email isn't available here,
        # but we stashed the ragflow_api_key inside the access token we're about to mint.
        # We look it up again from the OIDC callback path where we stored it implicitly
        # via the access token map. Here we need to reconstruct it — so we stored it
        # in _pending_oidc during callback. Instead, store api_key in auth code.
        # We work around this by stashing in a side dict keyed by auth code.
        api_key = self._auth_code_to_api_key.pop(authorization_code.code, HOST_API_KEY or "")

        access_token_str = secrets.token_urlsafe(32)
        refresh_token_str = secrets.token_urlsafe(32)
        expires_at = int(time.time()) + self._ACCESS_TOKEN_TTL

        at = _OIDCAccessToken(
            token=access_token_str,
            client_id=client.client_id,
            scopes=authorization_code.scopes,
            expires_at=expires_at,
            ragflow_api_key=api_key,
        )
        self._access_tokens[access_token_str] = at

        rt = RefreshToken(
            token=refresh_token_str,
            client_id=client.client_id,
            scopes=authorization_code.scopes,
        )
        self._refresh_tokens[refresh_token_str] = rt

        logging.info("oauth: issued access token for client %s", client.client_id)
        return OAuthToken(
            access_token=access_token_str,
            token_type="Bearer",
            expires_in=self._ACCESS_TOKEN_TTL,
            refresh_token=refresh_token_str,
            scope=" ".join(authorization_code.scopes),
        )

    async def load_refresh_token(self, client: OAuthClientInformationFull, refresh_token: str) -> RefreshToken | None:
        rt = self._refresh_tokens.get(refresh_token)
        if rt is None or rt.client_id != client.client_id:
            return None
        return rt

    async def exchange_refresh_token(self, client: OAuthClientInformationFull, refresh_token: RefreshToken, scopes: list[str]) -> OAuthToken:
        del self._refresh_tokens[refresh_token.token]

        # Locate the existing access token for this client to get the api_key
        api_key = HOST_API_KEY or ""
        for at in list(self._access_tokens.values()):
            if at.client_id == client.client_id:
                api_key = at.ragflow_api_key
                break

        effective_scopes = scopes if scopes else refresh_token.scopes
        access_token_str = secrets.token_urlsafe(32)
        new_refresh_token_str = secrets.token_urlsafe(32)
        expires_at = int(time.time()) + self._ACCESS_TOKEN_TTL

        at = _OIDCAccessToken(
            token=access_token_str,
            client_id=client.client_id,
            scopes=effective_scopes,
            expires_at=expires_at,
            ragflow_api_key=api_key,
        )
        self._access_tokens[access_token_str] = at
        self._refresh_tokens[new_refresh_token_str] = RefreshToken(
            token=new_refresh_token_str,
            client_id=client.client_id,
            scopes=effective_scopes,
        )
        return OAuthToken(
            access_token=access_token_str,
            token_type="Bearer",
            expires_in=self._ACCESS_TOKEN_TTL,
            refresh_token=new_refresh_token_str,
            scope=" ".join(effective_scopes),
        )

    async def load_access_token(self, token: str) -> _OIDCAccessToken | None:
        at = self._access_tokens.get(token)
        if at is None:
            return None
        if at.expires_at and time.time() > at.expires_at:
            del self._access_tokens[token]
            return None
        return at

    async def revoke_token(self, token: AccessToken | RefreshToken) -> None:
        if isinstance(token, AccessToken):
            self._access_tokens.pop(token.token, None)
        else:
            self._refresh_tokens.pop(token.token, None)


# Module-level singleton for the OAuth provider (created in main() when MODE==oauth)
_oauth_provider: OIDCDelegatingProvider | None = None


class RAGFlowCtx:
    def __init__(self, connector: RAGFlowConnector):
        self.conn = connector


@asynccontextmanager
async def sse_lifespan(server: Server) -> AsyncIterator[dict]:
    ctx = RAGFlowCtx(RAGFlowConnector(base_url=BASE_URL))

    logging.info("Legacy SSE application started with StreamableHTTP session manager!")
    try:
        yield {"ragflow_ctx": ctx}
    finally:
        await ctx.conn.close()
        logging.info("Legacy SSE application shutting down...")


app = Server("ragflow-mcp-server", lifespan=sse_lifespan)
AUTH_TOKEN_STATE_KEY = "ragflow_auth_token"


def _to_text(value: Any) -> str:
    if isinstance(value, bytes):
        return value.decode(errors="ignore")
    return str(value)


def _obfuscate_email(email: str) -> str:
    """Return a short SHA-256 prefix of the email to avoid logging PII."""
    return hashlib.sha256(email.encode()).hexdigest()[:12]


def _extract_token_from_headers(headers: Any) -> str | None:
    if not headers or not hasattr(headers, "get"):
        return None

    auth_keys = ("authorization", "Authorization", b"authorization", b"Authorization")
    for key in auth_keys:
        auth = headers.get(key)
        if not auth:
            continue
        auth_text = _to_text(auth).strip()
        if auth_text.lower().startswith("bearer "):
            token = auth_text[7:].strip()
            if token:
                return token

    api_key_keys = ("api_key", "x-api-key", "Api-Key", "X-API-Key", b"api_key", b"x-api-key", b"Api-Key", b"X-API-Key")
    for key in api_key_keys:
        token = headers.get(key)
        if token:
            token_text = _to_text(token).strip()
            if token_text:
                return token_text

    return None


def _extract_token_from_request(request: Any) -> str | None:
    if request is None:
        return None

    state = getattr(request, "state", None)
    if state is not None:
        token = getattr(state, AUTH_TOKEN_STATE_KEY, None)
        if token:
            return token

    token = _extract_token_from_headers(getattr(request, "headers", None))
    if token and state is not None:
        setattr(state, AUTH_TOKEN_STATE_KEY, token)

    return token


def with_api_key(required: bool = True):
    def decorator(func):
        @wraps(func)
        async def wrapper(*args, **kwargs):
            ctx = app.request_context
            ragflow_ctx = ctx.lifespan_context.get("ragflow_ctx")
            if not ragflow_ctx:
                raise ValueError("Get RAGFlow Context failed")

            connector = ragflow_ctx.conn
            api_key = HOST_API_KEY

            if MODE == LaunchMode.HOST:
                api_key = _extract_token_from_request(getattr(ctx, "request", None)) or ""
                if required and not api_key:
                    raise ValueError("RAGFlow API key or Bearer token is required.")
            elif MODE == LaunchMode.OAUTH and _oauth_provider is not None:
                bearer = _extract_token_from_request(getattr(ctx, "request", None)) or ""
                if bearer:
                    at = await _oauth_provider.load_access_token(bearer)
                    if at is not None:
                        api_key = at.ragflow_api_key
                if required and not api_key:
                    raise ValueError("Valid OAuth access token is required.")

            return await func(*args, connector=connector, api_key=api_key, **kwargs)

        return wrapper

    return decorator


@app.list_tools()
@with_api_key(required=True)
async def list_tools(*, connector: RAGFlowConnector, api_key: str) -> list[types.Tool]:
    dataset_description = await connector.list_datasets(api_key=api_key)

    return [
        types.Tool(
            name="ragflow_retrieval",
            description="Retrieve relevant chunks from the RAGFlow retrieve interface based on the question. You can optionally specify dataset_ids to search only specific datasets, or omit dataset_ids entirely to search across ALL available datasets. You can also optionally specify document_ids to search within specific documents. When dataset_ids is not provided or is empty, the system will automatically search across all available datasets. Below is the list of all available datasets, including their descriptions and IDs:"
            + dataset_description,
            inputSchema={
                "type": "object",
                "properties": {
                    "dataset_ids": {"type": "array", "items": {"type": "string"}, "description": "Optional array of dataset IDs to search. If not provided or empty, all datasets will be searched."},
                    "document_ids": {"type": "array", "items": {"type": "string"}, "description": "Optional array of document IDs to search within."},
                    "question": {"type": "string", "description": "The question or query to search for."},
                    "page": {
                        "type": "integer",
                        "description": "Page number for pagination",
                        "default": 1,
                        "minimum": 1,
                    },
                    "page_size": {
                        "type": "integer",
                        "description": "Number of results to return per page (default: 10, max recommended: 50 to avoid token limits)",
                        "default": 10,
                        "minimum": 1,
                        "maximum": 100,
                    },
                    "similarity_threshold": {
                        "type": "number",
                        "description": "Minimum similarity threshold for results",
                        "default": 0.2,
                        "minimum": 0.0,
                        "maximum": 1.0,
                    },
                    "vector_similarity_weight": {
                        "type": "number",
                        "description": "Weight for vector similarity vs term similarity",
                        "default": 0.3,
                        "minimum": 0.0,
                        "maximum": 1.0,
                    },
                    "keyword": {
                        "type": "boolean",
                        "description": "Enable keyword-based search",
                        "default": False,
                    },
                    "top_k": {
                        "type": "integer",
                        "description": "Maximum results to consider before ranking",
                        "default": 1024,
                        "minimum": 1,
                        "maximum": 1024,
                    },
                    "rerank_id": {
                        "type": "string",
                        "description": "Optional reranking model identifier",
                    },
                    "force_refresh": {
                        "type": "boolean",
                        "description": "Set to true only if fresh dataset and document metadata is explicitly required. Otherwise, cached metadata is used (default: false).",
                        "default": False,
                    },
                },
                "required": ["question"],
            },
        ),
    ]


@app.call_tool()
@with_api_key(required=True)
async def call_tool(
    name: str,
    arguments: dict,
    *,
    connector: RAGFlowConnector,
    api_key: str,
) -> list[types.TextContent | types.ImageContent | types.EmbeddedResource]:
    if name == "ragflow_retrieval":
        document_ids = arguments.get("document_ids", [])
        dataset_ids = arguments.get("dataset_ids", [])
        question = arguments.get("question", "")
        page = arguments.get("page", 1)
        page_size = arguments.get("page_size", 10)
        similarity_threshold = arguments.get("similarity_threshold", 0.2)
        vector_similarity_weight = arguments.get("vector_similarity_weight", 0.3)
        keyword = arguments.get("keyword", False)
        top_k = arguments.get("top_k", 1024)
        rerank_id = arguments.get("rerank_id")
        force_refresh = arguments.get("force_refresh", False)

        return await connector.retrieval(
            api_key=api_key,
            dataset_ids=dataset_ids,
            document_ids=document_ids,
            question=question,
            page=page,
            page_size=page_size,
            similarity_threshold=similarity_threshold,
            vector_similarity_weight=vector_similarity_weight,
            keyword=keyword,
            top_k=top_k,
            rerank_id=rerank_id,
            force_refresh=force_refresh,
        )
    raise ValueError(f"Tool not found: {name}")


def create_starlette_app():
    routes = []
    middleware = None
    if MODE == LaunchMode.HOST:
        from starlette.types import ASGIApp, Receive, Scope, Send

        class AuthMiddleware:
            def __init__(self, app: ASGIApp):
                self.app = app

            async def __call__(self, scope: Scope, receive: Receive, send: Send):
                if scope["type"] != "http":
                    await self.app(scope, receive, send)
                    return

                path = scope["path"]
                if path.startswith("/messages/") or path.startswith("/sse") or path.startswith("/mcp"):
                    headers = dict(scope["headers"])
                    token = _extract_token_from_headers(headers)

                    if not token:
                        response = JSONResponse({"error": "Missing or invalid authorization header"}, status_code=401)
                        await response(scope, receive, send)
                        return
                    scope.setdefault("state", {})[AUTH_TOKEN_STATE_KEY] = token

                await self.app(scope, receive, send)

        middleware = [Middleware(AuthMiddleware)]

    elif MODE == LaunchMode.OAUTH and _oauth_provider is not None:
        # Add RFC 8414 / RFC 7591 / PKCE OAuth routes from the MCP SDK
        oauth_routes = create_auth_routes(
            provider=_oauth_provider,
            issuer_url=AnyHttpUrl(OAUTH_ISSUER_URL),
            client_registration_options=ClientRegistrationOptions(
                enabled=True,
                valid_scopes=["ragflow"],
                default_scopes=["ragflow"],
            ),
            revocation_options=RevocationOptions(enabled=True),
        )
        routes.extend(oauth_routes)

        # OIDC callback route — the external IdP redirects here after user login
        async def handle_oidc_callback(request):
            code = request.query_params.get("code")
            state = request.query_params.get("state")
            error = request.query_params.get("error")

            if error:
                logging.warning("oauth: OIDC callback error: %s", error)
                return JSONResponse({"error": error, "error_description": request.query_params.get("error_description", "")}, status_code=400)

            if not code or not state:
                return JSONResponse({"error": "invalid_request", "error_description": "Missing code or state"}, status_code=400)

            try:
                redirect_uri, mcp_code, state_part = await _oauth_provider.handle_oidc_callback(code, state)
            except Exception as exc:
                logging.error("oauth: OIDC callback processing failed: %s", exc)
                return JSONResponse({"error": "server_error", "error_description": str(exc)}, status_code=500)

            sep = "&" if "?" in redirect_uri else "?"
            location = f"{redirect_uri}{sep}code={mcp_code}{state_part}"
            from starlette.responses import RedirectResponse
            return RedirectResponse(url=location, status_code=302)

        routes.append(Route("/oauth/callback", endpoint=handle_oidc_callback, methods=["GET"]))

    # Add SSE routes if enabled
    if TRANSPORT_SSE_ENABLED:
        from mcp.server.sse import SseServerTransport

        sse = SseServerTransport("/messages/")

        async def handle_sse(request):
            async with sse.connect_sse(request.scope, request.receive, request._send) as streams:
                await app.run(streams[0], streams[1], app.create_initialization_options(experimental_capabilities={"headers": dict(request.headers)}))
            return Response()

        routes.extend(
            [
                Route("/sse", endpoint=handle_sse, methods=["GET"]),
                Mount("/messages/", app=sse.handle_post_message),
            ]
        )

    # Add streamable HTTP route if enabled
    streamablehttp_lifespan = None
    if TRANSPORT_STREAMABLE_HTTP_ENABLED:
        from mcp.server.streamable_http_manager import StreamableHTTPSessionManager
        from starlette.types import Receive, Scope, Send

        session_manager = StreamableHTTPSessionManager(
            app=app,
            event_store=None,
            json_response=JSON_RESPONSE,
            stateless=True,
        )

        class StreamableHTTPEntry:
            async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
                await session_manager.handle_request(scope, receive, send)

        streamable_http_entry = StreamableHTTPEntry()

        @asynccontextmanager
        async def streamablehttp_lifespan(app: Starlette) -> AsyncIterator[None]:
            async with session_manager.run():
                logging.info("StreamableHTTP application started with StreamableHTTP session manager!")
                try:
                    yield
                finally:
                    logging.info("StreamableHTTP application shutting down...")

        routes.extend(
            [
                Route("/mcp", endpoint=streamable_http_entry, methods=["GET", "POST", "DELETE"]),
                Mount("/mcp", app=streamable_http_entry),
            ]
        )

    return Starlette(
        debug=False,
        routes=routes,
        middleware=middleware,
        lifespan=streamablehttp_lifespan,
    )


@click.command()
@click.option("--base-url", type=str, default="http://127.0.0.1:9380", help="API base URL for RAGFlow backend")
@click.option("--host", type=str, default="127.0.0.1", help="Host to bind the RAGFlow MCP server")
@click.option("--port", type=int, default=9382, help="Port to bind the RAGFlow MCP server")
@click.option(
    "--mode",
    type=click.Choice(["self-host", "host", "oauth"]),
    default="self-host",
    help=(
        "Launch mode:\n"
        "  self-host: run MCP for a single tenant (requires --api-key)\n"
        "  host: multi-tenant mode, users must provide Authorization headers\n"
        "  oauth: OAuth 2.1/OIDC delegation mode (requires --oidc-issuer, --oidc-client-id, --oidc-client-secret, --oauth-issuer-url)"
    ),
)
@click.option("--api-key", type=str, default="", help="API key to use when in self-host mode (also used as fallback in oauth mode)")
@click.option(
    "--transport-sse-enabled/--no-transport-sse-enabled",
    default=True,
    help="Enable or disable legacy SSE transport mode (default: enabled)",
)
@click.option(
    "--transport-streamable-http-enabled/--no-transport-streamable-http-enabled",
    default=True,
    help="Enable or disable streamable-http transport mode (default: enabled)",
)
@click.option(
    "--json-response/--no-json-response",
    default=True,
    help="Enable or disable JSON response mode for streamable-http (default: enabled)",
)
@click.option("--oauth-issuer-url", type=str, default="", envvar="RAGFLOW_MCP_OAUTH_ISSUER_URL", help="Public HTTPS URL of this MCP server (used as OAuth issuer, e.g. https://mcp.example.com). Required for --mode=oauth.")
@click.option("--oidc-issuer", type=str, default="", envvar="RAGFLOW_MCP_OIDC_ISSUER", help="External OIDC provider base URL (e.g. https://accounts.google.com). Required for --mode=oauth.")
@click.option("--oidc-client-id", type=str, default="", envvar="RAGFLOW_MCP_OIDC_CLIENT_ID", help="Client ID registered with the OIDC provider. Required for --mode=oauth.")
@click.option("--oidc-client-secret", type=str, default="", envvar="RAGFLOW_MCP_OIDC_CLIENT_SECRET", help="Client secret registered with the OIDC provider. Required for --mode=oauth.")
@click.option("--oidc-scopes", type=str, default="openid email", envvar="RAGFLOW_MCP_OIDC_SCOPES", help="Space-separated OIDC scopes to request (default: 'openid email').")
def main(base_url, host, port, mode, api_key, transport_sse_enabled, transport_streamable_http_enabled, json_response,
         oauth_issuer_url, oidc_issuer, oidc_client_id, oidc_client_secret, oidc_scopes):
    import os

    import uvicorn
    from dotenv import load_dotenv

    load_dotenv()

    def parse_bool_flag(key: str, default: bool) -> bool:
        val = os.environ.get(key, str(default))
        return str(val).strip().lower() in ("1", "true", "yes", "on")

    global BASE_URL, HOST, PORT, MODE, HOST_API_KEY, TRANSPORT_SSE_ENABLED, TRANSPORT_STREAMABLE_HTTP_ENABLED, JSON_RESPONSE
    global OAUTH_ISSUER_URL, OIDC_ISSUER, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET, OIDC_SCOPES
    global _oauth_provider
    BASE_URL = os.environ.get("RAGFLOW_MCP_BASE_URL", base_url)
    HOST = os.environ.get("RAGFLOW_MCP_HOST", host)
    PORT = os.environ.get("RAGFLOW_MCP_PORT", str(port))
    MODE = os.environ.get("RAGFLOW_MCP_LAUNCH_MODE", mode)
    HOST_API_KEY = os.environ.get("RAGFLOW_MCP_HOST_API_KEY", api_key)
    TRANSPORT_SSE_ENABLED = parse_bool_flag("RAGFLOW_MCP_TRANSPORT_SSE_ENABLED", transport_sse_enabled)
    TRANSPORT_STREAMABLE_HTTP_ENABLED = parse_bool_flag("RAGFLOW_MCP_TRANSPORT_STREAMABLE_ENABLED", transport_streamable_http_enabled)
    JSON_RESPONSE = parse_bool_flag("RAGFLOW_MCP_JSON_RESPONSE", json_response)
    OAUTH_ISSUER_URL = oauth_issuer_url
    OIDC_ISSUER = oidc_issuer
    OIDC_CLIENT_ID = oidc_client_id
    OIDC_CLIENT_SECRET = oidc_client_secret
    OIDC_SCOPES = oidc_scopes

    if MODE == LaunchMode.SELF_HOST and not HOST_API_KEY:
        raise click.UsageError("--api-key is required when --mode is 'self-host'")

    if MODE == LaunchMode.OAUTH:
        missing = [name for name, val in [("--oauth-issuer-url", OAUTH_ISSUER_URL), ("--oidc-issuer", OIDC_ISSUER), ("--oidc-client-id", OIDC_CLIENT_ID), ("--oidc-client-secret", OIDC_CLIENT_SECRET)] if not val]
        if missing:
            raise click.UsageError(f"--mode=oauth requires: {', '.join(missing)}")
        _oauth_provider = OIDCDelegatingProvider(
            oidc_issuer=OIDC_ISSUER,
            oidc_client_id=OIDC_CLIENT_ID,
            oidc_client_secret=OIDC_CLIENT_SECRET,
            oidc_scopes=OIDC_SCOPES,
            oauth_issuer_url=OAUTH_ISSUER_URL,
        )

    if not TRANSPORT_STREAMABLE_HTTP_ENABLED and JSON_RESPONSE:
        JSON_RESPONSE = False

    print(
        r"""
__  __  ____ ____       ____  _____ ______     _______ ____
|  \/  |/ ___|  _ \     / ___|| ____|  _ \ \   / / ____|  _ \
| |\/| | |   | |_) |    \___ \|  _| | |_) \ \ / /|  _| | |_) |
| |  | | |___|  __/      ___) | |___|  _ < \ V / | |___|  _ <
|_|  |_|\____|_|        |____/|_____|_| \_\ \_/  |_____|_| \_\
        """,
        flush=True,
    )
    print(f"MCP launch mode: {MODE}", flush=True)
    print(f"MCP host: {HOST}", flush=True)
    print(f"MCP port: {PORT}", flush=True)
    print(f"MCP base_url: {BASE_URL}", flush=True)
    if MODE == LaunchMode.OAUTH:
        print(f"OAuth issuer URL: {OAUTH_ISSUER_URL}", flush=True)
        print(f"OIDC provider: {OIDC_ISSUER}", flush=True)
        print(f"OIDC client ID: {OIDC_CLIENT_ID}", flush=True)
        print(f"OIDC scopes: {OIDC_SCOPES}", flush=True)
        print(f"OAuth metadata: {OAUTH_ISSUER_URL}/.well-known/oauth-authorization-server", flush=True)
        print(f"OIDC callback URL: {OAUTH_ISSUER_URL}/oauth/callback", flush=True)

    if not any([TRANSPORT_SSE_ENABLED, TRANSPORT_STREAMABLE_HTTP_ENABLED]):
        print("At least one transport should be enabled, enable streamable-http automatically", flush=True)
        TRANSPORT_STREAMABLE_HTTP_ENABLED = True

    if TRANSPORT_SSE_ENABLED:
        print("SSE transport enabled: yes", flush=True)
        print("SSE endpoint available at /sse", flush=True)
    else:
        print("SSE transport enabled: no", flush=True)

    if TRANSPORT_STREAMABLE_HTTP_ENABLED:
        print("Streamable HTTP transport enabled: yes", flush=True)
        print("Streamable HTTP endpoint available at /mcp", flush=True)
        if JSON_RESPONSE:
            print("Streamable HTTP mode: JSON response enabled", flush=True)
        else:
            print("Streamable HTTP mode: SSE over HTTP enabled", flush=True)
    else:
        print("Streamable HTTP transport enabled: no", flush=True)
        if JSON_RESPONSE:
            print("Warning: --json-response ignored because streamable transport is disabled.", flush=True)

    uvicorn.run(
        create_starlette_app(),
        host=HOST,
        port=int(PORT),
    )


if __name__ == "__main__":
    """
    Launch examples:

    1. Self-host mode with both SSE and Streamable HTTP (in JSON response mode) enabled (default):
        uv run mcp/server/server.py --host=127.0.0.1 --port=9382 \
            --base-url=http://127.0.0.1:9380 \
            --mode=self-host --api-key=ragflow-xxxxx

    2. Host mode (multi-tenant, clients must provide Authorization headers):
        uv run mcp/server/server.py --host=127.0.0.1 --port=9382 \
            --base-url=http://127.0.0.1:9380 \
            --mode=host

    3. OAuth 2.1/OIDC delegation mode (e.g. Keycloak, Entra ID, Okta):
        uv run mcp/server/server.py --host=0.0.0.0 --port=9382 \
            --base-url=http://127.0.0.1:9380 \
            --mode=oauth \
            --oauth-issuer-url=https://mcp.example.com \
            --oidc-issuer=https://accounts.example.com/realms/myrealm \
            --oidc-client-id=ragflow-mcp \
            --oidc-client-secret=changeme \
            --oidc-scopes="openid email profile"

        Environment variable equivalents:
            RAGFLOW_MCP_OAUTH_ISSUER_URL, RAGFLOW_MCP_OIDC_ISSUER,
            RAGFLOW_MCP_OIDC_CLIENT_ID, RAGFLOW_MCP_OIDC_CLIENT_SECRET,
            RAGFLOW_MCP_OIDC_SCOPES

    4. Disable legacy SSE (only streamable HTTP will be active):
        uv run mcp/server/server.py --no-transport-sse-enabled \
            --mode=self-host --api-key=ragflow-xxxxx

    5. Disable streamable HTTP (only legacy SSE will be active):
        uv run mcp/server/server.py --no-transport-streamable-http-enabled \
            --mode=self-host --api-key=ragflow-xxxxx

    6. Use streamable HTTP with SSE-style events (disable JSON response):
        uv run mcp/server/server.py --transport-streamable-http-enabled --no-json-response \
            --mode=self-host --api-key=ragflow-xxxxx

    7. Disable both transports (for testing):
        uv run mcp/server/server.py --no-transport-sse-enabled --no-transport-streamable-http-enabled \
            --mode=self-host --api-key=ragflow-xxxxx
    """
    main()
