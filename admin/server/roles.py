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
import inspect
import logging

from typing import Dict, Any, List

from api.common.exceptions import AdminException


class RoleMgr:
    @staticmethod
    def create_role(role_name: str, description: str):
        error_msg = f"not implement: create role: {role_name}, description: {description}"
        logging.error(error_msg)
        raise AdminException(error_msg)

    @staticmethod
    def update_role_description(role_name: str, description: str) -> Dict[str, Any]:
        error_msg = f"not implement: update role: {role_name} with description: {description}"
        logging.error(error_msg)
        raise AdminException(error_msg)

    @staticmethod
    def delete_role(role_name: str) -> Dict[str, Any]:
        error_msg = f"not implement: drop role: {role_name}"
        logging.error(error_msg)
        raise AdminException(error_msg)

    @staticmethod
    def list_roles() -> Dict[str, Any]:
        error_msg = f"not implement: list roles"
        logging.error(error_msg)
        raise AdminException(error_msg)

    @staticmethod
    def get_role_permissions(role_name: str) -> Dict[str, Any]:
        error_msg = f"not implement: show role {role_name}"
        logging.error(error_msg)
        raise AdminException(error_msg)

    @staticmethod
    def grant_role_permissions(role_name: str, permissions: str) -> Dict[str, Any]:
        raise AdminException(f"Not implement {inspect.currentframe().f_code.co_name}")

    @staticmethod
    def revoke_role_permissions(role_name: str, permissions: str) -> Dict[str, Any]:
        raise AdminException(f"Not implement {inspect.currentframe().f_code.co_name}")

    @staticmethod
    def update_user_role(user_name: str, role_name: str) -> Dict[str, Any]:
        raise AdminException(f"Not implement {inspect.currentframe().f_code.co_name}")

    @staticmethod
    def get_user_permissions(user_name: str) -> Dict[str, Any]:
        raise AdminException(f"Not implement {inspect.currentframe().f_code.co_name}")
