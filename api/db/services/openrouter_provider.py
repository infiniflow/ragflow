#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import httpx
import logging
import hashlib
import json
from typing import List, Dict, Optional, Tuple
from rag.utils.redis_conn import REDIS_CONN
from api.db.services.dynamic_model_provider import (
    DynamicModelProvider,
    DynamicModelCapability
)


class OpenRouterProvider(DynamicModelProvider):
    """OpenRouter-specific implementation of dynamic model discovery"""

    OPENROUTER_MODELS_URL = "https://openrouter.ai/api/v1/models"
    CACHE_KEY = "openrouter:models:v1"
    CACHE_TTL = 3600  # 1 hour

    def __init__(self, redis=None):
        self.redis = redis or REDIS_CONN

    def _build_cache_key(self, base_url: Optional[str] = None, api_key: Optional[str] = None) -> str:
        """Build a cache key scoped to base_url and api_key without storing raw secrets"""
        url_part = base_url or self.OPENROUTER_MODELS_URL
        
        # Hash the API key if present, use sentinel if None
        if api_key:
            key_hash = hashlib.sha256(api_key.encode()).hexdigest()[:16]  # First 16 chars of hash
        else:
            key_hash = "public"
        
        # Create cache key: base_key:url_hash:key_hash
        url_hash = hashlib.sha256(url_part.encode()).hexdigest()[:8]
        return f"{self.CACHE_KEY}:{url_hash}:{key_hash}"

    async def fetch_available_models(
        self,
        api_key: Optional[str] = None,
        base_url: Optional[str] = None
    ) -> Tuple[List[Dict], bool]:
        """
        Fetch models from OpenRouter API with caching.
        API key is optional - OpenRouter's /models endpoint is public.
        
        Returns:
            tuple[List[Dict], bool]: Models and cache hit boolean
        """
        # Build dynamic cache key based on base_url and api_key
        cache_key = self._build_cache_key(base_url, api_key)
        
        # Check cache first
        cached = self._get_cached_models(cache_key)
        if cached:
            logging.info("Returning cached OpenRouter models")
            return cached, True

        # Fetch from OpenRouter API
        try:
            logging.info(f"Fetching models from OpenRouter API: {base_url or self.OPENROUTER_MODELS_URL}")
            async with httpx.AsyncClient(timeout=10.0) as client:
                headers = {}
                if api_key:
                    headers["Authorization"] = f"Bearer {api_key}"

                response = await client.get(
                    base_url or self.OPENROUTER_MODELS_URL,
                    headers=headers
                )
                response.raise_for_status()
                
                try:
                    data = response.json()
                except json.JSONDecodeError as e:
                    logging.error(f"Failed to parse JSON from OpenRouter response: {type(e).__name__}: {e}")
                    return self._get_fallback_models(), False

            # Transform to RAGFlow format
            models = self._transform_models(data.get("data", []))
            logging.info(f"Fetched {len(models)} models from OpenRouter")

            # Cache the results
            self._cache_models(models, cache_key)

            return models, False

        except (httpx.HTTPStatusError, httpx.RequestError) as e:
            logging.error(f"Failed to fetch models from OpenRouter: {type(e).__name__}: {e}")
            # Fallback to hardcoded popular models if API fails
            return self._get_fallback_models(), False

    def _transform_models(self, openrouter_models: List[Dict]) -> List[Dict]:
        """Transform OpenRouter model format to RAGFlow format"""
        transformed = []

        def safe_float(value, default=0.0):
            """Safely convert value to float, returning default on error"""
            try:
                return float(value)
            except (ValueError, TypeError):
                return default

        for model in openrouter_models:
            # Safely get model ID with fallback
            safe_id = model.get("id") or model.get("name")
            if not safe_id:
                # Skip models without any identifiable id or name
                continue

            # Determine model type from architecture
            model_type = self._infer_model_type(model, safe_id)

            # Skip if we can't support this model type
            if not model_type:
                continue

            # Extract provider for frontend filtering
            provider = self._extract_provider(safe_id)

            # Extract pricing safely, handling None values
            pricing = model.get("pricing") or {}
            
            transformed.append({
                "id": safe_id,
                "llm_name": safe_id,
                "name": model.get("name", safe_id),
                "model_type": model_type,
                "provider": provider,  # NEW - enables provider filtering
                "max_tokens": model.get("context_length", 8192),
                "is_tools": self._supports_tools(safe_id),
                "pricing": {
                    "prompt": safe_float(pricing.get("prompt", 0)),
                    "completion": safe_float(pricing.get("completion", 0))
                },
                "tags": self._generate_tags(model, model_type),
                "architecture": model.get("architecture", {}),
                # Auto-detected capabilities
                "supports_vision": "image" in model.get("architecture", {}).get("modality", ""),
            })

        return transformed

    def _infer_model_type(self, model: Dict, model_id: str) -> Optional[str]:
        """
        Infer RAGFlow model type from OpenRouter model metadata.

        Strategy:
        1. Check model name for explicit patterns (embed, rerank, whisper, tts)
        2. Check architecture modality
        3. Default to chat for ambiguous text models

        OpenRouter models have:
        - architecture.modality: "text", "text+image", "text+audio"
        - id patterns: "openai/gpt-4-vision" contains hints
        """
        modality = model.get("architecture", {}).get("modality", "text")
        model_id_lower = model_id.lower()
        model_name_lower = model.get("name", "").lower()

        # Embedding models (explicit pattern - high priority)
        if "embed" in model_id_lower or "embedding" in model_id_lower:
            return "embedding"

        # Rerank models (rare but exist on OpenRouter)
        if "rerank" in model_id_lower or ("cohere" in model_id_lower and "rerank" in model_name_lower):
            return "rerank"

        # Audio/Speech models (whisper, etc.)
        if "whisper" in model_id_lower or "audio" in modality or "speech" in model_id_lower:
            return "speech2text"

        # TTS models (text-to-speech)
        if "tts" in model_id_lower or "-tts" in model_id_lower or "text-to-speech" in model_name_lower:
            return "tts"

        # Vision models → chat (multimodal)
        if "vision" in model_id_lower or "image" in modality or modality == "text+image":
            return "chat"  # Vision-capable chat model

        # Default to chat for text-based models
        if "text" in modality or modality == "text->text":
            return "chat"

        return None

    def _extract_provider(self, model_id: str) -> str:
        """
        Extract provider from model ID.

        Examples:
            anthropic/claude-3.5-sonnet → anthropic
            openai/gpt-4 → openai
            meta-llama/llama-3-70b → meta-llama
            standalone-model → unknown
        """
        if "/" in model_id:
            return model_id.split("/")[0]
        return "unknown"

    def _supports_tools(self, model_id: str) -> bool:
        """
        Check if model supports function calling (tool use).
        
        IMPORTANT: This is a heuristic-based check since OpenRouter API doesn't
        explicitly mark tool/function calling support in model metadata.
        
        TODO: Make this configurable (env var, config file, or injectable list)
              and update patterns when new tool-capable models are released.
              Consider checking OpenRouter docs periodically:
              https://openrouter.ai/docs/models
        
        Note: Claude 2 has limited/unreliable tool support compared to Claude 3+,
              so it's excluded from the tool_capable list.
        """
        model_id_lower = model_id.lower()
        
        # Patterns for known tool-capable models
        # Excluded: "claude-2" (limited/unreliable tool support)
        tool_capable = [
            "gpt-4", "gpt-3.5-turbo", "claude-3",
            "gemini", "command", "mistral-large", "llama-3"
        ]
        return any(pattern in model_id_lower for pattern in tool_capable)

    def _generate_tags(self, model: Dict, model_type: str) -> str:
        """Generate RAGFlow-style tags"""
        tags = []

        # Add "LLM" tag only for chat models (generative language models)
        if model_type == "chat":
            tags.append("LLM")
            tags.append("CHAT")
        elif model_type == "embedding":
            tags.append("TEXT EMBEDDING")
        elif model_type == "speech2text":
            tags.append("SPEECH2TEXT")
        elif model_type == "rerank":
            tags.append("RERANK")
        elif model_type == "tts":
            tags.append("TTS")

        # Add context length tag
        ctx_length = model.get("context_length", 0)
        if ctx_length >= 1_000_000:
            tags.append(f"{ctx_length//1_000_000}M")
        elif ctx_length >= 1000:
            tags.append(f"{ctx_length//1000}K")

        # Add modality tags
        modality = model.get("architecture", {}).get("modality", "")
        if "image" in modality:
            tags.append("IMAGE2TEXT")

        return ",".join(tags)

    def _get_cached_models(self, cache_key: str) -> Optional[List[Dict]]:
        """Retrieve models from Redis cache"""
        try:
            cached = self.redis.get(cache_key)
            if cached:
                # Decode bytes to string if necessary
                if isinstance(cached, bytes):
                    cached = cached.decode('utf-8')
                elif not isinstance(cached, str):
                    # Unexpected type - log and return None instead of forcing conversion
                    logging.warning(f"Cached value has unexpected type {type(cached).__name__}, returning None")
                    return None
                return json.loads(cached)
            return None
        except Exception as e:
            logging.warning(f"Failed to get cached models: {e}")
            return None

    def _cache_models(self, models: List[Dict], cache_key: str):
        """Store models in Redis cache"""
        try:
            self.redis.set(cache_key, json.dumps(models), self.CACHE_TTL)
            logging.info(f"Cached {len(models)} models with TTL {self.CACHE_TTL}s")
        except Exception as e:
            # Log but don't fail if caching fails
            logging.warning(f"Failed to cache models: {e}")

    def _get_fallback_models(self) -> List[Dict]:
        """
        Return hardcoded popular models as fallback when API is unavailable.
        
        Includes models for multiple types: chat (LLM), embedding, and speech2text.
        
        IMPORTANT: These values can become stale and should be verified periodically.
        
        Last verified: January 16, 2026
        Source: https://openrouter.ai/docs/models
        Pricing reference: https://openrouter.ai/docs/pricing
        
        TODO: Refresh pricing, max_tokens, and model availability quarterly or
              migrate to a more resilient fallback mechanism (e.g., bundled JSON,
              secondary API endpoint, or graceful degradation without models).
        """
        logging.warning("Using fallback models (includes chat, embedding, and speech2text defaults)")
        return [
            {
                "id": "anthropic/claude-3.5-sonnet",
                "llm_name": "anthropic/claude-3.5-sonnet",
                "name": "Claude 3.5 Sonnet",
                "model_type": "chat",
                "provider": "anthropic",
                "max_tokens": 200000,
                "is_tools": True,
                "pricing": {"prompt": 0.003, "completion": 0.015},
                "tags": "LLM,CHAT,200K",
                "architecture": {},
                "supports_vision": True
            },
            {
                "id": "openai/gpt-4-turbo",
                "llm_name": "openai/gpt-4-turbo",
                "name": "GPT-4 Turbo",
                "model_type": "chat",
                "provider": "openai",
                "max_tokens": 128000,
                "is_tools": True,
                "pricing": {"prompt": 0.01, "completion": 0.03},
                "tags": "LLM,CHAT,128K",
                "architecture": {},
                "supports_vision": True
            },
            {
                "id": "google/gemini-pro",
                "llm_name": "google/gemini-pro",
                "name": "Gemini Pro",
                "model_type": "chat",
                "provider": "google",
                "max_tokens": 32000,
                "is_tools": True,
                "pricing": {"prompt": 0.000125, "completion": 0.000375},
                "tags": "LLM,CHAT,32K",
                "architecture": {},
                "supports_vision": False
            },
            {
                "id": "meta-llama/llama-3-70b-instruct",
                "llm_name": "meta-llama/llama-3-70b-instruct",
                "name": "Llama 3 70B Instruct",
                "model_type": "chat",
                "provider": "meta-llama",
                "max_tokens": 8192,
                "is_tools": True,
                "pricing": {"prompt": 0.00059, "completion": 0.00079},
                "tags": "LLM,CHAT,8K",
                "architecture": {},
                "supports_vision": False
            },
            {
                "id": "mistralai/mistral-large",
                "llm_name": "mistralai/mistral-large",
                "name": "Mistral Large",
                "model_type": "chat",
                "provider": "mistralai",
                "max_tokens": 32000,
                "is_tools": True,
                "pricing": {"prompt": 0.004, "completion": 0.012},
                "tags": "LLM,CHAT,32K",
                "architecture": {},
                "supports_vision": False
            },
            # Embedding models
            {
                "id": "openai/text-embedding-3-small",
                "llm_name": "openai/text-embedding-3-small",
                "name": "Text Embedding 3 Small",
                "model_type": "embedding",
                "provider": "openai",
                "max_tokens": 8191,
                "is_tools": False,
                "pricing": {"prompt": 0.00002, "completion": 0.0},
                "tags": "TEXT EMBEDDING,8K",
                "architecture": {},
                "supports_vision": False
            },
            {
                "id": "openai/text-embedding-ada-002",
                "llm_name": "openai/text-embedding-ada-002",
                "name": "Text Embedding Ada 002",
                "model_type": "embedding",
                "provider": "openai",
                "max_tokens": 8191,
                "is_tools": False,
                "pricing": {"prompt": 0.0001, "completion": 0.0},
                "tags": "TEXT EMBEDDING,8K",
                "architecture": {},
                "supports_vision": False
            },
            # Speech2Text models
            {
                "id": "openai/whisper-large-v3",
                "llm_name": "openai/whisper-large-v3",
                "name": "Whisper Large V3",
                "model_type": "speech2text",
                "provider": "openai",
                "max_tokens": 0,
                "is_tools": False,
                "pricing": {"prompt": 0.006, "completion": 0.0},
                "tags": "SPEECH2TEXT",
                "architecture": {},
                "supports_vision": False
            }
        ]

    def get_cache_key(self, base_url: Optional[str] = None, api_key: Optional[str] = None) -> str:
        """Get cache key for this provider, optionally scoped to base_url and api_key.
        
        If base_url or api_key are provided, returns a dynamic cache key specific to those parameters.
        Otherwise returns the base cache key prefix.
        """
        return self._build_cache_key(base_url, api_key)

    def get_cache_ttl(self) -> int:
        return self.CACHE_TTL

    def supports_capability(self, capability: DynamicModelCapability) -> bool:
        return capability in [
            DynamicModelCapability.MODEL_DISCOVERY,
            DynamicModelCapability.COST_ESTIMATION
        ]

    def get_supported_categories(self) -> set[str]:
        """OpenRouter can support these RAGFlow model categories"""
        return {"chat", "embedding", "rerank", "speech2text", "tts"}

    def get_default_base_url(self) -> Optional[str]:
        """Default OpenRouter API endpoint"""
        return "https://openrouter.ai/api/v1"


# Register OpenRouter provider
from api.db.services.dynamic_model_provider import register_provider
register_provider("OpenRouter", OpenRouterProvider)
