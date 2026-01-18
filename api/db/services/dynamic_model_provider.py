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
from abc import ABC, abstractmethod
from typing import List, Dict, Optional, Type, Tuple, Set
from enum import Enum
import logging
import threading
import importlib.util

logger = logging.getLogger(__name__)


class DynamicModelCapability(Enum):
    """Providers can declare which capabilities they support"""

    MODEL_DISCOVERY = "model_discovery"
    MODEL_VALIDATION = "model_validation"
    COST_ESTIMATION = "cost_estimation"


class DynamicModelProvider(ABC):
    """
    Abstract interface for LLM providers that support dynamic model discovery.
    Providers like OpenRouter, Ollama, LocalAI can implement this interface.
    """

    @abstractmethod
    async def fetch_available_models(self, api_key: Optional[str] = None, base_url: Optional[str] = None) -> Tuple[List[Dict], bool]:
        """
        Fetch available models from the provider's API.

        Returns:
            Tuple[List[Dict], bool]: A tuple containing:
                - List of models in standardized format (Python dict example):
                    {
                        "id": "anthropic/claude-3.5-sonnet",
                        "name": "Claude 3.5 Sonnet",
                        "model_type": "chat",
                        "max_tokens": 200000,
                        "is_tools": True,
                        "pricing": {"prompt": 0.003, "completion": 0.015},
                        "tags": "LLM,CHAT,200K,IMAGE2TEXT"
                    }
                - Boolean indicating if results came from cache (True) or API (False)
        """
        pass

    @abstractmethod
    def get_cache_key(self) -> str:
        """Return Redis cache key for this provider's models"""
        pass

    @abstractmethod
    def get_cache_ttl(self) -> int:
        """Return cache TTL in seconds (recommended: 3600)"""
        pass

    @abstractmethod
    def supports_capability(self, capability: DynamicModelCapability) -> bool:
        """Check if provider supports a specific capability"""
        pass

    @abstractmethod
    def get_supported_categories(self) -> Set[str]:
        """
        Return which RAGFlow model_types this provider can support.

        Examples:
            OpenRouter: {"chat", "embedding", "speech2text", "tts", "image2text"}
            Ollama: {"chat", "embedding"}
            LocalAI: {"chat", "embedding", "tts"}

        Returns:
            Set of supported model_type values from RAGFlow's LLMType enum
        """
        pass

    @abstractmethod
    def get_default_base_url(self) -> Optional[str]:
        """
        Return default base_url for this provider.

        Examples:
            OpenRouter: "https://openrouter.ai/api/v1"
            Ollama: "http://localhost:11434"
            LocalAI: None (user must provide)

        Returns:
            Default base URL string, or None if provider has no default
        """
        pass

    # Default mapping from provider-specific model types to RAGFlow's LLMType
    DEFAULT_TYPE_MAPPING = {
        "chat": "chat",
        "completion": "chat",
        "embedding": "embedding",
        "rerank": "rerank",
        "image": "image2text",
        "vision": "image2text",
        "audio": "speech2text",
        "tts": "tts",
    }

    def get_type_mapping(self) -> Dict[str, str]:
        """
        Get the model type mapping for this provider.

        Subclasses can override this to extend or modify the default mapping.
        Returns a dict mapping provider-specific types to RAGFlow LLMType values.

        Example override in subclass:
            def get_type_mapping(self):
                mapping = super().get_type_mapping().copy()
                mapping.update({"custom_type": "chat"})
                return mapping
        """
        return self.DEFAULT_TYPE_MAPPING.copy()

    def map_model_type(self, provider_type: str) -> str:
        """
        Map provider-specific model type to RAGFlow's LLMType.
        Override get_type_mapping() if provider uses different taxonomy.
        """
        if not provider_type:
            return "chat"
        return self.get_type_mapping().get(str(provider_type).lower(), "chat")


# Provider registry for dynamic model providers
DYNAMIC_PROVIDERS = {}
# Provider instance cache to avoid repeated instantiation
PROVIDER_INSTANCES = {}
# Thread lock for safe concurrent access to registries
_registry_lock = threading.RLock()


def register_provider(factory_name: str, provider_class: Type[DynamicModelProvider]):
    """Register a dynamic model provider

    Args:
        factory_name: Name of the factory/provider
        provider_class: Provider class (must be subclass of DynamicModelProvider)

    Raises:
        TypeError: If provider_class is not a subclass of DynamicModelProvider
    """
    if not isinstance(provider_class, type) or not issubclass(provider_class, DynamicModelProvider):
        raise TypeError(f"provider_class must be a subclass of DynamicModelProvider, got {type(provider_class).__name__}")
    with _registry_lock:
        DYNAMIC_PROVIDERS[factory_name] = provider_class


def get_provider(factory_name: str) -> Optional[DynamicModelProvider]:
    """Get provider instance by factory name (cached, thread-safe)

    Returns cached instance if available (may be None for failed instantiation),
    otherwise creates, caches, and returns new instance.
    Uses double-checked locking to avoid race conditions during lazy instantiation.
    """
    # First check without lock (fast path for already-cached instances or failures)
    if factory_name in PROVIDER_INSTANCES:
        return PROVIDER_INSTANCES[factory_name]

    # Acquire lock for cache miss
    with _registry_lock:
        # Double-check: another thread may have created the instance or cached a failure
        if factory_name in PROVIDER_INSTANCES:
            return PROVIDER_INSTANCES[factory_name]

        # Create new instance if provider is registered
        provider_class = DYNAMIC_PROVIDERS.get(factory_name)
        if provider_class:
            try:
                instance = provider_class()
                PROVIDER_INSTANCES[factory_name] = instance
                return instance
            except Exception as e:
                # Log instantiation failure and cache None to avoid repeated attempts
                logger.error(f"Failed to instantiate provider '{factory_name}': {type(e).__name__}: {e}", exc_info=True)
                # Cache None as failure marker to prevent repeated instantiation attempts
                PROVIDER_INSTANCES[factory_name] = None
                return None
        return None


def is_dynamic_provider(factory_name: str) -> bool:
    """Check if factory supports dynamic model discovery (thread-safe)"""
    with _registry_lock:
        return factory_name in DYNAMIC_PROVIDERS


# Import and register providers
def _register_providers():
    """Import and register all dynamic model providers

    Iterates over a list of provider modules and imports them.
    Each provider module should register itself upon import.
    """
    # List of provider modules to register
    provider_modules = [
        "api.db.services.openrouter_provider",
        # Add more provider modules here as they are implemented
        # "api.db.services.ollama_provider",
        # "api.db.services.localai_provider",
    ]

    for module_name in provider_modules:
        try:
            # Import provider module (triggers its registration)
            __import__(module_name)
        except Exception as e:
            # Catch all exceptions to prevent one bad provider from crashing the system
            # Use find_spec to distinguish between missing modules and import errors
            spec = importlib.util.find_spec(module_name)
            if spec is None:
                # Module not found - expected for optional providers
                logger.debug(f"Optional provider module {module_name} not present (not installed)")
            else:
                # Module exists but failed to import - this is a real error
                logger.warning(f"Failed to import provider module {module_name} (module exists but import failed): {type(e).__name__}: {e}", exc_info=True)
            # All exceptions are caught and logged; continue with other providers


# Register providers on module import
_register_providers()
