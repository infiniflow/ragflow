#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

"""Persistence and EVA adapter for the Documents-only connection."""

import base64
import hashlib
import json
import os
from dataclasses import dataclass
from urllib.parse import urlparse

from cryptography.fernet import Fernet, InvalidToken

from api.db.db_models import Connector
from api.db.services.system_settings_service import SystemSettingsService
from common import settings
from common.data_source.config import DocumentSource
from common.data_source.eva_wiki_connector import EvaWikiConnector
from common.data_source.exceptions import ConnectorValidationError
from common.misc_utils import get_uuid

BUSINESS_DOCUMENTS_EVA_CONNECTOR_SETTING = "business_documents.eva_connector_id"
BUSINESS_DOCUMENTS_EVA_CONNECTION_SETTING = "business_documents.eva_connection"


@dataclass(frozen=True)
class DocumentsEvaConnection:
    id: str
    config: dict
    name: str = "EVA · Документы"
    source: str = DocumentSource.EVA_WIKI.value


def _stored_connection() -> dict | None:
    rows = SystemSettingsService.get_by_name(BUSINESS_DOCUMENTS_EVA_CONNECTION_SETTING)
    return json.loads(rows[0].value) if len(rows) == 1 else None


def _token_cipher() -> Fernet:
    key = os.getenv("RAGFLOW_CREDENTIALS_KEY") or settings.get_secret_key()
    if not key:
        raise ConnectorValidationError("EVA connection encryption key is unavailable")
    digest = hashlib.sha256(f"ragflow-documents-eva:{key}".encode()).digest()
    return Fernet(base64.urlsafe_b64encode(digest))


def get_documents_eva_connection(*, with_token: bool = False) -> DocumentsEvaConnection | None:
    stored = _stored_connection()
    if not stored or not stored.get("id"):
        return None
    config = {key: stored[key] for key in ("api_base_url", "web_base_url", "project_id", "verify_ssl", "include_archived")}
    if with_token and stored.get("encrypted_token"):
        try:
            token = _token_cipher().decrypt(stored["encrypted_token"].encode()).decode()
        except InvalidToken as error:
            raise ConnectorValidationError("EVA connection token could not be decrypted") from error
        config["credentials"] = {"eva_api_token": token}
    return DocumentsEvaConnection(id=stored["id"], config=config)


def get_business_documents_eva_connector_id() -> str | None:
    stored = _stored_connection()
    if stored is not None:
        return stored.get("id") or None
    rows = SystemSettingsService.get_by_name(BUSINESS_DOCUMENTS_EVA_CONNECTOR_SETTING)
    return (str(rows[0].value or "").strip() or None) if len(rows) == 1 else None


def _existing_connection():
    connection = get_documents_eva_connection()
    if connection or _stored_connection() is not None:
        return connection
    # Persisted document/change IDs still refer to the previously selected source.
    connector_id = get_business_documents_eva_connector_id()
    connector = Connector.get_or_none(Connector.id == connector_id) if connector_id else None
    return connector if connector and connector.source == DocumentSource.EVA_WIKI.value else None


def get_business_documents_settings() -> dict:
    connection = get_documents_eva_connection()
    stored = _stored_connection()
    if stored is None:
        connection = _existing_connection()
    config = connection.config if connection else {}
    return {
        "eva_connection": {
            "api_base_url": config.get("api_base_url", ""),
            "web_base_url": config.get("web_base_url", ""),
            "project_id": config.get("project_id", ""),
            "verify_ssl": config.get("verify_ssl", True),
            "include_archived": config.get("include_archived", False),
            "token_configured": bool(stored.get("encrypted_token")) if stored is not None else bool((config.get("credentials") or {}).get("eva_api_token")),
        }
    }


def _validated_config(value: object, *, require_project: bool = True) -> dict:
    if not isinstance(value, dict):
        raise ValueError("eva_connection must be an object")
    config = {}
    for key in ("api_base_url", "web_base_url", "project_id"):
        raw = value.get(key, "")
        if not isinstance(raw, str) or len(raw) > 2048:
            raise ValueError(f"{key} must be a string of at most 2048 characters")
        config[key] = raw.strip()
    for key in ("api_base_url", "web_base_url"):
        if key == "web_base_url" and not config[key]:
            config[key] = config["api_base_url"]
        try:
            parsed = urlparse(config[key])
            valid = parsed.scheme in {"http", "https"} and parsed.hostname and not (parsed.username or parsed.password or parsed.query or parsed.fragment)
            parsed.port
        except ValueError:
            valid = False
        if not valid:
            raise ValueError(f"{key} must be an absolute HTTP(S) URL without credentials, query or fragment")
        config[key] = config[key].rstrip("/")
    if require_project and not config["project_id"]:
        raise ValueError("project_id is required")
    for key, default in (("verify_ssl", True), ("include_archived", False)):
        config[key] = value.get(key, default)
        if not isinstance(config[key], bool):
            raise ValueError(f"{key} must be a boolean")
    return config


def _connection_token(value: dict, config: dict, existing) -> str:
    token = value.get("eva_api_token", "")
    if not isinstance(token, str) or len(token) > 4096:
        raise ValueError("eva_api_token must be a string of at most 4096 characters")
    if not isinstance(value.get("clear_token", False), bool):
        raise ValueError("clear_token must be a boolean")
    if value.get("clear_token"):
        return ""
    if token.strip():
        return token.strip()
    if existing and str(existing.config.get("api_base_url", "")).rstrip("/") == config["api_base_url"]:
        if isinstance(existing, DocumentsEvaConnection):
            existing = get_documents_eva_connection(with_token=True)
        return str((existing.config.get("credentials") or {}).get("eva_api_token") or "").strip()
    return ""


def prepare_business_documents_connection(value: object) -> dict:
    if value is None:
        return {}  # An explicit disabled setting must never fall back to a data source.
    config = _validated_config(value)
    existing = _existing_connection()
    token = _connection_token(value, config, existing)
    same_scope = existing and all(str(existing.config.get(key, "")).rstrip("/") == config[key] for key in ("api_base_url", "project_id"))
    return {
        **config,
        "id": existing.id if same_scope else get_uuid(),
        "encrypted_token": _token_cipher().encrypt(token.encode()).decode() if token else "",
    }


def discover_business_documents_eva_spaces(value: object) -> list[dict]:
    config = _validated_config(value, require_project=False)
    token = _connection_token(value, config, _existing_connection())
    client = EvaWikiConnector(**config, include_attachments=False)
    client.load_credentials({"eva_api_token": token})
    try:
        return client.list_projects()
    finally:
        client._session.close()
