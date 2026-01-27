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

from lark import Transformer

GRAMMAR = r"""
start: command

command: sql_command | meta_command

sql_command: login_user
           | ping_server
           | list_services
           | show_service
           | startup_service
           | shutdown_service
           | restart_service
           | register_user
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
           | grant_admin
           | revoke_admin
           | set_variable
           | show_variable
           | list_variables
           | list_configs
           | list_environments
           | generate_key
           | list_keys
           | drop_key
           | show_current_user
           | set_default_llm
           | set_default_vlm
           | set_default_embedding
           | set_default_reranker
           | set_default_asr
           | set_default_tts
           | reset_default_llm
           | reset_default_vlm
           | reset_default_embedding
           | reset_default_reranker
           | reset_default_asr
           | reset_default_tts
           | create_model_provider
           | drop_model_provider
           | create_user_dataset_with_parser
           | create_user_dataset_with_pipeline
           | drop_user_dataset
           | list_user_datasets
           | list_user_dataset_files
           | list_user_agents
           | list_user_chats
           | create_user_chat
           | drop_user_chat
           | list_user_model_providers
           | list_user_default_models
           | parse_dataset_docs
           | parse_dataset_sync
           | parse_dataset_async
           | import_docs_into_dataset
           | search_on_datasets
           | benchmark

// meta command definition
meta_command: "\\" meta_command_name [meta_args]

meta_command_name: /[a-zA-Z?]+/
meta_args: (meta_arg)+

meta_arg: /[^\\s"']+/ | quoted_string

// command definition

LOGIN: "LOGIN"i
REGISTER: "REGISTER"i
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
ADMIN: "ADMIN"i
PASSWORD: "PASSWORD"i
DATASET: "DATASET"i
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
RESET: "RESET"i
VERSION: "VERSION"i
VAR: "VAR"i
VARS: "VARS"i
CONFIGS: "CONFIGS"i
ENVS: "ENVS"i
KEY: "KEY"i
KEYS: "KEYS"i
GENERATE: "GENERATE"i
MODEL: "MODEL"i
MODELS: "MODELS"i
PROVIDER: "PROVIDER"i
PROVIDERS: "PROVIDERS"i
DEFAULT: "DEFAULT"i
CHATS: "CHATS"i
CHAT: "CHAT"i
FILES: "FILES"i
AS: "AS"i
PARSE: "PARSE"i
IMPORT: "IMPORT"i
INTO: "INTO"i
WITH: "WITH"i
PARSER: "PARSER"i
PIPELINE: "PIPELINE"i
SEARCH: "SEARCH"i
CURRENT: "CURRENT"i
LLM: "LLM"i
VLM: "VLM"i
EMBEDDING: "EMBEDDING"i
RERANKER: "RERANKER"i
ASR: "ASR"i
TTS: "TTS"i
ASYNC: "ASYNC"i
SYNC: "SYNC"i
BENCHMARK: "BENCHMARK"i
PING: "PING"i

login_user: LOGIN USER quoted_string ";"
list_services: LIST SERVICES ";"
show_service: SHOW SERVICE NUMBER ";"
startup_service: STARTUP SERVICE NUMBER ";"
shutdown_service: SHUTDOWN SERVICE NUMBER ";"
restart_service: RESTART SERVICE NUMBER ";"

register_user: REGISTER USER quoted_string AS quoted_string PASSWORD quoted_string ";"
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

grant_permission: GRANT identifier_list ON identifier TO ROLE identifier ";"
revoke_permission: REVOKE identifier_list ON identifier FROM ROLE identifier ";"
alter_user_role: ALTER USER quoted_string SET ROLE identifier ";"
show_user_permission: SHOW USER PERMISSION quoted_string ";"

show_version: SHOW VERSION ";"

grant_admin: GRANT ADMIN quoted_string ";"
revoke_admin: REVOKE ADMIN quoted_string ";"

generate_key: GENERATE KEY FOR USER quoted_string ";"
list_keys: LIST KEYS OF quoted_string ";"
drop_key: DROP KEY quoted_string OF quoted_string ";"

set_variable: SET VAR identifier identifier ";"
show_variable: SHOW VAR identifier ";"
list_variables: LIST VARS ";"
list_configs: LIST CONFIGS ";"
list_environments: LIST ENVS ";"

benchmark: BENCHMARK NUMBER NUMBER user_statement

user_statement: ping_server
                | show_current_user
                | create_model_provider
                | drop_model_provider
                | set_default_llm
                | set_default_vlm
                | set_default_embedding
                | set_default_reranker
                | set_default_asr
                | set_default_tts
                | reset_default_llm
                | reset_default_vlm
                | reset_default_embedding
                | reset_default_reranker
                | reset_default_asr
                | reset_default_tts
                | create_user_dataset_with_parser
                | create_user_dataset_with_pipeline
                | drop_user_dataset
                | list_user_datasets
                | list_user_dataset_files
                | list_user_agents
                | list_user_chats
                | create_user_chat
                | drop_user_chat
                | list_user_model_providers
                | list_user_default_models
                | import_docs_into_dataset
                | search_on_datasets

ping_server: PING ";"
show_current_user: SHOW CURRENT USER ";"
create_model_provider: CREATE MODEL PROVIDER quoted_string quoted_string ";"
drop_model_provider: DROP MODEL PROVIDER quoted_string ";"
set_default_llm: SET DEFAULT LLM quoted_string ";"
set_default_vlm: SET DEFAULT VLM quoted_string ";"
set_default_embedding: SET DEFAULT EMBEDDING quoted_string ";"
set_default_reranker: SET DEFAULT RERANKER quoted_string ";"
set_default_asr: SET DEFAULT ASR quoted_string ";"
set_default_tts: SET DEFAULT TTS quoted_string ";"

reset_default_llm: RESET DEFAULT LLM ";"
reset_default_vlm: RESET DEFAULT VLM ";"
reset_default_embedding: RESET DEFAULT EMBEDDING ";"
reset_default_reranker: RESET DEFAULT RERANKER ";"
reset_default_asr: RESET DEFAULT ASR ";"
reset_default_tts: RESET DEFAULT TTS ";"

list_user_datasets: LIST DATASETS ";"
create_user_dataset_with_parser: CREATE DATASET quoted_string WITH EMBEDDING quoted_string PARSER quoted_string ";" 
create_user_dataset_with_pipeline: CREATE DATASET quoted_string WITH EMBEDDING quoted_string PIPELINE quoted_string ";" 
drop_user_dataset: DROP DATASET quoted_string ";"
list_user_dataset_files: LIST FILES OF DATASET quoted_string ";"
list_user_agents: LIST AGENTS ";"
list_user_chats: LIST CHATS ";"
create_user_chat: CREATE CHAT quoted_string ";"
drop_user_chat: DROP CHAT quoted_string ";"
list_user_model_providers: LIST MODEL PROVIDERS ";"
list_user_default_models: LIST DEFAULT MODELS ";"
import_docs_into_dataset: IMPORT quoted_string INTO DATASET quoted_string ";"
search_on_datasets: SEARCH quoted_string ON DATASETS quoted_string ";"

parse_dataset_docs: PARSE quoted_string OF DATASET quoted_string ";"
parse_dataset_sync: PARSE DATASET quoted_string SYNC ";"
parse_dataset_async: PARSE DATASET quoted_string ASYNC ";"

identifier_list: identifier ("," identifier)*

identifier: WORD
quoted_string: QUOTED_STRING
status: WORD

QUOTED_STRING: /'[^']+'/ | /"[^"]+"/
WORD: /[a-zA-Z0-9_\-\.]+/
NUMBER: /[0-9]+/

%import common.WS
%ignore WS
"""


