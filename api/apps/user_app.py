#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import json
import logging
import string
import os
import re
import secrets
import time
from datetime import datetime
from typing import Any, Dict, List, Optional, Match

from flask import redirect, request, session, make_response, Response
from flask_login import current_user, login_required, login_user, logout_user
from werkzeug.security import check_password_hash, generate_password_hash

from api.apps.auth import get_auth_client
from api.db import FileType, UserTenantRole
from api.db.db_models import TenantLLM, User
from api.db.services.file_service import FileService
from api.db.services.llm_service import get_init_tenant_llm
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.user_service import TenantService, UserService, UserTenantService
from common.time_utils import current_timestamp, datetime_format, get_format_time
from common.misc_utils import download_img, get_uuid
from common.constants import RetCode
from common.connection_utils import construct_response
from api.utils.api_utils import (
    get_data_error_result,
    get_json_result,
    server_error_response,
    validate_request,
)
from api.utils.crypt import decrypt
from rag.utils.redis_conn import REDIS_CONN
from api.apps import smtp_mail_server
from api.utils.web_utils import (
    send_email_html,
    OTP_LENGTH,
    OTP_TTL_SECONDS,
    ATTEMPT_LIMIT,
    ATTEMPT_LOCK_SECONDS,
    RESEND_COOLDOWN_SECONDS,
    otp_keys,
    hash_code,
    captcha_key,
)
from common import settings


def sanitize_nickname(nickname: str) -> str:
    """Sanitize nickname to prevent XSS and control character injection.
    
    Args:
        nickname: Raw nickname string to sanitize.
        
    Returns:
        Sanitized nickname with XSS payloads and control characters removed.
    """
    if not nickname:
        return nickname
    
    # Remove control characters (null bytes, carriage returns, line feeds)
    nickname = nickname.replace("\x00", "")  # Null byte
    nickname = nickname.replace("\r", "")    # Carriage return
    nickname = nickname.replace("\n", "")    # Line feed
    
    # Remove XSS payloads - script tags (case insensitive, handles nested/escaped tags)
    # First remove complete script tags with content
    nickname = re.sub(r"<script[^>]*>.*?</script>", "", nickname, flags=re.IGNORECASE | re.DOTALL)
    nickname = re.sub(r"<iframe[^>]*>.*?</iframe>", "", nickname, flags=re.IGNORECASE | re.DOTALL)
    
    # Remove javascript: protocol (case insensitive)
    nickname = re.sub(r"javascript:", "", nickname, flags=re.IGNORECASE)
    
    # Remove img tags with event handlers (onerror, onclick, etc.)
    nickname = re.sub(r"<img[^>]*on\w+\s*=[^>]*>", "", nickname, flags=re.IGNORECASE)
    
    # Remove any remaining script/iframe opening and closing tags (handles cases like <<SCRIPT>)
    # Match <script> or </script> (case insensitive, handles variations)
    nickname = re.sub(r"<+/?\s*script[^>]*>", "", nickname, flags=re.IGNORECASE)
    nickname = re.sub(r"<+/?\s*iframe[^>]*>", "", nickname, flags=re.IGNORECASE)
    
    return nickname


@manager.route("/login", methods=["POST", "GET"])  # noqa: F821
def login():
    """
    User login endpoint.
    ---
    tags:
      - User
    parameters:
      - in: body
        name: body
        description: Login credentials.
        required: true
        schema:
          type: object
          properties:
            email:
              type: string
              description: User email.
            password:
              type: string
              description: User password.
    responses:
      200:
        description: Login successful.
        schema:
          type: object
      401:
        description: Authentication failed.
        schema:
          type: object
    """
    if not request.json:
        return get_json_result(data=False, code=RetCode.AUTHENTICATION_ERROR, message="Unauthorized!")

    email = request.json.get("email", "")
    users = UserService.query(email=email)
    if not users:
        return get_json_result(
            data=False,
            code=RetCode.AUTHENTICATION_ERROR,
            message=f"Email: {email} is not registered!",
        )

    password = request.json.get("password")
    try:
        password = decrypt(password)
    except BaseException:
        return get_json_result(data=False, code=RetCode.SERVER_ERROR, message="Fail to crypt password")

    user = UserService.query_user(email, password)

    if user and hasattr(user, 'is_active') and user.is_active == "0":
        return get_json_result(
            data=False,
            code=RetCode.FORBIDDEN,
            message="This account has been disabled, please contact the administrator!",
        )
    elif user:
        response_data = user.to_json()
        user.access_token = get_uuid()
        login_user(user)
        user.update_time = (current_timestamp(),)
        user.update_date = (datetime_format(datetime.now()),)
        user.save()
        msg = "Welcome back!"
        return construct_response(data=response_data, auth=user.get_id(), message=msg)
    else:
        return get_json_result(
            data=False,
            code=RetCode.AUTHENTICATION_ERROR,
            message="Email and password do not match!",
        )


@manager.route("/login/channels", methods=["GET"])  # noqa: F821
def get_login_channels():
    """
    Get all supported authentication channels.
    """
    try:
        channels = []
        for channel, config in settings.OAUTH_CONFIG.items():
            channels.append(
                {
                    "channel": channel,
                    "display_name": config.get("display_name", channel.title()),
                    "icon": config.get("icon", "sso"),
                }
            )
        return get_json_result(data=channels)
    except Exception as e:
        logging.exception(e)
        return get_json_result(data=[], message=f"Load channels failure, error: {str(e)}", code=RetCode.EXCEPTION_ERROR)


@manager.route("/login/<channel>", methods=["GET"])  # noqa: F821
def oauth_login(channel):
    channel_config = settings.OAUTH_CONFIG.get(channel)
    if not channel_config:
        raise ValueError(f"Invalid channel name: {channel}")
    auth_cli = get_auth_client(channel_config)

    state = get_uuid()
    session["oauth_state"] = state
    auth_url = auth_cli.get_authorization_url(state)
    return redirect(auth_url)


