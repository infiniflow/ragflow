#
#  Copyright 2019 The FATE Authors. All Rights Reserved.
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
import socket
from pathlib import Path
from web_server import utils
from .db_models import DB, ServiceRegistryInfo, ServerRegistryInfo
from .reload_config_base import ReloadConfigBase


class ServiceRegistry(ReloadConfigBase):
    @classmethod
    @DB.connection_context()
    def load_service(cls, **kwargs) -> [ServiceRegistryInfo]:
        service_registry_list = ServiceRegistryInfo.query(**kwargs)
        return [service for service in service_registry_list]

    @classmethod
    @DB.connection_context()
    def save_service_info(cls, server_name, service_name, uri, method="POST", server_info=None, params=None, data=None, headers=None, protocol="http"):
        if not server_info:
            server_list = ServerRegistry.query_server_info_from_db(server_name=server_name)
            if not server_list:
                raise Exception(f"no found server {server_name}")
            server_info = server_list[0]
            url = f"{server_info.f_protocol}://{server_info.f_host}:{server_info.f_port}{uri}"
        else:
            url = f"{server_info.get('protocol', protocol)}://{server_info.get('host')}:{server_info.get('port')}{uri}"
        service_info = {
            "f_server_name": server_name,
            "f_service_name": service_name,
            "f_url": url,
            "f_method": method,
            "f_params": params if params else {},
            "f_data": data if data else {},
            "f_headers": headers if headers else {}
        }
        entity_model, status = ServiceRegistryInfo.get_or_create(
            f_server_name=server_name,
            f_service_name=service_name,
            defaults=service_info)
        if status is False:
            for key in service_info:
                setattr(entity_model, key, service_info[key])
            entity_model.save(force_insert=False)


class ServerRegistry(ReloadConfigBase):
    FATEBOARD = None
    FATE_ON_STANDALONE = None
    FATE_ON_EGGROLL = None
    FATE_ON_SPARK = None
    MODEL_STORE_ADDRESS = None
    SERVINGS = None
    FATEMANAGER = None
    STUDIO = None

    @classmethod
    def load(cls):
        cls.load_server_info_from_conf()
        cls.load_server_info_from_db()

    @classmethod
    def load_server_info_from_conf(cls):
        path = Path(utils.file_utils.get_project_base_directory()) / 'conf' / utils.SERVICE_CONF
        conf = utils.file_utils.load_yaml_conf(path)
        if not isinstance(conf, dict):
            raise ValueError('invalid config file')

        local_path = path.with_name(f'local.{utils.SERVICE_CONF}')
        if local_path.exists():
            local_conf = utils.file_utils.load_yaml_conf(local_path)
            if not isinstance(local_conf, dict):
                raise ValueError('invalid local config file')
            conf.update(local_conf)
        for k, v in conf.items():
            if isinstance(v, dict):
                setattr(cls, k.upper(), v)

    @classmethod
    def register(cls, server_name, server_info):
        cls.save_server_info_to_db(server_name, server_info.get("host"), server_info.get("port"), protocol=server_info.get("protocol", "http"))
        setattr(cls, server_name, server_info)

    @classmethod
    def save(cls, service_config):
        update_server = {}
        for server_name, server_info in service_config.items():
            cls.parameter_check(server_info)
            api_info = server_info.pop("api", {})
            for service_name, info in api_info.items():
                ServiceRegistry.save_service_info(server_name, service_name, uri=info.get('uri'), method=info.get('method', 'POST'), server_info=server_info)
            cls.save_server_info_to_db(server_name, server_info.get("host"), server_info.get("port"), protocol="http")
            setattr(cls, server_name.upper(), server_info)
        return update_server

    @classmethod
    def parameter_check(cls, service_info):
        if "host" in service_info and "port" in service_info:
            cls.connection_test(service_info.get("host"), service_info.get("port"))

    @classmethod
    def connection_test(cls, ip, port):
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        result = s.connect_ex((ip, port))
        if result != 0:
            raise ConnectionRefusedError(f"connection refused: host {ip}, port {port}")

    @classmethod
    def query(cls, service_name, default=None):
        service_info = getattr(cls, service_name, default)
        if not service_info:
            service_info = utils.get_base_config(service_name, default)
        return service_info

    @classmethod
    @DB.connection_context()
    def query_server_info_from_db(cls, server_name=None) -> [ServerRegistryInfo]:
        if server_name:
            server_list = ServerRegistryInfo.select().where(ServerRegistryInfo.f_server_name==server_name.upper())
        else:
            server_list = ServerRegistryInfo.select()
        return [server for server in server_list]

    @classmethod
    @DB.connection_context()
    def load_server_info_from_db(cls):
        for server in cls.query_server_info_from_db():
            server_info = {
                "host": server.f_host,
                "port": server.f_port,
                "protocol": server.f_protocol
            }
            setattr(cls, server.f_server_name.upper(), server_info)


    @classmethod
    @DB.connection_context()
    def save_server_info_to_db(cls, server_name, host, port, protocol="http"):
        server_info = {
            "f_server_name": server_name,
            "f_host": host,
            "f_port": port,
            "f_protocol": protocol
        }
        entity_model, status = ServerRegistryInfo.get_or_create(
            f_server_name=server_name,
            defaults=server_info)
        if status is False:
            for key in server_info:
                setattr(entity_model, key, server_info[key])
            entity_model.save(force_insert=False)