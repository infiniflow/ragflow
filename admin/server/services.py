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

import json
import os
import logging
import re
from typing import Any

from werkzeug.security import check_password_hash
from common.constants import ActiveEnum
from api.db.services import UserService
from api.db.joint_services.user_account_service import create_new_user, delete_user_data
from api.db.services.canvas_service import UserCanvasService
from api.db.services.user_service import TenantService, UserTenantService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.system_settings_service import SystemSettingsService
from api.db.services.api_service import APITokenService
from api.db.db_models import APIToken
from api.utils.crypt import decrypt
from api.utils import health_utils

from api.common.exceptions import AdminException, UserAlreadyExistsError, UserNotFoundError
from config import SERVICE_CONFIGS


class UserMgr:
    @staticmethod
    def get_all_users():
        users = UserService.get_all_users()
        result = []
        for user in users:
            result.append(
                {
                    "email": user.email,
                    "nickname": user.nickname,
                    "create_date": user.create_date,
                    "is_active": user.is_active,
                    "is_superuser": user.is_superuser,
                }
            )
        return result

    @staticmethod
    def get_user_details(username):
        # use email to query
        users = UserService.query_user_by_email(username)
        result = []
        for user in users:
            result.append(
                {
                    "avatar": user.avatar,
                    "email": user.email,
                    "language": user.language,
                    "last_login_time": user.last_login_time,
                    "is_active": user.is_active,
                    "is_anonymous": user.is_anonymous,
                    "login_channel": user.login_channel,
                    "status": user.status,
                    "is_superuser": user.is_superuser,
                    "create_date": user.create_date,
                    "update_date": user.update_date,
                }
            )
        return result

    @staticmethod
    def create_user(username, password, role="user") -> dict:
        # Validate the email address
        if not re.match(r"^[\w\._-]+@([\w_-]+\.)+[\w-]{2,}$", username):
            raise AdminException(f"Invalid email address: {username}!")
        # Check if the email address is already used
        if UserService.query(email=username):
            raise UserAlreadyExistsError(username)
        # Construct user info data
        user_info_dict = {
            "email": username,
            "nickname": "",  # ask user to edit it manually in settings.
            "password": decrypt(password),
            "login_channel": "password",
            "is_superuser": role == "admin",
        }
        return create_new_user(user_info_dict)

    @staticmethod
    def delete_user(username):
        # use email to delete
        user_list = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        if len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")
        usr = user_list[0]
        return delete_user_data(usr.id)

    @staticmethod
    def update_user_password(username, new_password) -> str:
        # use email to find user. check exist and unique.
        user_list = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")
        # check new_password different from old.
        usr = user_list[0]
        psw = decrypt(new_password)
        if check_password_hash(usr.password, psw):
            return "Same password, no need to update!"
        # update password
        UserService.update_user_password(usr.id, psw)
        return "Password updated successfully!"

    @staticmethod
    def update_user_activate_status(username, activate_status: str):
        # use email to find user. check exist and unique.
        user_list = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")
        # check activate status different from new
        usr = user_list[0]
        # format activate_status before handle
        _activate_status = activate_status.lower()
        target_status = {
            "on": ActiveEnum.ACTIVE.value,
            "off": ActiveEnum.INACTIVE.value,
        }.get(_activate_status)
        if not target_status:
            raise AdminException(f"Invalid activate_status: {activate_status}")
        if target_status == usr.is_active:
            return f"User activate status is already {_activate_status}!"
        # update is_active
        UserService.update_user(usr.id, {"is_active": target_status})
        return f"Turn {_activate_status} user activate status successfully!"

    @staticmethod
    def get_user_api_key(username: str) -> list[dict[str, Any]]:
        # use email to find user. check exist and unique.
        user_list: list[Any] = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"More than one user with username '{username}' found!")

        usr: Any = user_list[0]
        # tenant_id is typically the same as user_id for the owner tenant
        tenant_id: str = usr.id

        # Query all API keys for this tenant
        api_keys: Any = APITokenService.query(tenant_id=tenant_id)

        result: list[dict[str, Any]] = []
        for key in api_keys:
            result.append(key.to_dict())

        return result

    @staticmethod
    def save_api_key(api_key: dict[str, Any]) -> bool:
        return APITokenService.save(**api_key)

    @staticmethod
    def delete_api_key(username: str, key: str) -> bool:
        # use email to find user. check exist and unique.
        user_list: list[Any] = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")

        usr: Any = user_list[0]
        # tenant_id is typically the same as user_id for the owner tenant
        tenant_id: str = usr.id

        # Delete the API key
        deleted_count: int = APITokenService.filter_delete([APIToken.tenant_id == tenant_id, APIToken.token == key])
        return deleted_count > 0

    @staticmethod
    def grant_admin(username: str):
        # use email to find user. check exist and unique.
        user_list = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")

        # check activate status different from new
        usr = user_list[0]
        if usr.is_superuser:
            return f"{usr} is already superuser!"
        # update is_active
        UserService.update_user(usr.id, {"is_superuser": True})
        return "Grant successfully!"

    @staticmethod
    def revoke_admin(username: str):
        # use email to find user. check exist and unique.
        user_list = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")
        # check activate status different from new
        usr = user_list[0]
        if not usr.is_superuser:
            return f"{usr} isn't superuser, yet!"
        # update is_active
        UserService.update_user(usr.id, {"is_superuser": False})
        return "Revoke successfully!"