class RAGFlowCLITransformer(Transformer):
    def start(self, items):
        return items[0]

    def command(self, items):
        return items[0]

    def login_user(self, items):
        email = items[2].children[0].strip("'\"")
        return {"type": "login_user", "email": email}

    def ping_server(self, items):
        return {"type": "ping_server"}

    def list_services(self, items):
        result = {"type": "list_services"}
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

    def register_user(self, items):
        user_name: str = items[2].children[0].strip("'\"")
        nickname: str = items[4].children[0].strip("'\"")
        password: str = items[6].children[0].strip("'\"")
        return {"type": "register_user", "user_name": user_name, "nickname": nickname, "password": password}

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
        return {"type": "revoke_permission", "role_name": role_name, "resource": resource, "actions": action_list}

    def alter_user_role(self, items):
        user_name = items[2]
        role_name = items[5]
        return {"type": "alter_user_role", "user_name": user_name, "role_name": role_name}

    def show_user_permission(self, items):
        user_name = items[3]
        return {"type": "show_user_permission", "user_name": user_name}

    def show_version(self, items):
        return {"type": "show_version"}

    def grant_admin(self, items):
        user_name = items[2]
        return {"type": "grant_admin", "user_name": user_name}

    def revoke_admin(self, items):
        user_name = items[2]
        return {"type": "revoke_admin", "user_name": user_name}

    def generate_key(self, items):
        user_name = items[4]
        return {"type": "generate_key", "user_name": user_name}

    def list_keys(self, items):
        user_name = items[3]
        return {"type": "list_keys", "user_name": user_name}

    def drop_key(self, items):
        key = items[2]
        user_name = items[4]
        return {"type": "drop_key", "key": key, "user_name": user_name}

    def set_variable(self, items):
        var_name = items[2]
        var_value = items[3]
        return {"type": "set_variable", "var_name": var_name, "var_value": var_value}

    def show_variable(self, items):
        var_name = items[2]
        return {"type": "show_variable", "var_name": var_name}

    def list_variables(self, items):
        return {"type": "list_variables"}

    def list_configs(self, items):
        return {"type": "list_configs"}

    def list_environments(self, items):
        return {"type": "list_environments"}

    def create_model_provider(self, items):
        provider_name = items[3].children[0].strip("'\"")
        provider_key = items[4].children[0].strip("'\"")
        return {"type": "create_model_provider", "provider_name": provider_name, "provider_key": provider_key}

    def drop_model_provider(self, items):
        provider_name = items[3].children[0].strip("'\"")
        return {"type": "drop_model_provider", "provider_name": provider_name}

    def show_current_user(self, items):
        return {"type": "show_current_user"}

    def set_default_llm(self, items):
        llm_id = items[3].children[0].strip("'\"")
        return {"type": "set_default_model", "model_type": "llm_id", "model_id": llm_id}

    def set_default_vlm(self, items):
        vlm_id = items[3].children[0].strip("'\"")
        return {"type": "set_default_model", "model_type": "img2txt_id", "model_id": vlm_id}

    def set_default_embedding(self, items):
        embedding_id = items[3].children[0].strip("'\"")
        return {"type": "set_default_model", "model_type": "embd_id", "model_id": embedding_id}

    def set_default_reranker(self, items):
        reranker_id = items[3].children[0].strip("'\"")
        return {"type": "set_default_model", "model_type": "reranker_id", "model_id": reranker_id}

    def set_default_asr(self, items):
        asr_id = items[3].children[0].strip("'\"")
        return {"type": "set_default_model", "model_type": "asr_id", "model_id": asr_id}

    def set_default_tts(self, items):
        tts_id = items[3].children[0].strip("'\"")
        return {"type": "set_default_model", "model_type": "tts_id", "model_id": tts_id}

    def reset_default_llm(self, items):
        return {"type": "reset_default_model", "model_type": "llm_id"}

    def reset_default_vlm(self, items):
        return {"type": "reset_default_model", "model_type": "img2txt_id"}

    def reset_default_embedding(self, items):
        return {"type": "reset_default_model", "model_type": "embd_id"}

    def reset_default_reranker(self, items):
        return {"type": "reset_default_model", "model_type": "reranker_id"}

    def reset_default_asr(self, items):
        return {"type": "reset_default_model", "model_type": "asr_id"}

    def reset_default_tts(self, items):
        return {"type": "reset_default_model", "model_type": "tts_id"}

    def list_user_datasets(self, items):
        return {"type": "list_user_datasets"}

    def create_user_dataset_with_parser(self, items):
        dataset_name = items[2].children[0].strip("'\"")
        embedding = items[5].children[0].strip("'\"")
        parser_type = items[7].children[0].strip("'\"")
        return {"type": "create_user_dataset", "dataset_name": dataset_name, "embedding": embedding,
                "parser_type": parser_type}

    def create_user_dataset_with_pipeline(self, items):
        dataset_name = items[2].children[0].strip("'\"")
        embedding = items[5].children[0].strip("'\"")
        pipeline = items[7].children[0].strip("'\"")
        return {"type": "create_user_dataset", "dataset_name": dataset_name, "embedding": embedding,
                "pipeline": pipeline}

    def drop_user_dataset(self, items):
        dataset_name = items[2].children[0].strip("'\"")
        return {"type": "drop_user_dataset", "dataset_name": dataset_name}

    def list_user_dataset_files(self, items):
        dataset_name = items[4].children[0].strip("'\"")
        return {"type": "list_user_dataset_files", "dataset_name": dataset_name}

    def list_user_agents(self, items):
        return {"type": "list_user_agents"}

    def list_user_chats(self, items):
        return {"type": "list_user_chats"}

    def create_user_chat(self, items):
        chat_name = items[2].children[0].strip("'\"")
        return {"type": "create_user_chat", "chat_name": chat_name}

    def drop_user_chat(self, items):
        chat_name = items[2].children[0].strip("'\"")
        return {"type": "drop_user_chat", "chat_name": chat_name}

    def list_user_model_providers(self, items):
        return {"type": "list_user_model_providers"}

    def list_user_default_models(self, items):
        return {"type": "list_user_default_models"}

    def parse_dataset_docs(self, items):
        document_list_str = items[1].children[0].strip("'\"")
        document_names = document_list_str.split(",")
        if len(document_names) == 1:
            document_names = document_names[0]
            document_names = document_names.split(" ")
        dataset_name = items[4].children[0].strip("'\"")
        return {"type": "parse_dataset_docs", "dataset_name": dataset_name, "document_names": document_names}

    def parse_dataset_sync(self, items):
        dataset_name = items[2].children[0].strip("'\"")
        return {"type": "parse_dataset", "dataset_name": dataset_name, "method": "sync"}

    def parse_dataset_async(self, items):
        dataset_name = items[2].children[0].strip("'\"")
        return {"type": "parse_dataset", "dataset_name": dataset_name, "method": "async"}

    def import_docs_into_dataset(self, items):
        document_list_str = items[1].children[0].strip("'\"")
        document_paths = document_list_str.split(",")
        if len(document_paths) == 1:
            document_paths = document_paths[0]
            document_paths = document_paths.split(" ")
        dataset_name = items[4].children[0].strip("'\"")
        return {"type": "import_docs_into_dataset", "dataset_name": dataset_name, "document_paths": document_paths}

    def search_on_datasets(self, items):
        question = items[1].children[0].strip("'\"")
        datasets_str = items[4].children[0].strip("'\"")
        datasets = datasets_str.split(",")
        if len(datasets) == 1:
            datasets = datasets[0]
            datasets = datasets.split(" ")
        return {"type": "search_on_datasets", "datasets": datasets, "question": question}

    def benchmark(self, items):
        concurrency: int = int(items[1])
        iterations: int = int(items[2])
        command = items[3].children[0]
        return {"type": "benchmark", "concurrency": concurrency, "iterations": iterations, "command": command}

    def action_list(self, items):
        return items

    def meta_command(self, items):
        command_name = str(items[0]).lower()
        args = items[1:] if len(items) > 1 else []

        # handle quoted parameter
        parsed_args = []
        for arg in args:
            if hasattr(arg, "value"):
                parsed_args.append(arg.value)
            else:
                parsed_args.append(str(arg))

        return {"type": "meta", "command": command_name, "args": parsed_args}

    def meta_command_name(self, items):
        return items[0]

    def meta_args(self, items):
        return items
