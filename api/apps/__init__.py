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
import os
import sys
import logging
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path
from quart import Blueprint, Quart, request, g, current_app, session
from werkzeug.wrappers.request import Request
from flasgger import Swagger
from itsdangerous.url_safe import URLSafeTimedSerializer as Serializer
from quart_cors import cors
from common.constants import StatusEnum
from api.db.db_models import close_connection
from api.db.services import UserService
from api.utils.json_encode import CustomJSONEncoder
from api.utils import commands

from flask_mail import Mail
from quart_auth import Unauthorized
from common import settings
from api.utils.api_utils import server_error_response
from api.constants import API_VERSION
from common.misc_utils import get_uuid

settings.init_settings()

__all__ = ["app"]

Request.json = property(lambda self: self.get_json(force=True, silent=True))

app = Quart(__name__)
app = cors(app, allow_origin="*")
smtp_mail_server = Mail()

# Add this at the beginning of your file to configure Swagger UI
swagger_config = {
    "headers": [],
    "specs": [
        {
            "endpoint": "apispec",
            "route": "/apispec.json",
            "rule_filter": lambda rule: True,  # Include all endpoints
            "model_filter": lambda tag: True,  # Include all models
        }
    ],
    "static_url_path": "/flasgger_static",
    "swagger_ui": True,
    "specs_route": "/apidocs/",
}

swagger = Swagger(
    app,
    config=swagger_config,
    template={
        "swagger": "2.0",
        "info": {
            "title": "RAGFlow API",
            "description": "",
            "version": "1.0.0",
        },
        "securityDefinitions": {
            "ApiKeyAuth": {"type": "apiKey", "name": "Authorization", "in": "header"}
        },
    },
)

app.url_map.strict_slashes = False
app.json_encoder = CustomJSONEncoder
app.errorhandler(Exception)(server_error_response)

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
        return

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
    except Exception as e:
        logging.warning(f"load_user got exception {e}")


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
        if not current_user:# or not session.get("_user_id"):
            raise Unauthorized()
        else:
            return await current_app.ensure_async(func)(*args, **kwargs)

    return wrapper


def login_user(user, remember=False, duration=None, force=False, fresh=True):
    """
    Logs a user in. You should pass the actual user object to this. If the
    user's `is_active` property is ``False``, they will not be logged in
    unless `force` is ``True``.

    This will return ``True`` if the log in attempt succeeds, and ``False`` if
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


@app.teardown_request
def _db_close(exception):
    if exception:
        logging.exception(f"Request failed: {exception}")
    close_connection()
