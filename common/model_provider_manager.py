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
import json
import logging
import os
from typing import Optional


_model_provider_manager: Optional["ModelProviderManager"] = None


def get_model_provider_manager() -> "ModelProviderManager":
    global _model_provider_manager
    if _model_provider_manager is None:
        _model_provider_manager = ModelProviderManager("conf/models")
    return _model_provider_manager


class ModelProviderManager:
    """Python equivalent of Go's entity.ProviderManager.

    Reads provider configuration from JSON files in conf/models/ directory.
    """

    def __init__(self, config_dir: str):
        self.providers: list[dict] = []
        self._provider_map: dict[str, dict] = {}
        self._load_providers(config_dir)

    def _load_providers(self, config_dir: str):
        if not os.path.isdir(config_dir):
            logging.warning("Model provider config directory not found: %s", config_dir)
            return

        for filename in sorted(os.listdir(config_dir)):
            if not filename.endswith(".json"):
                continue
            filepath = os.path.join(config_dir, filename)
            try:
                with open(filepath, "r") as f:
                    provider = json.load(f)
                name = provider.get("name", "")
                if name:
                    self.providers.append(provider)
                    self._provider_map[name.lower()] = provider
            except Exception:
                logging.exception("Failed to load provider config: %s", filepath)

    def list_providers(self) -> list[dict]:
        """List all providers with aggregated model types.

        Equivalent to Go's ProviderManager.ListProviders().
        """
        result = []
        for provider in self.providers:
            model_type_set: set[str] = set()
            for model in provider.get("models", []):
                for mt in model.get("model_types", []):
                    model_type_set.add(mt)

            provider_data = {
                "name": provider["name"],
                "url": provider.get("url", {}),
                "model_types": sorted(model_type_set),
                "url_suffix": provider.get("url_suffix", {}),
            }
            result.append(provider_data)
        return result

    def get_provider_by_name(self, provider_name: str) -> Optional[dict]:
        """Get provider info by name.

        Equivalent to Go's ProviderManager.GetProviderByName().
        Returns {"name", "base_url", "total_models"} or None.
        """
        provider = self.find_provider(provider_name)
        if provider is None:
            return None
        return {
            "name": provider["name"],
            "base_url": provider.get("url", {}),
            "total_models": len(provider.get("models", [])),
        }

    def list_models(self, provider_name: str) -> Optional[list[dict]]:
        """List all models for a provider.

        Equivalent to Go's ProviderManager.ListModels().
        """
        provider = self.find_provider(provider_name)
        if provider is None:
            return None

        models = []
        for model in provider.get("models", []):
            model_data = {
                "name": model["name"],
                "max_tokens": model.get("max_tokens", 0),
                "model_types": model.get("model_types", []),
                "features": model.get("features", {}),
            }
            models.append(model_data)
        return models

    def get_model_by_name(self, provider_name: str, model_name: str) -> Optional[dict]:
        """Get a specific model by name.

        Equivalent to Go's ProviderManager.GetModelByName().
        Returns the model dict or None.
        """
        provider = self.find_provider(provider_name)
        if provider is None:
            return None

        for model in provider.get("models", []):
            if model.get("name") == model_name:
                return model
        return None

    def find_provider(self, name: str) -> Optional[dict]:
        """Find a provider by name (case-insensitive).

        Equivalent to Go's ProviderManager.FindProvider().
        """
        return self._provider_map.get(name.lower())
