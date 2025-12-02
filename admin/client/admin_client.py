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

import argparse
import base64
from cmd import Cmd

from Cryptodome.PublicKey import RSA
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from typing import Dict, List, Any
from lark import Lark, Transformer, Tree
import requests
import getpass

GRAMMAR = r"""
start: command

command: sql_command | meta_command

sql_command: list_services
           | show_service
           | startup_service
           | shutdown_service
           | restart_service
           | list_users
           | show_user
           | drop_user
           | alter_user
           | create_user
           | activate_user
           | list_datasets
           | list_agents
           | create_role
           | drop_role
           | alter_role
           | list_roles
           | show_role
           | grant_permission
           | revoke_permission
           | alter_user_role
           | show_user_permission
           | show_version

// meta command definition
meta_command: "\\" meta_command_name [meta_args]

meta_command_name: /[a-zA-Z?]+/
meta_args: (meta_arg)+

meta_arg: /[^\\s"']+/ | quoted_string

// command definition

LIST: "LIST"i
SERVICES: "SERVICES"i
SHOW: "SHOW"i
CREATE: "CREATE"i
SERVICE: "SERVICE"i
SHUTDOWN: "SHUTDOWN"i
STARTUP: "STARTUP"i
RESTART: "RESTART"i
USERS: "USERS"i
DROP: "DROP"i
USER: "USER"i
ALTER: "ALTER"i
ACTIVE: "ACTIVE"i
PASSWORD: "PASSWORD"i
DATASETS: "DATASETS"i
OF: "OF"i
AGENTS: "AGENTS"i
ROLE: "ROLE"i
ROLES: "ROLES"i
DESCRIPTION: "DESCRIPTION"i
GRANT: "GRANT"i
REVOKE: "REVOKE"i
ALL: "ALL"i
PERMISSION: "PERMISSION"i
TO: "TO"i
FROM: "FROM"i
FOR: "FOR"i
RESOURCES: "RESOURCES"i
ON: "ON"i
SET: "SET"i
VERSION: "VERSION"i

list_services: LIST SERVICES ";"
show_service: SHOW SERVICE NUMBER ";"
startup_service: STARTUP SERVICE NUMBER ";"
shutdown_service: SHUTDOWN SERVICE NUMBER ";"
restart_service: RESTART SERVICE NUMBER ";"

list_users: LIST USERS ";"
drop_user: DROP USER quoted_string ";"
alter_user: ALTER USER PASSWORD quoted_string quoted_string ";"
show_user: SHOW USER quoted_string ";"
create_user: CREATE USER quoted_string quoted_string ";"
activate_user: ALTER USER ACTIVE quoted_string status ";"

list_datasets: LIST DATASETS OF quoted_string ";"
list_agents: LIST AGENTS OF quoted_string ";"

create_role: CREATE ROLE identifier [DESCRIPTION quoted_string] ";"
drop_role: DROP ROLE identifier ";"
alter_role: ALTER ROLE identifier SET DESCRIPTION quoted_string ";"
list_roles: LIST ROLES ";"
show_role: SHOW ROLE identifier ";"

grant_permission: GRANT action_list ON identifier TO ROLE identifier ";"
revoke_permission: REVOKE action_list ON identifier FROM ROLE identifier ";"
alter_user_role: ALTER USER quoted_string SET ROLE identifier ";"
show_user_permission: SHOW USER PERMISSION quoted_string ";"

show_version: SHOW VERSION ";"

action_list: identifier ("," identifier)*

identifier: WORD
quoted_string: QUOTED_STRING
status: WORD

QUOTED_STRING: /'[^']+'/ | /"[^"]+"/
WORD: /[a-zA-Z0-9_\-\.]+/
NUMBER: /[0-9]+/

%import common.WS
%ignore WS
"""


