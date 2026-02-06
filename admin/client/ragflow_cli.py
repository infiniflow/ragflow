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
from cmd import Cmd
from typing import Any, Dict, List

import requests
import warnings
from Cryptodome.Cipher import PKCS1_v1_5 as Cipher_pkcs1_v1_5
from Cryptodome.PublicKey import RSA
from lark import Lark, Tree
from parser import GRAMMAR, RAGFlowCLITransformer
from http_client import HttpClient
from ragflow_client import RAGFlowClient, run_command
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





class RAGFlowCLI(Cmd):
    def __init__(self):
        super().__init__()
        self.parser = Lark(GRAMMAR, start="start", parser="lalr", transformer=RAGFlowCLITransformer())
        self.command_history = []
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

    def verify_auth(self, arguments: dict, single_command: bool, auth: bool):
        server_type = arguments.get("type", "admin")
        http_client = HttpClient(arguments["host"], arguments["port"])
        if not auth:
            self.ragflow_client = RAGFlowClient(http_client, server_type)
            return True

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

    def run_interactive(self, args):
        if self.verify_auth(args, single_command=False, auth=args["auth"]):
            print(r"""
                ____  ___   ______________                 ________    ____
               / __ \/   | / ____/ ____/ /___ _      __   / ____/ /   /  _/
              / /_/ / /| |/ / __/ /_  / / __ \ | /| / /  / /   / /    / /  
             / _, _/ ___ / /_/ / __/ / / /_/ / |/ |/ /  / /___/ /____/ /   
            /_/ |_/_/  |_\____/_/   /_/\____/|__/|__/   \____/_____/___/   
            """)
            self.cmdloop()

        print("RAGFlow command line interface - Type '\\?' for help, '\\q' to quit")

    def run_single_command(self, args):
        if self.verify_auth(args, single_command=True, auth=args["auth"]):
            command = args["command"]
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

            if remaining_args:
                if remaining_args[0] == "command":
                    command_str = ' '.join(remaining_args[1:]) + ';'
                    auth = True
                    if remaining_args[1] == "register":
                        auth = False
                    else:
                        if username is None:
                            print("Error: username (-u) is required in user mode")
                            return {"error": "Username required"}
                    return {
                        "host": parsed_args.host,
                        "port": parsed_args.port,
                        "password": parsed_args.password,
                        "type": parsed_args.type,
                        "username": username,
                        "command": command_str,
                        "auth": auth
                    }
                else:
                    return {"error": "Invalid command"}
            else:
                auth = True
                if username is None:
                    auth = False
                return {
                    "host": parsed_args.host,
                    "port": parsed_args.port,
                    "type": parsed_args.type,
                    "username": username,
                    "auth": auth
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
        run_command(self.ragflow_client, command_dict)

def main():

    cli = RAGFlowCLI()

    args = cli.parse_connection_args(sys.argv)
    if "error" in args:
        print("Error: Invalid connection arguments")
        return

    if "command" in args:
        # single command mode
        # for user mode, api key or password is ok
        # for admin mode, only password
        if "password" not in args:
            print("Error: password is missing")
            return

        cli.run_single_command(args)
    else:
        cli.run_interactive(args)


if __name__ == "__main__":
    main()
