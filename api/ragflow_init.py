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
import threading
import uuid

from api.apps import app, smtp_mail_server
from api.db.runtime_config import RuntimeConfig
from api.db.services.document_service import DocumentService
from common.file_utils import get_project_base_directory
from common import settings
from api.db.db_models import init_database_tables as init_web_db
from api.db.init_data import init_web_data
from common.versions import get_ragflow_version
from common.config_utils import show_configs
from plugin import GlobalPluginManager
from rag.utils.redis_conn import RedisDistributedLock
from common.log_utils import init_root_logger

# Shared stop event for background tasks
stop_event = threading.Event()


def update_progress():
    """Background task to update document processing progress"""
    lock_value = str(uuid.uuid4())
    redis_lock = RedisDistributedLock("update_progress", lock_value=lock_value, timeout=60)
    logging.info(f"update_progress lock_value: {lock_value}")
    while not stop_event.is_set():
        try:
            if redis_lock.acquire():
                DocumentService.update_progress()
                redis_lock.release()
        except Exception:
            logging.exception("update_progress exception")
        finally:
            try:
                redis_lock.release()
            except Exception:
                logging.exception("update_progress exception")
            stop_event.wait(6)


def init_logging(logger_name="ragflow_server"):
    """Initialize logging system"""
    init_root_logger(logger_name)


def print_startup_banner():
    """Print RAGFlow startup banner"""
    logging.info(r"""
    ____   ___    ______ ______ __
   / __ \ /   |  / ____// ____// /____  _      __
  / /_/ // /| | / / __ / /_   / // __ \| | /| / /
 / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /
/_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/
""")
    logging.info(f"RAGFlow version: {get_ragflow_version()}")
    logging.info(f"project base: {get_project_base_directory()}")


def init_debugpy():
    """Initialize debugpy if RAGFLOW_DEBUGPY_LISTEN is set"""
    ragflow_debugpy_listen = int(os.environ.get("RAGFLOW_DEBUGPY_LISTEN", "0"))
    if ragflow_debugpy_listen > 0:
        logging.info(f"debugpy listen on {ragflow_debugpy_listen}")
        import debugpy

        debugpy.listen(("0.0.0.0", ragflow_debugpy_listen))


def init_smtp():
    """Initialize SMTP mail server configuration"""
    if settings.SMTP_CONF:
        app.config["MAIL_SERVER"] = settings.MAIL_SERVER
        app.config["MAIL_PORT"] = settings.MAIL_PORT
        app.config["MAIL_USE_SSL"] = settings.MAIL_USE_SSL
        app.config["MAIL_USE_TLS"] = settings.MAIL_USE_TLS
        app.config["MAIL_USERNAME"] = settings.MAIL_USERNAME
        app.config["MAIL_PASSWORD"] = settings.MAIL_PASSWORD
        app.config["MAIL_DEFAULT_SENDER"] = settings.MAIL_DEFAULT_SENDER
        smtp_mail_server.init_app(app)


def init_ragflow(debug_mode=False):
    """
    Initialize RAGFlow application with all common initialization steps

    Args:
        debug_mode: Whether to run in debug mode (default: False)
    """
    # 1. Initialize Logging
    init_logging("ragflow_server")

    # 2. Print startup banner and initialize settings
    print_startup_banner()
    show_configs()
    settings.init_settings()
    settings.print_rag_settings()

    # 3. Check for debugpy
    init_debugpy()

    # 4. Initialize DB and Data
    init_web_db()
    init_web_data()

    # 5. Initialize Runtime Config
    RuntimeConfig.DEBUG = debug_mode
    if RuntimeConfig.DEBUG:
        logging.info("run on debug mode")
    RuntimeConfig.init_env()
    RuntimeConfig.init_config(JOB_SERVER_HOST=settings.HOST_IP, HTTP_PORT=settings.HOST_PORT)

    # 6. Load Plugins
    GlobalPluginManager.load_plugins()

    # 7. Initialize SMTP
    init_smtp()


def start_update_progress_thread(delayed=False, delay_seconds=1.0):
    """
    Start the update_progress background thread

    Args:
        delayed: Whether to delay the start (default: False)
        delay_seconds: Delay in seconds if delayed=True (default: 1.0)
    """
    if delayed:

        def delayed_start():
            logging.info("Starting update_progress thread (delayed)")
            t = threading.Thread(target=update_progress, daemon=True)
            t.start()

        threading.Timer(delay_seconds, delayed_start).start()
    else:
        logging.info("Starting background tasks (update_progress)...")
        t = threading.Thread(target=update_progress, daemon=True)
        t.start()
