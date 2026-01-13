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

from core.providers.base import ProviderBase
from core.types.storage import ObjectStorageType
from rag.utils.azure_sas_conn import RAGFlowAzureSasBlob
from rag.utils.azure_spn_conn import RAGFlowAzureSpnBlob
from rag.utils.encrypted_storage import create_encrypted_storage
from rag.utils.gcs_conn import RAGFlowGCS
from rag.utils.minio_conn import RAGFlowMinio
from rag.utils.opendal_conn import OpenDALStorage
from rag.utils.oss_conn import RAGFlowOSS
from rag.utils.s3_conn import RAGFlowS3

__all__ = [
    'StorageProvider'
]


def _import_legacy_storages():
    return {
        ObjectStorageType.MINIO: RAGFlowMinio,
        ObjectStorageType.AZURE_SPN: RAGFlowAzureSpnBlob,
        ObjectStorageType.AZURE_SAS: RAGFlowAzureSasBlob,
        ObjectStorageType.S3: RAGFlowS3,
        ObjectStorageType.OSS: RAGFlowOSS,
        ObjectStorageType.OPENDAL: OpenDALStorage,
        ObjectStorageType.GCS: RAGFlowGCS,
    }


class StorageFactory:
    _mapping = _import_legacy_storages()

    @classmethod
    def create(cls, storage: ObjectStorageType):
        """Create a storage instance by type"""
        try:
            return cls._mapping[storage]()
        except KeyError:
            raise ValueError(f"Unsupported storage type: {storage}")


class StorageProvider(ProviderBase):
    """
    Provides a storage connection based on active storage type.
    Supports encryption wrapper if crypto is enabled in config.
    """

    def __init__(self, config):
        super().__init__(config)
        self._storage: Optional[object] = None

    @property
    def conn(self):
        if self._storage:
            return self._storage

        # Create the storage implementation
        impl = StorageFactory.create(self._config.storage.active)

        # Wrap with encryption if enabled
        if self._config.ragflow.crypto_enabled:
            impl = create_encrypted_storage(
                impl,
                algorithm=self._config.crypto.algorithm,
                key=self._config.crypto.key,
                encryption_enabled=True,
            )

        self._storage = impl
        return impl
