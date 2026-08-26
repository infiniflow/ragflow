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

from flask import request
from flask_login import current_user, login_user

from api.common.exceptions import AdminException, UserNotFoundError
from api.common.base64 import encode_to_base64
from api.db.services import UserService
from api.db import UserTenantRole
from api.db.services.user_service import TenantService, UserTenantService
from common.constants import ActiveEnum, StatusEnum
from common.file_utils import get_project_base_directory
from api.utils.crypt import CryptPayloadError, decrypt
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


_TRUSTED_PROXY_PEERS = frozenset({"127.0.0.1", "::1"})


def _client_key() -> str:
    """Identify the client for login throttling.

    X-Forwarded-For is trusted only when the TCP peer is loopback: the admin
    server and the bundled nginx run in the same container, so proxied
    requests arrive from 127.0.0.1 and nginx appends the real client address
    to any client-supplied value — the rightmost entry is then the one our
    own proxy added and cannot be spoofed. Everything else (the admin API is
    designed to be reachable externally, so requests may hit the published
    port directly) is keyed by its TCP peer address, and a client-supplied
    X-Forwarded-For cannot rotate throttle buckets. Deployments fronting the
    admin server with an external reverse proxy share that proxy's peer key
    until it is added to the trusted set.
    """
    peer = (request.remote_addr or "").strip()
    forwarded_for = request.headers.get("X-Forwarded-For", "")
    last_hop = forwarded_for.split(",")[-1].strip() if forwarded_for else ""
    if peer in _TRUSTED_PROXY_PEERS and last_hop:
        return last_hop
    return peer or "unknown"


def _reserve_login_attempt(key: str) -> tuple[int, float | None]:
    """Atomically admit one login attempt for ``key``.

    Returns ``(remaining_lockout_seconds, reserved_marker)``: ``remaining``
    is positive when the key is blocked (``reserved_marker`` is then None),
    or 0 with the marker of the recorded failure slot when the attempt is
    admitted. Admission records the slot under the same state lock that
    checks the lockout, so concurrent requests cannot all observe "not
    blocked" and run credential verification before any of them records the
    failure that should have locked the key. Callers keep the reservation
    when verification fails, release exactly their own slot via
    ``_release_login_attempt`` on verification errors, and clear all state
    via ``_reset_login_failures`` on success.

    The recorded timestamps are kept when the lockout arms — clearing them
    would let a single release (judged against the shortened list) lift a
    lockout earned by genuine failures. They age out with
    ``ADMIN_LOGIN_FAILURE_WINDOW`` instead.
    """
    now = time.monotonic()
    with _login_state_lock:
        deadline = _login_block_until.get(key, 0.0)
        remaining = deadline - now
        if remaining > 0:
            return int(remaining) + 1, None
        _login_block_until.pop(key, None)
        failures = [ts for ts in _login_failures.get(key, []) if now - ts < ADMIN_LOGIN_FAILURE_WINDOW]
        failures.append(now)
        _login_failures[key] = failures
        if len(failures) >= ADMIN_LOGIN_MAX_FAILURES:
            _login_block_until[key] = now + ADMIN_LOGIN_LOCKOUT_SECONDS
            logging.warning(f"Admin login from {key} blocked for {ADMIN_LOGIN_LOCKOUT_SECONDS}s after {ADMIN_LOGIN_MAX_FAILURES} failed attempts within {ADMIN_LOGIN_FAILURE_WINDOW}s.")
        return 0, now


