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

import time
from typing import Any, List, Optional
import multiprocessing as mp
from concurrent.futures import ProcessPoolExecutor, as_completed
import urllib.parse
from pathlib import Path
from http_client import HttpClient
from lark import Tree
from user import encrypt_password, login_user

import getpass
import base64
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from Cryptodome.PublicKey import RSA

try:
    from requests_toolbelt import MultipartEncoder
except Exception as e:  # pragma: no cover - fallback without toolbelt
    print(f"Fallback without belt: {e}")
    MultipartEncoder = None


def encrypt(input_string):
    pub = "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArq9XTUSeYr2+N1h3Afl/z8Dse/2yD0ZGrKwx+EEEcdsBLca9Ynmx3nIB5obmLlSfmskLpBo0UACBmB5rEjBp2Q2f3AG3Hjd4B+gNCG6BDaawuDlgANIhGnaTLrIqWrrcm4EMzJOnAOI1fgzJRsOOUEfaS318Eq9OVO3apEyCCt0lOQK6PuksduOjVxtltDav+guVAA068NrPYmRNabVKRNLJpL8w4D44sfth5RvZ3q9t+6RTArpEtc5sh5ChzvqPOzKGMXW83C95TxmXqpbK6olN4RevSfVjEAgCydH6HN6OhtOQEcnrU97r9H0iZOWwbw3pVrZiUkuRD1R56Wzs2wIDAQAB\n-----END PUBLIC KEY-----"
    pub_key = RSA.importKey(pub)
    cipher = Cipher_pkcs1_v1_5.new(pub_key)
    cipher_text = cipher.encrypt(base64.b64encode(input_string.encode("utf-8")))
    return base64.b64encode(cipher_text).decode("utf-8")


