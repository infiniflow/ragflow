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

import requests
from .oauth import OAuthClient, UserInfo


class GithubOAuthClient(OAuthClient):
    def __init__(self, config):
        """
        Initialize the GithubOAuthClient with the provider's configuration.
        """
        config.update({
            "authorization_url": "https://github.com/login/oauth/authorize",
            "token_url": "https://github.com/login/oauth/access_token",
            "userinfo_url": "https://api.github.com/user",
            "scope": "user:email"
        })
        super().__init__(config)


    def fetch_user_info(self, access_token, **kwargs):
        """
        Fetch github user info.
        """
        user_info = {}
        try:
            headers = {"Authorization": f"Bearer {access_token}"}
            # user info
            response = requests.get(self.userinfo_url, headers=headers, timeout=self.http_request_timeout)
            response.raise_for_status()
            user_info.update(response.json())
            # email info
            response = requests.get(self.userinfo_url+"/emails", headers=headers, timeout=self.http_request_timeout)
            response.raise_for_status()
            email_info = response.json()
            user_info["email"] = next(
                (email for email in email_info if email["primary"]), None
            )["email"]
            return self.normalize_user_info(user_info)
        except requests.exceptions.RequestException as e:
            raise ValueError(f"Failed to fetch github user info: {e}")


    def normalize_user_info(self, user_info):
        email = user_info.get("email")
        username = user_info.get("login", str(email).split("@")[0])
        nickname = user_info.get("name", username)
        avatar_url = user_info.get("avatar_url", "")
        return UserInfo(email=email, username=username, nickname=nickname, avatar_url=avatar_url)
