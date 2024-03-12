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
import sys
from importlib.util import module_from_spec, spec_from_file_location
from pathlib import Path
from flask import Blueprint, Flask, request
from werkzeug.wrappers.request import Request
from flask_cors import CORS

from api.db import StatusEnum
from api.db.services import UserService
from api.utils import CustomJSONEncoder

from flask_session import Session
from flask_login import LoginManager
from api.settings import RetCode, SECRET_KEY, stat_logger
from api.settings import API_VERSION, CLIENT_AUTHENTICATION, SITE_AUTHENTICATION, access_logger
from api.utils.api_utils import get_json_result, server_error_response
from itsdangerous.url_safe import URLSafeTimedSerializer as Serializer

__all__ = ['app']


logger = logging.getLogger('flask.app')
for h in access_logger.handlers:
    logger.addHandler(h)

Request.json = property(lambda self: self.get_json(force=True, silent=True))

app = Flask(__name__)
CORS(app, supports_credentials=True,max_age = 2592000)
app.url_map.strict_slashes = False
app.json_encoder = CustomJSONEncoder
app.errorhandler(Exception)(server_error_response)


## convince for dev and debug
#app.config["LOGIN_DISABLED"] = True
app.config["SESSION_PERMANENT"] = False
app.config["SESSION_TYPE"] = "filesystem"
app.config['MAX_CONTENT_LENGTH'] = 128 * 1024 * 1024

Session(app)
login_manager = LoginManager()
login_manager.init_app(app)



def search_pages_path(pages_dir):
    return [path for path in pages_dir.glob('*_app.py') if not path.name.startswith('.')]


def register_page(page_path):
    page_name = page_path.stem.rstrip('_app')
    module_name = '.'.join(page_path.parts[page_path.parts.index('api'):-1] + (page_name, ))

    spec = spec_from_file_location(module_name, page_path)
    page = module_from_spec(spec)
    page.app = app
    page.manager = Blueprint(page_name, module_name)
    sys.modules[module_name] = page
    spec.loader.exec_module(page)

    page_name = getattr(page, 'page_name', page_name)
    url_prefix = f'/{API_VERSION}/{page_name}'

    app.register_blueprint(page.manager, url_prefix=url_prefix)
    return url_prefix


pages_dir = [
    Path(__file__).parent,
    Path(__file__).parent.parent / 'api' / 'apps',
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