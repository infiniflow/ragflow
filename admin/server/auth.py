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
import uuid
from functools import wraps
from datetime import datetime

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
# delivered through a 0600 bootstrap file: the admin panel manages sandbox providers
# (including the "local" provider, i.e. host-level execution), so a guessable
# default here would effectively be host RCE.
ADMIN_DEFAULT_PASSWORD_ENV = "ADMIN_DEFAULT_PASSWORD"
DEFAULT_SUPERUSER_PASSWORD_ENV = "DEFAULT_SUPERUSER_PASSWORD"


# The admin server reports DEFAULT_SUPERUSER_EMAIL via
# `services.EnvironmentsMgr.get_all()` and the admin CLI's "list envs"
# output, so operators reasonably expect the env value to drive the
# bootstrap admin account's email. (DEFAULT_SUPERUSER_PASSWORD is
# deliberately NOT exposed through the env-listing API — it would
# leak the bootstrap credential into any audit log that records the
# full env map.) Read once at module load so the bootstrap functions
# below can reference the values as module-level constants, matching
# `api/db/init_data.py:init_superuser`. See infiniflow/ragflow#16876.
#
# Reject empty env values explicitly: ``os.getenv(name, fallback)``
# only uses the fallback when the variable is *unset*, so a misconfigured
# operator setting ``DEFAULT_SUPERUSER_EMAIL=`` would otherwise
# propagate an empty email into the bootstrap flow. We deliberately
# preserve raw password values (no whitespace stripping) so a password
# like ``"  s3cret  "`` keeps its leading/trailing whitespace and isn't
# silently mutated before being hashed/stored. Email/nickname strip
# separately because whitespace there would be invalid.
def _required_env(name, default):
    value = os.getenv(name, default)
    if not value:
        logging.error(
            "admin server: %s is set but empty; either unset it (the default %r will be used) or provide a real value",
            name,
            default,
        )
        raise RuntimeError(f"admin server: {name} is set but empty; either unset it (the default {default!r} will be used) or provide a real value")
    return value


def _required_env_stripped(name, default):
    """Like ``_required_env`` but rejects whitespace-only values.

    A raw value of ``" "`` would survive the empty-check in
    ``_required_env`` and then collapse to ``""`` after the caller
    ``.strip()``s it, propagating an empty nickname/email into the
    bootstrap flow and silently failing later checks. Strip before
    the empty check so the failure surfaces at startup with the
    intended config-error message."""
    raw = os.getenv(name, default)
    stripped = raw.strip()
    if not stripped:
        logging.error(
            "admin server: %s is set to whitespace-only value; either unset it (the default %r will be used) or provide a real value",
            name,
            default,
        )
        raise RuntimeError(f"admin server: {name} is set to whitespace-only value; either unset it (the default {default!r} will be used) or provide a real value")
    return stripped


