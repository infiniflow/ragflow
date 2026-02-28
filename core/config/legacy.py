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

from typing import Any

from pydantic_core import PydanticCustomError


def normalize_legacy_yaml(raw: dict[str, Any]) -> dict[str, Any]:
    """
    Temporarily normalize legacy YAML configuration to the new format.

    This function exists only to support legacy YAML keys for backward compatibility.
    It migrates old keys into the new configuration structure (database, storage,
    doc_engine, cache) if the new keys are not already present.

    Key behaviors:
    1. If legacy keys are found alongside new keys, a PydanticCustomError is raised
       to prevent ambiguity.
    2. Legacy keys are moved into the new structure and removed from the original dictionary.
    3. This is a temporary helper; users should migrate YAML files to the new format.

    Args:
        raw (dict[str, Any]): Raw configuration dictionary loaded from YAML.

    Returns:
        dict[str, Any]: Configuration dictionary normalized to the new format.

    Raises:
        PydanticCustomError: If both legacy and new keys exist, causing conflict.
    """
    if not raw:
        return raw

    raw = raw.copy()

    def _conflict(legacy_keys: tuple[str, ...], new_key: str):
        if new_key in raw and any(s in raw for s in legacy_keys):
            raise PydanticCustomError(
                "legacy_config_conflict",
                """Legacy config keys {legacy_keys} conflict with new config '{new_key}'.
                    Please remove the legacy configuration before continuing.
                    Only the new configuration format is supported.""",
                {"legacy_keys": legacy_keys, "new_key": new_key},
            )

    # database
    _conflict(("mysql", "postgres", "oceanbase"), "database")

    if "database" not in raw:
        db = {}
        for k in ("mysql", "postgres", "oceanbase"):
            if k in raw:
                db[k] = raw.pop(k)
        if db:
            raw["database"] = db

    # storage
    _conflict(
        ("minio", "s3", "oss", "azure", "azure_sas", "azure_spn", "opendal"),
        "storage"
    )

    if "storage" not in raw:
        storage = {}
        for k in ("minio", "s3", "oss", "azure", "azure_sas", "azure_spn", "opendal"):
            if k in raw:
                storage[k] = raw.pop(k)
        if storage:
            raw["storage"] = storage

    # doc_engine
    _conflict(("es", "os", "infinity"), "doc_engine")

    if "doc_engine" not in raw:
        doc = {}
        for k in ("es", "os", "infinity"):
            if k in raw:
                doc[k] = raw.pop(k)
        if doc:
            raw["doc_engine"] = doc

    # cache
    _conflict(("redis",), "cache")

    if "cache" not in raw and "redis" in raw:
        raw["cache"] = {"redis": raw.pop("redis")}

    # security
    _conflict(("password", "permission", "authentication", "secret_key"), "security")

    if "security" not in raw:
        security = {}
        if "password" in raw:
            security["password"] = raw.pop("password")
        if "permission" in raw:
            security["permission"] = raw.pop("permission")
        if "authentication" in raw:
            old_auth = raw.pop("authentication")
            auth = {}
            if "client" in old_auth:
                auth["client"] = old_auth.pop("client")
            if "site" in old_auth:
                auth["site"] = old_auth.pop("site")
            if auth:
                security["authentication"] = auth

        if security:
            raw["security"] = security

    # third party
    _conflict(("oauth", "tcadp-config"), "third_party")

    if "third_party" not in raw:
        raw["third_party"] = {}

        if "oauth" in raw:
            raw["third_party"]["oauth"] = raw.pop("oauth")

        if "tcadp-config" in raw:
            raw["third_party"]["tcadp"] = raw.pop("tcadp-config")

    return raw