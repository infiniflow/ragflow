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

import time
start_ts = time.time()

import os
import signal
import logging
import threading
import traceback
import faulthandler

from flask import Flask
from flask_login import LoginManager
from werkzeug.serving import run_simple
from routes import admin_bp
from common.log_utils import init_root_logger
from common.constants import SERVICE_CONF
from common.config_utils import show_configs
from common import settings
from config import load_configurations, SERVICE_CONFIGS
from auth import init_default_admin, setup_auth
from flask_session import Session
from common.versions import get_ragflow_version

stop_event = threading.Event()

if __name__ == '__main__':
    faulthandler.enable()
    init_root_logger("admin_service")
    logging.info(r"""
        ____  ___   ______________                 ___       __          _     
       / __ \/   | / ____/ ____/ /___ _      __   /   | ____/ /___ ___  (_)___ 
      / /_/ / /| |/ / __/ /_  / / __ \ | /| / /  / /| |/ __  / __ `__ \/ / __ \
     / _, _/ ___ / /_/ / __/ / / /_/ / |/ |/ /  / ___ / /_/ / / / / / / / / / /
    /_/ |_/_/  |_\____/_/   /_/\____/|__/|__/  /_/  |_\__,_/_/ /_/ /_/_/_/ /_/ 
    """)

    app = Flask(__name__)
    app.register_blueprint(admin_bp)
    app.config["SESSION_PERMANENT"] = False
    app.config["SESSION_TYPE"] = "filesystem"
    app.config["MAX_CONTENT_LENGTH"] = int(
        os.environ.get("MAX_CONTENT_LENGTH", 1024 * 1024 * 1024)
    )
    Session(app)
    logging.info(f'RAGFlow version: {get_ragflow_version()}')
    show_configs()
    login_manager = LoginManager()
    login_manager.init_app(app)
    settings.init_settings()
    setup_auth(login_manager)
    init_default_admin()
    SERVICE_CONFIGS.configs = load_configurations(SERVICE_CONF)

    try:
        logging.info(f"RAGFlow admin is ready after {time.time() - start_ts}s initialization.")
        run_simple(
            hostname="0.0.0.0",
            port=9381,
            application=app,
            threaded=True,
            use_reloader=False,
            use_debugger=True,
        )
    except Exception:
        traceback.print_exc()
        stop_event.set()
        time.sleep(1)
        os.kill(os.getpid(), signal.SIGKILL)