@manager.route("/oauth/callback/<channel>", methods=["GET"])  # noqa: F821
def oauth_callback(channel):
    """
    Handle the OAuth/OIDC callback for various channels dynamically.
    """
    try:
        channel_config = settings.OAUTH_CONFIG.get(channel)
        if not channel_config:
            raise ValueError(f"Invalid channel name: {channel}")
        auth_cli = get_auth_client(channel_config)

        # Check the state
        state = request.args.get("state")
        if not state or state != session.get("oauth_state"):
            return redirect("/?error=invalid_state")
        session.pop("oauth_state", None)

        # Obtain the authorization code
        code = request.args.get("code")
        if not code:
            return redirect("/?error=missing_code")

        # Exchange authorization code for access token
        token_info = auth_cli.exchange_code_for_token(code)
        access_token = token_info.get("access_token")
        if not access_token:
            return redirect("/?error=token_failed")

        id_token = token_info.get("id_token")

        # Fetch user info
        user_info = auth_cli.fetch_user_info(access_token, id_token=id_token)
        if not user_info.email:
            return redirect("/?error=email_missing")

        # Login or register
        users = UserService.query(email=user_info.email)
        user_id = get_uuid()

        if not users:
            try:
                try:
                    avatar = download_img(user_info.avatar_url)
                except Exception as e:
                    logging.exception(e)
                    avatar = ""

                users = user_register(
                    user_id,
                    {
                        "access_token": get_uuid(),
                        "email": user_info.email,
                        "avatar": avatar,
                        "nickname": user_info.nickname,
                        "login_channel": channel,
                        "last_login_time": get_format_time(),
                        "is_superuser": False,
                    },
                )

                if not users:
                    raise Exception(f"Failed to register {user_info.email}")
                if len(users) > 1:
                    raise Exception(f"Same email: {user_info.email} exists!")

                # Try to log in
                user = users[0]
                login_user(user)
                return redirect(f"/?auth={user.get_id()}")

            except Exception as e:
                rollback_user_registration(user_id)
                logging.exception(e)
                return redirect(f"/?error={str(e)}")

        # User exists, try to log in
        user = users[0]
        user.access_token = get_uuid()
        if user and hasattr(user, 'is_active') and user.is_active == "0":
            return redirect("/?error=user_inactive")

        login_user(user)
        user.save()
        return redirect(f"/?auth={user.get_id()}")
    except Exception as e:
        logging.exception(e)
        return redirect(f"/?error={str(e)}")


@manager.route("/github_callback", methods=["GET"])  # noqa: F821
def github_callback():
    """
    **Deprecated**, Use `/oauth/callback/<channel>` instead.

    GitHub OAuth callback endpoint.
    ---
    tags:
      - OAuth
    parameters:
      - in: query
        name: code
        type: string
        required: true
        description: Authorization code from GitHub.
    responses:
      200:
        description: Authentication successful.
        schema:
          type: object
    """
    import requests

    res = requests.post(
        settings.GITHUB_OAUTH.get("url"),
        data={
            "client_id": settings.GITHUB_OAUTH.get("client_id"),
            "client_secret": settings.GITHUB_OAUTH.get("secret_key"),
            "code": request.args.get("code"),
        },
        headers={"Accept": "application/json"},
    )
    res = res.json()
    if "error" in res:
        return redirect("/?error=%s" % res["error_description"])

    if "user:email" not in res["scope"].split(","):
        return redirect("/?error=user:email not in scope")

    session["access_token"] = res["access_token"]
    session["access_token_from"] = "github"
    user_info = user_info_from_github(session["access_token"])
    email_address = user_info["email"]
    users = UserService.query(email=email_address)
    user_id = get_uuid()
    if not users:
        # User isn't try to register
        try:
            try:
                avatar = download_img(user_info["avatar_url"])
            except Exception as e:
                logging.exception(e)
                avatar = ""
            users = user_register(
                user_id,
                {
                    "access_token": session["access_token"],
                    "email": email_address,
                    "avatar": avatar,
                    "nickname": user_info["login"],
                    "login_channel": "github",
                    "last_login_time": get_format_time(),
                    "is_superuser": False,
                },
            )
            if not users:
                raise Exception(f"Fail to register {email_address}.")
            if len(users) > 1:
                raise Exception(f"Same email: {email_address} exists!")

            # Try to log in
            user = users[0]
            login_user(user)
            return redirect("/?auth=%s" % user.get_id())
        except Exception as e:
            rollback_user_registration(user_id)
            logging.exception(e)
            return redirect("/?error=%s" % str(e))

    # User has already registered, try to log in
    user = users[0]
    user.access_token = get_uuid()
    if user and hasattr(user, 'is_active') and user.is_active == "0":
        return redirect("/?error=user_inactive")
    login_user(user)
    user.save()
    return redirect("/?auth=%s" % user.get_id())