class UserServiceMgr:
    @staticmethod
    def get_user_datasets(username):
        # use email to find user.
        user_list = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")
        # find tenants
        usr = user_list[0]
        tenants = TenantService.get_joined_tenants_by_user_id(usr.id)
        tenant_ids = [m["tenant_id"] for m in tenants]
        # filter permitted kb and owned kb
        return KnowledgebaseService.get_all_kb_by_tenant_ids(tenant_ids, usr.id)

    @staticmethod
    def get_user_agents(username):
        # use email to find user.
        user_list = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")
        # find tenants
        usr = user_list[0]
        tenants = TenantService.get_joined_tenants_by_user_id(usr.id)
        tenant_ids = [m["tenant_id"] for m in tenants]
        # filter permitted agents and owned agents
        res = UserCanvasService.get_all_agents_by_tenant_ids(tenant_ids, usr.id)
        return [{"title": r["title"], "permission": r["permission"], "canvas_category": r["canvas_category"].split("_")[0], "avatar": r["avatar"]} for r in res]

    @staticmethod
    def get_user_tenants(email: str) -> list[dict[str, Any]]:
        users: list[Any] = UserService.query_user_by_email(email)
        if not users:
            raise UserNotFoundError(email)
        user: Any = users[0]

        tenants: list[dict[str, Any]] = UserTenantService.get_tenants_by_user_id(user.id)
        return tenants


class ServiceMgr:
    @staticmethod
    def get_all_services():
        doc_engine = os.getenv("DOC_ENGINE", "elasticsearch")
        result = []
        configs = SERVICE_CONFIGS.configs
        for service_id, config in enumerate(configs):
            config_dict = config.to_dict()
            if config_dict["service_type"] == "retrieval":
                if config_dict["extra"]["retrieval_type"] != doc_engine:
                    continue
            try:
                service_detail = ServiceMgr.get_service_details(service_id)
                if "status" in service_detail:
                    config_dict["status"] = service_detail["status"]
                else:
                    config_dict["status"] = "timeout"
            except Exception as e:
                logging.warning(f"Can't get service details, error: {e}")
                config_dict["status"] = "timeout"
            if not config_dict["host"]:
                config_dict["host"] = "-"
            if not config_dict["port"]:
                config_dict["port"] = "-"
            result.append(config_dict)
        return result

    @staticmethod
    def get_services_by_type(service_type_str: str):
        raise AdminException("get_services_by_type: not implemented")

    @staticmethod
    def get_service_details(service_id: int):
        service_idx = int(service_id)
        configs = SERVICE_CONFIGS.configs
        if service_idx < 0 or service_idx >= len(configs):
            raise AdminException(f"invalid service_index: {service_idx}")

        service_config = configs[service_idx]

        # exclude retrieval service if retrieval_type is not matched
        doc_engine = os.getenv("DOC_ENGINE", "elasticsearch")
        if service_config.service_type == "retrieval":
            if service_config.retrieval_type != doc_engine:
                raise AdminException(f"invalid service_index: {service_idx}")

        service_info = {"name": service_config.name, "detail_func_name": service_config.detail_func_name}

        detail_func = getattr(health_utils, service_info.get("detail_func_name"))
        res = detail_func()
        res.update({"service_name": service_info.get("name")})
        return res

    @staticmethod
    def shutdown_service(service_id: int):
        raise AdminException("shutdown_service: not implemented")

    @staticmethod
    def restart_service(service_id: int):
        raise AdminException("restart_service: not implemented")


