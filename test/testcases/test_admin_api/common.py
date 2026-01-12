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

import os
import urllib.parse
from typing import Any

import requests
from configs import VERSION

# Admin API runs on port 9381
ADMIN_HOST_ADDRESS = os.getenv("ADMIN_HOST_ADDRESS", "http://127.0.0.1:9381")

UNAUTHORIZED_ERROR_MESSAGE = "<!doctype html>\n<html lang=en>\n<title>401 unauthorized</title>\n<h1>unauthorized</h1>\n<p>the server could not verify that you are authorized to access the url requested. you either supplied the wrong credentials (e.g. a bad password), or your browser doesn&#39;t understand how to supply the credentials required.</p>\n"


def generate_user_api_key(session: requests.Session, user_name: str) -> dict[str, Any]:
    """Helper function to generate API key for a user

    Returns:
        Dict containing the full API response with keys: code, message, data
    """
    url: str = f"{ADMIN_HOST_ADDRESS}/api/{VERSION}/admin/users/{user_name}/new_token"
    response: requests.Response = session.post(url)

    # Some error responses (e.g., 401) may return HTML instead of JSON.
    try:
        res_json: dict[str, Any] = response.json()
    except requests.exceptions.JSONDecodeError:
        return {
            "code": response.status_code,
            "message": response.text,
            "data": None,
        }
    return res_json


def get_user_api_key(session: requests.Session, username: str) -> dict[str, Any]:
    """Helper function to get API keys for a user

    Returns:
        Dict containing the full API response with keys: code, message, data
    """
    url: str = f"{ADMIN_HOST_ADDRESS}/api/{VERSION}/admin/users/{username}/token_list"
    response: requests.Response = session.get(url)

    try:
        res_json: dict[str, Any] = response.json()
    except requests.exceptions.JSONDecodeError:
        return {
            "code": response.status_code,
            "message": response.text,
            "data": None,
        }
    return res_json


def delete_user_api_key(session: requests.Session, username: str, token: str) -> dict[str, Any]:
    """Helper function to delete an API key for a user

    Returns:
        Dict containing the full API response with keys: code, message, data
    """
    # URL encode the token to handle special characters
    encoded_token: str = urllib.parse.quote(token, safe="")
    url: str = f"{ADMIN_HOST_ADDRESS}/api/{VERSION}/admin/users/{username}/token/{encoded_token}"
    response: requests.Response = session.delete(url)

    try:
        res_json: dict[str, Any] = response.json()
    except requests.exceptions.JSONDecodeError:
        return {
            "code": response.status_code,
            "message": response.text,
            "data": None,
        }
    return res_json