@manager.route("/feishu_callback", methods=["GET"])  # noqa: F821
def feishu_callback():
    """
    Feishu OAuth callback endpoint.
    ---
    tags:
      - OAuth
    parameters:
      - in: query
        name: code
        type: string
        required: true
        description: Authorization code from Feishu.
    responses:
      200:
        description: Authentication successful.
        schema:
          type: object
    """
    import requests

    app_access_token_res = requests.post(
        settings.FEISHU_OAUTH.get("app_access_token_url"),
        data=json.dumps(
            {
                "app_id": settings.FEISHU_OAUTH.get("app_id"),
                "app_secret": settings.FEISHU_OAUTH.get("app_secret"),
            }
        ),
        headers={"Content-Type": "application/json; charset=utf-8"},
    )
    app_access_token_res = app_access_token_res.json()
    if app_access_token_res["code"] != 0:
        return redirect("/?error=%s" % app_access_token_res)

    res = requests.post(
        settings.FEISHU_OAUTH.get("user_access_token_url"),
        data=json.dumps(
            {
                "grant_type": settings.FEISHU_OAUTH.get("grant_type"),
                "code": request.args.get("code"),
            }
        ),
        headers={
            "Content-Type": "application/json; charset=utf-8",
            "Authorization": f"Bearer {app_access_token_res['app_access_token']}",
        },
    )
    res = res.json()
    if res["code"] != 0:
        return redirect("/?error=%s" % res["message"])

    if "contact:user.email:readonly" not in res["data"]["scope"].split():
        return redirect("/?error=contact:user.email:readonly not in scope")
    session["access_token"] = res["data"]["access_token"]
    session["access_token_from"] = "feishu"
    user_info = user_info_from_feishu(session["access_token"])
    email_address = user_info["email"]
    users = UserService.query(email=email_address)
    user_id = get_uuid()
    if not users:
        # User isn't try to register
        try:
            try:
                avatar = download_img(user_info["avatar_url"])
            except Exception as e:
                logging.exception(e)
                avatar = ""
            users = user_register(
                user_id,
                {
                    "access_token": session["access_token"],
                    "email": email_address,
                    "avatar": avatar,
                    "nickname": user_info["en_name"],
                    "login_channel": "feishu",
                    "last_login_time": get_format_time(),
                    "is_superuser": False,
                },
            )
            if not users:
                raise Exception(f"Fail to register {email_address}.")
            if len(users) > 1:
                raise Exception(f"Same email: {email_address} exists!")

            # Try to log in
            user = users[0]
            login_user(user)
            return redirect("/?auth=%s" % user.get_id())
        except Exception as e:
            rollback_user_registration(user_id)
            logging.exception(e)
            return redirect("/?error=%s" % str(e))

    # User has already registered, try to log in
    user = users[0]
    if user and hasattr(user, 'is_active') and user.is_active == "0":
        return redirect("/?error=user_inactive")
    user.access_token = get_uuid()
    login_user(user)
    user.save()
    return redirect("/?auth=%s" % user.get_id())


def user_info_from_feishu(access_token):
    import requests

    headers = {
        "Content-Type": "application/json; charset=utf-8",
        "Authorization": f"Bearer {access_token}",
    }
    res = requests.get("https://open.feishu.cn/open-apis/authen/v1/user_info", headers=headers)
    user_info = res.json()["data"]
    user_info["email"] = None if user_info.get("email") == "" else user_info["email"]
    return user_info


def user_info_from_github(access_token):
    import requests

    headers = {"Accept": "application/json", "Authorization": f"token {access_token}"}
    res = requests.get(f"https://api.github.com/user?access_token={access_token}", headers=headers)
    user_info = res.json()
    email_info = requests.get(
        f"https://api.github.com/user/emails?access_token={access_token}",
        headers=headers,
    ).json()
    user_info["email"] = next((email for email in email_info if email["primary"]), None)["email"]
    return user_info


@manager.route("/logout", methods=["GET"])  # noqa: F821
@login_required
def log_out():
    """
    User logout endpoint.
    ---
    tags:
      - User
    security:
      - ApiKeyAuth: []
    responses:
      200:
        description: Logout successful.
        schema:
          type: object
    """
    current_user.access_token = f"INVALID_{secrets.token_hex(16)}"
    current_user.save()
    logout_user()
    return get_json_result(data=True)


@manager.route("/setting", methods=["POST"])  # noqa: F821
@login_required
def setting_user():
    """
    Update user settings.
    ---
    tags:
      - User
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        description: User settings to update.
        required: true
        schema:
          type: object
          properties:
            nickname:
              type: string
              description: New nickname.
            email:
              type: string
              description: New email.
    responses:
      200:
        description: Settings updated successfully.
        schema:
          type: object
    """
    update_dict = {}
    request_data = request.json
    if request_data.get("password"):
        new_password = request_data.get("new_password")
        if not check_password_hash(current_user.password, decrypt(request_data["password"])):
            return get_json_result(
                data=False,
                code=RetCode.AUTHENTICATION_ERROR,
                message="Password error!",
            )

        if new_password:
            update_dict["password"] = generate_password_hash(decrypt(new_password))

    for k in request_data.keys():
        if k in [
            "password",
            "new_password",
            "email",
            "status",
            "is_superuser",
            "login_channel",
            "is_anonymous",
            "is_active",
            "is_authenticated",
            "last_login_time",
        ]:
            continue
        update_dict[k] = request_data[k]

    try:
        UserService.update_by_id(current_user.id, update_dict)
        return get_json_result(data=True)
    except Exception as e:
        logging.exception(e)
        return get_json_result(data=False, message="Update failure!", code=RetCode.EXCEPTION_ERROR)


@manager.route("/info", methods=["GET"])  # noqa: F821
@login_required
def user_profile():
    """
    Get user profile information.
    ---
    tags:
      - User
    security:
      - ApiKeyAuth: []
    responses:
      200:
        description: User profile retrieved successfully.
        schema:
          type: object
          properties:
            id:
              type: string
              description: User ID.
            nickname:
              type: string
              description: User nickname.
            email:
              type: string
              description: User email.
    """
    return get_json_result(data=current_user.to_dict())


def rollback_user_registration(user_id):
    try:
        UserService.delete_by_id(user_id)
    except Exception:
        pass
    try:
        TenantService.delete_by_id(user_id)
    except Exception:
        pass
    try:
        u = UserTenantService.query(tenant_id=user_id)
        if u:
            UserTenantService.delete_by_id(u[0].id)
    except Exception:
        pass
    try:
        TenantLLM.delete().where(TenantLLM.tenant_id == user_id).execute()
    except Exception:
        pass


