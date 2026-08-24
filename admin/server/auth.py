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
import os
import secrets
import threading
import time
import uuid
from functools import wraps
from datetime import datetime

from flask import jsonify, request
from flask_login import current_user, login_user

from api.common.exceptions import AdminException, UserNotFoundError
from api.common.base64 import encode_to_base64
from api.db.services import UserService
from api.db import UserTenantRole
from api.db.services.user_service import TenantService, UserTenantService
from common.constants import ActiveEnum, StatusEnum
from api.utils.crypt import decrypt
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp, datetime_format, get_format_time
from common.connection_utils import sync_construct_response
from common import settings


# Bootstrap password for the default superuser created by init_default_admin().
# ADMIN_DEFAULT_PASSWORD is admin-server specific; DEFAULT_SUPERUSER_PASSWORD is
# shared with api/db/init_data.py so a deployment can keep one bootstrap
# password for both sides. Unlike api/db/init_data.py (which still falls back
# to the literal "admin"), the admin server falls back to a random password
# printed once to the log: the admin panel manages sandbox providers
# (including the "local" provider, i.e. host-level execution), so a guessable
# default here would effectively be host RCE.
ADMIN_DEFAULT_PASSWORD_ENV = "ADMIN_DEFAULT_PASSWORD"
DEFAULT_SUPERUSER_PASSWORD_ENV = "DEFAULT_SUPERUSER_PASSWORD"

# Login throttling for the admin auth endpoints. Deliberately dependency-free
# and in-process: the admin server runs a single Flask process
# (werkzeug run_simple, threaded=True), so one lock-guarded dict is shared by
# every request. Limitation: the counters are process-local; if the admin
# server is ever run with multiple workers/processes, each worker keeps its
# own counters and the effective limit multiplies accordingly.
ADMIN_LOGIN_MAX_FAILURES = 5
ADMIN_LOGIN_FAILURE_WINDOW = 300  # seconds in which failures accumulate
ADMIN_LOGIN_LOCKOUT_SECONDS = 300  # seconds an address stays blocked

_login_state_lock = threading.Lock()
_login_failures: dict[str, list[float]] = {}  # client key -> recent failure timestamps (time.monotonic)
_login_block_until: dict[str, float] = {}  # client key -> monotonic lockout deadline


def _client_key() -> str:
    """Identify the client for login throttling.

    Prefers the first hop of X-Forwarded-For so that requests proxied by the
    bundled nginx (which appends the real client address) are tracked per
    client instead of all sharing nginx's address. This header is only
    trustworthy when port 9381 is not directly reachable from untrusted
    networks; keep the published port bound to 127.0.0.1 (see
    docker/docker-compose.yml).
    """
    forwarded_for = request.headers.get("X-Forwarded-For", "")
    first_hop = forwarded_for.split(",")[0].strip() if forwarded_for else ""
    return first_hop or (request.remote_addr or "").strip() or "unknown"


def _login_block_remaining(key: str) -> int:
    """Return the seconds left in the lockout for ``key``; 0 when not blocked."""
    with _login_state_lock:
        deadline = _login_block_until.get(key, 0.0)
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            _login_block_until.pop(key, None)
            return 0
        return int(remaining) + 1


def _record_login_failure(key: str) -> None:
    now = time.monotonic()
    with _login_state_lock:
        failures = [ts for ts in _login_failures.get(key, []) if now - ts < ADMIN_LOGIN_FAILURE_WINDOW]
        failures.append(now)
        _login_failures[key] = failures
        if len(failures) >= ADMIN_LOGIN_MAX_FAILURES:
            _login_block_until[key] = now + ADMIN_LOGIN_LOCKOUT_SECONDS
            _login_failures[key] = []
            logging.warning(f"Admin login from {key} blocked for {ADMIN_LOGIN_LOCKOUT_SECONDS}s after {ADMIN_LOGIN_MAX_FAILURES} failed attempts within {ADMIN_LOGIN_FAILURE_WINDOW}s.")


