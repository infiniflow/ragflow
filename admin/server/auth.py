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


import logging
import uuid
from functools import wraps
from datetime import datetime
from flask import request, jsonify
from flask_login import current_user, login_user
from itsdangerous.url_safe import URLSafeTimedSerializer as Serializer

from api import settings
from api.common.exceptions import AdminException, UserNotFoundError
from api.db.init_data import encode_to_base64
from api.db.services import UserService
from api.db import ActiveEnum, StatusEnum
from api.utils.crypt import decrypt
from api.utils import (
    current_timestamp,
    datetime_format,
    get_uuid,
)
from api.utils.api_utils import (
    construct_response,
)


def setup_auth(login_manager):
    @login_manager.request_loader
    def load_user(web_request):
        jwt = Serializer(secret_key=settings.SECRET_KEY)
        authorization = web_request.headers.get("Authorization")
        if authorization:
            try:
                access_token = str(jwt.loads(authorization))

                if not access_token or not access_token.strip():
                    logging.warning("Authentication attempt with empty access token")
                    return None

                # Access tokens should be UUIDs (32 hex characters)
                if len(access_token.strip()) < 32:
                    logging.warning(f"Authentication attempt with invalid token format: {len(access_token)} chars")
                    return None

                user = UserService.query(
                    access_token=access_token, status=StatusEnum.VALID.value
                )
                if user:
                    if not user[0].access_token or not user[0].access_token.strip():
                        logging.warning(f"User {user[0].email} has empty access_token in database")
                        return None
                    return user[0]
                else:
                    return None
            except Exception as e:
                logging.warning(f"load_user got exception {e}")
                return None
        else:
            return None


def init_default_admin():
    # Verify that at least one active admin user exists. If not, create a default one.
    users = UserService.query(is_superuser=True)
    if not users:
        default_admin = {
            "id": uuid.uuid1().hex,
            "password": encode_to_base64("admin"),
            "nickname": "admin",
            "is_superuser": True,
            "email": "admin@ragflow.io",
            "creator": "system",
            "status": "1",
        }
        if not UserService.save(**default_admin):
            raise AdminException("Can't init admin.", 500)
    elif not any([u.is_active == ActiveEnum.ACTIVE.value for u in users]):
        raise AdminException("No active admin. Please update 'is_active' in db manually.", 500)


def check_admin_auth(func):
    @wraps(func)
    def wrapper(*args, **kwargs):
        user = UserService.filter_by_id(current_user.id)
        if not user:
            raise UserNotFoundError(current_user.email)
        if not user.is_superuser:
            raise AdminException("Not admin", 403)
        if user.is_active == ActiveEnum.INACTIVE.value:
            raise AdminException(f"User {current_user.email} inactive", 403)

        return func(*args, **kwargs)

    return wrapper


def login_admin(email: str, password: str):
    """
    :param email: admin email
    :param password: string before decrypt
    """
    users = UserService.query(email=email)
    if not users:
        raise UserNotFoundError(email)
    psw = decrypt(password)
    user = UserService.query_user(email, psw)
    if not user:
        raise AdminException("Email and password do not match!")
    if not user.is_superuser:
        raise AdminException("Not admin", 403)
    if user.is_active == ActiveEnum.INACTIVE.value:
        raise AdminException(f"User {email} inactive", 403)

    resp = user.to_json()
    user.access_token = get_uuid()
    login_user(user)
    user.update_time = (current_timestamp(),)
    user.update_date = (datetime_format(datetime.now()),)
    user.save()
    msg = "Welcome back!"
    return construct_response(data=resp, auth=user.get_id(), message=msg)


def check_admin(username: str, password: str):
    users = UserService.query(email=username)
    if not users:
        logging.info(f"Username: {username} is not registered!")
        user_info = {
            "id": uuid.uuid1().hex,
            "password": encode_to_base64("admin"),
            "nickname": "admin",
            "is_superuser": True,
            "email": "admin@ragflow.io",
            "creator": "system",
            "status": "1",
        }
        if not UserService.save(**user_info):
            raise AdminException("Can't init admin.", 500)

    user = UserService.query_user(username, password)
    if user:
        return True
    else:
        return False


def login_verify(f):
    @wraps(f)
    def decorated(*args, **kwargs):
        auth = request.authorization
        if not auth or 'username' not in auth.parameters or 'password' not in auth.parameters:
            return jsonify({
                "code": 401,
                "message": "Authentication required",
                "data": None
            }), 200

        username = auth.parameters['username']
        password = auth.parameters['password']
        try:
            if check_admin(username, password) is False:
                return jsonify({
                    "code": 500,
                    "message": "Access denied",
                    "data": None
                }), 200
        except Exception as e:
            error_msg = str(e)
            return jsonify({
                "code": 500,
                "message": error_msg
            }), 200

        return f(*args, **kwargs)

    return decorated