def user_register(user_id, user):
    user["id"] = user_id
    tenant = {
        "id": user_id,
        "name": user["nickname"] + "‘s Kingdom",
        "llm_id": settings.CHAT_MDL,
        "embd_id": settings.EMBEDDING_MDL,
        "asr_id": settings.ASR_MDL,
        "parser_ids": settings.PARSERS,
        "img2txt_id": settings.IMAGE2TEXT_MDL,
        "rerank_id": settings.RERANK_MDL,
    }
    usr_tenant = {
        "tenant_id": user_id,
        "user_id": user_id,
        "invited_by": user_id,
        "role": UserTenantRole.OWNER,
    }
    file_id = get_uuid()
    file = {
        "id": file_id,
        "parent_id": file_id,
        "tenant_id": user_id,
        "created_by": user_id,
        "name": "/",
        "type": FileType.FOLDER.value,
        "size": 0,
        "location": "",
    }

    tenant_llm = get_init_tenant_llm(user_id)

    if not UserService.save(**user):
        return
    TenantService.insert(**tenant)
    UserTenantService.insert(**usr_tenant)
    TenantLLMService.insert_many(tenant_llm)
    FileService.insert(file)
    return UserService.query(email=user["email"])


@manager.route("/register", methods=["POST"])  # noqa: F821
@validate_request("nickname", "email", "password")
def user_add():
    """
    Register a new user.
    ---
    tags:
      - User
    parameters:
      - in: body
        name: body
        description: Registration details.
        required: true
        schema:
          type: object
          properties:
            nickname:
              type: string
              description: User nickname.
            email:
              type: string
              description: User email.
            password:
              type: string
              description: User password.
    responses:
      200:
        description: Registration successful.
        schema:
          type: object
    """

    if not settings.REGISTER_ENABLED:
        return get_json_result(
            data=False,
            message="User registration is disabled!",
            code=RetCode.OPERATING_ERROR,
        )

    req = request.json
    email_address = req["email"]

    # Validate the email address (allow + in local part)
    if not re.match(r"^[\w\._\+-]+@([\w_-]+\.)+[\w-]{2,}$", email_address):
        return get_json_result(
            data=False,
            message=f"Invalid email address: {email_address}!",
            code=RetCode.OPERATING_ERROR,
        )

    # Check if the email address is already used
    if UserService.query(email=email_address):
        return get_json_result(
            data=False,
            message=f"Email: {email_address} has already registered!",
            code=RetCode.OPERATING_ERROR,
        )

    # Construct user info data
    nickname = req["nickname"]
    user_dict = {
        "access_token": get_uuid(),
        "email": email_address,
        "nickname": nickname,
        "password": decrypt(req["password"]),
        "login_channel": "password",
        "last_login_time": get_format_time(),
        "is_superuser": False,
    }

    user_id = get_uuid()
    try:
        users = user_register(user_id, user_dict)
        if not users:
            raise Exception(f"Fail to register {email_address}.")
        if len(users) > 1:
            raise Exception(f"Same email: {email_address} exists!")
        user = users[0]
        login_user(user)
        return construct_response(
            data=user.to_json(),
            auth=user.get_id(),
            message=f"{nickname}, welcome aboard!",
        )
    except Exception as e:
        rollback_user_registration(user_id)
        logging.exception(e)
        return get_json_result(
            data=False,
            message=f"User registration failure, error: {str(e)}",
            code=RetCode.EXCEPTION_ERROR,
        )


@manager.route("/create", methods=["POST"])  # noqa: F821
@login_required
@validate_request("nickname", "email", "password")
def create_user() -> Response:
    """
    Create a new user.

    ---
    tags:
      - User
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        description: User creation details.
        required: true
        schema:
          type: object
          properties:
            nickname:
              type: string
              description: User nickname.
            email:
              type: string
              description: User email.
            password:
              type: string
              description: User password (plain text or RSA-encrypted).
            is_superuser:
              type: boolean
              description: Whether the user should be a superuser (admin).
              default: false
    responses:
      200:
        description: User created successfully.
        schema:
          type: object
          properties:
            id:
              type: string
              description: User ID.
            email:
              type: string
              description: User email.
            nickname:
              type: string
              description: User nickname.
      400:
        description: Invalid request or email already exists.
        schema:
          type: object
      500:
        description: Server error during user creation.
        schema:
          type: object
    """
    if request.json is None:
        return get_json_result(
            data=False,
            message="Request body is required!",
            code=RetCode.ARGUMENT_ERROR,
        )

    req: Dict[str, Any] = request.json
    email_address: str = str(req.get("email", ""))
    
    # Validate email is provided
    if not email_address:
        return get_json_result(
            data=False,
            message="Email is required!",
            code=RetCode.ARGUMENT_ERROR,
        )

    # Sanitize control characters from email (null bytes, carriage returns, line feeds)
    email_address = email_address.replace("\x00", "")  # Null byte
    email_address = email_address.replace("\r", "")    # Carriage return
    email_address = email_address.replace("\n", "")    # Line feed
    
    # Validate email length (RFC 5321: local part max 64 chars, total max 254 chars)
    if len(email_address) > 254:
        return get_json_result(
            data=False,
            message=f"Invalid email address: email is too long (max 254 characters)!",
            code=RetCode.OPERATING_ERROR,
        )
    
    # Split email to check local part length
    email_parts: List[str] = email_address.split("@")
    if len(email_parts) != 2:
        return get_json_result(
            data=False,
            message=f"Invalid email address: {email_address}!",
            code=RetCode.OPERATING_ERROR,
        )
    
    local_part: str = email_parts[0]
    if len(local_part) > 64:
        return get_json_result(
            data=False,
            message=f"Invalid email address: local part is too long (max 64 characters)!",
            code=RetCode.OPERATING_ERROR,
        )

    # Validate the email address format (allow + in local part)
    email_match: Optional[Match[str]] = re.match(
        r"^[\w\._\+-]+@([\w_-]+\.)+[\w-]{2,}$", email_address
    )
    if not email_match:
        return get_json_result(
            data=False,
            message=f"Invalid email address: {email_address}!",
            code=RetCode.OPERATING_ERROR,
        )

    # Check if the email address is already used
    existing_users_query = UserService.query(email=email_address)
    existing_users_list: List[User] = list(existing_users_query)
    if existing_users_list:
        return get_json_result(
            data=False,
            message=f"Email: {email_address} has already registered!",
            code=RetCode.OPERATING_ERROR,
        )

    # Construct user info data
    nickname: str = str(req.get("nickname", ""))
    # Sanitize nickname to prevent XSS and control character injection
    nickname = sanitize_nickname(nickname)
    
    is_superuser: bool = bool(req.get("is_superuser", False))
    
    # Accept both encrypted (like /user/register) and plain text passwords
    password_input: str = str(req.get("password", ""))
    
    # Validate password is not empty
    if not password_input or not password_input.strip():
        return get_json_result(
            data=False,
            message="Password cannot be empty!",
            code=RetCode.ARGUMENT_ERROR,
        )
    
    # Try to decrypt password (if it's RSA-encrypted like from /user/register)
    # If decryption fails, treat as plain text (backward compatibility)
    try:
        password: str = decrypt(password_input)
    except BaseException:
        # Not encrypted, use as plain text
        password = password_input

    user_dict: Dict[str, Any] = {
        "access_token": get_uuid(),
        "email": email_address,
        "nickname": nickname,
        "password": password,
        "login_channel": "password",
        "last_login_time": get_format_time(),
        "is_superuser": is_superuser,
    }

    user_id: str = get_uuid()
    try:
        users_query = user_register(user_id, user_dict)
        if not users_query:
            raise Exception(f"Fail to create user {email_address}.")
        users_list: List[User] = list(users_query)
        if len(users_list) > 1:
            raise Exception(f"Same email: {email_address} exists!")

        user: User = users_list[0]
        return get_json_result(
            data=user.to_dict(),
            message=f"User {nickname} created successfully!",
        )
    except Exception as e:
        rollback_user_registration(user_id)
        logging.exception(e)
        return get_json_result(
            data=False,
            message=f"User creation failure, error: {str(e)}",
            code=RetCode.EXCEPTION_ERROR,
        )