def _release_login_attempt(key: str, marker: float | None) -> None:
    """Undo exactly the reservation identified by ``marker`` for ``key``.

    Used when verification could not evaluate the credentials (an internal
    error) or rejected the caller before credential evaluation, so a
    non-credential failure does not consume the failure budget. Removing one
    marker leaves the other recorded failures intact — including the
    ADMIN_LOGIN_MAX_FAILURES - 1 genuine ones — so a lockout is lifted only
    when the released reservation is what armed it.
    """
    with _login_state_lock:
        failures = _login_failures.get(key)
        if failures:
            try:
                failures.remove(marker)
            except ValueError:
                pass  # already expired out of the window; nothing to undo
            if not failures:
                _login_failures.pop(key, None)
        if len(_login_failures.get(key, [])) < ADMIN_LOGIN_MAX_FAILURES:
            _login_block_until.pop(key, None)


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
    password that is delivered through a 0600 bootstrap file, never the log.
    """
    for env_name in (ADMIN_DEFAULT_PASSWORD_ENV, DEFAULT_SUPERUSER_PASSWORD_ENV):
        value = os.getenv(env_name)
        if value:
            return value, env_name
    return secrets.token_urlsafe(18), None


def _write_bootstrap_password_file(password: str) -> str | None:
    """Write the generated bootstrap password to a 0600 file beside the logs.

    Returns the file path, or None when the file could not be written (the
    password is then unrecoverable and the operator must set
    ADMIN_DEFAULT_PASSWORD and recreate the account — failing closed beats
    leaking the credential into a log stream).
    """
    target_dir = os.path.join(get_project_base_directory(), "logs")
    path = os.path.join(target_dir, "admin_bootstrap_password.txt")
    try:
        os.makedirs(target_dir, exist_ok=True)
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
        with os.fdopen(fd, "w") as fh:
            fh.write(password + "\n")
        return path
    except OSError as e:
        logging.error(f"Failed to write the admin bootstrap password file {path}: {e}")
        return None


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
            password_file = _write_bootstrap_password_file(password)
            if password_file:
                logging.warning(
                    f"No superuser found and neither {ADMIN_DEFAULT_PASSWORD_ENV} nor {DEFAULT_SUPERUSER_PASSWORD_ENV} is set. "
                    f"Created default superuser admin@ragflow.io; its randomly generated password was written once to {password_file} (mode 0600). Read it there and change it immediately after the first login."
                )
            else:
                logging.error(
                    f"No superuser found and neither {ADMIN_DEFAULT_PASSWORD_ENV} nor {DEFAULT_SUPERUSER_PASSWORD_ENV} is set, "
                    f"and the bootstrap password file could not be written. The randomly generated password for admin@ragflow.io is NOT recoverable: "
                    f"delete the account and re-run with {ADMIN_DEFAULT_PASSWORD_ENV} set."
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
    # Admission and the failure record are one atomic step: the attempt slot
    # is consumed before credential verification, so a concurrent burst from
    # one client cannot exceed ADMIN_LOGIN_MAX_FAILURES verifications.
    remaining, marker = _reserve_login_attempt(key)
    if remaining > 0:
        raise AdminException(f"Too many failed login attempts. Retry in {remaining}s.", 429)

    try:
        users = UserService.query(email=email)
        if not users:
            raise UserNotFoundError(email)
        try:
            decrypted = decrypt(password)
        except CryptPayloadError:
            # A payload the client could not encode correctly is the
            # client's failure, not a server fault: it keeps the reserved
            # slot like any other credential rejection. Server-side faults
            # (missing/invalid private key) propagate to the outer handler
            # and release the reservation instead.
            logging.info("Admin login with undecryptable password payload.")
            raise AdminException("Email and password do not match!")
        user = UserService.query_user(email, decrypted)
        if not user:
            raise AdminException("Email and password do not match!")
        if not user.is_superuser:
            # Valid credentials, but not an admin account: reject without
            # consuming the client's failure budget.
            _release_login_attempt(key, marker)
            raise AdminException("Not admin", 403)
        if user.is_active == ActiveEnum.INACTIVE.value:
            _release_login_attempt(key, marker)
            raise AdminException(f"User {email} inactive", 403)
    except (UserNotFoundError, AdminException):
        # Deliberate rejections keep the reserved failure slot (the 403
        # branches released theirs above).
        raise
    except Exception:
        # Verification errors (unexpected server-side faults) release the
        # reserved slot so they do not consume the client's failure budget.
        _release_login_attempt(key, marker)
        raise

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