def _reset_login_failures(key: str) -> None:
    with _login_state_lock:
        _login_failures.pop(key, None)
        _login_block_until.pop(key, None)


def setup_auth(login_manager):
    @login_manager.request_loader
    def load_user(web_request):
        # Authorization header contains JWT-encoded access token
        # First decode JWT to get the UUID, then query database
        from itsdangerous.url_safe import URLSafeTimedSerializer as Serializer
        from common import settings

        authorization = web_request.headers.get("Authorization")
        if authorization:
            try:
                # Strip "Bearer " prefix if present
                jwt_token = authorization
                if jwt_token.startswith("Bearer "):
                    jwt_token = jwt_token[7:]

                jwt_token = jwt_token.strip()
                if not jwt_token:
                    logging.warning("Authentication attempt with empty JWT token")
                    return None

                # Decode JWT to get the UUID access_token
                jwt = Serializer(secret_key=settings.get_secret_key())
                access_token = str(jwt.loads(jwt_token))

                if not access_token or not access_token.strip():
                    logging.warning("Authentication attempt with empty access token after JWT decode")
                    return None

                # Access tokens stored in database are UUIDs (32 hex characters)
                if len(access_token) < 32:
                    logging.warning(f"Authentication attempt with invalid token format: {len(access_token)} chars")
                    return None

                user = UserService.query(access_token=access_token, status=StatusEnum.VALID.value)
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


def _resolve_bootstrap_admin_password() -> tuple[str, str | None]:
    """Return ``(plain_password, env_var_name)`` for the bootstrap superuser.

    Precedence: ADMIN_DEFAULT_PASSWORD, then DEFAULT_SUPERUSER_PASSWORD (the
    variable honoured by api/db/init_data.py on the web side), then a random
    password that is only revealed once in the startup log.
    """
    for env_name in (ADMIN_DEFAULT_PASSWORD_ENV, DEFAULT_SUPERUSER_PASSWORD_ENV):
        value = os.getenv(env_name)
        if value:
            return value, env_name
    return secrets.token_urlsafe(18), None


def init_default_admin():
    # Verify that at least one active admin user exists. If not, create a default one.
    # This runs only from the explicit startup path in admin_server.py; the
    # authentication handlers below must never create accounts.
    users = UserService.query(is_superuser=True)
    if not users:
        password, env_name = _resolve_bootstrap_admin_password()
        default_admin = {
            "id": uuid.uuid1().hex,
            # UserService.save() hashes this value and both login paths compare
            # against base64(<plain password>) - same convention as
            # api/db/init_data.py.
            "password": encode_to_base64(password),
            "nickname": "admin",
            "is_superuser": True,
            "email": "admin@ragflow.io",
            "creator": "system",
            "status": "1",
        }
        if not UserService.save(**default_admin):
            raise AdminException("Can't init admin.", 500)
        add_tenant_for_admin(default_admin, UserTenantRole.OWNER)
        if env_name:
            logging.info(f"Created default superuser admin@ragflow.io with the password from {env_name}. Change it after the first login.")
        else:
            logging.warning(
                f"No superuser found and neither {ADMIN_DEFAULT_PASSWORD_ENV} nor {DEFAULT_SUPERUSER_PASSWORD_ENV} is set. "
                f"Created default superuser admin@ragflow.io with the randomly generated password (printed only once, change it immediately): {password}"
            )
    elif not any([u.is_active == ActiveEnum.ACTIVE.value for u in users]):
        raise AdminException("No active admin. Please update 'is_active' in db manually.", 500)
    else:
        default_admin_rows = [u for u in users if u.email == "admin@ragflow.io"]
        if default_admin_rows:
            default_admin = default_admin_rows[0].to_dict()
            exist, default_admin_tenant = TenantService.get_by_id(default_admin["id"])
            if not exist:
                add_tenant_for_admin(default_admin, UserTenantRole.OWNER)


