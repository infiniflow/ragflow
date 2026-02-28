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

from pydantic import BaseModel, Field, field_validator


class PermissionConfig(BaseModel):
    switch: bool = Field(default=False)
    component: bool = Field(default=False)
    dataset: bool = Field(default=False)


class AuthClientConfig(BaseModel):
    switch: bool = Field(default=False)
    http_app_key: Optional[str] = None
    http_secret_key: Optional[str] = None


class AuthSiteConfig(BaseModel):
    switch: bool = Field(default=False)


class AuthenticationConfig(BaseModel):
    client: AuthClientConfig = Field(default_factory=AuthClientConfig)
    site: AuthSiteConfig = Field(default_factory=AuthSiteConfig)


class PasswordConfig(BaseModel):
    encrypt_enabled: bool = Field(default=False, description="Enable password decryption")
    encrypt_module: Optional[str] = Field(default=None)
    private_key: Optional[str] = Field(default=None)

    @field_validator("encrypt_enabled", mode="before")
    @classmethod
    def parse_bool(cls, v):
        if isinstance(v, str):
            return v.lower() in {"1", "true", "yes"}
        return bool(v)


class SecurityConfig(BaseModel):
    password: PasswordConfig = Field(default_factory=PasswordConfig)
    permission: PermissionConfig = Field(default_factory=PermissionConfig)
    authentication: AuthenticationConfig = Field(default_factory=AuthenticationConfig)
