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
import logging
import os
import sys
import time
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path
from quart import Blueprint, Quart, request, g, current_app, session, jsonify
from itsdangerous.url_safe import URLSafeTimedSerializer as Serializer
from quart_cors import cors
from common.constants import StatusEnum, RetCode
from api.db.db_models import close_connection, APIToken
from api.db.services import UserService
from api.utils.json_encode import CustomJSONEncoder
from api.utils import commands

from quart_auth import Unauthorized as QuartAuthUnauthorized
from werkzeug.exceptions import Unauthorized as WerkzeugUnauthorized
from quart_schema import QuartSchema
from common import settings
from api.utils.api_utils import server_error_response, get_json_result
from api.constants import API_VERSION
from common.misc_utils import get_uuid

settings.init_settings()

__all__ = ["app"]

UNAUTHORIZED_MESSAGE = "<Unauthorized '401: Unauthorized'>"


def _unauthorized_message(error):
    if error is None:
        return UNAUTHORIZED_MESSAGE
    try:
        msg = repr(error)
    except Exception:
        return UNAUTHORIZED_MESSAGE
    if msg == UNAUTHORIZED_MESSAGE:
        return msg
    if "Unauthorized" in msg and "401" in msg:
        return msg
    return UNAUTHORIZED_MESSAGE

app = Quart(__name__)
app = cors(app, allow_origin="*")

# openapi supported
QuartSchema(app)

app.url_map.strict_slashes = False
app.json_encoder = CustomJSONEncoder
app.errorhandler(Exception)(server_error_response)

# Configure Quart timeouts for slow LLM responses (e.g., local Ollama on CPU)
# Default Quart timeouts are 60 seconds which is too short for many LLM backends
app.config["RESPONSE_TIMEOUT"] = int(os.environ.get("QUART_RESPONSE_TIMEOUT", 600))
app.config["BODY_TIMEOUT"] = int(os.environ.get("QUART_BODY_TIMEOUT", 600))

## convince for dev and debug
# app.config["LOGIN_DISABLED"] = True
app.config["SESSION_PERMANENT"] = False
app.config["SESSION_TYPE"] = "redis"
app.config["SESSION_REDIS"] = settings.decrypt_database_config(name="redis")
app.config["MAX_CONTENT_LENGTH"] = int(
    os.environ.get("MAX_CONTENT_LENGTH", 1024 * 1024 * 1024)
)
app.config['SECRET_KEY'] = settings.SECRET_KEY
app.secret_key = settings.SECRET_KEY
commands.register_commands(app)

from functools import wraps
from typing import ParamSpec, TypeVar
from collections.abc import Awaitable, Callable
from werkzeug.local import LocalProxy

T = TypeVar("T")
P = ParamSpec("P")


def _load_user():
    jwt = Serializer(secret_key=settings.SECRET_KEY)
    authorization = request.headers.get("Authorization")
    g.user = None
    if not authorization:
        return None

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
            g.user = user[0]
            return user[0]
    except Exception as e_auth:
        logging.warning(f"load_user got exception {e_auth}")
        try:
            authorization = request.headers.get("Authorization")
            if len(authorization.split()) == 2:
                objs = APIToken.query(token=authorization.split()[1])
                if objs:
                    user = UserService.query(id=objs[0].tenant_id, status=StatusEnum.VALID.value)
                    if user:
                        if not user[0].access_token or not user[0].access_token.strip():
                            logging.warning(f"User {user[0].email} has empty access_token in database")
                            return None
                        g.user = user[0]
                        return user[0]
        except Exception as e_api_token:
            logging.warning(f"load_user got exception {e_api_token}")


current_user = LocalProxy(_load_user)


def login_required(func: Callable[P, Awaitable[T]]) -> Callable[P, Awaitable[T]]:
    """A decorator to restrict route access to authenticated users.

    This should be used to wrap a route handler (or view function) to
    enforce that only authenticated requests can access it. Note that
    it is important that this decorator be wrapped by the route
    decorator and not vice, versa, as below.

    .. code-block:: python

        @app.route('/')
        @login_required
        async def index():
            ...

    If the request is not authenticated a
    `quart.exceptions.Unauthorized` exception will be raised.

    """

    @wraps(func)
    async def wrapper(*args: P.args, **kwargs: P.kwargs) -> T:
        timing_enabled = os.getenv("RAGFLOW_API_TIMING")
        t_start = time.perf_counter() if timing_enabled else None
        user = current_user
        if timing_enabled:
            logging.info(
                "api_timing login_required auth_ms=%.2f path=%s",
                (time.perf_counter() - t_start) * 1000,
                request.path,
            )
        if not user:  # or not session.get("_user_id"):
            raise QuartAuthUnauthorized()
        return await current_app.ensure_async(func)(*args, **kwargs)

    return wrapper