DEFAULT_SUPERUSER_NICKNAME = _required_env_stripped("DEFAULT_SUPERUSER_NICKNAME", "admin")
DEFAULT_SUPERUSER_EMAIL = _required_env_stripped("DEFAULT_SUPERUSER_EMAIL", "admin@ragflow.io")
# NOTE: there is deliberately no DEFAULT_SUPERUSER_PASSWORD module constant
# here. The bootstrap password resolves at startup via
# _resolve_bootstrap_admin_password() — ADMIN_DEFAULT_PASSWORD, then
# DEFAULT_SUPERUSER_PASSWORD, then a random password delivered through a
# 0600 file — instead of falling back to the literal "admin" that
# api/db/init_data.py still uses.


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
            "nickname": DEFAULT_SUPERUSER_NICKNAME,
            "is_superuser": True,
            "email": DEFAULT_SUPERUSER_EMAIL,
            "creator": "system",
            "status": "1",
        }
        if not UserService.save(**default_admin):
            raise AdminException("Can't init admin.", 500)
        add_tenant_for_admin(default_admin, UserTenantRole.OWNER)
        if env_name:
            logging.info(f"Created default superuser {DEFAULT_SUPERUSER_EMAIL} with the password from {env_name}. Change it after the first login.")
        else:
            password_file = _write_bootstrap_password_file(password)
            if password_file:
                logging.warning(
                    f"No superuser found and neither {ADMIN_DEFAULT_PASSWORD_ENV} nor {DEFAULT_SUPERUSER_PASSWORD_ENV} is set. "
                    f"Created default superuser {DEFAULT_SUPERUSER_EMAIL}; its randomly generated password was written once to {password_file} (mode 0600). Read it there and change it immediately after the first login."
                )
            else:
                logging.error(
                    f"No superuser found and neither {ADMIN_DEFAULT_PASSWORD_ENV} nor {DEFAULT_SUPERUSER_PASSWORD_ENV} is set, "
                    f"and the bootstrap password file could not be written. The randomly generated password for {DEFAULT_SUPERUSER_EMAIL} is NOT recoverable: "
                    f"delete the account and re-run with {ADMIN_DEFAULT_PASSWORD_ENV} set."
                )
    elif not any([u.is_active == ActiveEnum.ACTIVE.value for u in users]):
        raise AdminException("No active admin. Please update 'is_active' in db manually.", 500)
    else:
        # Filter existing superuser rows by the configured
        # ``DEFAULT_SUPERUSER_EMAIL``. Per @Lynn-Inf's review on
        # PR #17674, this intentionally does NOT also filter on
        # ``is_active`` — an inactive row matching the configured
        # email must still be treated as the configured bootstrap
        # admin, otherwise the tenant-backfill branch above could
        # silently create a duplicate entry for the (already-existing,
        # just-disabled) default admin. Activity gating already lives
        # in ``login_admin`` (``is_active == INACTIVE`` rejects the
        # session at login time), so the bootstrap path stays
        # read-only.
        default_admin_rows = [u for u in users if u.email == DEFAULT_SUPERUSER_EMAIL]
        if default_admin_rows:
            default_admin = default_admin_rows[0].to_dict()
            exist, default_admin_tenant = TenantService.get_by_id(default_admin["id"])
            if not exist:
                add_tenant_for_admin(default_admin, UserTenantRole.OWNER)
        elif any(u.email != DEFAULT_SUPERUSER_EMAIL for u in users):
            # Active superuser(s) exist but none match the configured
            # DEFAULT_SUPERUSER_EMAIL — the operator set the env var to a
            # value that doesn't match any existing admin row. Don't
            # silently fall back to the original "admin@ragflow.io"
            # identity (that would diverge the bootstrap credential
            # from the env), and don't auto-create a new admin (the
            # user may have intentionally kept the old admin). Surface
            # the mismatch with explicit migration guidance.
            existing_emails = sorted({u.email for u in users})
            logging.error(
                "admin server: configured DEFAULT_SUPERUSER_EMAIL (%r) "
                "does not match any active superuser row; existing admin "
                "emails: %r. Refusing to bootstrap to avoid diverging the "
                "credential from the env value.",
                DEFAULT_SUPERUSER_EMAIL,
                existing_emails,
            )
            raise AdminException(
                f"Active superuser(s) exist but none match DEFAULT_SUPERUSER_EMAIL "
                f"({DEFAULT_SUPERUSER_EMAIL!r}). Either unset the env var to "
                f"keep the existing admin, or run the API-side migration "
                f"(``python -m api.ragflow_server --init-superuser``) to align "
                f"the configured email with an existing admin row before "
                f"restarting.",
                500,
            )


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
    users = UserService.query(email=email)
    if not users:
        raise UserNotFoundError(email)
    try:
        decrypted = decrypt(password)
    except CryptPayloadError:
        # A payload the client could not encode correctly is the client's
        # failure, not a server fault: answer with the same generic mismatch
        # as any other credential rejection. Server-side faults
        # (missing/invalid private key) propagate to the caller instead.
        logging.info("Admin login with undecryptable password payload.")
        raise AdminException("Email and password do not match!")
    user = UserService.query_user(email, decrypted)
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
    user.last_login_time = get_format_time()
    user.save()
    msg = "Welcome back!"
    return sync_construct_response(data=resp, auth=user.get_id(), message=msg)
