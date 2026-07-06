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
# distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

"""
Provider manager for sandbox providers.

Since sandbox configuration is global (system-level), we only use one
active provider at a time. This manager is a thin wrapper that holds a reference
to the currently active provider.
"""

from typing import Optional
from .base import SandboxProvider


class ProviderManager:
    """
    Manages the currently active sandbox provider.

    With global configuration, there's only one active provider at a time.
    This manager simply holds a reference to that provider.
    """

    def __init__(self):
        """Initialize an empty provider manager."""
        self.current_provider: Optional[SandboxProvider] = None
        self.current_provider_name: Optional[str] = None

    def set_provider(self, name: str, provider: SandboxProvider):
        """
        Set the active provider.

        Args:
            name: Provider identifier (e.g., "self_managed", "e2b")
            provider: Provider instance
        """
        self.current_provider = provider
        self.current_provider_name = name

    def get_provider(self) -> Optional[SandboxProvider]:
        """
        Get the active provider.

        Returns:
            Currently active SandboxProvider instance, or None if not set
        """
        return self.current_provider

    def get_provider_name(self) -> Optional[str]:
        """
        Get the active provider name.

        Returns:
            Provider name (e.g., "self_managed"), or None if not set
        """
        return self.current_provider_name

    def is_configured(self) -> bool:
        """
        Check if a provider is configured.

        Returns:
            True if a provider is set, False otherwise
        """
        return self.current_provider is not None