def login_user(user, remember=False, duration=None, force=False, fresh=True):
    """
    Logs a user in. You should pass the actual user object to this. If the
    user's `is_active` property is ``False``, they will not be logged in
    unless `force` is ``True``.

    This will return ``True`` if the login attempt succeeds, and ``False`` if
    it fails (i.e. because the user is inactive).

    :param user: The user object to log in.
    :type user: object
    :param remember: Whether to remember the user after their session expires.
        Defaults to ``False``.
    :type remember: bool
    :param duration: The amount of time before the remember cookie expires. If
        ``None`` the value set in the settings is used. Defaults to ``None``.
    :type duration: :class:`datetime.timedelta`
    :param force: If the user is inactive, setting this to ``True`` will log
        them in regardless. Defaults to ``False``.
    :type force: bool
    :param fresh: setting this to ``False`` will log in the user with a session
        marked as not "fresh". Defaults to ``True``.
    :type fresh: bool
    """
    if not force and not user.is_active:
        return False

    session["_user_id"] = user.id
    session["_fresh"] = fresh
    session["_id"] = get_uuid()
    return True


def logout_user():
    """
    Logs a user out. (You do not need to pass the actual user.) This will
    also clean up the remember me cookie if it exists.
    """
    if "_user_id" in session:
        session.pop("_user_id")

    if "_fresh" in session:
        session.pop("_fresh")

    if "_id" in session:
        session.pop("_id")

    COOKIE_NAME = "remember_token"
    cookie_name = current_app.config.get("REMEMBER_COOKIE_NAME", COOKIE_NAME)
    if cookie_name in request.cookies:
        session["_remember"] = "clear"
        if "_remember_seconds" in session:
            session.pop("_remember_seconds")

    return True


def search_pages_path(page_path):
    app_path_list = [
        path for path in page_path.glob("*_app.py") if not path.name.startswith(".")
    ]
    api_path_list = [
        path for path in page_path.glob("*sdk/*.py") if not path.name.startswith(".")
    ]
    app_path_list.extend(api_path_list)
    return app_path_list


def register_page(page_path):
    path = f"{page_path}"

    page_name = page_path.stem.removesuffix("_app")
    module_name = ".".join(
        page_path.parts[page_path.parts.index("api"): -1] + (page_name,)
    )

    spec = spec_from_file_location(module_name, page_path)
    page = module_from_spec(spec)
    page.app = app
    page.manager = Blueprint(page_name, module_name)
    sys.modules[module_name] = page
    spec.loader.exec_module(page)
    page_name = getattr(page, "page_name", page_name)
    sdk_path = "\\sdk\\" if sys.platform.startswith("win") else "/sdk/"
    url_prefix = (
        f"/api/{API_VERSION}" if sdk_path in path else f"/{API_VERSION}/{page_name}"
    )

    app.register_blueprint(page.manager, url_prefix=url_prefix)
    return url_prefix


pages_dir = [
    Path(__file__).parent,
    Path(__file__).parent.parent / "api" / "apps",
    Path(__file__).parent.parent / "api" / "apps" / "sdk",
]

client_urls_prefix = [
    register_page(path) for directory in pages_dir for path in search_pages_path(directory)
]


@app.errorhandler(404)
async def not_found(error):
    logging.error(f"The requested URL {request.path} was not found")
    message = f"Not Found: {request.path}"
    response = {
        "code": RetCode.NOT_FOUND,
        "message": message,
        "data": None,
        "error": "Not Found",
    }
    return jsonify(response), RetCode.NOT_FOUND


@app.errorhandler(401)
async def unauthorized(error):
    logging.warning("Unauthorized request")
    return get_json_result(code=RetCode.UNAUTHORIZED, message=_unauthorized_message(error)), RetCode.UNAUTHORIZED


@app.errorhandler(QuartAuthUnauthorized)
async def unauthorized_quart_auth(error):
    logging.warning("Unauthorized request (quart_auth)")
    return get_json_result(code=RetCode.UNAUTHORIZED, message=repr(error)), RetCode.UNAUTHORIZED


@app.errorhandler(WerkzeugUnauthorized)
async def unauthorized_werkzeug(error):
    logging.warning("Unauthorized request (werkzeug)")
    return get_json_result(code=RetCode.UNAUTHORIZED, message=_unauthorized_message(error)), RetCode.UNAUTHORIZED

@app.teardown_request
def _db_close(exception):
    if exception:
        logging.exception(f"Request failed: {exception}")
    close_connection()