class SettingsMgr:
    @staticmethod
    def get_all():
        settings = SystemSettingsService.get_all()
        result = []
        for setting in settings:
            result.append(
                {
                    "name": setting.name,
                    "source": setting.source,
                    "data_type": setting.data_type,
                    "value": setting.value,
                }
            )
        return result

    @staticmethod
    def get_by_name(name: str):
        settings = SystemSettingsService.get_by_name(name)
        if len(settings) == 0:
            raise AdminException(f"Can't get setting: {name}")
        result = []
        for setting in settings:
            result.append(
                {
                    "name": setting.name,
                    "source": setting.source,
                    "data_type": setting.data_type,
                    "value": setting.value,
                }
            )
        return result

    @staticmethod
    def update_by_name(name: str, value: str):
        settings = SystemSettingsService.get_by_name(name)
        if len(settings) == 1:
            setting = settings[0]
            setting.value = value
            setting_dict = setting.to_dict()
            SystemSettingsService.update_by_name(name, setting_dict)
        elif len(settings) > 1:
            raise AdminException(f"Can't update more than 1 setting: {name}")
        else:
            # Create new setting if it doesn't exist

            # Determine data_type based on name and value
            if name.startswith("sandbox."):
                data_type = "json"
            elif name.endswith(".enabled"):
                data_type = "boolean"
            else:
                data_type = "string"

            new_setting = {
                "name": name,
                "value": str(value),
                "source": "admin",
                "data_type": data_type,
            }
            SystemSettingsService.save(**new_setting)


class ConfigMgr:
    @staticmethod
    def get_all():
        result = []
        configs = SERVICE_CONFIGS.configs
        for config in configs:
            config_dict = config.to_dict()
            result.append(config_dict)
        return result


class EnvironmentsMgr:
    @staticmethod
    def get_all():
        result = []

        env_kv = {"env": "DOC_ENGINE", "value": os.getenv("DOC_ENGINE")}
        result.append(env_kv)

        env_kv = {"env": "DEFAULT_SUPERUSER_EMAIL", "value": os.getenv("DEFAULT_SUPERUSER_EMAIL", "admin@ragflow.io")}
        result.append(env_kv)

        env_kv = {"env": "DB_TYPE", "value": os.getenv("DB_TYPE", "mysql")}
        result.append(env_kv)

        env_kv = {"env": "DEVICE", "value": os.getenv("DEVICE", "cpu")}
        result.append(env_kv)

        env_kv = {"env": "STORAGE_IMPL", "value": os.getenv("STORAGE_IMPL", "MINIO")}
        result.append(env_kv)

        return result