class AdminTransformer(Transformer):

    def start(self, items):
        return items[0]

    def command(self, items):
        return items[0]

    def list_services(self, items):
        result = {'type': 'list_services'}
        return result

    def show_service(self, items):
        service_id = int(items[2])
        return {"type": "show_service", "number": service_id}

    def startup_service(self, items):
        service_id = int(items[2])
        return {"type": "startup_service", "number": service_id}

    def shutdown_service(self, items):
        service_id = int(items[2])
        return {"type": "shutdown_service", "number": service_id}

    def restart_service(self, items):
        service_id = int(items[2])
        return {"type": "restart_service", "number": service_id}

    def list_users(self, items):
        return {"type": "list_users"}

    def show_user(self, items):
        user_name = items[2]
        return {"type": "show_user", "user_name": user_name}

    def drop_user(self, items):
        user_name = items[2]
        return {"type": "drop_user", "user_name": user_name}

    def alter_user(self, items):
        user_name = items[3]
        new_password = items[4]
        return {"type": "alter_user", "user_name": user_name, "password": new_password}

    def create_user(self, items):
        user_name = items[2]
        password = items[3]
        return {"type": "create_user", "user_name": user_name, "password": password, "role": "user"}

    def activate_user(self, items):
        user_name = items[3]
        activate_status = items[4]
        return {"type": "activate_user", "activate_status": activate_status, "user_name": user_name}

    def list_datasets(self, items):
        user_name = items[3]
        return {"type": "list_datasets", "user_name": user_name}

    def list_agents(self, items):
        user_name = items[3]
        return {"type": "list_agents", "user_name": user_name}

    def create_role(self, items):
        role_name = items[2]
        if len(items) > 4:
            description = items[4]
            return {"type": "create_role", "role_name": role_name, "description": description}
        else:
            return {"type": "create_role", "role_name": role_name}

    def drop_role(self, items):
        role_name = items[2]
        return {"type": "drop_role", "role_name": role_name}

    def alter_role(self, items):
        role_name = items[2]
        description = items[5]
        return {"type": "alter_role", "role_name": role_name, "description": description}

    def list_roles(self, items):
        return {"type": "list_roles"}

    def show_role(self, items):
        role_name = items[2]
        return {"type": "show_role", "role_name": role_name}

    def grant_permission(self, items):
        action_list = items[1]
        resource = items[3]
        role_name = items[6]
        return {"type": "grant_permission", "role_name": role_name, "resource": resource, "actions": action_list}

    def revoke_permission(self, items):
        action_list = items[1]
        resource = items[3]
        role_name = items[6]
        return {
            "type": "revoke_permission",
            "role_name": role_name,
            "resource": resource, "actions": action_list
        }

    def alter_user_role(self, items):
        user_name = items[2]
        role_name = items[5]
        return {"type": "alter_user_role", "user_name": user_name, "role_name": role_name}

    def show_user_permission(self, items):
        user_name = items[3]
        return {"type": "show_user_permission", "user_name": user_name}

    def show_version(self, items):
        return {"type": "show_version"}

    def action_list(self, items):
        return items

    def meta_command(self, items):
        command_name = str(items[0]).lower()
        args = items[1:] if len(items) > 1 else []

        # handle quoted parameter
        parsed_args = []
        for arg in args:
            if hasattr(arg, 'value'):
                parsed_args.append(arg.value)
            else:
                parsed_args.append(str(arg))

        return {'type': 'meta', 'command': command_name, 'args': parsed_args}

    def meta_command_name(self, items):
        return items[0]

    def meta_args(self, items):
        return items


def encrypt(input_string):
    pub = '-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArq9XTUSeYr2+N1h3Afl/z8Dse/2yD0ZGrKwx+EEEcdsBLca9Ynmx3nIB5obmLlSfmskLpBo0UACBmB5rEjBp2Q2f3AG3Hjd4B+gNCG6BDaawuDlgANIhGnaTLrIqWrrcm4EMzJOnAOI1fgzJRsOOUEfaS318Eq9OVO3apEyCCt0lOQK6PuksduOjVxtltDav+guVAA068NrPYmRNabVKRNLJpL8w4D44sfth5RvZ3q9t+6RTArpEtc5sh5ChzvqPOzKGMXW83C95TxmXqpbK6olN4RevSfVjEAgCydH6HN6OhtOQEcnrU97r9H0iZOWwbw3pVrZiUkuRD1R56Wzs2wIDAQAB\n-----END PUBLIC KEY-----'
    pub_key = RSA.importKey(pub)
    cipher = Cipher_pkcs1_v1_5.new(pub_key)
    cipher_text = cipher.encrypt(base64.b64encode(input_string.encode('utf-8')))
    return base64.b64encode(cipher_text).decode("utf-8")


