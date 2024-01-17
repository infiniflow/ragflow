#
#  Copyright 2021 The RAG Flow Authors. All Rights Reserved.
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
import abc
import json
import time
from functools import wraps
from shortuuid import ShortUUID

from web_server.versions import get_rag_version

from web_server.errors.error_services import *
from web_server.settings import (
    GRPC_PORT, HOST, HTTP_PORT,
    RANDOM_INSTANCE_ID,  stat_logger,
)


instance_id = ShortUUID().random(length=8) if RANDOM_INSTANCE_ID else f'flow-{HOST}-{HTTP_PORT}'
server_instance = (
    f'{HOST}:{GRPC_PORT}',
    json.dumps({
        'instance_id': instance_id,
        'timestamp': round(time.time() * 1000),
        'version': get_rag_version() or '',
        'host': HOST,
        'grpc_port': GRPC_PORT,
        'http_port': HTTP_PORT,
    }),
)


def check_service_supported(method):
    """Decorator to check if `service_name` is supported.
    The attribute `supported_services` MUST be defined in class.
    The first and second arguments of `method` MUST be `self` and `service_name`.

    :param Callable method: The class method.
    :return: The inner wrapper function.
    :rtype: Callable
    """
    @wraps(method)
    def magic(self, service_name, *args, **kwargs):
        if service_name not in self.supported_services:
            raise ServiceNotSupported(service_name=service_name)
        return method(self, service_name, *args, **kwargs)
    return magic


class ServicesDB(abc.ABC):
    """Database for storage service urls.
    Abstract base class for the real backends.

    """
    @property
    @abc.abstractmethod
    def supported_services(self):
        """The names of supported services.
        The returned list SHOULD contain `ragflow` (model download) and `servings` (RAG-Serving).

        :return: The service names.
        :rtype: list
        """
        pass

    @abc.abstractmethod
    def _get_serving(self):
        pass

    def get_serving(self):

        try:
            return self._get_serving()
        except ServicesError as e:
            stat_logger.exception(e)
            return []

    @abc.abstractmethod
    def _insert(self, service_name, service_url, value=''):
        pass

    @check_service_supported
    def insert(self, service_name, service_url, value=''):
        """Insert a service url to database.

        :param str service_name: The service name.
        :param str service_url: The service url.
        :return: None
        """
        try:
            self._insert(service_name, service_url, value)
        except ServicesError as e:
            stat_logger.exception(e)

    @abc.abstractmethod
    def _delete(self, service_name, service_url):
        pass

    @check_service_supported
    def delete(self, service_name, service_url):
        """Delete a service url from database.

        :param str service_name: The service name.
        :param str service_url: The service url.
        :return: None
        """
        try:
            self._delete(service_name, service_url)
        except ServicesError as e:
            stat_logger.exception(e)

    def register_flow(self):
        """Call `self.insert` for insert the flow server address to databae.

        :return: None
        """
        self.insert('flow-server', *server_instance)

    def unregister_flow(self):
        """Call `self.delete` for delete the flow server address from databae.

        :return: None
        """
        self.delete('flow-server', server_instance[0])

    @abc.abstractmethod
    def _get_urls(self, service_name, with_values=False):
        pass

    @check_service_supported
    def get_urls(self, service_name, with_values=False):
        """Query service urls from database. The urls may belong to other nodes.
        Currently, only `ragflow` (model download) urls and `servings` (RAG-Serving) urls are supported.
        `ragflow` is a url containing scheme, host, port and path,
        while `servings` only contains host and port.

        :param str service_name: The service name.
        :return: The service urls.
        :rtype: list
        """
        try:
            return self._get_urls(service_name, with_values)
        except ServicesError as e:
            stat_logger.exception(e)
            return []
