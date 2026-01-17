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

        # Query all API tokens for this tenant
        api_tokens: Any = APITokenService.query(tenant_id=tenant_id)

        result: list[dict[str, Any]] = []
        for token_obj in api_tokens:
            result.append(token_obj.to_dict())

        return result

    @staticmethod
    def save_api_token(api_token: dict[str, Any]) -> bool:
        return APITokenService.save(**api_token)

    @staticmethod
    def delete_api_token(username: str, token: str) -> bool:
        # use email to find user. check exist and unique.
        user_list: list[Any] = UserService.query_user_by_email(username)
        if not user_list:
            raise UserNotFoundError(username)
        elif len(user_list) > 1:
            raise AdminException(f"Exist more than 1 user: {username}!")

        usr: Any = user_list[0]
        # tenant_id is typically the same as user_id for the owner tenant
        tenant_id: str = usr.id

        # Delete the API token
        deleted_count: int = APITokenService.filter_delete([APIToken.tenant_id == tenant_id, APIToken.token == token])
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
            raise AdminException(f"No setting: {name}")


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