def encode_to_base64(input_string):
    base64_encoded = base64.b64encode(input_string.encode('utf-8'))
    return base64_encoded.decode('utf-8')


class AdminCLI(Cmd):
    def __init__(self):
        super().__init__()
        self.parser = Lark(GRAMMAR, start='start', parser='lalr', transformer=AdminTransformer())
        self.command_history = []
        self.is_interactive = False
        self.admin_account = "admin@ragflow.io"
        self.admin_password: str = "admin"
        self.session = requests.Session()
        self.access_token: str = ""
        self.host: str = ""
        self.port: int = 0

    intro = r"""Type "\h" for help."""
    prompt = "admin> "

    def onecmd(self, command: str) -> bool:
        try:
            result = self.parse_command(command)

            if isinstance(result, dict):
                if 'type' in result and result.get('type') == 'empty':
                    return False

            self.execute_command(result)

            if isinstance(result, Tree):
                return False

            if result.get('type') == 'meta' and result.get('command') in ['q', 'quit', 'exit']:
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
            return {'type': 'empty'}

        self.command_history.append(command_str)

        try:
            result = self.parser.parse(command_str)
            return result
        except Exception as e:
            return {'type': 'error', 'message': f'Parse error: {str(e)}'}

    def verify_admin(self, arguments: dict, single_command: bool):
        self.host = arguments['host']
        self.port = arguments['port']
        print(f"Attempt to access ip: {self.host}, port: {self.port}")
        url = f"http://{self.host}:{self.port}/api/v1/admin/login"

        attempt_count = 3
        if single_command:
            attempt_count = 1

        try_count = 0
        while True:
            try_count += 1
            if try_count > attempt_count:
                return False

            if single_command:
                admin_passwd = arguments['password']
            else:
                admin_passwd = getpass.getpass(f"password for {self.admin_account}: ").strip()
            try:
                self.admin_password = encrypt(admin_passwd)
                response = self.session.post(url, json={'email': self.admin_account, 'password': self.admin_password})
                if response.status_code == 200:
                    res_json = response.json()
                    error_code = res_json.get('code', -1)
                    if error_code == 0:
                        self.session.headers.update({
                            'Content-Type': 'application/json',
                            'Authorization': response.headers['Authorization'],
                            'User-Agent': 'RAGFlow-CLI/0.22.1'
                        })
                        print("Authentication successful.")
                        return True
                    else:
                        error_message = res_json.get('message', 'Unknown error')
                        print(f"Authentication failed: {error_message}, try again")
                        continue
                else:
                    print(f"Bad response，status: {response.status_code}, password is wrong")
            except Exception as e:
                print(str(e))
                print(f"Can't access {self.host}, port: {self.port}")

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
            task_executor_list.append({
                "task_executor_name": k,
                **heartbeats[0],
            } if heartbeats else {"task_executor_name": k})
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
            half_width_chars = (
                " !\"#$%&'()*+,-./0123456789:;<=>?@"
                "ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`"
                "abcdefghijklmnopqrstuvwxyz{|}~"
                "\t\n\r"
            )
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
                value_len = get_string_width(str(item.get(col, '')))
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
                value = str(item.get(col, ''))
                if get_string_width(value) > col_widths[col]:
                    value = value[:col_widths[col] - 3] + "..."
                row += f" {value:<{col_widths[col] - (get_string_width(value) - len(value))}} |"
            print(row)

        print(separator)

    def run_interactive(self):

        self.is_interactive = True
        print("RAGFlow Admin command line interface - Type '\\?' for help, '\\q' to quit")

        while True:
            try:
                command = input("admin> ").strip()
                if not command:
                    continue

                print(f"command: {command}")
                result = self.parse_command(command)
                self.execute_command(result)

                if isinstance(result, Tree):
                    continue

                if result.get('type') == 'meta' and result.get('command') in ['q', 'quit', 'exit']:
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
        parser = argparse.ArgumentParser(description='Admin CLI Client', add_help=False)
        parser.add_argument('-h', '--host', default='localhost', help='Admin service host')
        parser.add_argument('-p', '--port', type=int, default=9381, help='Admin service port')
        parser.add_argument('-w', '--password', default='admin', type=str, help='Superuser password')
        parser.add_argument('command', nargs='?', help='Single command')
        try:
            parsed_args, remaining_args = parser.parse_known_args(args)
            if remaining_args:
                command = remaining_args[0]
                return {
                    'host': parsed_args.host,
                    'port': parsed_args.port,
                    'password': parsed_args.password,
                    'command': command
                }
            else:
                return {
                    'host': parsed_args.host,
                    'port': parsed_args.port,
                }
        except SystemExit:
            return {'error': 'Invalid connection arguments'}

    def execute_command(self, parsed_command: Dict[str, Any]):

        command_dict: dict
        if isinstance(parsed_command, Tree):
            command_dict = parsed_command.children[0]
        else:
            if parsed_command['type'] == 'error':
                print(f"Error: {parsed_command['message']}")
                return
            else:
                command_dict = parsed_command

        # print(f"Parsed command: {command_dict}")

        command_type = command_dict['type']

        match command_type:
            case 'list_services':
                self._handle_list_services(command_dict)
            case 'show_service':
                self._handle_show_service(command_dict)
            case 'restart_service':
                self._handle_restart_service(command_dict)
            case 'shutdown_service':
                self._handle_shutdown_service(command_dict)
            case 'startup_service':
                self._handle_startup_service(command_dict)
            case 'list_users':
                self._handle_list_users(command_dict)
            case 'show_user':
                self._handle_show_user(command_dict)
            case 'drop_user':
                self._handle_drop_user(command_dict)
            case 'alter_user':
                self._handle_alter_user(command_dict)
            case 'create_user':
                self._handle_create_user(command_dict)
            case 'activate_user':
                self._handle_activate_user(command_dict)
            case 'list_datasets':
                self._handle_list_datasets(command_dict)
            case 'list_agents':
                self._handle_list_agents(command_dict)
            case 'create_role':
                self._create_role(command_dict)
            case 'drop_role':
                self._drop_role(command_dict)
            case 'alter_role':
                self._alter_role(command_dict)
            case 'list_roles':
                self._list_roles(command_dict)
            case 'show_role':
                self._show_role(command_dict)
            case 'grant_permission':
                self._grant_permission(command_dict)
            case 'revoke_permission':
                self._revoke_permission(command_dict)
            case 'alter_user_role':
                self._alter_user_role(command_dict)
            case 'show_user_permission':
                self._show_user_permission(command_dict)
            case 'show_version':
                self._show_version(command_dict)
            case 'meta':
                self._handle_meta_command(command_dict)
            case _:
                print(f"Command '{command_type}' would be executed with API")

    def _handle_list_services(self, command):
        print("Listing all services")

        url = f'http://{self.host}:{self.port}/api/v1/admin/services'
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to get all services, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_show_service(self, command):
        service_id: int = command['number']
        print(f"Showing service: {service_id}")

        url = f'http://{self.host}:{self.port}/api/v1/admin/services/{service_id}'
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            res_data = res_json['data']
            if 'status' in res_data and res_data['status'] == 'alive':
                print(f"Service {res_data['service_name']} is alive, ")
                if isinstance(res_data['message'], str):
                    print(res_data['message'])
                else:
                    data = self._format_service_detail_table(res_data['message'])
                    self._print_table_simple(data)
            else:
                print(f"Service {res_data['service_name']} is down, {res_data['message']}")
        else:
            print(f"Fail to show service, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_restart_service(self, command):
        service_id: int = command['number']
        print(f"Restart service {service_id}")

    def _handle_shutdown_service(self, command):
        service_id: int = command['number']
        print(f"Shutdown service {service_id}")

    def _handle_startup_service(self, command):
        service_id: int = command['number']
        print(f"Startup service {service_id}")

    def _handle_list_users(self, command):
        print("Listing all users")

        url = f'http://{self.host}:{self.port}/api/v1/admin/users'
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to get all users, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_show_user(self, command):
        username_tree: Tree = command['user_name']
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Showing user: {user_name}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users/{user_name}'
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            table_data = res_json['data']
            table_data.pop('avatar')
            self._print_table_simple(table_data)
        else:
            print(f"Fail to get user {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_drop_user(self, command):
        username_tree: Tree = command['user_name']
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Drop user: {user_name}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users/{user_name}'
        response = self.session.delete(url)
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(f"Fail to drop user, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_alter_user(self, command):
        user_name_tree: Tree = command['user_name']
        user_name: str = user_name_tree.children[0].strip("'\"")
        password_tree: Tree = command['password']
        password: str = password_tree.children[0].strip("'\"")
        print(f"Alter user: {user_name}, password: {password}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/password'
        response = self.session.put(url, json={'new_password': encrypt(password)})
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(f"Fail to alter password, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_create_user(self, command):
        user_name_tree: Tree = command['user_name']
        user_name: str = user_name_tree.children[0].strip("'\"")
        password_tree: Tree = command['password']
        password: str = password_tree.children[0].strip("'\"")
        role: str = command['role']
        print(f"Create user: {user_name}, password: {password}, role: {role}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users'
        response = self.session.post(
            url,
            json={'user_name': user_name, 'password': encrypt(password), 'role': role}
        )
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to create user {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_activate_user(self, command):
        user_name_tree: Tree = command['user_name']
        user_name: str = user_name_tree.children[0].strip("'\"")
        activate_tree: Tree = command['activate_status']
        activate_status: str = activate_tree.children[0].strip("'\"")
        if activate_status.lower() in ['on', 'off']:
            print(f"Alter user {user_name} activate status, turn {activate_status.lower()}.")
            url = f'http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/activate'
            response = self.session.put(url, json={'activate_status': activate_status})
            res_json = response.json()
            if response.status_code == 200:
                print(res_json["message"])
            else:
                print(f"Fail to alter activate status, code: {res_json['code']}, message: {res_json['message']}")
        else:
            print(f"Unknown activate status: {activate_status}.")

    def _handle_list_datasets(self, command):
        username_tree: Tree = command['user_name']
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Listing all datasets of user: {user_name}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/datasets'
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            table_data = res_json['data']
            for t in table_data:
                t.pop('avatar')
            self._print_table_simple(table_data)
        else:
            print(f"Fail to get all datasets of {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_list_agents(self, command):
        username_tree: Tree = command['user_name']
        user_name: str = username_tree.children[0].strip("'\"")
        print(f"Listing all agents of user: {user_name}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users/{user_name}/agents'
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            table_data = res_json['data']
            for t in table_data:
                t.pop('avatar')
            self._print_table_simple(table_data)
        else:
            print(f"Fail to get all agents of {user_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _create_role(self, command):
        role_name_tree: Tree = command['role_name']
        role_name: str = role_name_tree.children[0].strip("'\"")
        desc_str: str = ''
        if 'description' in command:
            desc_tree: Tree = command['description']
            desc_str = desc_tree.children[0].strip("'\"")

        print(f"create role name: {role_name}, description: {desc_str}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/roles'
        response = self.session.post(
            url,
            json={'role_name': role_name, 'description': desc_str}
        )
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to create role {role_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _drop_role(self, command):
        role_name_tree: Tree = command['role_name']
        role_name: str = role_name_tree.children[0].strip("'\"")
        print(f"drop role name: {role_name}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/roles/{role_name}'
        response = self.session.delete(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to drop role {role_name}, code: {res_json['code']}, message: {res_json['message']}")

    def _alter_role(self, command):
        role_name_tree: Tree = command['role_name']
        role_name: str = role_name_tree.children[0].strip("'\"")
        desc_tree: Tree = command['description']
        desc_str: str = desc_tree.children[0].strip("'\"")

        print(f"alter role name: {role_name}, description: {desc_str}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/roles/{role_name}'
        response = self.session.put(
            url,
            json={'description': desc_str}
        )
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(
                f"Fail to update role {role_name} with description: {desc_str}, code: {res_json['code']}, message: {res_json['message']}")

    def _list_roles(self, command):
        print("Listing all roles")
        url = f'http://{self.host}:{self.port}/api/v1/admin/roles'
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to list roles, code: {res_json['code']}, message: {res_json['message']}")

    def _show_role(self, command):
        role_name_tree: Tree = command['role_name']
        role_name: str = role_name_tree.children[0].strip("'\"")
        print(f"show role: {role_name}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/roles/{role_name}/permission'
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to list roles, code: {res_json['code']}, message: {res_json['message']}")

    def _grant_permission(self, command):
        role_name_tree: Tree = command['role_name']
        role_name_str: str = role_name_tree.children[0].strip("'\"")
        resource_tree: Tree = command['resource']
        resource_str: str = resource_tree.children[0].strip("'\"")
        action_tree_list: list = command['actions']
        actions: list = []
        for action_tree in action_tree_list:
            action_str: str = action_tree.children[0].strip("'\"")
            actions.append(action_str)
        print(f"grant role_name: {role_name_str}, resource: {resource_str}, actions: {actions}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/roles/{role_name_str}/permission'
        response = self.session.post(
            url,
            json={'actions': actions, 'resource': resource_str}
        )
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(
                f"Fail to grant role {role_name_str} with {actions} on {resource_str}, code: {res_json['code']}, message: {res_json['message']}")

    def _revoke_permission(self, command):
        role_name_tree: Tree = command['role_name']
        role_name_str: str = role_name_tree.children[0].strip("'\"")
        resource_tree: Tree = command['resource']
        resource_str: str = resource_tree.children[0].strip("'\"")
        action_tree_list: list = command['actions']
        actions: list = []
        for action_tree in action_tree_list:
            action_str: str = action_tree.children[0].strip("'\"")
            actions.append(action_str)
        print(f"revoke role_name: {role_name_str}, resource: {resource_str}, actions: {actions}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/roles/{role_name_str}/permission'
        response = self.session.delete(
            url,
            json={'actions': actions, 'resource': resource_str}
        )
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(
                f"Fail to revoke role {role_name_str} with {actions} on {resource_str}, code: {res_json['code']}, message: {res_json['message']}")

    def _alter_user_role(self, command):
        role_name_tree: Tree = command['role_name']
        role_name_str: str = role_name_tree.children[0].strip("'\"")
        user_name_tree: Tree = command['user_name']
        user_name_str: str = user_name_tree.children[0].strip("'\"")
        print(f"alter_user_role user_name: {user_name_str}, role_name: {role_name_str}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users/{user_name_str}/role'
        response = self.session.put(
            url,
            json={'role_name': role_name_str}
        )
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(
                f"Fail to alter user: {user_name_str} to role {role_name_str}, code: {res_json['code']}, message: {res_json['message']}")

    def _show_user_permission(self, command):
        user_name_tree: Tree = command['user_name']
        user_name_str: str = user_name_tree.children[0].strip("'\"")
        print(f"show_user_permission user_name: {user_name_str}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users/{user_name_str}/permission'
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(
                f"Fail to show user: {user_name_str} permission, code: {res_json['code']}, message: {res_json['message']}")

    def _show_version(self, command):
        print("show_version")
        url = f'http://{self.host}:{self.port}/api/v1/admin/version'
        response = self.session.get(url)
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to show version, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_meta_command(self, command):
        meta_command = command['command']
        args = command.get('args', [])

        if meta_command in ['?', 'h', 'help']:
            self.show_help()
        elif meta_command in ['q', 'quit', 'exit']:
            print("Goodbye!")
        else:
            print(f"Meta command '{meta_command}' with args {args}")

    def show_help(self):
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

Meta Commands:
  \\?, \\h, \\help     Show this help
  \\q, \\quit, \\exit   Quit the CLI
        """
        print(help_text)


def main():
    import sys

    cli = AdminCLI()

    args = cli.parse_connection_args(sys.argv)
    if 'error' in args:
        print(f"Error: {args['error']}")
        return

    if 'command' in args:
        if 'password' not in args:
            print("Error: password is missing")
            return
        if cli.verify_admin(args, single_command=True):
            command: str = args['command']
            print(f"Run single command: {command}")
            cli.run_single_command(command)
    else:
        if cli.verify_admin(args, single_command=False):
            print(r"""
                ____  ___   ______________                 ___       __          _     
               / __ \/   | / ____/ ____/ /___ _      __   /   | ____/ /___ ___  (_)___ 
              / /_/ / /| |/ / __/ /_  / / __ \ | /| / /  / /| |/ __  / __ `__ \/ / __ \
             / _, _/ ___ / /_/ / __/ / / /_/ / |/ |/ /  / ___ / /_/ / / / / / / / / / /
            /_/ |_/_/  |_\____/_/   /_/\____/|__/|__/  /_/  |_\__,_/_/ /_/ /_/_/_/ /_/ 
            """)
            cli.cmdloop()


if __name__ == '__main__':
    main()