@manager.route("/update", methods=["PUT"])  # noqa: F821
@login_required
@validate_request()
def update_user() -> Response:
    """
    Update an existing user. Users can only update their own account.
    ---
    tags:
      - User
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        description: User update details.
        required: true
        schema:
          type: object
          properties:
            user_id:
              type: string
              description: User ID to update (optional if email is provided).
            email:
              type: string
              description: User email to identify the user (optional if user_id
                is provided). If user_id is provided, this can be used as
                new_email.
            new_email:
              type: string
              description: New email address (optional). Use this to update email
                when identifying user by user_id.
            nickname:
              type: string
              description: New nickname (optional).
            password:
              type: string
              description: New password (encrypted, optional).
            is_superuser:
              type: boolean
              description: Whether the user should be a superuser (optional).
    responses:
      200:
        description: User updated successfully.
        schema:
          type: object
          properties:
            id:
              type: string
              description: User ID.
            email:
              type: string
              description: User email.
            nickname:
              type: string
              description: User nickname.
      400:
        description: Invalid request or user not found.
        schema:
          type: object
      500:
        description: Server error during user update.
        schema:
          type: object
    """
    if request.json is None:
        return get_json_result(
            data=False,
            message="Request body is required!",
            code=RetCode.ARGUMENT_ERROR,
        )

    req: Dict[str, Any] = request.json
    user_id: Optional[str] = req.get("user_id")
    email: Optional[str] = req.get("email")
    identified_by_user_id: bool = bool(user_id)

    # Validate that either user_id or email is provided
    if not user_id and not email:
        return get_json_result(
            data=False,
            message="Either user_id or email must be provided!",
            code=RetCode.ARGUMENT_ERROR,
        )

    # Find the user by user_id or email
    user: Optional[User] = None

    if user_id:
        user = UserService.filter_by_id(user_id)
    elif email:
        # Validate the email address format (allow + in local part)
        email_match: Optional[Match[str]] = re.match(
            r"^[\w\._\+-]+@([\w_-]+\.)+[\w-]{2,}$", email
        )
        if not email_match:
            return get_json_result(
                data=False,
                message=f"Invalid email address: {email}!",
                code=RetCode.OPERATING_ERROR,
            )

        users_query = UserService.query(email=email)
        users_list: List[User] = list(users_query)
        if not users_list:
            return get_json_result(
                data=False,
                message=f"User with email: {email} not found!",
                code=RetCode.DATA_ERROR,
            )
        if len(users_list) > 1:
            return get_json_result(
                data=False,
                message=f"Multiple users found with email: {email}!",
                code=RetCode.DATA_ERROR,
            )
        user = users_list[0]
        user_id = user.id

    if not user:
        return get_json_result(
            data=False,
            message="User not found!",
            code=RetCode.DATA_ERROR,
        )

    # Ensure user can only update themselves
    if user.id != current_user.id:
        return get_json_result(
            data=False,
            message="You can only update your own account!",
            code=RetCode.FORBIDDEN,
        )

    # Build update dictionary
    update_dict: Dict[str, Any] = {}

    # Handle nickname update
    # Allow empty nickname (empty string is a valid value)
    if "nickname" in req:
        nickname: Any = req.get("nickname")
        # Only skip if explicitly None, allow empty strings
        if nickname is not None:
            update_dict["nickname"] = nickname

    # Handle password update
    if "password" in req and req["password"]:
        try:
            password: str = decrypt(req["password"])
            update_dict["password"] = generate_password_hash(password)
        except BaseException:
            return get_json_result(
                data=False,
                code=RetCode.SERVER_ERROR,
                message="Fail to decrypt password",
            )

    # Handle email update
    # If user_id was used to identify, "email" in req can be the new email
    # Otherwise, use "new_email" field
    new_email: Optional[str] = None
    if identified_by_user_id and "email" in req and req["email"]:
        new_email = req["email"]
    elif "new_email" in req and req["new_email"]:
        new_email = req["new_email"]

    if new_email:
        # Validate the new email address format (allow + in local part)
        email_match: Optional[Match[str]] = re.match(
            r"^[\w\._\+-]+@([\w_-]+\.)+[\w-]{2,}$", new_email
        )
        if not email_match:
            return get_json_result(
                data=False,
                message=f"Invalid email address: {new_email}!",
                code=RetCode.OPERATING_ERROR,
            )

        # Check if the new email is already used by another user
        existing_users_query = UserService.query(email=new_email)
        existing_users_list: List[User] = list(existing_users_query)
        if existing_users_list and existing_users_list[0].id != user_id:
            return get_json_result(
                data=False,
                message=(
                    f"Email: {new_email} is already in use by another user!"
                ),
                code=RetCode.OPERATING_ERROR,
            )
        update_dict["email"] = new_email

    # Handle is_superuser update
    if "is_superuser" in req:
        is_superuser: bool = req.get("is_superuser", False)
        update_dict["is_superuser"] = is_superuser

    # If no fields to update, return error
    if not update_dict:
        return get_json_result(
            data=False,
            message="No valid fields to update!",
            code=RetCode.ARGUMENT_ERROR,
        )

    # Update the user
    try:
        UserService.update_user(user_id, update_dict)
        # Fetch updated user
        updated_user: Optional[User] = UserService.filter_by_id(user_id)
        if not updated_user:
            return get_json_result(
                data=False,
                message="User updated but could not retrieve updated data!",
                code=RetCode.EXCEPTION_ERROR,
            )
        return get_json_result(
            data=updated_user.to_dict(),
            message=f"User {updated_user.nickname} updated successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return get_json_result(
            data=False,
            message=f"User update failure, error: {str(e)}",
            code=RetCode.EXCEPTION_ERROR,
        )


