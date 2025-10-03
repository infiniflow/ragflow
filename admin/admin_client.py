import argparse
import base64

from Cryptodome.PublicKey import RSA
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from typing import Dict, List, Any
from lark import Lark, Transformer, Tree
import requests
from requests.auth import HTTPBasicAuth
from api.common.base64 import encode_to_base64

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
        return {"type": "show_user", "username": user_name}

    def drop_user(self, items):
        user_name = items[2]
        return {"type": "drop_user", "username": user_name}

    def alter_user(self, items):
        user_name = items[3]
        new_password = items[4]
        return {"type": "alter_user", "username": user_name, "password": new_password}

    def create_user(self, items):
        user_name = items[2]
        password = items[3]
        return {"type": "create_user", "username": user_name, "password": password, "role": "user"}

    def activate_user(self, items):
        user_name = items[3]
        activate_status = items[4]
        return {"type": "activate_user", "activate_status": activate_status, "username": user_name}

    def list_datasets(self, items):
        user_name = items[3]
        return {"type": "list_datasets", "username": user_name}

    def list_agents(self, items):
        user_name = items[3]
        return {"type": "list_agents", "username": user_name}

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


class AdminCommandParser:
    def __init__(self):
        self.parser = Lark(GRAMMAR, start='start', parser='lalr', transformer=AdminTransformer())
        self.command_history = []

    def parse_command(self, command_str: str) -> Dict[str, Any]:
        if not command_str.strip():
            return {'type': 'empty'}

        self.command_history.append(command_str)

        try:
            result = self.parser.parse(command_str)
            return result
        except Exception as e:
            return {'type': 'error', 'message': f'Parse error: {str(e)}'}


