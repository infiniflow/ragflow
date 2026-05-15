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

"""
Sandbox client for agent components.

This module provides a unified interface for agent components to interact
with the configured sandbox provider.
"""

import json
import logging
import os
from typing import Dict, Any, Optional

from api.db.services.system_settings_service import SystemSettingsService
from agent.sandbox.providers import ProviderManager
from agent.sandbox.providers.base import ExecutionResult, SandboxProviderConfigError

logger = logging.getLogger(__name__)


# Global provider manager instance
_provider_manager: Optional[ProviderManager] = None


def get_provider_manager() -> ProviderManager:
    """
    Get the global provider manager instance.

    Returns:
        ProviderManager instance with active provider loaded
    """
    global _provider_manager

    if _provider_manager is not None:
        return _provider_manager

    # Initialize provider manager with system settings
    _provider_manager = ProviderManager()
    _load_provider_from_settings()

    return _provider_manager


def _load_provider_from_settings() -> None:
    """
    Load sandbox provider from system settings and configure the provider manager.

    This function resolves the active provider type, then loads configuration
    from system settings with environment overrides for that provider.
    """
    global _provider_manager

    if _provider_manager is None:
        return

    try:
        provider_type, provider_type_from_env = _resolve_provider_type()
        config = _load_provider_config(provider_type)

        # Import and instantiate the provider
        from agent.sandbox.providers import (
            SelfManagedProvider,
            AliyunCodeInterpreterProvider,
            E2BProvider,
            LocalProvider,
        )

        provider_classes = {
            "self_managed": SelfManagedProvider,
            "aliyun_codeinterpreter": AliyunCodeInterpreterProvider,
            "e2b": E2BProvider,
            "local": LocalProvider,
        }

        if provider_type not in provider_classes:
            if provider_type_from_env:
                raise SandboxProviderConfigError(f"Unknown sandbox provider type: {provider_type}")
            logger.error(f"Unknown provider type: {provider_type}")
            return

        provider_class = provider_classes[provider_type]
        provider = provider_class()

        # Initialize the provider
        if not provider.initialize(config):
            message = f"Failed to initialize sandbox provider: {provider_type}. Config keys: {list(config.keys())}"
            if provider_type == "local" or provider_type_from_env:
                raise SandboxProviderConfigError(message)
            logger.error(message)
            return

        # Set the active provider
        _provider_manager.set_provider(provider_type, provider)
        logger.info(f"Sandbox provider '{provider_type}' initialized successfully")

    except SandboxProviderConfigError:
        raise
    except Exception as e:
        logger.error(f"Failed to load sandbox provider from settings: {e}")
        import traceback
        traceback.print_exc()


def _load_provider_config_from_settings(provider_type: str) -> Dict[str, Any]:
    provider_config_settings = SystemSettingsService.get_by_name(f"sandbox.{provider_type}")
    if not provider_config_settings:
        logger.warning(f"No configuration found for provider: {provider_type}")
        return {}

    try:
        return json.loads(provider_config_settings[0].value)
    except json.JSONDecodeError as e:
        logger.error(f"Failed to parse sandbox config for {provider_type}: {e}")
        return {}


def _resolve_provider_type() -> tuple[str, bool]:
    provider_type = os.environ.get("SANDBOX_PROVIDER_TYPE", "").strip()
    if provider_type:
        return provider_type, True

    provider_type_settings = SystemSettingsService.get_by_name("sandbox.provider_type")
    if not provider_type_settings:
        raise RuntimeError(
            "Sandbox provider type not configured. Please set 'sandbox.provider_type' in system settings."
        )
    return provider_type_settings[0].value, False


def _load_provider_config(provider_type: str) -> Dict[str, Any]:
    config = _load_provider_config_from_settings(provider_type)
    env_config = _load_provider_config_from_env(provider_type)
    if env_config:
        config.update(env_config)
    return config


def _load_provider_config_from_env(provider_type: str) -> Dict[str, Any]:
    if provider_type == "local":
        return _load_local_provider_config_from_env()
    if provider_type == "self_managed":
        return _load_self_managed_provider_config_from_env()
    return {}