@manager.route("/list", methods=["GET"])  # noqa: F821
@login_required
def list_users() -> Response:
    """
    List all users.

    ---
    tags:
      - User
    security:
      - ApiKeyAuth: []
    parameters:
      - in: query
        name: page
        type: integer
        description: Page number for pagination (optional).
        required: false
      - in: query
        name: page_size
        type: integer
        description: Number of items per page (optional).
        required: false
      - in: query
        name: email
        type: string
        description: Filter by email address (optional).
        required: false
    responses:
      200:
        description: Users retrieved successfully.
        schema:
          type: object
          properties:
            data:
              type: array
              items:
                type: object
                properties:
                  id:
                    type: string
                    description: User ID.
                  email:
                    type: string
                    description: User email.
                  nickname:
                    type: string
                    description: User nickname.
                  is_superuser:
                    type: boolean
                    description: Whether the user is a superuser.
            total:
              type: integer
              description: Total number of users.
      401:
        description: Unauthorized - authentication required.
        schema:
          type: object
      500:
        description: Server error during user listing.
        schema:
          type: object
    """
    # Explicitly check authentication status
    if not current_user.is_authenticated:
        return get_json_result(
            data=False,
            message="Unauthorized",
            code=RetCode.UNAUTHORIZED,
        )
    
    try:
        # Get query parameters
        page: Optional[int] = None
        page_size: Optional[int] = None
        email_filter: Optional[str] = None

        if request.args:
            page_str: Optional[str] = request.args.get("page")
            if page_str:
                try:
                    page = int(page_str)
                except ValueError:
                    return get_json_result(
                        data=False,
                        message="Invalid page parameter!",
                        code=RetCode.ARGUMENT_ERROR,
                    )

            page_size_str: Optional[str] = request.args.get("page_size")
            if page_size_str:
                try:
                    page_size = int(page_size_str)
                except ValueError:
                    return get_json_result(
                        data=False,
                        message="Invalid page_size parameter!",
                        code=RetCode.ARGUMENT_ERROR,
                    )

            email_filter = request.args.get("email")

        # Query users
        if email_filter:
            # Validate email format if provided (allow + in local part)
            email_match: Optional[Match[str]] = re.match(
                r"^[\w\._\+-]+@([\w_-]+\.)+[\w-]{2,}$", email_filter
            )
            if not email_match:
                return get_json_result(
                    data=False,
                    message=f"Invalid email address: {email_filter}!",
                    code=RetCode.OPERATING_ERROR,
                )
            users_query = UserService.query(email=email_filter)
            users_list: List[User] = list(users_query)
        else:
            users_list: List[User] = UserService.get_all_users()

        # Convert users to dictionaries
        users_data: List[Dict[str, Any]] = [
            user.to_dict() for user in users_list
        ]

        # Apply pagination if requested
        total: int = len(users_data)
        if page is not None and page_size is not None:
            if page < 1:
                return get_json_result(
                    data=False,
                    message="Page number must be greater than 0!",
                    code=RetCode.ARGUMENT_ERROR,
                )
            if page_size < 1:
                return get_json_result(
                    data=False,
                    message="Page size must be greater than 0!",
                    code=RetCode.ARGUMENT_ERROR,
                )

            start_idx: int = (page - 1) * page_size
            end_idx: int = start_idx + page_size
            users_data = users_data[start_idx:end_idx]

        return get_json_result(
            data=users_data,
            message=f"Retrieved {len(users_data)} user(s) successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return get_json_result(
            data=False,
            message=f"User listing failure, error: {str(e)}",
            code=RetCode.EXCEPTION_ERROR,
        )


