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

import logging
from collections.abc import Mapping
from typing import TypedDict

from botocore import UNSIGNED
from botocore.awsrequest import AWSRequest
from botocore.client import BaseClient
from botocore.config import Config
from botocore.exceptions import BotoCoreError, ClientError

from common.constants import LLMType
from rag.utils.bedrock_endpoint import (
    mantle_model_catalog_url,
    normalize_bedrock_endpoint,
    resolve_bedrock_endpoint,
    validate_bedrock_api_key,
    validate_bedrock_endpoint_target,
    validate_bedrock_region,
)


DEFAULT_BEDROCK_MAX_TOKENS = 8192
BEDROCK_DISCOVERY_TIMEOUT_SECONDS = 10


class BedrockModelDiscoveryError(RuntimeError):
    pass


class DiscoveredBedrockModel(TypedDict):
    name: str
    model_types: list[str]
    max_tokens: int
    features: list[str]


def create_bedrock_bearer_client(
    service_name: str,
    api_key: str,
    region_name: str,
    endpoint_url: str = "",
    *,
    timeout_seconds: int = BEDROCK_DISCOVERY_TIMEOUT_SECONDS,
    max_attempts: int = 0,
) -> BaseClient:
    import boto3

    api_key = validate_bedrock_api_key(api_key)

    client_args: dict[str, object] = {
        "service_name": service_name,
        "region_name": region_name,
        "config": Config(
            signature_version=UNSIGNED,
            connect_timeout=timeout_seconds,
            read_timeout=timeout_seconds,
            retries={"mode": "standard", "max_attempts": max_attempts},
        ),
    }
    if endpoint_url:
        client_args["endpoint_url"] = endpoint_url.rstrip("/")
    client = boto3.client(**client_args)

    def add_bearer_token(request: AWSRequest, **_kwargs: object) -> None:
        request.headers["Authorization"] = f"Bearer {api_key}"

    client.meta.events.register(f"before-sign.{service_name}.*", add_bearer_token)
    return client


def _discover_runtime_models(api_key: str, region_name: str, catalog_endpoint_url: str = "") -> list[DiscoveredBedrockModel]:
    try:
        response = create_bedrock_bearer_client("bedrock", api_key, region_name, catalog_endpoint_url).list_foundation_models(byInferenceType="ON_DEMAND")
    except (BotoCoreError, ClientError) as error:
        raise BedrockModelDiscoveryError("Failed to list models from Amazon Bedrock") from error
    models: list[DiscoveredBedrockModel] = []
    for summary in response.get("modelSummaries", []):
        model_id = summary.get("modelId")
        input_modalities = summary.get("inputModalities", [])
        output_modalities = summary.get("outputModalities", [])
        inference_types = summary.get("inferenceTypesSupported", [])
        lifecycle_status = (summary.get("modelLifecycle") or {}).get("status")
        if not model_id or "TEXT" not in input_modalities or "rerank" in model_id.lower():
            continue
        if inference_types and "ON_DEMAND" not in inference_types:
            continue
        if lifecycle_status and lifecycle_status != "ACTIVE":
            continue
        if "EMBEDDING" in output_modalities and (model_id.startswith("amazon.titan-embed-text") or model_id.startswith("cohere.embed-")):
            model_types = [LLMType.EMBEDDING.value]
        elif "TEXT" in output_modalities:
            model_types = [LLMType.CHAT.value]
            if "IMAGE" in input_modalities:
                model_types.append(LLMType.VISION.value)
        else:
            continue
        models.append({"name": model_id, "model_types": model_types, "max_tokens": DEFAULT_BEDROCK_MAX_TOKENS, "features": []})
    return models


def _discover_mantle_models(api_key: str, endpoint_url: str) -> list[DiscoveredBedrockModel]:
    from openai import OpenAI, OpenAIError

    try:
        response = OpenAI(
            api_key=api_key,
            base_url=mantle_model_catalog_url(endpoint_url),
            timeout=BEDROCK_DISCOVERY_TIMEOUT_SECONDS,
            max_retries=0,
        ).models.list()
    except OpenAIError as error:
        raise BedrockModelDiscoveryError("Failed to list models from Bedrock Mantle") from error
    models: list[DiscoveredBedrockModel] = []
    for model in response.data:
        if not isinstance(model.id, str):
            continue
        model_id = model.id.strip()
        if not model_id:
            continue
        models.append({"name": model_id, "model_types": [], "max_tokens": DEFAULT_BEDROCK_MAX_TOKENS, "features": []})
    return models


def discover_bedrock_models(config: Mapping[str, object]) -> list[DiscoveredBedrockModel]:
    api_key = config.get("bedrock_api_key")
    if not api_key:
        raise ValueError("Bedrock API key must be provided")
    api_key = validate_bedrock_api_key(api_key)
    region_name = config.get("bedrock_region")
    if not region_name:
        raise ValueError("Bedrock region must be provided in the key")
    if not isinstance(region_name, str):
        raise ValueError("Bedrock region must be a valid AWS region identifier")
    validate_bedrock_region(region_name)
    endpoint_type, endpoint_url = resolve_bedrock_endpoint("bedrock_api_key", config.get("bedrock_endpoint_type"), config.get("bedrock_endpoint_url"))
    logging.info("Discovering Bedrock models using endpoint type %s", endpoint_type)
    if endpoint_type == "runtime":
        raw_catalog_endpoint_url = config.get("bedrock_discovery_endpoint_url")
        if raw_catalog_endpoint_url is not None and not isinstance(raw_catalog_endpoint_url, str):
            raise ValueError("Bedrock discovery endpoint URL must be a string")
        catalog_endpoint_url = normalize_bedrock_endpoint("runtime", raw_catalog_endpoint_url or "")
        validate_bedrock_endpoint_target(catalog_endpoint_url)
        models = _discover_runtime_models(api_key, region_name, catalog_endpoint_url)
    else:
        models = _discover_mantle_models(api_key, endpoint_url)
    unique_models = {model["name"]: model for model in models}
    if not unique_models:
        raise ValueError("No Bedrock models were discovered")
    logging.info("Discovered %d Bedrock models", len(unique_models))
    return list(unique_models.values())