class RAGFlowClient:
    def __init__(self, http_client: HttpClient, server_type: str):
        self.http_client = http_client
        self.server_type = server_type

    def login_user(self, command):
        try:
            response = self.http_client.request("GET", "/system/ping", use_api_base=False, auth_kind="web")
            if response.status_code == 200 and response.content == b"pong":
                pass
            else:
                print("Server is down")
                return
        except Exception as e:
            print(str(e))
            print("Can't access server for login (connection failed)")
            return

        email : str = command["email"]
        user_password = getpass.getpass(f"password for {email}: ").strip()
        try:
            token = login_user(self.http_client, self.server_type, email, user_password)
            self.http_client.login_token = token
            print(f"Login user {email} successfully")
        except Exception as e:
            print(str(e))
            print("Can't access server for login (connection failed)")

    def ping_server(self, command):
        iterations = command.get("iterations", 1)
        if iterations > 1:
            response = self.http_client.request("GET", "/system/ping", use_api_base=False, auth_kind="web",
                                                iterations=iterations)
            return response
        else:
            response = self.http_client.request("GET", "/system/ping", use_api_base=False, auth_kind="web")
            if response.status_code == 200 and response.content == b"pong":
                print("Server is alive")
            else:
                print("Server is down")
            return None

    def register_user(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")
        username: str = command["user_name"]
        nickname: str = command["nickname"]
        password: str = command["password"]
        enc_password = encrypt_password(password)
        print(f"Register user: {nickname}, email: {username}, password: ******")
        payload = {"email": username, "nickname": nickname, "password": enc_password}
        response = self.http_client.request(method="POST", path="/user/register",
                                            json_body=payload, use_api_base=False, auth_kind="web")
        res_json = response.json()
        if response.status_code == 200:
            if res_json["code"] == 0:
                self._print_table_simple(res_json["data"])
            else:
                print(f"Fail to register user {username}, code: {res_json['code']}, message: {res_json['message']}")
        else:
            print(f"Fail to register user {username}, code: {res_json['code']}, message: {res_json['message']}")

    def list_services(self):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        response = self.http_client.request("GET", "/admin/services", use_api_base=True, auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to get all services, code: {res_json['code']}, message: {res_json['message']}")
        pass

    def show_service(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        service_id: int = command["number"]

        response = self.http_client.request("GET", f"/admin/services/{service_id}", use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            res_data = res_json["data"]
            if "status" in res_data and res_data["status"] == "alive":
                print(f"Service {res_data['service_name']} is alive, ")
                res_message = res_data["message"]
                if res_message is None:
                    return
                elif isinstance(res_message, str):
                    print(res_message)
                else:
                    data = self._format_service_detail_table(res_message)
                    self._print_table_simple(data)
            else:
                print(f"Service {res_data['service_name']} is down, {res_data['message']}")
        else:
            print(f"Fail to show service, code: {res_json['code']}, message: {res_json['message']}")

    def restart_service(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        # service_id: int = command["number"]
        print("Restart service isn't implemented")

    def shutdown_service(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        # service_id: int = command["number"]
        print("Shutdown service isn't implemented")

    def startup_service(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        # service_id: int = command["number"]
        print("Startup service isn't implemented")

    def list_users(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        response = self.http_client.request("GET", "/admin/users", use_api_base=True, auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to get all users, code: {res_json['code']}, message: {res_json['message']}")

    def show_user(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Showing user: {user_name}")
        response = self.http_client.request("GET", f"/admin/users/{user_name}", use_api_base=True, auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            table_data = res_json["data"][0]
            table_data.pop("avatar")
            self._print_table_simple(table_data)
        else:
            print(f"Fail to get user {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def drop_user(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Drop user: {user_name}")
        response = self.http_client.request("DELETE", f"/admin/users/{user_name}", use_api_base=True, auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(f"Fail to drop user, code: {res_json['code']}, message: {res_json['message']}")

    def alter_user(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        user_name_tree: Tree = command["user_name"]
        user_name: str = user_name_tree.children[0].strip("'\"")
        password_tree: Tree = command["password"]
        password: str = password_tree.children[0].strip("'\"")
        print(f"Alter user: {user_name}, password: ******")
        response = self.http_client.request("PUT", f"/admin/users/{user_name}/password",
                                            json_body={"new_password": encrypt_password(password)}, use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(f"Fail to alter password, code: {res_json['code']}, message: {res_json['message']}")

    def create_user(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        user_name_tree: Tree = command["user_name"]
        user_name: str = user_name_tree.children[0].strip("'\"")
        password_tree: Tree = command["password"]
        password: str = password_tree.children[0].strip("'\"")
        role: str = command["role"]
        print(f"Create user: {user_name}, password: ******, role: {role}")
        # enpass1 = encrypt(password)
        enc_password = encrypt_password(password)
        response = self.http_client.request(method="POST", path="/admin/users",
                                            json_body={"username": user_name, "password": enc_password, "role": role},
                                            use_api_base=True, auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to create user {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def activate_user(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        user_name_tree: Tree = command["user_name"]
        user_name: str = user_name_tree.children[0].strip("'\"")
        activate_tree: Tree = command["activate_status"]
        activate_status: str = activate_tree.children[0].strip("'\"")
        if activate_status.lower() in ["on", "off"]:
            print(f"Alter user {user_name} activate status, turn {activate_status.lower()}.")
            response = self.http_client.request("PUT", f"/admin/users/{user_name}/activate",
                                                json_body={"activate_status": activate_status}, use_api_base=True,
                                                auth_kind="admin")
            res_json = response.json()
            if response.status_code == 200:
                print(res_json["message"])
            else:
                print(f"Fail to alter activate status, code: {res_json['code']}, message: {res_json['message']}")
        else:
            print(f"Unknown activate status: {activate_status}.")

    def grant_admin(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        user_name_tree: Tree = command["user_name"]
        user_name: str = user_name_tree.children[0].strip("'\"")
        response = self.http_client.request("PUT", f"/admin/users/{user_name}/admin", use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(
                f"Fail to grant {user_name} admin authorization, code: {res_json['code']}, message: {res_json['message']}")

    def revoke_admin(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        user_name_tree: Tree = command["user_name"]
        user_name: str = user_name_tree.children[0].strip("'\"")
        response = self.http_client.request("DELETE", f"/admin/users/{user_name}/admin", use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(
                f"Fail to revoke {user_name} admin authorization, code: {res_json['code']}, message: {res_json['message']}")

    def create_role(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        role_name_tree: Tree = command["role_name"]
        role_name: str = role_name_tree.children[0].strip("'\"")
        desc_str: str = ""
        if "description" in command and command["description"] is not None:
            desc_tree: Tree = command["description"]
            desc_str = desc_tree.children[0].strip("'\"")

        print(f"create role name: {role_name}, description: {desc_str}")
        response = self.http_client.request("POST", "/admin/roles",
                                            json_body={"role_name": role_name, "description": desc_str},
                                            use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to create role {role_name}, code: {res_json['code']}, message: {res_json['message']}")

    def drop_role(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        role_name_tree: Tree = command["role_name"]
        role_name: str = role_name_tree.children[0].strip("'\"")
        print(f"drop role name: {role_name}")
        response = self.http_client.request("DELETE", f"/admin/roles/{role_name}",
                                            use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to drop role {role_name}, code: {res_json['code']}, message: {res_json['message']}")

    def alter_role(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        role_name_tree: Tree = command["role_name"]
        role_name: str = role_name_tree.children[0].strip("'\"")
        desc_tree: Tree = command["description"]
        desc_str: str = desc_tree.children[0].strip("'\"")

        print(f"alter role name: {role_name}, description: {desc_str}")
        response = self.http_client.request("PUT", f"/admin/roles/{role_name}",
                                            json_body={"description": desc_str},
                                            use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(
                f"Fail to update role {role_name} with description: {desc_str}, code: {res_json['code']}, message: {res_json['message']}")

    def list_roles(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        response = self.http_client.request("GET", "/admin/roles",
                                            use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list roles, code: {res_json['code']}, message: {res_json['message']}")

    def show_role(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        role_name_tree: Tree = command["role_name"]
        role_name: str = role_name_tree.children[0].strip("'\"")
        print(f"show role: {role_name}")
        response = self.http_client.request("GET", f"/admin/roles/{role_name}/permission",
                                            use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list roles, code: {res_json['code']}, message: {res_json['message']}")

    def grant_permission(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        role_name_tree: Tree = command["role_name"]
        role_name_str: str = role_name_tree.children[0].strip("'\"")
        resource_tree: Tree = command["resource"]
        resource_str: str = resource_tree.children[0].strip("'\"")
        action_tree_list: list = command["actions"]
        actions: list = []
        for action_tree in action_tree_list:
            action_str: str = action_tree.children[0].strip("'\"")
            actions.append(action_str)
        print(f"grant role_name: {role_name_str}, resource: {resource_str}, actions: {actions}")
        response = self.http_client.request("POST", f"/admin/roles/{role_name_str}/permission",
                                            json_body={"actions": actions, "resource": resource_str}, use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(
                f"Fail to grant role {role_name_str} with {actions} on {resource_str}, code: {res_json['code']}, message: {res_json['message']}")

    def revoke_permission(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        role_name_tree: Tree = command["role_name"]
        role_name_str: str = role_name_tree.children[0].strip("'\"")
        resource_tree: Tree = command["resource"]
        resource_str: str = resource_tree.children[0].strip("'\"")
        action_tree_list: list = command["actions"]
        actions: list = []
        for action_tree in action_tree_list:
            action_str: str = action_tree.children[0].strip("'\"")
            actions.append(action_str)
        print(f"revoke role_name: {role_name_str}, resource: {resource_str}, actions: {actions}")
        response = self.http_client.request("DELETE", f"/admin/roles/{role_name_str}/permission",
                                            json_body={"actions": actions, "resource": resource_str}, use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(
                f"Fail to revoke role {role_name_str} with {actions} on {resource_str}, code: {res_json['code']}, message: {res_json['message']}")

    def alter_user_role(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        role_name_tree: Tree = command["role_name"]
        role_name_str: str = role_name_tree.children[0].strip("'\"")
        user_name_tree: Tree = command["user_name"]
        user_name_str: str = user_name_tree.children[0].strip("'\"")
        print(f"alter_user_role user_name: {user_name_str}, role_name: {role_name_str}")
        response = self.http_client.request("PUT", f"/admin/users/{user_name_str}/role",
                                            json_body={"role_name": role_name_str}, use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(
                f"Fail to alter user: {user_name_str} to role {role_name_str}, code: {res_json['code']}, message: {res_json['message']}")

    def show_user_permission(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        user_name_tree: Tree = command["user_name"]
        user_name_str: str = user_name_tree.children[0].strip("'\"")
        print(f"show_user_permission user_name: {user_name_str}")
        response = self.http_client.request("GET", f"/admin/users/{user_name_str}/permission", use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(
                f"Fail to show user: {user_name_str} permission, code: {res_json['code']}, message: {res_json['message']}")

    def generate_key(self, command: dict[str, Any]) -> None:
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Generating API key for user: {user_name}")
        response = self.http_client.request("POST", f"/admin/users/{user_name}/keys", use_api_base=True,
                                            auth_kind="admin")
        res_json: dict[str, Any] = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(
                f"Failed to generate key for user {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def list_keys(self, command: dict[str, Any]) -> None:
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Listing API keys for user: {user_name}")
        response = self.http_client.request("GET", f"/admin/users/{user_name}/keys", use_api_base=True,
                                            auth_kind="admin")
        res_json: dict[str, Any] = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Failed to list keys for user {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def drop_key(self, command: dict[str, Any]) -> None:
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        key_tree: Tree = command["key"]
        key: str = key_tree.children[0].strip("'\"")
        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Dropping API key for user: {user_name}")
        # URL encode the key to handle special characters
        encoded_key: str = urllib.parse.quote(key, safe="")
        response = self.http_client.request("DELETE", f"/admin/users/{user_name}/keys/{encoded_key}", use_api_base=True,
                                            auth_kind="admin")
        res_json: dict[str, Any] = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(f"Failed to drop key for user {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def set_variable(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        var_name_tree: Tree = command["var_name"]
        var_name = var_name_tree.children[0].strip("'\"")
        var_value_tree: Tree = command["var_value"]
        var_value = var_value_tree.children[0].strip("'\"")
        response = self.http_client.request("PUT", "/admin/variables",
                                            json_body={"var_name": var_name, "var_value": var_value}, use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(
                f"Fail to set variable {var_name} to {var_value}, code: {res_json['code']}, message: {res_json['message']}")

    def show_variable(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        var_name_tree: Tree = command["var_name"]
        var_name = var_name_tree.children[0].strip("'\"")
        response = self.http_client.request(method="GET", path="/admin/variables", json_body={"var_name": var_name},
                                            use_api_base=True, auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to get variable {var_name}, code: {res_json['code']}, message: {res_json['message']}")

    def list_variables(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        response = self.http_client.request("GET", "/admin/variables", use_api_base=True, auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list variables, code: {res_json['code']}, message: {res_json['message']}")

    def list_configs(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        response = self.http_client.request("GET", "/admin/configs", use_api_base=True, auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list variables, code: {res_json['code']}, message: {res_json['message']}")

    def list_environments(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        response = self.http_client.request("GET", "/admin/environments", use_api_base=True, auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list variables, code: {res_json['code']}, message: {res_json['message']}")

    def handle_list_datasets(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Listing all datasets of user: {user_name}")

        response = self.http_client.request("GET", f"/admin/users/{user_name}/datasets", use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            table_data = res_json["data"]
            for t in table_data:
                t.pop("avatar")
            self._print_table_simple(table_data)
        else:
            print(f"Fail to get all datasets of {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def handle_list_agents(self, command):
        if self.server_type != "admin":
            print("This command is only allowed in ADMIN mode")

        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Listing all agents of user: {user_name}")
        response = self.http_client.request("GET", f"/admin/users/{user_name}/agents", use_api_base=True,
                                            auth_kind="admin")
        res_json = response.json()
        if response.status_code == 200:
            table_data = res_json["data"]
            for t in table_data:
                t.pop("avatar")
            self._print_table_simple(table_data)
        else:
            print(f"Fail to get all agents of {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def show_current_user(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")
        print("show current user")

    def create_model_provider(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")
        llm_factory: str = command["provider_name"]
        api_key: str = command["provider_key"]
        payload = {"api_key": api_key, "llm_factory": llm_factory}
        response = self.http_client.request("POST", "/llm/set_api_key", json_body=payload, use_api_base=False,
                                            auth_kind="web")
        res_json = response.json()
        if response.status_code == 200 and res_json["code"] == 0:
            print(f"Success to add model provider {llm_factory}")
        else:
            print(f"Fail to add model provider {llm_factory}, code: {res_json['code']}, message: {res_json['message']}")

    def drop_model_provider(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")
        llm_factory: str = command["provider_name"]
        payload = {"llm_factory": llm_factory}
        response = self.http_client.request("POST", "/llm/delete_factory", json_body=payload, use_api_base=False,
                                            auth_kind="web")
        res_json = response.json()
        if response.status_code == 200 and res_json["code"] == 0:
            print(f"Success to drop model provider {llm_factory}")
        else:
            print(
                f"Fail to drop model provider {llm_factory}, code: {res_json['code']}, message: {res_json['message']}")

    def set_default_model(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        model_type: str = command["model_type"]
        model_id: str = command["model_id"]
        self._set_default_models(model_type, model_id)

    def reset_default_model(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        model_type: str = command["model_type"]
        self._set_default_models(model_type, "")

    def list_user_datasets(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        iterations = command.get("iterations", 1)
        if iterations > 1:
            response = self.http_client.request("POST", "/kb/list", use_api_base=False, auth_kind="web",
                                                iterations=iterations)
            return response
        else:
            response = self.http_client.request("POST", "/kb/list", use_api_base=False, auth_kind="web")
            res_json = response.json()
            if response.status_code == 200:
                self._print_table_simple(res_json["data"]["kbs"])
            else:
                print(f"Fail to list datasets, code: {res_json['code']}, message: {res_json['message']}")
            return None

    def create_user_dataset(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")
        payload = {
            "name": command["dataset_name"],
            "embd_id": command["embedding"]
        }
        if "parser_id" in command:
            payload["parser_id"] = command["parser"]
        if "pipeline" in command:
            payload["pipeline_id"] = command["pipeline"]
        response = self.http_client.request("POST", "/kb/create", json_body=payload, use_api_base=False,
                                            auth_kind="web")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to create datasets, code: {res_json['code']}, message: {res_json['message']}")

    def drop_user_dataset(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        dataset_name = command["dataset_name"]
        dataset_id = self._get_dataset_id(dataset_name)
        if dataset_id is None:
            return
        payload = {"kb_id": dataset_id}
        response = self.http_client.request("POST", "/kb/rm", json_body=payload, use_api_base=False, auth_kind="web")
        res_json = response.json()
        if response.status_code == 200:
            print(f"Drop dataset {dataset_name} successfully")
        else:
            print(f"Fail to drop datasets, code: {res_json['code']}, message: {res_json['message']}")

    def list_user_dataset_files(self, command_dict):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        dataset_name = command_dict["dataset_name"]
        dataset_id = self._get_dataset_id(dataset_name)
        if dataset_id is None:
            return

        res_json = self._list_documents(dataset_name, dataset_id)
        if res_json is None:
            return
        self._print_table_simple(res_json)

    def list_user_agents(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        response = self.http_client.request("GET", "/canvas/list", use_api_base=False, auth_kind="web")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list datasets, code: {res_json['code']}, message: {res_json['message']}")

    def list_user_chats(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        res_json = self._list_chats(command)
        if res_json is None:
            return None
        if "iterations" in command:
            # for benchmark
            return res_json
        self._print_table_simple(res_json)

    def create_user_chat(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")
        '''
        description
        : 
        ""
        icon
        : 
        ""
        language
        : 
        "English"
        llm_id
        : 
        "glm-4-flash@ZHIPU-AI"
        llm_setting
        : 
        {}
        name
        : 
        "xx"
        prompt_config
        : 
        {empty_response: "", prologue: "Hi! I'm your assistant. What can I do for you?", quote: true,…}
        empty_response
        : 
        ""
        keyword
        : 
        false
        parameters
        : 
        [{key: "knowledge", optional: false}]
        prologue
        : 
        "Hi! I'm your assistant. What can I do for you?"
        quote
        : 
        true
        reasoning
        : 
        false
        refine_multiturn
        : 
        false
        system
        : 
        "You are an intelligent assistant. Your primary function is to answer questions based strictly on the provided knowledge base.\n\n      **Essential Rules:**\n        - Your answer must be derived **solely** from this knowledge base: `{knowledge}`.\n        - **When information is available**: Summarize the content to give a detailed answer.\n        - **When information is unavailable**: Your response must contain this exact sentence: \"The answer you are looking for is not found in the knowledge base!\"\n        - **Always consider** the entire conversation history."
        toc_enhance
        : 
        false
        tts
        : 
        false
        use_kg
        : 
        false
        similarity_threshold
        : 
        0.2
        top_n
        : 
        8
        vector_similarity_weight
        : 
        0.3
        '''
        chat_name = command["chat_name"]
        payload = {
            "description": "",
            "icon": "",
            "language": "English",
            "llm_setting": {},
            "prompt_config": {
                "empty_response": "",
                "prologue": "Hi! I'm your assistant. What can I do for you?",
                "quote": True,
                "keyword": False,
                "tts": False,
                "system": "You are an intelligent assistant. Your primary function is to answer questions based strictly on the provided knowledge base.\n\n      **Essential Rules:**\n        - Your answer must be derived **solely** from this knowledge base: `{knowledge}`.\n        - **When information is available**: Summarize the content to give a detailed answer.\n        - **When information is unavailable**: Your response must contain this exact sentence: \"The answer you are looking for is not found in the knowledge base!\"\n        - **Always consider** the entire conversation history.",
                "refine_multiturn": False,
                "use_kg": False,
                "reasoning": False,
                "parameters": [
                    {
                        "key": "knowledge",
                        "optional": False
                    }
                ],
                "toc_enhance": False
            },
            "similarity_threshold": 0.2,
            "top_n": 8,
            "vector_similarity_weight": 0.3
        }

        payload.update({"name": chat_name})
        response = self.http_client.request("POST", "/dialog/set", json_body=payload, use_api_base=False,
                                            auth_kind="web")
        res_json = response.json()
        if response.status_code == 200 and res_json["code"] == 0:
            print(f"Success to create chat: {chat_name}")
        else:
            print(f"Fail to create chat {chat_name}, code: {res_json['code']}, message: {res_json['message']}")

    def drop_user_chat(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")
        chat_name = command["chat_name"]
        res_json = self._list_chats(command)
        to_drop_chat_ids = []
        for elem in res_json:
            if elem["name"] == chat_name:
                to_drop_chat_ids.append(elem["id"])
        payload = {"dialog_ids": to_drop_chat_ids}
        response = self.http_client.request("POST", "/dialog/rm", json_body=payload, use_api_base=False,
                                            auth_kind="web")
        res_json = response.json()
        if response.status_code == 200 and res_json["code"] == 0:
            print(f"Success to drop chat: {chat_name}")
        else:
            print(f"Fail to drop chat {chat_name}, code: {res_json['code']}, message: {res_json['message']}")

    def list_user_model_providers(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        response = self.http_client.request("GET", "/llm/my_llms", use_api_base=False, auth_kind="web")
        res_json = response.json()
        if response.status_code == 200:
            new_input = []
            for key, value in res_json["data"].items():
                new_input.append({"model provider": key, "models": value})
            self._print_table_simple(new_input)
        else:
            print(f"Fail to list model provider, code: {res_json['code']}, message: {res_json['message']}")

    def list_user_default_models(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        res_json = self._get_default_models()
        if res_json is None:
            return
        else:
            new_input = []
            for key, value in res_json.items():
                if key == "asr_id" and value != "":
                    new_input.append({"model_category": "ASR", "model_name": value})
                elif key == "embd_id" and value != "":
                    new_input.append({"model_category": "Embedding", "model_name": value})
                elif key == "llm_id" and value != "":
                    new_input.append({"model_category": "LLM", "model_name": value})
                elif key == "rerank_id" and value != "":
                    new_input.append({"model_category": "Reranker", "model_name": value})
                elif key == "tts_id" and value != "":
                    new_input.append({"model_category": "TTS", "model_name": value})
                elif key == "img2txt_id" and value != "":
                    new_input.append({"model_category": "VLM", "model_name": value})
                else:
                    continue
            self._print_table_simple(new_input)

    def parse_dataset_docs(self, command_dict):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        dataset_name = command_dict["dataset_name"]
        dataset_id = self._get_dataset_id(dataset_name)
        if dataset_id is None:
            return

        res_json = self._list_documents(dataset_name, dataset_id)
        if res_json is None:
            return

        document_names = command_dict["document_names"]
        document_ids = []
        to_parse_doc_names = []
        for doc in res_json:
            doc_name = doc["name"]
            if doc_name in document_names:
                document_ids.append(doc["id"])
                document_names.remove(doc_name)
                to_parse_doc_names.append(doc_name)

        if len(document_ids) == 0:
            print(f"No documents found in {dataset_name}")
            return

        if len(document_names) != 0:
            print(f"Documents {document_names} not found in {dataset_name}")

        payload = {"doc_ids": document_ids, "run": 1}
        response = self.http_client.request("POST", "/document/run", json_body=payload, use_api_base=False,
                                            auth_kind="web")
        res_json = response.json()
        if response.status_code == 200 and res_json["code"] == 0:
            print(f"Success to parse {to_parse_doc_names} of {dataset_name}")
        else:
            print(
                f"Fail to parse documents {res_json["data"]["docs"]}, code: {res_json['code']}, message: {res_json['message']}")

    def parse_dataset(self, command_dict):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        dataset_name = command_dict["dataset_name"]
        dataset_id = self._get_dataset_id(dataset_name)
        if dataset_id is None:
            return

        res_json = self._list_documents(dataset_name, dataset_id)
        if res_json is None:
            return
        document_ids = []
        for doc in res_json:
            document_ids.append(doc["id"])

        payload = {"doc_ids": document_ids, "run": 1}
        response = self.http_client.request("POST", "/document/run", json_body=payload, use_api_base=False,
                                            auth_kind="web")
        res_json = response.json()
        if response.status_code == 200 and res_json["code"] == 0:
            pass
        else:
            print(f"Fail to parse dataset {dataset_name}, code: {res_json['code']}, message: {res_json['message']}")

        if command_dict["method"] == "async":
            print(f"Success to start parse dataset {dataset_name}")
            return
        else:
            print(f"Start to parse dataset {dataset_name}, please wait...")
            if self._wait_parse_done(dataset_name, dataset_id):
                print(f"Success to parse dataset {dataset_name}")
            else:
                print(f"Parse dataset {dataset_name} timeout")

    def import_docs_into_dataset(self, command_dict):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        dataset_name = command_dict["dataset_name"]
        dataset_id = self._get_dataset_id(dataset_name)
        if dataset_id is None:
            return

        document_paths = command_dict["document_paths"]
        paths = [Path(p) for p in document_paths]

        fields = []
        file_handles = []
        try:
            for path in paths:
                fh = path.open("rb")
                fields.append(("file", (path.name, fh)))
                file_handles.append(fh)
            fields.append(("kb_id", dataset_id))
            encoder = MultipartEncoder(fields=fields)
            headers = {"Content-Type": encoder.content_type}
            response = self.http_client.request(
                "POST",
                "/document/upload",
                headers=headers,
                data=encoder,
                json_body=None,
                params=None,
                stream=False,
                auth_kind="web",
                use_api_base=False
            )
            res = response.json()
            if res.get("code") == 0:
                print(f"Success to import documents into dataset {dataset_name}")
            else:
                print(f"Fail to import documents: code: {res['code']}, message: {res['message']}")
        except Exception as exc:
            print(f"Fail to import document into dataset: {dataset_name}, error: {exc}")
        finally:
            for fh in file_handles:
                fh.close()

    def search_on_datasets(self, command_dict):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        dataset_names = command_dict["datasets"]
        dataset_ids = []
        for dataset_name in dataset_names:
            dataset_id = self._get_dataset_id(dataset_name)
            if dataset_id is None:
                return
            dataset_ids.append(dataset_id)

        payload = {
            "question": command_dict["question"],
            "kb_id": dataset_ids,
            "similarity_threshold": 0.2,
            "vector_similarity_weight": 0.3,
            # "top_k": 1024,
            # "kb_id": command_dict["datasets"][0],
        }
        iterations = command_dict.get("iterations", 1)
        if iterations > 1:
            response = self.http_client.request("POST", "/chunk/retrieval_test", json_body=payload, use_api_base=False,
                                                auth_kind="web", iterations=iterations)
            return response
        else:
            response = self.http_client.request("POST", "/chunk/retrieval_test", json_body=payload, use_api_base=False,
                                                auth_kind="web")
            res_json = response.json()
            if response.status_code == 200:
                if res_json["code"] == 0:
                    self._print_table_simple(res_json["data"]["chunks"])
                else:
                    print(
                        f"Fail to search datasets: {dataset_names}, code: {res_json['code']}, message: {res_json['message']}")
            else:
                print(
                    f"Fail to search datasets: {dataset_names}, code: {res_json['code']}, message: {res_json['message']}")

    def show_version(self, command):
        if self.server_type == "admin":
            response = self.http_client.request("GET", "/admin/version", use_api_base=True, auth_kind="admin")
        else:
            response = self.http_client.request("GET", "/system/version", use_api_base=False, auth_kind="admin")

        res_json = response.json()
        if response.status_code == 200:
            if self.server_type == "admin":
                self._print_table_simple(res_json["data"])
            else:
                self._print_table_simple({"version": res_json["data"]})
        else:
            print(f"Fail to show version, code: {res_json['code']}, message: {res_json['message']}")

    def _wait_parse_done(self, dataset_name: str, dataset_id: str):
        start = time.monotonic()
        while True:
            docs = self._list_documents(dataset_name, dataset_id)
            if docs is None:
                return False
            all_done = True
            for doc in docs:
                if doc.get("run") != "3":
                    print(f"Document {doc["name"]} is not done, status: {doc.get("run")}")
                    all_done = False
                    break
            if all_done:
                return True
            if time.monotonic() - start > 60:
                return False
            time.sleep(0.5)

    def _list_documents(self, dataset_name: str, dataset_id: str):
        response = self.http_client.request("POST", f"/document/list?kb_id={dataset_id}", use_api_base=False,
                                            auth_kind="web")
        res_json = response.json()
        if response.status_code != 200:
            print(
                f"Fail to list files from dataset {dataset_name}, code: {res_json['code']}, message: {res_json['message']}")
            return None
        return res_json["data"]["docs"]

    def _get_dataset_id(self, dataset_name: str):
        response = self.http_client.request("POST", "/kb/list", use_api_base=False, auth_kind="web")
        res_json = response.json()
        if response.status_code != 200:
            print(f"Fail to list datasets, code: {res_json['code']}, message: {res_json['message']}")
            return None

        dataset_list = res_json["data"]["kbs"]
        dataset_id: str = ""
        for dataset in dataset_list:
            if dataset["name"] == dataset_name:
                dataset_id = dataset["id"]

        if dataset_id == "":
            print(f"Dataset {dataset_name} not found")
            return None
        return dataset_id

    def _list_chats(self, command):
        iterations = command.get("iterations", 1)
        if iterations > 1:
            response = self.http_client.request("POST", "/dialog/next", use_api_base=False, auth_kind="web",
                                                iterations=iterations)
            return response
        else:
            response = self.http_client.request("POST", "/dialog/next", use_api_base=False, auth_kind="web",
                                                iterations=iterations)
            res_json = response.json()
            if response.status_code == 200 and res_json["code"] == 0:
                return res_json["data"]["dialogs"]
            else:
                print(f"Fail to list datasets, code: {res_json['code']}, message: {res_json['message']}")
                return None

    def _get_default_models(self):
        response = self.http_client.request("GET", "/user/tenant_info", use_api_base=False, auth_kind="web")
        res_json = response.json()
        if response.status_code == 200:
            if res_json["code"] == 0:
                return res_json["data"]
            else:
                print(f"Fail to list user default models, code: {res_json['code']}, message: {res_json['message']}")
                return None
        else:
            print(f"Fail to list user default models, HTTP code: {response.status_code}, message: {res_json}")
            return None

    def _set_default_models(self, model_type, model_id):
        current_payload = self._get_default_models()
        if current_payload is None:
            return
        else:
            current_payload.update({model_type: model_id})
        payload = {
            "tenant_id": current_payload["tenant_id"],
            "llm_id": current_payload["llm_id"],
            "embd_id": current_payload["embd_id"],
            "img2txt_id": current_payload["img2txt_id"],
            "asr_id": current_payload["asr_id"],
            "tts_id": current_payload["tts_id"],
        }
        response = self.http_client.request("POST", "/user/set_tenant_info", json_body=payload, use_api_base=False,
                                            auth_kind="web")
        res_json = response.json()
        if response.status_code == 200 and res_json["code"] == 0:
            print(f"Success to set default llm to {model_type}")
        else:
            print(f"Fail to set default llm to {model_type}, code: {res_json['code']}, message: {res_json['message']}")

    def _format_service_detail_table(self, data):
        if isinstance(data, list):
            return data
        if not all([isinstance(v, list) for v in data.values()]):
            # normal table
            return data
        # handle task_executor heartbeats map, for example {'name': [{'done': 2, 'now': timestamp1}, {'done': 3, 'now': timestamp2}]
        task_executor_list = []
        for k, v in data.items():
            # display latest status
            heartbeats = sorted(v, key=lambda x: x["now"], reverse=True)
            task_executor_list.append(
                {
                    "task_executor_name": k,
                    **heartbeats[0],
                }
                if heartbeats
                else {"task_executor_name": k}
            )
        return task_executor_list

    def _print_table_simple(self, data):
        if not data:
            print("No data to print")
            return
        if isinstance(data, dict):
            # handle single row data
            data = [data]

        columns = list(set().union(*(d.keys() for d in data)))
        columns.sort()
        col_widths = {}

        def get_string_width(text):
            half_width_chars = " !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~\t\n\r"
            width = 0
            for char in text:
                if char in half_width_chars:
                    width += 1
                else:
                    width += 2
            return width

        for col in columns:
            max_width = get_string_width(str(col))
            for item in data:
                value_len = get_string_width(str(item.get(col, "")))
                if value_len > max_width:
                    max_width = value_len
            col_widths[col] = max(2, max_width)

        # Generate delimiter
        separator = "+" + "+".join(["-" * (col_widths[col] + 2) for col in columns]) + "+"

        # Print header
        print(separator)
        header = "|" + "|".join([f" {col:<{col_widths[col]}} " for col in columns]) + "|"
        print(header)
        print(separator)

        # Print data
        for item in data:
            row = "|"
            for col in columns:
                value = str(item.get(col, ""))
                if get_string_width(value) > col_widths[col]:
                    value = value[: col_widths[col] - 3] + "..."
                row += f" {value:<{col_widths[col] - (get_string_width(value) - len(value))}} |"
            print(row)

        print(separator)


def run_command(client: RAGFlowClient, command_dict: dict):
    command_type = command_dict["type"]

    match command_type:
        case "benchmark":
            run_benchmark(client, command_dict)
        case "login_user":
            client.login_user(command_dict)
        case "ping_server":
            return client.ping_server(command_dict)
        case "register_user":
            client.register_user(command_dict)
        case "list_services":
            client.list_services()
        case "show_service":
            client.show_service(command_dict)
        case "restart_service":
            client.restart_service(command_dict)
        case "shutdown_service":
            client.shutdown_service(command_dict)
        case "startup_service":
            client.startup_service(command_dict)
        case "list_users":
            client.list_users(command_dict)
        case "show_user":
            client.show_user(command_dict)
        case "drop_user":
            client.drop_user(command_dict)
        case "alter_user":
            client.alter_user(command_dict)
        case "create_user":
            client.create_user(command_dict)
        case "activate_user":
            client.activate_user(command_dict)
        case "list_datasets":
            client.handle_list_datasets(command_dict)
        case "list_agents":
            client.handle_list_agents(command_dict)
        case "create_role":
            client.create_role(command_dict)
        case "drop_role":
            client.drop_role(command_dict)
        case "alter_role":
            client.alter_role(command_dict)
        case "list_roles":
            client.list_roles(command_dict)
        case "show_role":
            client.show_role(command_dict)
        case "grant_permission":
            client.grant_permission(command_dict)
        case "revoke_permission":
            client.revoke_permission(command_dict)
        case "alter_user_role":
            client.alter_user_role(command_dict)
        case "show_user_permission":
            client.show_user_permission(command_dict)
        case "show_version":
            client.show_version(command_dict)
        case "grant_admin":
            client.grant_admin(command_dict)
        case "revoke_admin":
            client.revoke_admin(command_dict)
        case "generate_key":
            client.generate_key(command_dict)
        case "list_keys":
            client.list_keys(command_dict)
        case "drop_key":
            client.drop_key(command_dict)
        case "set_variable":
            client.set_variable(command_dict)
        case "show_variable":
            client.show_variable(command_dict)
        case "list_variables":
            client.list_variables(command_dict)
        case "list_configs":
            client.list_configs(command_dict)
        case "list_environments":
            client.list_environments(command_dict)
        case "create_model_provider":
            client.create_model_provider(command_dict)
        case "drop_model_provider":
            client.drop_model_provider(command_dict)
        case "show_current_user":
            client.show_current_user(command_dict)
        case "set_default_model":
            client.set_default_model(command_dict)
        case "reset_default_model":
            client.reset_default_model(command_dict)
        case "list_user_datasets":
            return client.list_user_datasets(command_dict)
        case "create_user_dataset":
            client.create_user_dataset(command_dict)
        case "drop_user_dataset":
            client.drop_user_dataset(command_dict)
        case "list_user_dataset_files":
            return client.list_user_dataset_files(command_dict)
        case "list_user_agents":
            return client.list_user_agents(command_dict)
        case "list_user_chats":
            return client.list_user_chats(command_dict)
        case "create_user_chat":
            client.create_user_chat(command_dict)
        case "drop_user_chat":
            client.drop_user_chat(command_dict)
        case "list_user_model_providers":
            client.list_user_model_providers(command_dict)
        case "list_user_default_models":
            client.list_user_default_models(command_dict)
        case "parse_dataset_docs":
            client.parse_dataset_docs(command_dict)
        case "parse_dataset":
            client.parse_dataset(command_dict)
        case "import_docs_into_dataset":
            client.import_docs_into_dataset(command_dict)
        case "search_on_datasets":
            return client.search_on_datasets(command_dict)
        case "meta":
            _handle_meta_command(command_dict)
        case _:
            print(f"Command '{command_type}' would be executed with API")


def _handle_meta_command(command: dict):
    meta_command = command["command"]
    args = command.get("args", [])

    if meta_command in ["?", "h", "help"]:
        show_help()
    elif meta_command in ["q", "quit", "exit"]:
        print("Goodbye!")
    else:
        print(f"Meta command '{meta_command}' with args {args}")


def show_help():
    """Help info"""
    help_text = """
Commands:
LIST SERVICES
SHOW SERVICE <service>
STARTUP SERVICE <service>
SHUTDOWN SERVICE <service>
RESTART SERVICE <service>
LIST USERS
SHOW USER <user>
DROP USER <user>
CREATE USER <user> <password>
ALTER USER PASSWORD <user> <new_password>
ALTER USER ACTIVE <user> <on/off>
LIST DATASETS OF <user>
LIST AGENTS OF <user>
CREATE ROLE <role>
DROP ROLE <role>
ALTER ROLE <role> SET DESCRIPTION <description>
LIST ROLES
SHOW ROLE <role>
GRANT <action_list> ON <function> TO ROLE <role>
REVOKE <action_list> ON <function> TO ROLE <role>
ALTER USER <user> SET ROLE <role>
SHOW USER PERMISSION <user>
SHOW VERSION
GRANT ADMIN <user>
REVOKE ADMIN <user>
GENERATE KEY FOR USER <user>
LIST KEYS OF <user>
DROP KEY <key> OF <user>

Meta Commands:
\\?, \\h, \\help     Show this help
\\q, \\quit, \\exit   Quit the CLI
    """
    print(help_text)


def run_benchmark(client: RAGFlowClient, command_dict: dict):
    concurrency = command_dict.get("concurrency", 1)
    iterations = command_dict.get("iterations", 1)
    command: dict = command_dict["command"]
    command.update({"iterations": iterations})

    command_type = command["type"]
    if concurrency < 1:
        print("Concurrency must be greater than 0")
        return
    elif concurrency == 1:
        result = run_command(client, command)
        success_count: int = 0
        response_list = result["response_list"]
        for response in response_list:
            match command_type:
                case "ping_server":
                    if response.status_code == 200:
                        success_count += 1
                case _:
                    res_json = response.json()
                    if response.status_code == 200 and res_json["code"] == 0:
                        success_count += 1

        total_duration = result["duration"]
        qps = iterations / total_duration if total_duration > 0 else None
        print(f"command: {command}, Concurrency: {concurrency}, iterations: {iterations}")
        print(
            f"total duration: {total_duration:.4f}s, QPS: {qps}, COMMAND_COUNT: {iterations}, SUCCESS: {success_count}, FAILURE: {iterations - success_count}")
        pass
    else:
        results: List[Optional[dict]] = [None] * concurrency
        mp_context = mp.get_context("spawn")
        start_time = time.perf_counter()
        with ProcessPoolExecutor(max_workers=concurrency, mp_context=mp_context) as executor:
            future_map = {
                executor.submit(
                    run_command,
                    client,
                    command
                ): idx
                for idx in range(concurrency)
            }
            for future in as_completed(future_map):
                idx = future_map[future]
                results[idx] = future.result()
        end_time = time.perf_counter()
        success_count = 0
        for result in results:
            response_list = result["response_list"]
            for response in response_list:
                match command_type:
                    case "ping_server":
                        if response.status_code == 200:
                            success_count += 1
                    case _:
                        res_json = response.json()
                        if response.status_code == 200 and res_json["code"] == 0:
                            success_count += 1

        total_duration = end_time - start_time
        total_command_count = iterations * concurrency
        qps = total_command_count / total_duration if total_duration > 0 else None
        print(f"command: {command}, Concurrency: {concurrency} , iterations: {iterations}")
        print(
            f"total duration: {total_duration:.4f}s, QPS: {qps}, COMMAND_COUNT: {total_command_count}, SUCCESS: {success_count}, FAILURE: {total_command_count - success_count}")

    pass