def add_tenant_for_admin(user_info: dict, role: str):

    tenant = {
        "id": user_info["id"],
        "name": user_info["nickname"] + "‘s Kingdom",
        "llm_id": settings.CHAT_MDL,
        "embd_id": settings.EMBEDDING_MDL,
        "asr_id": settings.ASR_MDL,
        "parser_ids": settings.PARSERS,
        "img2txt_id": settings.VISION_MDL,
        "rerank_id": settings.RERANK_MDL,
    }
    usr_tenant = {"tenant_id": user_info["id"], "user_id": user_info["id"], "invited_by": user_info["id"], "role": role}

    # tenant_llm = get_init_tenant_llm(user_info["id"])
    TenantService.insert(**tenant)
    UserTenantService.insert(**usr_tenant)
    # TenantLLMService.insert_many(tenant_llm)
    logging.info(f"Added tenant for email: {user_info['email']}, A default tenant has been set; changing the default models after login is strongly recommended.")


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
    :param password: string before decrypt (RSA encrypted + base64 encoded)
    """
    key = _client_key()
    remaining = _login_block_remaining(key)
    if remaining > 0:
        raise AdminException(f"Too many failed login attempts. Retry in {remaining}s.", 429)

    users = UserService.query(email=email)
    if not users:
        _record_login_failure(key)
        raise UserNotFoundError(email)
    decrypted = decrypt(password)
    user = UserService.query_user(email, decrypted)
    if not user:
        _record_login_failure(key)
        raise AdminException("Email and password do not match!")
    if not user.is_superuser:
        raise AdminException("Not admin", 403)
    if user.is_active == ActiveEnum.INACTIVE.value:
        raise AdminException(f"User {email} inactive", 403)

    _reset_login_failures(key)
    resp = user.to_json()
    user.access_token = get_uuid()
    login_user(user)
    user.update_time = (current_timestamp(),)
    user.update_date = (datetime_format(datetime.now()),)
    user.last_login_time = get_format_time()
    user.save()
    msg = "Welcome back!"
    return sync_construct_response(data=resp, auth=user.get_id(), message=msg)


def check_admin(username: str, password: str):
    # Authentication must never create accounts. Historically, probing this
    # function with any unregistered username silently created a superuser
    # admin@ragflow.io with a fixed password, so a single anonymous request
    # could resurrect a default admin account that an operator had removed
    # (CWE-798 hardcoded credentials + CWE-306 missing authentication for a
    # privileged action). A failed check is now just a failure; bootstrap
    # account creation only happens in init_default_admin() at startup.
    users = UserService.query(email=username)
    if not users:
        logging.info(f"Username: {username} is not registered!")
        return False

    user = UserService.query_user(username, password)
    if user:
        return True
    else:
        return False


def login_verify(f):
    @wraps(f)
    def decorated(*args, **kwargs):
        auth = request.authorization
        if not auth or "username" not in auth.parameters or "password" not in auth.parameters:
            return jsonify({"code": 401, "message": "Authentication required", "data": None}), 200

        key = _client_key()
        remaining = _login_block_remaining(key)
        if remaining > 0:
            logging.warning(f"Admin basic-auth verification from {key} throttled, retry in {remaining}s.")
            return jsonify({"code": 429, "message": f"Too many failed login attempts. Retry in {remaining}s.", "data": None}), 200

        username = auth.parameters["username"]
        password = auth.parameters["password"]
        try:
            if not check_admin(username, password):
                _record_login_failure(key)
                return jsonify({"code": 500, "message": "Access denied", "data": None}), 200
        except Exception:
            logging.exception("An error occurred during admin login verification.")
            return jsonify({"code": 500, "message": "An internal server error occurred."}), 200

        _reset_login_failures(key)
        return f(*args, **kwargs)

    return decorated