def _load_local_provider_config_from_env() -> Dict[str, Any]:
    env_to_config = {
        "SANDBOX_LOCAL_PYTHON_BIN": "python_bin",
        "SANDBOX_LOCAL_NODE_BIN": "node_bin",
        "SANDBOX_LOCAL_WORK_DIR": "work_dir",
        "SANDBOX_LOCAL_TIMEOUT": "timeout",
        "SANDBOX_LOCAL_MAX_MEMORY_MB": "max_memory_mb",
        "SANDBOX_LOCAL_MAX_OUTPUT_BYTES": "max_output_bytes",
        "SANDBOX_LOCAL_MAX_ARTIFACTS": "max_artifacts",
        "SANDBOX_LOCAL_MAX_ARTIFACT_BYTES": "max_artifact_bytes",
    }
    config = {}
    for env_name, config_name in env_to_config.items():
        if env_name in os.environ:
            config[config_name] = os.environ[env_name]
    return config


def _load_self_managed_provider_config_from_env() -> Dict[str, Any]:
    host = os.environ.get("SANDBOX_HOST", "").strip()
    port = os.environ.get("SANDBOX_EXECUTOR_MANAGER_PORT", "").strip()
    pool_size = os.environ.get("SANDBOX_EXECUTOR_MANAGER_POOL_SIZE", "").strip()

    config = {}
    if host:
        config["endpoint"] = f"http://{host}:{port or '9385'}"
    if pool_size:
        config["pool_size"] = pool_size
    return config


def reload_provider() -> None:
    """
    Reload the sandbox provider from system settings.

    Use this function when sandbox settings have been updated.
    """
    global _provider_manager
    _provider_manager = None
    _load_provider_from_settings()


def execute_code(
    code: str,
    language: str = "python",
    timeout: int = 30,
    arguments: Optional[Dict[str, Any]] = None
) -> ExecutionResult:
    """
    Execute code in the configured sandbox.

    This is the main entry point for agent components to execute code.

    Args:
        code: Source code to execute
        language: Programming language (python, nodejs, javascript)
        timeout: Maximum execution time in seconds
        arguments: Optional arguments dict to pass to main() function

    Returns:
        ExecutionResult containing stdout, stderr, exit_code, and metadata

    Raises:
        RuntimeError: If no provider is configured or execution fails
    """
    provider_manager = get_provider_manager()

    if not provider_manager.is_configured():
        raise RuntimeError(
            "No sandbox provider configured. Please configure sandbox settings in the admin panel."
        )

    provider = provider_manager.get_provider()

    # Create a sandbox instance
    instance = provider.create_instance(template=language)

    try:
        # Execute the code
        result = provider.execute_code(
            instance_id=instance.instance_id,
            code=code,
            language=language,
            timeout=timeout,
            arguments=arguments
        )

        return result

    finally:
        # Clean up the instance
        try:
            provider.destroy_instance(instance.instance_id)
        except Exception as e:
            logger.warning(f"Failed to destroy sandbox instance {instance.instance_id}: {e}")


def health_check() -> bool:
    """
    Check if the sandbox provider is healthy.

    Returns:
        True if provider is configured and healthy, False otherwise
    """
    try:
        provider_manager = get_provider_manager()

        if not provider_manager.is_configured():
            return False

        provider = provider_manager.get_provider()
        return provider.health_check()

    except Exception as e:
        logger.error(f"Sandbox health check failed: {e}")
        return False


def get_provider_info() -> Dict[str, Any]:
    """
    Get information about the current sandbox provider.

    Returns:
        Dictionary with provider information:
        - provider_type: Type of the active provider
        - configured: Whether provider is configured
        - healthy: Whether provider is healthy
    """
    try:
        provider_manager = get_provider_manager()

        return {
            "provider_type": provider_manager.get_provider_name(),
            "configured": provider_manager.is_configured(),
            "healthy": health_check(),
        }

    except Exception as e:
        logger.error(f"Failed to get provider info: {e}")
        return {
            "provider_type": None,
            "configured": False,
            "healthy": False,
        }
