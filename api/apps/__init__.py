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
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path
from typing import Union

from apiflask import APIFlask, APIBlueprint, HTTPTokenAuth
from flask_cors import CORS
from flask_login import LoginManager
from flask_session import Session
from itsdangerous.url_safe import URLSafeTimedSerializer as Serializer
from werkzeug.wrappers.request import Request

from api.db import StatusEnum
from api.db.db_models import close_connection, APIToken
from api.db.services import UserService
from api.settings import API_VERSION, access_logger, RAG_FLOW_SERVICE_NAME
from api.settings import SECRET_KEY, stat_logger
from api.utils import CustomJSONEncoder, commands
from api.utils.api_utils import server_error_response

__all__ = ['app']

logger = logging.getLogger('flask.app')
for h in access_logger.handlers:
    logger.addHandler(h)

Request.json = property(lambda self: self.get_json(force=True, silent=True))

# Integrate APIFlask: Flask class -> APIFlask class.
app = APIFlask(__name__, title=RAG_FLOW_SERVICE_NAME, version=API_VERSION, docs_path=f'/{API_VERSION}/docs',
               spec_path=f'/{API_VERSION}/openapi.json')
# Integrate APIFlask: Use apiflask.HTTPTokenAuth for the HTTP Bearer or API Keys authentication.
http_token_auth = HTTPTokenAuth()


# Current logged-in user class
class AuthUser:
    def __init__(self, tenant_id, token):
        self.id = tenant_id
        self.token = token

    def get_token(self):
        return self.token


# Verify if the token is valid
@http_token_auth.verify_token
def verify_token(token: str) -> Union[AuthUser, None]:
    try:
        objs = APIToken.query(token=token)
        if objs:
            api_token = objs[0]
            user = AuthUser(api_token.tenant_id, api_token.token)
            return user
    except Exception as e:
        server_error_response(e)
    return None


CORS(app, supports_credentials=True, max_age=2592000)
app.url_map.strict_slashes = False
app.json_encoder = CustomJSONEncoder
app.errorhandler(Exception)(server_error_response)

## convince for dev and debug
# app.config["LOGIN_DISABLED"] = True
app.config["SESSION_PERMANENT"] = False
app.config["SESSION_TYPE"] = "filesystem"
app.config['MAX_CONTENT_LENGTH'] = int(os.environ.get("MAX_CONTENT_LENGTH", 128 * 1024 * 1024))

Session(app)
login_manager = LoginManager()
login_manager.init_app(app)

commands.register_commands(app)


def search_pages_path(pages_dir):
    app_path_list = [path for path in pages_dir.glob('*_app.py') if not path.name.startswith('.')]
    api_path_list = [path for path in pages_dir.glob('*sdk/*.py') if not path.name.startswith('.')]
    restful_api_path_list = [path for path in pages_dir.glob('*apis/*.py') if not path.name.startswith('.')]
    app_path_list.extend(api_path_list)
    app_path_list.extend(restful_api_path_list)
    return app_path_list


def register_page(page_path):
    path = f'{page_path}'

    page_name = page_path.stem.rstrip('_app')
    module_name = '.'.join(page_path.parts[page_path.parts.index('api'):-1] + (page_name,))

    spec = spec_from_file_location(module_name, page_path)
    page = module_from_spec(spec)
    page.app = app
    # Integrate APIFlask: Blueprint class -> APIBlueprint class
    page.manager = APIBlueprint(page_name, module_name)
    sys.modules[module_name] = page
    spec.loader.exec_module(page)
    page_name = getattr(page, 'page_name', page_name)
    if "/sdk/" in path or "/apis/" in path:
        url_prefix = f'/api/{API_VERSION}/{page_name}'
    # elif "/apis/" in path:
    #     url_prefix = f'/{API_VERSION}/api/{page_name}'
    else:
        url_prefix = f'/{API_VERSION}/{page_name}'

    app.register_blueprint(page.manager, url_prefix=url_prefix)
    return url_prefix


pages_dir = [
    Path(__file__).parent,
    Path(__file__).parent.parent / 'api' / 'apps',
    Path(__file__).parent.parent / 'api' / 'apps' / 'sdk',
    Path(__file__).parent.parent / 'api' / 'apps' / 'apis',
]

client_urls_prefix = [
    register_page(path)
    for dir in pages_dir
    for path in search_pages_path(dir)
]


@login_manager.request_loader
def load_user(web_request):
    jwt = Serializer(secret_key=SECRET_KEY)
    authorization = web_request.headers.get("Authorization")
    if authorization:
        try:
            access_token = str(jwt.loads(authorization))
            user = UserService.query(access_token=access_token, status=StatusEnum.VALID.value)
            if user:
                return user[0]
            else:
                return None
        except Exception as e:
            stat_logger.exception(e)
            return None
    else:
        return None


@app.teardown_request
def _db_close(exc):
    close_connection()