class AdminCLI:
    def __init__(self):
        self.parser = AdminCommandParser()
        self.is_interactive = False
        self.admin_account = "admin@ragflow.io"
        self.admin_password: str = "admin"
        self.host: str = ""
        self.port: int = 0

    def verify_admin(self, args):

        conn_info = self._parse_connection_args(args)
        if 'error' in conn_info:
            print(f"Error: {conn_info['error']}")
            return

        self.host = conn_info['host']
        self.port = conn_info['port']
        print(f"Attempt to access ip: {self.host}, port: {self.port}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/auth'

        try_count = 0
        while True:
            try_count += 1
            if try_count > 3:
                return False

            admin_passwd = input(f"password for {self.admin_account}: ").strip()
            try:
                self.admin_password = encode_to_base64(admin_passwd)
                response = requests.get(url, auth=HTTPBasicAuth(self.admin_account, self.admin_password))
                if response.status_code == 200:
                    res_json = response.json()
                    error_code = res_json.get('code', -1)
                    if error_code == 0:
                        print("Authentication successful.")
                        return True
                    else:
                        error_message = res_json.get('message', 'Unknown error')
                        print(f"Authentication failed: {error_message}, try again")
                        continue
                else:
                    print(f"Bad response，status: {response.status_code}, try again")
            except Exception:
                print(f"Can't access {self.host}, port: {self.port}")

    def _print_table_simple(self, data):
        if not data:
            print("No data to print")
            return
        if isinstance(data, dict):
            # handle single row data
            data = [data]

        columns = list(data[0].keys())
        col_widths = {}

        for col in columns:
            max_width = len(str(col))
            for item in data:
                value_len = len(str(item.get(col, '')))
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
                if len(value) > col_widths[col]:
                    value = value[:col_widths[col] - 3] + "..."
                row += f" {value:<{col_widths[col]}} |"
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
                result = self.parser.parse_command(command)
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

    def run_single_command(self, args):
        conn_info = self._parse_connection_args(args)
        if 'error' in conn_info:
            print(f"Error: {conn_info['error']}")
            return

    def _parse_connection_args(self, args: List[str]) -> Dict[str, Any]:
        parser = argparse.ArgumentParser(description='Admin CLI Client', add_help=False)
        parser.add_argument('-h', '--host', default='localhost', help='Admin service host')
        parser.add_argument('-p', '--port', type=int, default=8080, help='Admin service port')

        try:
            parsed_args, remaining_args = parser.parse_known_args(args)
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
            case 'meta':
                self._handle_meta_command(command_dict)
            case _:
                print(f"Command '{command_type}' would be executed with API")

    def _handle_list_services(self, command):
        print("Listing all services")

        url = f'http://{self.host}:{self.port}/api/v1/admin/services'
        response = requests.get(url, auth=HTTPBasicAuth(self.admin_account, self.admin_password))
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to get all users, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_show_service(self, command):
        service_id: int = command['number']
        print(f"Showing service: {service_id}")

        url = f'http://{self.host}:{self.port}/api/v1/admin/services/{service_id}'
        response = requests.get(url, auth=HTTPBasicAuth(self.admin_account, self.admin_password))
        res_json = response.json()
        if response.status_code == 200:
            res_data = res_json['data']
            if res_data['alive']:
                print(f"Service {res_data['service_name']} is alive. Detail:")
                if isinstance(res_data['message'], str):
                    print(res_data['message'])
                else:
                    self._print_table_simple(res_data['message'])
            else:
                print(f"Service {res_data['service_name']} is down. Detail: {res_data['message']}")
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
        response = requests.get(url, auth=HTTPBasicAuth(self.admin_account, self.admin_password))
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to get all users, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_show_user(self, command):
        username_tree: Tree = command['username']
        username: str = username_tree.children[0].strip("'\"")
        print(f"Showing user: {username}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users/{username}'
        response = requests.get(url, auth=HTTPBasicAuth(self.admin_account, self.admin_password))
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to get user {username}, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_drop_user(self, command):
        username_tree: Tree = command['username']
        username: str = username_tree.children[0].strip("'\"")
        print(f"Drop user: {username}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users/{username}'
        response = requests.delete(url, auth=HTTPBasicAuth(self.admin_account, self.admin_password))
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(f"Fail to drop user, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_alter_user(self, command):
        username_tree: Tree = command['username']
        username: str = username_tree.children[0].strip("'\"")
        password_tree: Tree = command['password']
        password: str = password_tree.children[0].strip("'\"")
        print(f"Alter user: {username}, password: {password}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users/{username}/password'
        response = requests.put(url, auth=HTTPBasicAuth(self.admin_account, self.admin_password),
                                json={'new_password': encrypt(password)})
        res_json = response.json()
        if response.status_code == 200:
            print(res_json["message"])
        else:
            print(f"Fail to alter password, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_create_user(self, command):
        username_tree: Tree = command['username']
        username: str = username_tree.children[0].strip("'\"")
        password_tree: Tree = command['password']
        password: str = password_tree.children[0].strip("'\"")
        role: str = command['role']
        print(f"Create user: {username}, password: {password}, role: {role}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users'
        response = requests.post(
            url,
            auth=HTTPBasicAuth(self.admin_account, self.admin_password),
            json={'username': username, 'password': encrypt(password), 'role': role}
        )
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to create user {username}, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_activate_user(self, command):
        username_tree: Tree = command['username']
        username: str = username_tree.children[0].strip("'\"")
        activate_tree: Tree = command['activate_status']
        activate_status: str = activate_tree.children[0].strip("'\"")
        if activate_status.lower() in ['on', 'off']:
            print(f"Alter user {username} activate status, turn {activate_status.lower()}.")
            url = f'http://{self.host}:{self.port}/api/v1/admin/users/{username}/activate'
            response = requests.put(url, auth=HTTPBasicAuth(self.admin_account, self.admin_password),
                                    json={'activate_status': activate_status})
            res_json = response.json()
            if response.status_code == 200:
                print(res_json["message"])
            else:
                print(f"Fail to alter activate status, code: {res_json['code']}, message: {res_json['message']}")
        else:
            print(f"Unknown activate status: {activate_status}.")

    def _handle_list_datasets(self, command):
        username_tree: Tree = command['username']
        username: str = username_tree.children[0].strip("'\"")
        print(f"Listing all datasets of user: {username}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users/{username}/datasets'
        response = requests.get(url, auth=HTTPBasicAuth(self.admin_account, self.admin_password))
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to get all datasets of {username}, code: {res_json['code']}, message: {res_json['message']}")

    def _handle_list_agents(self, command):
        username_tree: Tree = command['username']
        username: str = username_tree.children[0].strip("'\"")
        print(f"Listing all agents of user: {username}")
        url = f'http://{self.host}:{self.port}/api/v1/admin/users/{username}/agents'
        response = requests.get(url, auth=HTTPBasicAuth(self.admin_account, self.admin_password))
        res_json = response.json()
        if response.status_code == 200:
            self._print_table_simple(res_json['data'])
        else:
            print(f"Fail to get all agents of {username}, code: {res_json['code']}, message: {res_json['message']}")

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

    if len(sys.argv) == 1 or (len(sys.argv) > 1 and sys.argv[1] == '-'):
        print(r"""
            ____  ___   ______________                 ___       __          _     
           / __ \/   | / ____/ ____/ /___ _      __   /   | ____/ /___ ___  (_)___ 
          / /_/ / /| |/ / __/ /_  / / __ \ | /| / /  / /| |/ __  / __ `__ \/ / __ \
         / _, _/ ___ / /_/ / __/ / / /_/ / |/ |/ /  / ___ / /_/ / / / / / / / / / /
        /_/ |_/_/  |_\____/_/   /_/\____/|__/|__/  /_/  |_\__,_/_/ /_/ /_/_/_/ /_/ 
        """)
        if cli.verify_admin(sys.argv):
            cli.run_interactive()
    else:
        if cli.verify_admin(sys.argv):
            cli.run_interactive()
            # cli.run_single_command(sys.argv[1:])


if __name__ == '__main__':
    main()
