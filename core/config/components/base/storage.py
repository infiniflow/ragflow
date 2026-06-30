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

from typing import Dict, Optional

from pydantic import BaseModel, Field, field_validator, model_validator

from core.config.components.abilities.security import PasswordConfig
from core.config.utils.decrypt import decrypt_password
from core.config.types import ObjectStorageType


class MinioConfig(BaseModel):
    user: str = Field(default="rag_flow")
    password: Optional[str] = Field(default=None)
    host: str = Field(default="localhost:9000")
    port: int = Field(default=9000)
    console_port: int = Field(default=9001)
    bucket: str = Field(default=None)
    prefix_path: str = Field(default="")

    @property
    def endpoint(self):
        return f"{self.host}:{self.port}"

    @field_validator("bucket", mode="after")
    @classmethod
    def empty_to_none(cls, v):
        return v or None

    @model_validator(mode="before")
    @classmethod
    def _handle_host_uri(cls, values: dict) -> dict:
        """
        Handle host string in the format 'hostname:port' and populate host and port separately.
        """
        host = values.get("host")
        if host:
            if ":" in host:
                h, p = host.split(":", 1)
                values["host"] = h
                values["port"] = int(p)
            else:
                values["host"] = host  # keep host
        return values


class OSSConfig(BaseModel):
    access_key: str = Field(default="", description="OSS access key")
    secret_key: str = Field(default="", description="OSS secret key")
    endpoint_url: str = Field(default="", description="OSS endpoint URL")
    region: str = Field(default="", description="OSS region")
    bucket: str = Field(default="", description="OSS bucket name")


class S3Config(BaseModel):
    access_key: str = Field(default="", description="S3 access key")
    secret_key: str = Field(default="", description="S3 secret key")
    region: str = Field(default="", description="S3 region")
    endpoint_url: str = Field(default="", description="S3 endpoint URL")
    bucket: str = Field(default="", description="S3 bucket name")
    prefix_path: str = Field(default="", description="S3 prefix path")
    signature_version: str = Field(default="v4", description="S3 signature version")
    addressing_style: str = Field(default="path", description="S3 addressing style")


class GCSConfig(BaseModel):
    bucket: str = Field(default="bridgtl-edm-d-bucket-ragflow", description="GCS bucket name")


class AzureSASConfig(BaseModel):
    auth_type: str = Field(default="sas")
    container_url: str = Field(default="")
    sas_token: str = Field(default="")


class AzureSPNConfig(BaseModel):
    auth_type: str = Field(default="spn")
    account_url: str = Field(default="")
    client_id: str = Field(default="")
    secret: str = Field(default="")
    tenant_id: str = Field(default="")
    container_name: str = Field(default="")


class OpendalConfig(BaseModel):
    scheme: str = Field(default="mysql")  # s3, oss, azure, etc.
    config: Dict[str, str] = Field(default_factory=lambda: {"oss_table": "opendal_storage"})


class StorageConfig(BaseModel):
    active: ObjectStorageType = Field(default=ObjectStorageType.MINIO)
    minio: MinioConfig = Field(default_factory=MinioConfig)
    s3: S3Config = Field(default_factory=S3Config)
    gcs: GCSConfig = Field(default_factory=GCSConfig)
    oss: OSSConfig = Field(default_factory=OSSConfig)
    azure_sas: AzureSASConfig = Field(default_factory=AzureSASConfig)
    azure_spn: AzureSPNConfig = Field(default_factory=AzureSPNConfig)
    opendal: OpendalConfig = Field(default_factory=OpendalConfig)

    @property
    def current(self) -> BaseModel:
        name = self.active.value
        try:
            return getattr(self, name)
        except AttributeError:
            raise ValueError(f"Storage '{name}' is not configured")

    def decrypt_password(self, password_conf: PasswordConfig) -> None:
        """Decrypt password of the currently active cache if needed."""
        conf = self.current
        if self.active == ObjectStorageType.MINIO:
            conf.password = decrypt_password(conf.password, password_conf)