@manager.route("/delete", methods=["DELETE"])  # noqa: F821
@login_required
@validate_request()
def delete_user() -> Response:
    """
    Delete a user. Users can only delete their own account.

    ---
    tags:
      - User
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        description: User identification details.
        required: true
        schema:
          type: object
          properties:
            user_id:
              type: string
              description: User ID to delete (optional if email is provided).
            email:
              type: string
              description: User email to identify the user (optional if user_id
                is provided).
    responses:
      200:
        description: User deleted successfully.
        schema:
          type: object
          properties:
            data:
              type: boolean
              description: Deletion success status.
            message:
              type: string
              description: Success message.
      401:
        description: Unauthorized - authentication required.
        schema:
          type: object
      403:
        description: Forbidden - users can only delete their own account.
        schema:
          type: object
      400:
        description: Invalid request or user not found.
        schema:
          type: object
      500:
        description: Server error during user deletion.
        schema:
          type: object
    """
    # Explicitly check authentication status
    if not current_user.is_authenticated:
        return get_json_result(
            data=False,
            message="Unauthorized",
            code=RetCode.UNAUTHORIZED,
        )
    
    if request.json is None:
        return get_json_result(
            data=False,
            message="Request body is required!",
            code=RetCode.ARGUMENT_ERROR,
        )

    req: Dict[str, Any] = request.json
    user_id: Optional[str] = req.get("user_id")
    email: Optional[str] = req.get("email")

    # Validate that either user_id or email is provided
    if not user_id and not email:
        return get_json_result(
            data=False,
            message="Either user_id or email must be provided!",
            code=RetCode.ARGUMENT_ERROR,
        )

    # Find the user by user_id or email
    user: Optional[User] = None

    if user_id:
        user = UserService.filter_by_id(user_id)
    elif email:
        # Validate the email address format (allow + in local part)
        email_match: Optional[Match[str]] = re.match(
            r"^[\w\._\+-]+@([\w_-]+\.)+[\w-]{2,}$", email
        )
        if not email_match:
            return get_json_result(
                data=False,
                message=f"Invalid email address: {email}!",
                code=RetCode.OPERATING_ERROR,
            )

        users_query = UserService.query(email=email)
        users_list: List[User] = list(users_query)
        if not users_list:
            return get_json_result(
                data=False,
                message=f"User with email: {email} not found!",
                code=RetCode.DATA_ERROR,
            )
        if len(users_list) > 1:
            return get_json_result(
                data=False,
                message=f"Multiple users found with email: {email}!",
                code=RetCode.DATA_ERROR,
            )
        user = users_list[0]
        user_id = user.id

    if not user:
        return get_json_result(
            data=False,
            message="User not found!",
            code=RetCode.DATA_ERROR,
        )

    # Ensure user can only delete themselves
    if user.id != current_user.id:
        return get_json_result(
            data=False,
            message="You can only delete your own account!",
            code=RetCode.FORBIDDEN,
        )

    # Delete the user
    try:
        # Use hard delete to actually remove the user
        deleted_count: int = UserService.delete_by_id(user_id)
        if deleted_count == 0:
            return get_json_result(
                data=False,
                message="User not found or could not be deleted!",
                code=RetCode.DATA_ERROR,
            )
        return get_json_result(
            data=True,
            message=f"User {user.email} deleted successfully!",
        )
    except Exception as e:
        logging.exception(e)
        return get_json_result(
            data=False,
            message=f"User deletion failure, error: {str(e)}",
            code=RetCode.EXCEPTION_ERROR,
        )


@manager.route("/tenant_info", methods=["GET"])  # noqa: F821
@login_required
def tenant_info():
    """
    Get tenant information.
    ---
    tags:
      - Tenant
    security:
      - ApiKeyAuth: []
    responses:
      200:
        description: Tenant information retrieved successfully.
        schema:
          type: object
          properties:
            tenant_id:
              type: string
              description: Tenant ID.
            name:
              type: string
              description: Tenant name.
            llm_id:
              type: string
              description: LLM ID.
            embd_id:
              type: string
              description: Embedding model ID.
    """
    try:
        tenants = TenantService.get_info_by(current_user.id)
        if not tenants:
            return get_data_error_result(message="Tenant not found!")
        return get_json_result(data=tenants[0])
    except Exception as e:
        return server_error_response(e)


@manager.route("/set_tenant_info", methods=["POST"])  # noqa: F821
@login_required
@validate_request("tenant_id", "asr_id", "embd_id", "img2txt_id", "llm_id")
def set_tenant_info():
    """
    Update tenant information.
    ---
    tags:
      - Tenant
    security:
      - ApiKeyAuth: []
    parameters:
      - in: body
        name: body
        description: Tenant information to update.
        required: true
        schema:
          type: object
          properties:
            tenant_id:
              type: string
              description: Tenant ID.
            llm_id:
              type: string
              description: LLM ID.
            embd_id:
              type: string
              description: Embedding model ID.
            asr_id:
              type: string
              description: ASR model ID.
            img2txt_id:
              type: string
              description: Image to Text model ID.
    responses:
      200:
        description: Tenant information updated successfully.
        schema:
          type: object
    """
    req = request.json
    try:
        tid = req.pop("tenant_id")
        TenantService.update_by_id(tid, req)
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)
        

@manager.route("/forget/captcha", methods=["GET"])  # noqa: F821
def forget_get_captcha():
    """
    GET /forget/captcha?email=<email>
    - Generate an image captcha and cache it in Redis under key captcha:{email} with TTL = OTP_TTL_SECONDS.
    - Returns the captcha as a PNG image.
    """
    email = (request.args.get("email") or "")
    if not email:
        return get_json_result(data=False, code=RetCode.ARGUMENT_ERROR, message="email is required")

    users = UserService.query(email=email)
    if not users:
        return get_json_result(data=False, code=RetCode.DATA_ERROR, message="invalid email")

    # Generate captcha text
    allowed = string.ascii_uppercase + string.digits
    captcha_text = "".join(secrets.choice(allowed) for _ in range(OTP_LENGTH))
    REDIS_CONN.set(captcha_key(email), captcha_text, 60) # Valid for 60 seconds

    from captcha.image import ImageCaptcha
    image = ImageCaptcha(width=300, height=120, font_sizes=[50, 60, 70])
    img_bytes = image.generate(captcha_text).read()
    response = make_response(img_bytes)
    response.headers.set("Content-Type", "image/JPEG")
    return response


