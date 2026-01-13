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

from typing import Optional

from core.config.utils.decrypt import decrypt_password, DecryptPasswordError
from core.providers.base import ProviderBase
from rag.utils.redis_conn import RedisDB


class CacheProvider(ProviderBase):
    """
    Cache provider.

    Responsible for initializing and caching the active cache backend
    (currently Redis).
    """
    _client: Optional[RedisDB] = None

    def __init__(self, config):
        super().__init__(config)
        self._conn: Optional[object] = None
        self._client = None

    @property
    def conn(self):
        """
        Return an initialized cache client (singleton).

        The password is expected to be already decrypted in AppConfig.
        """
        if self._client:
            return self._client

        self._client = RedisDB()  # Initialize with the config dict
        return self._client

    def get_redis_config(self) -> dict:
        """
        Retrieve Redis configuration, handling decryption of password if needed.
        """
        conf = self._config.cache.current
        config_dict = {
            "host": conf.host,
            "port": conf.port,
            "password": conf.password or None,
            "db": conf.db,
        }

        # Attempt password decryption
        try:
            config_dict["password"] = decrypt_password(config_dict["password"], self._config.security.password)
        except DecryptPasswordError:
            pass  # If decryption fails, we just leave the password as-is

        return config_dict
