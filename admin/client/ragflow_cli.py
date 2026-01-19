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

import sys
import argparse
import base64
import getpass
import urllib.parse
from cmd import Cmd
from typing import Any, Dict, List

import requests
import warnings
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from Cryptodome.PublicKey import RSA
from lark import Lark, Tree
from parser import GRAMMAR, RAGFlowCLITransformer
from http_client import HttpClient
from ragflow_client import RAGFlowClient
from user import login_user

warnings.filterwarnings("ignore", category=getpass.GetPassWarning)

def encrypt(input_string):
    pub = "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArq9XTUSeYr2+N1h3Afl/z8Dse/2yD0ZGrKwx+EEEcdsBLca9Ynmx3nIB5obmLlSfmskLpBo0UACBmB5rEjBp2Q2f3AG3Hjd4B+gNCG6BDaawuDlgANIhGnaTLrIqWrrcm4EMzJOnAOI1fgzJRsOOUEfaS318Eq9OVO3apEyCCt0lOQK6PuksduOjVxtltDav+guVAA068NrPYmRNabVKRNLJpL8w4D44sfth5RvZ3q9t+6RTArpEtc5sh5ChzvqPOzKGMXW83C95TxmXqpbK6olN4RevSfVjEAgCydH6HN6OhtOQEcnrU97r9H0iZOWwbw3pVrZiUkuRD1R56Wzs2wIDAQAB\n-----END PUBLIC KEY-----"
    pub_key = RSA.importKey(pub)
    cipher = Cipher_pkcs1_v1_5.new(pub_key)
    cipher_text = cipher.encrypt(base64.b64encode(input_string.encode("utf-8")))
    return base64.b64encode(cipher_text).decode("utf-8")


def encode_to_base64(input_string):
    base64_encoded = base64.b64encode(input_string.encode("utf-8"))
    return base64_encoded.decode("utf-8")


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