@manager.route("/forget/otp", methods=["POST"])  # noqa: F821
def forget_send_otp():
    """
    POST /forget/otp
    - Verify the image captcha stored at captcha:{email} (case-insensitive).
    - On success, generate an email OTP (A–Z with length = OTP_LENGTH), store hash + salt (and timestamp) in Redis with TTL, reset attempts and cooldown, and send the OTP via email.
    """
    req = request.get_json()
    email = req.get("email") or ""
    captcha = (req.get("captcha") or "").strip()

    if not email or not captcha:
        return get_json_result(data=False, code=RetCode.ARGUMENT_ERROR, message="email and captcha required")

    users = UserService.query(email=email)
    if not users:
        return get_json_result(data=False, code=RetCode.DATA_ERROR, message="invalid email")

    stored_captcha = REDIS_CONN.get(captcha_key(email))
    if not stored_captcha:
        return get_json_result(data=False, code=RetCode.NOT_EFFECTIVE, message="invalid or expired captcha")
    if (stored_captcha or "").strip().lower() != captcha.lower():
        return get_json_result(data=False, code=RetCode.AUTHENTICATION_ERROR, message="invalid or expired captcha")

    # Delete captcha to prevent reuse
    REDIS_CONN.delete(captcha_key(email))

    k_code, k_attempts, k_last, k_lock = otp_keys(email)
    now = int(time.time())
    last_ts = REDIS_CONN.get(k_last)
    if last_ts:
        try:
            elapsed = now - int(last_ts)
        except Exception:
            elapsed = RESEND_COOLDOWN_SECONDS
        remaining = RESEND_COOLDOWN_SECONDS - elapsed
        if remaining > 0:
            return get_json_result(data=False, code=RetCode.NOT_EFFECTIVE, message=f"you still have to wait {remaining} seconds")

    # Generate OTP (uppercase letters only) and store hashed
    otp = "".join(secrets.choice(string.ascii_uppercase) for _ in range(OTP_LENGTH))
    salt = os.urandom(16)
    code_hash = hash_code(otp, salt)
    REDIS_CONN.set(k_code, f"{code_hash}:{salt.hex()}", OTP_TTL_SECONDS)
    REDIS_CONN.set(k_attempts, 0, OTP_TTL_SECONDS)
    REDIS_CONN.set(k_last, now, OTP_TTL_SECONDS)
    REDIS_CONN.delete(k_lock)

    ttl_min = OTP_TTL_SECONDS // 60

    if not smtp_mail_server:
        logging.warning("SMTP mail server not initialized; skip sending email.")
    else:
        try:
            send_email_html(
                subject="Your Password Reset Code",
                to_email=email,
                template_key="reset_code",
                code=otp,
                ttl_min=ttl_min,
            )
        except Exception:
            return get_json_result(data=False, code=RetCode.SERVER_ERROR, message="failed to send email")
        
    return get_json_result(data=True, code=RetCode.SUCCESS, message="verification passed, email sent")


@manager.route("/forget", methods=["POST"])  # noqa: F821
def forget():
    """
    POST: Verify email + OTP and reset password, then log the user in.
    Request JSON: { email, otp, new_password, confirm_new_password }
    """
    req = request.get_json()
    email = req.get("email") or ""
    otp = (req.get("otp") or "").strip()
    new_pwd = req.get("new_password")
    new_pwd2 = req.get("confirm_new_password")

    if not all([email, otp, new_pwd, new_pwd2]):
        return get_json_result(data=False, code=RetCode.ARGUMENT_ERROR, message="email, otp and passwords are required")

    # For reset, passwords are provided as-is (no decrypt needed)
    if new_pwd != new_pwd2:
        return get_json_result(data=False, code=RetCode.ARGUMENT_ERROR, message="passwords do not match")

    users = UserService.query(email=email)
    if not users:
        return get_json_result(data=False, code=RetCode.DATA_ERROR, message="invalid email")

    user = users[0]
    # Verify OTP from Redis
    k_code, k_attempts, k_last, k_lock = otp_keys(email)
    if REDIS_CONN.get(k_lock):
        return get_json_result(data=False, code=RetCode.NOT_EFFECTIVE, message="too many attempts, try later")

    stored = REDIS_CONN.get(k_code)
    if not stored:
        return get_json_result(data=False, code=RetCode.NOT_EFFECTIVE, message="expired otp")

    try:
        stored_hash, salt_hex = str(stored).split(":", 1)
        salt = bytes.fromhex(salt_hex)
    except Exception:
        return get_json_result(data=False, code=RetCode.EXCEPTION_ERROR, message="otp storage corrupted")

    # Case-insensitive verification: OTP generated uppercase
    calc = hash_code(otp.upper(), salt)
    if calc != stored_hash:
        # bump attempts
        try:
            attempts = int(REDIS_CONN.get(k_attempts) or 0) + 1
        except Exception:
            attempts = 1
        REDIS_CONN.set(k_attempts, attempts, OTP_TTL_SECONDS)
        if attempts >= ATTEMPT_LIMIT:
            REDIS_CONN.set(k_lock, int(time.time()), ATTEMPT_LOCK_SECONDS)
        return get_json_result(data=False, code=RetCode.AUTHENTICATION_ERROR, message="expired otp")

    # Success: consume OTP and reset password
    REDIS_CONN.delete(k_code)
    REDIS_CONN.delete(k_attempts)
    REDIS_CONN.delete(k_last)
    REDIS_CONN.delete(k_lock)

    try:
        UserService.update_user_password(user.id, new_pwd)
    except Exception as e:
        logging.exception(e)
        return get_json_result(data=False, code=RetCode.EXCEPTION_ERROR, message="failed to reset password")

    # Auto login (reuse login flow)
    user.access_token = get_uuid()
    login_user(user)
    user.update_time = (current_timestamp(),)
    user.update_date = (datetime_format(datetime.now()),)
    user.save()
    msg = "Password reset successful. Logged in."
    return construct_response(data=user.to_json(), auth=user.get_id(), message=msg)
