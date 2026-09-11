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

from pydantic import BaseModel, Field


class OAuth2Config(BaseModel):
    display_name: str = "OAuth2"
    client_id: Optional[str] = None
    client_secret: Optional[str] = None
    authorization_url: Optional[str] = None
    token_url: Optional[str] = None
    userinfo_url: Optional[str] = None
    redirect_uri: Optional[str] = None


class OIDCConfig(BaseModel):
    display_name: str = "OIDC"
    client_id: Optional[str] = None
    client_secret: Optional[str] = None
    issuer: Optional[str] = None
    scope: str = "openid email profile"
    redirect_uri: Optional[str] = None


class GithubConfig(BaseModel):
    type: str = "github"
    icon: str = "github"
    display_name: str = "Github"
    client_id: Optional[str] = None
    client_secret: Optional[str] = None
    redirect_uri: Optional[str] = None


class FeishuConfig(BaseModel):
    app_access_token_url: str = ""
    user_access_token_url: str = ""
    app_id: str = ""
    app_secret: str = ""
    grant_type: str = ""


class OAuthConfig(BaseModel):
    oauth2: OAuth2Config = Field(default_factory=OAuth2Config)
    oidc: OIDCConfig = Field(default_factory=OIDCConfig)
    github: GithubConfig = Field(default_factory=GithubConfig)
    feishu: FeishuConfig = Field(default_factory=FeishuConfig)