class RAGFlowCLI(Cmd):
    def __init__(self):
        super().__init__()
        self.parser = Lark(GRAMMAR, start="start", parser="lalr", transformer=RAGFlowCLITransformer())
        self.command_history = []
        self.is_interactive = False
        self.account = "admin@ragflow.io"
        self.account_password: str = "admin"
        self.session = requests.Session()
        self.host: str = ""
        self.port: int = 0
        self.mode: str = "admin"
        self.ragflow_client = None

    intro = r"""Type "\h" for help."""
    prompt = "ragflow> "

    def onecmd(self, command: str) -> bool:
        try:
            result = self.parse_command(command)

            if isinstance(result, dict):
                if "type" in result and result.get("type") == "empty":
                    return False

            self.execute_command(result)

            if isinstance(result, Tree):
                return False

            if result.get("type") == "meta" and result.get("command") in ["q", "quit", "exit"]:
                return True

        except KeyboardInterrupt:
            print("\nUse '\\q' to quit")
        except EOFError:
            print("\nGoodbye!")
            return True
        return False

    def emptyline(self) -> bool:
        return False

    def default(self, line: str) -> bool:
        return self.onecmd(line)

    def parse_command(self, command_str: str) -> dict[str, str]:
        if not command_str.strip():
            return {"type": "empty"}

        self.command_history.append(command_str)

        try:
            result = self.parser.parse(command_str)
            return result
        except Exception as e:
            return {"type": "error", "message": f"Parse error: {str(e)}"}

    def verify_auth(self, arguments: dict, single_command: bool):
        server_type = arguments.get("type", "admin")
        http_client = HttpClient(arguments["host"], arguments["port"])

        user_name = arguments["username"]
        attempt_count = 3
        if single_command:
            attempt_count = 1

        try_count = 0
        while True:
            try_count += 1
            if try_count > attempt_count:
                return False

            if single_command:
                user_password = arguments["password"]
            else:
                user_password = getpass.getpass(f"password for {user_name}: ").strip()

            try:
                token = login_user(http_client, server_type, user_name, user_password)
                http_client.login_token = token
                self.ragflow_client = RAGFlowClient(http_client, server_type)
                return True
            except Exception as e:
                print(str(e))
                print("Can't access server for login (connection failed)")


        # self.host = arguments["host"]
        # self.port = arguments["port"]
        # # Determine mode and username
        # self.mode = arguments.get("type", "admin")
        # username = arguments.get("username", "admin@ragflow.io")
        # self.account = username
        #
        # # Set login endpoint based on mode
        # if self.mode == "admin":
        #     url = f"http://{self.host}:{self.port}/api/v1/admin/login"
        #     print("Attempt to access server for admin login")
        # else:  # user mode
        #     url = f"http://{self.host}:{self.port}/v1/user/login"
        #     print("Attempt to access server for user login")
        #
        # attempt_count = 3
        # if single_command:
        #     attempt_count = 1
        #
        # try_count = 0
        # while True:
        #     try_count += 1
        #     if try_count > attempt_count:
        #         return False
        #
        #     if single_command:
        #         account_passwd = arguments["password"]
        #     else:
        #         account_passwd = getpass.getpass(f"password for {self.account}: ").strip()
        #     try:
        #         self.account_password = encrypt(account_passwd)
        #         response = self.session.post(url, json={"email": self.account, "password": self.account_password})
        #         if response.status_code == 200:
        #             res_json = response.json()
        #             error_code = res_json.get("code", -1)
        #             if error_code == 0:
        #                 self.session.headers.update(
        #                     {"Content-Type": "application/json", "Authorization": response.headers["Authorization"],
        #                      "User-Agent": "RAGFlow-CLI/0.23.1"})
        #                 print("Authentication successful.")
        #                 return True
        #             else:
        #                 error_message = res_json.get("message", "Unknown error")
        #                 print(f"Authentication failed: {error_message}, try again")
        #                 continue
        #         else:
        #             print(f"Bad response，status: {response.status_code}, password is wrong")
        #     except Exception as e:
        #         print(str(e))
        #         print("Can't access server for login (connection failed)")

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

    def run_interactive(self):
        self.is_interactive = True
        print("RAGFlow command line interface - Type '\\?' for help, '\\q' to quit")

        while True:
            try:
                command = input("ragflow> ").strip()
                if not command:
                    continue

                print(f"command: {command}")
                result = self.parse_command(command)
                self.execute_command(result)

                if isinstance(result, Tree):
                    continue

                if result.get("type") == "meta" and result.get("command") in ["q", "quit", "exit"]:
                    break

            except KeyboardInterrupt:
                print("\nUse '\\q' to quit")
            except EOFError:
                print("\nGoodbye!")
                break

    def run_single_command(self, command: str):
        result = self.parse_command(command)
        self.execute_command(result)

    def parse_connection_args(self, args: List[str]) -> Dict[str, Any]:
        parser = argparse.ArgumentParser(description="RAGFlow CLI Client", add_help=False)
        parser.add_argument("-h", "--host", default="127.0.0.1", help="Admin or RAGFlow service host")
        parser.add_argument("-p", "--port", type=int, default=9381, help="Admin or RAGFlow service port")
        parser.add_argument("-w", "--password", default="admin", type=str, help="Superuser password")
        parser.add_argument("-t", "--type", default="admin", type=str, help="CLI mode, admin or user")
        parser.add_argument("-u", "--username", default=None,
                            help="Username (email). In admin mode defaults to admin@ragflow.io, in user mode required.")
        parser.add_argument("command", nargs="?", help="Single command")
        try:
            parsed_args, remaining_args = parser.parse_known_args(args)
            # Determine username based on mode
            username = parsed_args.username
            if parsed_args.type == "admin":
                if username is None:
                    username = "admin@ragflow.io"
            else:  # user mode
                if username is None:
                    print("Error: username (-u) is required in user mode")
                    return {"error": "Username required"}

            if remaining_args:
                command = remaining_args[0]
                return {
                    "host": parsed_args.host,
                    "port": parsed_args.port,
                    "password": parsed_args.password,
                    "type": parsed_args.type,
                    "username": username,
                    "command": command
                }
            else:
                return {
                    "host": parsed_args.host,
                    "port": parsed_args.port,
                    "type": parsed_args.type,
                    "username": username,
                }
        except SystemExit:
            return {"error": "Invalid connection arguments"}

    def execute_command(self, parsed_command: Dict[str, Any]):
        command_dict: dict
        if isinstance(parsed_command, Tree):
            command_dict = parsed_command.children[0]
        else:
            if parsed_command["type"] == "error":
                print(f"Error: {parsed_command['message']}")
                return
            else:
                command_dict = parsed_command

        # print(f"Parsed command: {command_dict}")

        command_type = command_dict["type"]

        match command_type:
            case "list_services":
                self.ragflow_client.list_services()
            case "show_service":
                self.ragflow_client.show_service(command_dict)
            case "restart_service":
                self.ragflow_client.restart_service(command_dict)
            case "shutdown_service":
                self.ragflow_client.shutdown_service(command_dict)
            case "startup_service":
                self.ragflow_client.startup_service(command_dict)
            case "list_users":
                self.ragflow_client.list_users(command_dict)
            case "show_user":
                self.ragflow_client.show_user(command_dict)
            case "drop_user":
                self.ragflow_client.drop_user(command_dict)
            case "alter_user":
                self.ragflow_client.alter_user(command_dict)
            case "create_user":
                self.ragflow_client.create_user(command_dict)
            case "activate_user":
                self.ragflow_client.activate_user(command_dict)
            case "list_datasets":
                self._handle_list_datasets(command_dict)
            case "list_agents":
                self._handle_list_agents(command_dict)
            case "create_role":
                self._create_role(command_dict)
            case "drop_role":
                self._drop_role(command_dict)
            case "alter_role":
                self._alter_role(command_dict)
            case "list_roles":
                self._list_roles(command_dict)
            case "show_role":
                self._show_role(command_dict)
            case "grant_permission":
                self._grant_permission(command_dict)
            case "revoke_permission":
                self._revoke_permission(command_dict)
            case "alter_user_role":
                self._alter_user_role(command_dict)
            case "show_user_permission":
                self._show_user_permission(command_dict)
            case "show_version":
                self.ragflow_client.show_version(command_dict)
            case "grant_admin":
                self.ragflow_client.grant_admin(command_dict)
            case "revoke_admin":
                self.ragflow_client.revoke_admin(command_dict)
            case "generate_key":
                self.ragflow_client.generate_key(command_dict)
            case "list_keys":
                self.ragflow_client.list_keys(command_dict)
            case "drop_key":
                self.ragflow_client.drop_key(command_dict)
            case "set_variable":
                self.ragflow_client.set_variable(command_dict)
            case "show_variable":
                self.ragflow_client.show_variable(command_dict)
            case "list_variables":
                self.ragflow_client.list_variables(command_dict)
            case "list_configs":
                self.ragflow_client.list_configs(command_dict)
            case "list_environments":
                self.ragflow_client.list_environments(command_dict)
            case "list_user_datasets":
                self._list_user_datasets(command_dict)
            case "list_user_agents":
                self._list_user_agents(command_dict)
            case "list_user_chats":
                self._list_user_chats(command_dict)
            case "list_user_model_providers":
                self._list_user_model_providers(command_dict)
            case "list_user_default_models":
                self._list_user_default_models(command_dict)
            case "meta":
                self._handle_meta_command(command_dict)
            case _:
                print(f"Command '{command_type}' would be executed with API")

    def _handle_list_services(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        url = f"http://{self.host}:{self.port}/api/v1/admin/services"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to get all services, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_show_service(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        service_id: int = command["number"]

        url = f"http://{self.host}:{self.port}/api/v1/admin/services/{service_id}"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            res_data = res_json["data"]
            if "status" in res_data and res_data["status"] == "alive":
                print(f"Service {res_data['service_name']} is alive, ")
                if isinstance(res_data["message"], str):
                    print(res_data["message"])
                else:
                    data = self._format_service_detail_table(res_data["message"])
                    self._print_table_simple(data)
            else:
                print(f"Service {res_data['service_name']} is down, {res_data['message']}")
        else:
            print(f"Fail to show service, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_restart_service(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        service_id: int = command["number"]
        print(f"Restart service {service_id}")

    def _handle_shutdown_service(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        service_id: int = command["number"]
        print(f"Shutdown service {service_id}")

    def _handle_startup_service(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        service_id: int = command["number"]
        print(f"Startup service {service_id}")

    def _handle_list_users(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        url = f"http://{self.host}:{self.port}/api/v1/admin/users"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to get all users, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_show_user(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Showing user: {user_name}")
        url = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name}"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            table_data = res_json["data"][0]
            table_data.pop("avatar")
            self._print_table_simple(table_data)
        else:
            print(f"Fail to get user {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_drop_user(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Drop user: {user_name}")
        url = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name}"
        response = self.session.delete(url)
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(f"Fail to drop user, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_alter_user(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        user_name_tree: Tree = command["user_name"]
        user_name: str = user_name_tree.children[0].strip("'\"")
        password_tree: Tree = command["password"]
        password: str = password_tree.children[0].strip("'\"")
        print(f"Alter user: {user_name}, password: ******")
        url = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/password"
        response = self.session.put(url, json={"new_password": encrypt(password)})
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(f"Fail to alter password, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_create_user(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        user_name_tree: Tree = command["user_name"]
        user_name: str = user_name_tree.children[0].strip("'\"")
        password_tree: Tree = command["password"]
        password: str = password_tree.children[0].strip("'\"")
        role: str = command["role"]
        print(f"Create user: {user_name}, password: ******, role: {role}")
        url = f"http://{self.host}:{self.port}/api/v1/admin/users"
        response = self.session.post(url, json={"user_name": user_name, "password": encrypt(password), "role": role})
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to create user {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_activate_user(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        user_name_tree: Tree = command["user_name"]
        user_name: str = user_name_tree.children[0].strip("'\"")
        activate_tree: Tree = command["activate_status"]
        activate_status: str = activate_tree.children[0].strip("'\"")
        if activate_status.lower() in ["on", "off"]:
            print(f"Alter user {user_name} activate status, turn {activate_status.lower()}.")
            url = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/activate"
            response = self.session.put(url, json={"activate_status": activate_status})
            res_json = response.json()
            if response.status_code == 200:
                print(res_json["message"])
            else:
                print(f"Fail to alter activate status, code: {res_json['code']}, message: {res_json['message']}")
        else:
            print(f"Unknown activate status: {activate_status}.")

    def _grant_admin(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        user_name_tree: Tree = command["user_name"]
        user_name: str = user_name_tree.children[0].strip("'\"")
        url = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/admin"
        # print(f"Grant admin: {url}")
        # return
        response = self.session.put(url)
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(
                f"Fail to grant {user_name} admin authorization, code: {res_json['code']}, message: {res_json['message']}")

    def _revoke_admin(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        user_name_tree: Tree = command["user_name"]
        user_name: str = user_name_tree.children[0].strip("'\"")
        url = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/admin"
        # print(f"Revoke admin: {url}")
        # return
        response = self.session.delete(url)
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(
                f"Fail to revoke {user_name} admin authorization, code: {res_json['code']}, message: {res_json['message']}")

    def _generate_key(self, command: dict[str, Any]) -> None:
        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Generating API key for user: {user_name}")
        url: str = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/new_token"
        response: requests.Response = self.session.post(url)
        res_json: dict[str, Any] = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(
                f"Failed to generate key for user {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _list_keys(self, command: dict[str, Any]) -> None:
        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Listing API keys for user: {user_name}")
        url: str = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/token_list"
        response: requests.Response = self.session.get(url)
        res_json: dict[str, Any] = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Failed to list keys for user {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _drop_key(self, command: dict[str, Any]) -> None:
        key_tree: Tree = command["key"]
        key: str = key_tree.children[0].strip("'\"")
        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Dropping API key for user: {user_name}")
        # URL encode the key to handle special characters
        encoded_key: str = urllib.parse.quote(key, safe="")
        url: str = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/token/{encoded_key}"
        response: requests.Response = self.session.delete(url)
        res_json: dict[str, Any] = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(f"Failed to drop key for user {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _set_variable(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        var_name_tree: Tree = command["var_name"]
        var_name = var_name_tree.children[0].strip("'\"")
        var_value_tree: Tree = command["var_value"]
        var_value = var_value_tree.children[0].strip("'\"")
        url = f"http://{self.host}:{self.port}/api/v1/admin/variables"
        response = self.session.put(url, json={"var_name": var_name, "var_value": var_value})
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(
                f"Fail to set variable {var_name} to {var_value}, code: {res_json['code']}, message: {res_json['message']}")

    def _show_variable(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        var_name_tree: Tree = command["var_name"]
        var_name = var_name_tree.children[0].strip("'\"")
        url = f"http://{self.host}:{self.port}/api/v1/admin/variables"
        response = self.session.get(url, json={"var_name": var_name})
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to get variable {var_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _list_variables(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        url = f"http://{self.host}:{self.port}/api/v1/admin/variables"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list variables, code: {res_json['code']}, message: {res_json['message']}")

    def _list_configs(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        url = f"http://{self.host}:{self.port}/api/v1/admin/configs"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list variables, code: {res_json['code']}, message: {res_json['message']}")

    def _list_environments(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        url = f"http://{self.host}:{self.port}/api/v1/admin/environments"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list variables, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_list_datasets(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Listing all datasets of user: {user_name}")
        url = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/datasets"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            table_data = res_json["data"]
            for t in table_data:
                t.pop("avatar")
            self._print_table_simple(table_data)
        else:
            print(f"Fail to get all datasets of {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_list_agents(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        username_tree: Tree = command["user_name"]
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Listing all agents of user: {user_name}")
        url = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/agents"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            table_data = res_json["data"]
            for t in table_data:
                t.pop("avatar")
            self._print_table_simple(table_data)
        else:
            print(f"Fail to get all agents of {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _create_role(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        role_name_tree: Tree = command["role_name"]
        role_name: str = role_name_tree.children[0].strip("'\"")
        desc_str: str = ""
        if "description" in command:
            desc_tree: Tree = command["description"]
            desc_str = desc_tree.children[0].strip("'\"")

        print(f"create role name: {role_name}, description: {desc_str}")
        url = f"http://{self.host}:{self.port}/api/v1/admin/roles"
        response = self.session.post(url, json={"role_name": role_name, "description": desc_str})
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to create role {role_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _drop_role(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        role_name_tree: Tree = command["role_name"]
        role_name: str = role_name_tree.children[0].strip("'\"")
        print(f"drop role name: {role_name}")
        url = f"http://{self.host}:{self.port}/api/v1/admin/roles/{role_name}"
        response = self.session.delete(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to drop role {role_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _alter_role(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        role_name_tree: Tree = command["role_name"]
        role_name: str = role_name_tree.children[0].strip("'\"")
        desc_tree: Tree = command["description"]
        desc_str: str = desc_tree.children[0].strip("'\"")

        print(f"alter role name: {role_name}, description: {desc_str}")
        url = f"http://{self.host}:{self.port}/api/v1/admin/roles/{role_name}"
        response = self.session.put(url, json={"description": desc_str})
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(
                f"Fail to update role {role_name} with description: {desc_str}, code: {res_json['code']}, message: {res_json['message']}")

    def _list_roles(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        url = f"http://{self.host}:{self.port}/api/v1/admin/roles"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list roles, code: {res_json['code']}, message: {res_json['message']}")

    def _show_role(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        role_name_tree: Tree = command["role_name"]
        role_name: str = role_name_tree.children[0].strip("'\"")
        print(f"show role: {role_name}")
        url = f"http://{self.host}:{self.port}/api/v1/admin/roles/{role_name}/permission"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list roles, code: {res_json['code']}, message: {res_json['message']}")

    def _grant_permission(self, command):
        if self.mode != "admin":
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
        url = f"http://{self.host}:{self.port}/api/v1/admin/roles/{role_name_str}/permission"
        response = self.session.post(url, json={"actions": actions, "resource": resource_str})
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(
                f"Fail to grant role {role_name_str} with {actions} on {resource_str}, code: {res_json['code']}, message: {res_json['message']}")

    def _revoke_permission(self, command):
        if self.mode != "admin":
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
        url = f"http://{self.host}:{self.port}/api/v1/admin/roles/{role_name_str}/permission"
        response = self.session.delete(url, json={"actions": actions, "resource": resource_str})
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(
                f"Fail to revoke role {role_name_str} with {actions} on {resource_str}, code: {res_json['code']}, message: {res_json['message']}")

    def _alter_user_role(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        role_name_tree: Tree = command["role_name"]
        role_name_str: str = role_name_tree.children[0].strip("'\"")
        user_name_tree: Tree = command["user_name"]
        user_name_str: str = user_name_tree.children[0].strip("'\"")
        print(f"alter_user_role user_name: {user_name_str}, role_name: {role_name_str}")
        url = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name_str}/role"
        response = self.session.put(url, json={"role_name": role_name_str})
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(
                f"Fail to alter user: {user_name_str} to role {role_name_str}, code: {res_json['code']}, message: {res_json['message']}")

    def _show_user_permission(self, command):
        if self.mode != "admin":
            print("This command is only allowed in ADMIN mode")

        user_name_tree: Tree = command["user_name"]
        user_name_str: str = user_name_tree.children[0].strip("'\"")
        print(f"show_user_permission user_name: {user_name_str}")
        url = f"http://{self.host}:{self.port}/api/v1/admin/users/{user_name_str}/permission"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(
                f"Fail to show user: {user_name_str} permission, code: {res_json['code']}, message: {res_json['message']}")

    def _show_version(self, command):
        if self.mode == "admin":
            url = f"http://{self.host}:{self.port}/api/v1/admin/version"
        else:
            url = f"http://{self.host}:{self.port}/v1/system/version"

        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            if self.mode == "admin":
                self._print_table_simple(res_json["data"])
            else:
                self._print_table_simple({"version": res_json["data"]})
        else:
            print(f"Fail to show version, code: {res_json['code']}, message: {res_json['message']}")

    def _list_user_datasets(self, command):
        if self.mode != "user":
            print("This command is only allowed in USER mode")

        url = f"http://{self.host}:{self.port}/v1/kb/list"
        response = self.session.post(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list datasets, code: {res_json['code']}, message: {res_json['message']}")

    def _list_user_agents(self, command):
        if self.mode != "user":
            print("This command is only allowed in USER mode")

        url = f"http://{self.host}:{self.port}/v1/canvas/list"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list datasets, code: {res_json['code']}, message: {res_json['message']}")

    def _list_user_chats(self, command):
        if self.mode != "user":
            print("This command is only allowed in USER mode")

        url = f"http://{self.host}:{self.port}/v1/dialog/next"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json["data"])
        else:
            print(f"Fail to list datasets, code: {res_json['code']}, message: {res_json['message']}")

    def _list_user_model_providers(self, command):
        if self.mode != "user":
            print("This command is only allowed in USER mode")

        url = f"http://{self.host}:{self.port}/v1/llm/my_llms"
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            new_input = []
            for key, value in res_json["data"].items():
                new_input.append({"model provider": key, "models": value})
            self._print_table_simple(new_input)

    def _list_user_default_models(self, command):
        if self.mode != "user":
            print("This command is only allowed in USER mode")

        url = f"http://{self.host}:{self.port}/v1/user/tenant_info"
        response = self.session.get(url)
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

    def _handle_meta_command(self, command):
        meta_command = command["command"]
        args = command.get("args", [])

        if meta_command in ["?", "h", "help"]:
            show_help()
        elif meta_command in ["q", "quit", "exit"]:
            print("Goodbye!")
        else:
            print(f"Meta command '{meta_command}' with args {args}")


def main():

    cli = RAGFlowCLI()

    args = cli.parse_connection_args(sys.argv)
    if "error" in args:
        print("Error: Invalid connection arguments")
        return

    if "command" in args:
        if "password" not in args:
            print("Error: password is missing")
            return
        if cli.verify_auth(args, single_command=True):
            command: str = args["command"]
            # print(f"Run single command: {command}")
            cli.run_single_command(command)
    else:

        # login with user and password

        # login_user(http_client, args["user"], args["password"])
        # cli.ragflow_client = RAGFlowClient(http_client, args["user"], args["password"])
        # cli.ragflow_client.login()

        if cli.verify_auth(args, single_command=False):
            print(r"""
                ____  ___   ______________                 ________    ____
               / __ \/   | / ____/ ____/ /___ _      __   / ____/ /   /  _/
              / /_/ / /| |/ / __/ /_  / / __ \ | /| / /  / /   / /    / /  
             / _, _/ ___ / /_/ / __/ / / /_/ / |/ |/ /  / /___/ /____/ /   
            /_/ |_/_/  |_\____/_/   /_/\____/|__/|__/   \____/_____/___/   
            """)
            cli.cmdloop()


if __name__ == "__main__":
    main()