class SandboxMgr:
    """Manager for sandbox provider configuration and operations."""

    # Provider registry with metadata
    PROVIDER_REGISTRY = {
        "self_managed": {
            "name": "Self-Managed",
            "description": "On-premise deployment using Daytona/Docker",
            "tags": ["self-hosted", "low-latency", "secure"],
        },
        "aliyun_codeinterpreter": {
            "name": "Aliyun Code Interpreter",
            "description": "Aliyun Function Compute Code Interpreter - Code execution in serverless microVMs",
            "tags": ["saas", "cloud", "scalable", "aliyun"],
        },
        "e2b": {
            "name": "E2B",
            "description": "E2B Cloud - Code Execution Sandboxes",
            "tags": ["saas", "fast", "global"],
        },
    }

    @staticmethod
    def list_providers():
        """List all available sandbox providers."""
        result = []
        for provider_id, metadata in SandboxMgr.PROVIDER_REGISTRY.items():
            result.append({
                "id": provider_id,
                **metadata
            })
        return result

    @staticmethod
    def get_provider_config_schema(provider_id: str):
        """Get configuration schema for a specific provider."""
        from agent.sandbox.providers import (
            SelfManagedProvider,
            AliyunCodeInterpreterProvider,
            E2BProvider,
        )

        schemas = {
            "self_managed": SelfManagedProvider.get_config_schema(),
            "aliyun_codeinterpreter": AliyunCodeInterpreterProvider.get_config_schema(),
            "e2b": E2BProvider.get_config_schema(),
        }

        if provider_id not in schemas:
            raise AdminException(f"Unknown provider: {provider_id}")

        return schemas.get(provider_id, {})

    @staticmethod
    def get_config():
        """Get current sandbox configuration."""
        try:
            # Get active provider type
            provider_type_settings = SystemSettingsService.get_by_name("sandbox.provider_type")
            if not provider_type_settings:
                # Return default config if not set
                provider_type = "self_managed"
            else:
                provider_type = provider_type_settings[0].value

            # Get provider-specific config
            provider_config_settings = SystemSettingsService.get_by_name(f"sandbox.{provider_type}")
            if not provider_config_settings:
                provider_config = {}
            else:
                try:
                    provider_config = json.loads(provider_config_settings[0].value)
                except json.JSONDecodeError:
                    provider_config = {}

            return {
                "provider_type": provider_type,
                "config": provider_config,
            }
        except Exception as e:
            raise AdminException(f"Failed to get sandbox config: {str(e)}")

    @staticmethod
    def set_config(provider_type: str, config: dict, set_active: bool = True):
        """
        Set sandbox provider configuration.

        Args:
            provider_type: Provider identifier (e.g., "self_managed", "e2b")
            config: Provider configuration dictionary
            set_active: If True, also update the active provider. If False,
                       only update the configuration without switching providers.
                       Default: True

        Returns:
            Dictionary with updated provider_type and config
        """
        from agent.sandbox.providers import (
            SelfManagedProvider,
            AliyunCodeInterpreterProvider,
            E2BProvider,
        )

        try:
            # Validate provider type
            if provider_type not in SandboxMgr.PROVIDER_REGISTRY:
                raise AdminException(f"Unknown provider type: {provider_type}")

            # Get provider schema for validation
            schema = SandboxMgr.get_provider_config_schema(provider_type)

            # Validate config against schema
            for field_name, field_schema in schema.items():
                if field_schema.get("required", False) and field_name not in config:
                    raise AdminException(f"Required field '{field_name}' is missing")

                # Type validation
                if field_name in config:
                    field_type = field_schema.get("type")
                    if field_type == "integer":
                        if not isinstance(config[field_name], int):
                            raise AdminException(f"Field '{field_name}' must be an integer")
                    elif field_type == "string":
                        if not isinstance(config[field_name], str):
                            raise AdminException(f"Field '{field_name}' must be a string")
                    elif field_type == "bool":
                        if not isinstance(config[field_name], bool):
                            raise AdminException(f"Field '{field_name}' must be a boolean")

                    # Range validation for integers
                    if field_type == "integer" and field_name in config:
                        min_val = field_schema.get("min")
                        max_val = field_schema.get("max")
                        if min_val is not None and config[field_name] < min_val:
                            raise AdminException(f"Field '{field_name}' must be >= {min_val}")
                        if max_val is not None and config[field_name] > max_val:
                            raise AdminException(f"Field '{field_name}' must be <= {max_val}")

            # Provider-specific custom validation
            provider_classes = {
                "self_managed": SelfManagedProvider,
                "aliyun_codeinterpreter": AliyunCodeInterpreterProvider,
                "e2b": E2BProvider,
            }
            provider = provider_classes[provider_type]()
            is_valid, error_msg = provider.validate_config(config)
            if not is_valid:
                raise AdminException(f"Provider validation failed: {error_msg}")

            # Update provider_type only if set_active is True
            if set_active:
                SettingsMgr.update_by_name("sandbox.provider_type", provider_type)

            # Always update the provider config
            config_json = json.dumps(config)
            SettingsMgr.update_by_name(f"sandbox.{provider_type}", config_json)

            return {"provider_type": provider_type, "config": config}
        except AdminException:
            raise
        except Exception as e:
            raise AdminException(f"Failed to set sandbox config: {str(e)}")

    @staticmethod
    def test_connection(provider_type: str, config: dict):
        """
        Test connection to sandbox provider by executing a simple Python script.

        This creates a temporary sandbox instance and runs a test code to verify:
        - Connection credentials are valid
        - Sandbox can be created
        - Code execution works correctly

        Args:
            provider_type: Provider identifier
            config: Provider configuration dictionary

        Returns:
            dict with test results including stdout, stderr, exit_code, execution_time
        """
        try:
            from agent.sandbox.providers import (
                SelfManagedProvider,
                AliyunCodeInterpreterProvider,
                E2BProvider,
            )

            # Instantiate provider based on type
            provider_classes = {
                "self_managed": SelfManagedProvider,
                "aliyun_codeinterpreter": AliyunCodeInterpreterProvider,
                "e2b": E2BProvider,
            }

            if provider_type not in provider_classes:
                raise AdminException(f"Unknown provider type: {provider_type}")

            provider = provider_classes[provider_type]()

            # Initialize with config
            if not provider.initialize(config):
                raise AdminException(f"Failed to initialize provider '{provider_type}'")

            # Create a temporary sandbox instance for testing
            instance = provider.create_instance(template="python")

            if not instance or instance.status != "READY":
                raise AdminException(f"Failed to create sandbox instance. Status: {instance.status if instance else 'None'}")

            # Simple test code that exercises basic Python functionality
            test_code = """
# Test basic Python functionality
import sys
import json
import math

print("Python version:", sys.version)
print("Platform:", sys.platform)

# Test basic calculations
result = 2 + 2
print(f"2 + 2 = {result}")

# Test JSON operations
data = {"test": "data", "value": 123}
print(f"JSON dump: {json.dumps(data)}")

# Test math operations
print(f"Math.sqrt(16) = {math.sqrt(16)}")

# Test error handling
try:
    x = 1 / 1
    print("Division test: OK")
except Exception as e:
    print(f"Error: {e}")

# Return success indicator
print("TEST_PASSED")
"""

            # Execute test code with timeout
            execution_result = provider.execute_code(
                instance_id=instance.instance_id,
                code=test_code,
                language="python",
                timeout=10  # 10 seconds timeout
            )

            # Clean up the test instance (if provider supports it)
            try:
                if hasattr(provider, 'terminate_instance'):
                    provider.terminate_instance(instance.instance_id)
                    logging.info(f"Cleaned up test instance {instance.instance_id}")
                else:
                    logging.warning(f"Provider {provider_type} does not support terminate_instance, test instance may leak")
            except Exception as cleanup_error:
                logging.warning(f"Failed to cleanup test instance {instance.instance_id}: {cleanup_error}")

            # Build detailed result message
            success = execution_result.exit_code == 0 and "TEST_PASSED" in execution_result.stdout

            message_parts = [
                f"Test {success and 'PASSED' or 'FAILED'}",
                f"Exit code: {execution_result.exit_code}",
                f"Execution time: {execution_result.execution_time:.2f}s"
            ]

            if execution_result.stdout.strip():
                stdout_preview = execution_result.stdout.strip()[:200]
                message_parts.append(f"Output: {stdout_preview}...")

            if execution_result.stderr.strip():
                stderr_preview = execution_result.stderr.strip()[:200]
                message_parts.append(f"Errors: {stderr_preview}...")

            message = " | ".join(message_parts)

            return {
                "success": success,
                "message": message,
                "details": {
                    "exit_code": execution_result.exit_code,
                    "execution_time": execution_result.execution_time,
                    "stdout": execution_result.stdout,
                    "stderr": execution_result.stderr,
                }
            }

        except AdminException:
            raise
        except Exception as e:
            import traceback
            error_details = traceback.format_exc()
            raise AdminException(f"Connection test failed: {str(e)}\\n\\nStack trace:\\n{error_details}")
