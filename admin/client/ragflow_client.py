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

from typing import Any
import urllib.parse
from http_client import HttpClient
from lark import Tree
from user import encrypt_password

import base64
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from Cryptodome.PublicKey import RSA


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

    def list_user_datasets(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        response = self.http_client.request("POST", "/kb/list", use_api_base=False, auth_kind="web")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list datasets, code: {res_json['code']}, message: {res_json['message']}")

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

        response = self.http_client.request("POST", "/dialog/next", use_api_base=False, auth_kind="web")
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list datasets, code: {res_json['code']}, message: {res_json['message']}")

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

    def list_user_default_models(self, command):
        if self.server_type != "user":
            print("This command is only allowed in USER mode")

        response = self.http_client.request("GET", "/user/tenant_info", use_api_base=False, auth_kind="web")
        res_json = response.json()
        if response.status_code == 200:
            new_input = []
            for key, value in res_json["data"].items():
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
