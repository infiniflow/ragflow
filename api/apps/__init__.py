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
from flask import Blueprint, Flask
from werkzeug.wrappers.request import Request
from flask_cors import CORS

from api.db import StatusEnum
from api.db.db_models import close_connection
from api.db.services import UserService
from api.utils import CustomJSONEncoder

from flask_session import Session
from flask_login import LoginManager
from api.settings import POCKETBASE_HOST
from api.settings import SECRET_KEY, stat_logger
from api.settings import API_VERSION, access_logger
from api.utils.api_utils import server_error_response
from itsdangerous.url_safe import URLSafeTimedSerializer as Serializer
from pocketbase import PocketBase
from api.db.db_models import User
from api.utils.user_utils import user_register, rollback_user_registration
from api.utils import get_format_time
from threading import Lock
registration_lock = Lock()

__all__ = ['app']


logger = logging.getLogger('flask.app')
for h in access_logger.handlers:
    logger.addHandler(h)

Request.json = property(lambda self: self.get_json(force=True, silent=True))

app = Flask(__name__)
CORS(app, supports_credentials=True,max_age=2592000)
app.url_map.strict_slashes = False
app.json_encoder = CustomJSONEncoder
app.errorhandler(Exception)(server_error_response)


## convince for dev and debug
#app.config["LOGIN_DISABLED"] = True
app.config["SESSION_PERMANENT"] = False
app.config["SESSION_TYPE"] = "filesystem"
app.config['MAX_CONTENT_LENGTH'] = int(os.environ.get("MAX_CONTENT_LENGTH", 128 * 1024 * 1024))

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

# @login_manager.request_loader
# def load_user(web_request):
#     jwt = Serializer(secret_key=SECRET_KEY)
#     authorization = web_request.headers.get("Authorization")
#     if authorization:
#         try:
#             access_token = str(jwt.loads(authorization))
#             user = UserService.query(access_token=access_token, status=StatusEnum.VALID.value)
#             if user:
#                 return user[0]
#             else:
#                 return None
#         except Exception as e:
#             stat_logger.exception(e)
#             return None
#     else:
#         return None

# Integrate with Penless
@login_manager.request_loader
def load_user(web_request):
    pb = PocketBase(POCKETBASE_HOST)
    jwt = Serializer(secret_key=SECRET_KEY)
    authorization = web_request.headers.get("Authorization")
    stat_logger.warning(authorization)
    if authorization:
        try:
            pb.auth_store.base_token=str(jwt.loads(authorization))
            pb.collection("users").authRefresh()

            if pb.auth_store.model and pb.auth_store.model.id:
                user = User(
                    id=pb.auth_store.model.id,
                    nickname=pb.auth_store.model.username,
                    email=pb.auth_store.model.email,
                    access_token=pb.auth_store.token
                )
                
                userInDb = UserService.filter_by_id(user.id)
                if userInDb is None:
                    # Synchronize this section to prevent race conditions
                    with registration_lock:
                        # Re-check if user is already registered after acquiring the lock to avoid race condition
                        userInDb = UserService.filter_by_id(user.id)
                        if userInDb is not None:
                            return user
                        stat_logger.info(f"User {user.email} not in local DB, which comes from penless, register it")
                        user_dict = {
                            "access_token": user.access_token,
                            "email": user.email,
                            "nickname": user.nickname,
                            "password": '',
                            "login_channel": "password",
                            "last_login_time": get_format_time(),
                            "is_superuser": False,
                        }

                        user_id = user.id
                        try:
                            users = user_register(user_id, user_dict)
                            if not users:
                                raise Exception('Register user failure.')
                            if len(users) > 1:
                                raise Exception('Same E-mail exist!')
                            # User registration logic goes here
                        except Exception as e:
                            rollback_user_registration(user_id)
                            stat_logger.exception(e)
                            return None
                return user
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