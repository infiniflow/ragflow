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

# from beartype import BeartypeConf
# from beartype.claw import beartype_all  # <-- you didn't sign up for this
# beartype_all(conf=BeartypeConf(violation_type=UserWarning))    # <-- emit warnings from all code

import time
start_ts = time.time()

import logging
import os
import signal
import sys
import traceback
import threading
import uuid
import faulthandler

from api.apps import app
from api.db.runtime_config import RuntimeConfig
from api.db.services.document_service import DocumentService
from common.file_utils import get_project_base_directory
from common import settings
from api.db.db_models import init_database_tables as init_web_db
from api.db.init_data import init_web_data, init_superuser
from common.versions import get_ragflow_version
from common.config_utils import show_configs
from common.mcp_tool_call_conn import shutdown_all_mcp_sessions
from common.log_utils import init_root_logger
from agent.plugin import GlobalPluginManager
from rag.utils.redis_conn import RedisDistributedLock

stop_event = threading.Event()

RAGFLOW_DEBUGPY_LISTEN = int(os.environ.get('RAGFLOW_DEBUGPY_LISTEN', "0"))

def update_progress():
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

def signal_handler(sig, frame):
    logging.info("Received interrupt signal, shutting down...")
    shutdown_all_mcp_sessions()
    stop_event.set()
    stop_event.wait(1)
    sys.exit(0)

if __name__ == '__main__':
    faulthandler.enable()
    init_root_logger("ragflow_server")
    logging.info(r"""
        ____   ___    ______ ______ __
       / __ \ /   |  / ____// ____// /____  _      __
      / /_/ // /| | / / __ / /_   / // __ \| | /| / /
     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /
    /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/

    """)
    logging.info(
        f'RAGFlow version: {get_ragflow_version()}'
    )
    logging.info(
        f'project base: {get_project_base_directory()}'
    )
    show_configs()
    settings.init_settings()
    settings.print_rag_settings()

    if RAGFLOW_DEBUGPY_LISTEN > 0:
        logging.info(f"debugpy listen on {RAGFLOW_DEBUGPY_LISTEN}")
        import debugpy
        debugpy.listen(("0.0.0.0", RAGFLOW_DEBUGPY_LISTEN))

    # init db
    init_web_db()
    init_web_data()
    # init runtime config
    import argparse

    parser = argparse.ArgumentParser()
    parser.add_argument(
        "--version", default=False, help="RAGFlow version", action="store_true"
    )
    parser.add_argument(
        "--debug", default=False, help="debug mode", action="store_true"
    )
    parser.add_argument(
        "--init-superuser", default=False, help="init superuser", action="store_true"
    )
    args = parser.parse_args()
    if args.version:
        print(get_ragflow_version())
        sys.exit(0)

    if args.init_superuser:
        init_superuser()
    RuntimeConfig.DEBUG = args.debug
    if RuntimeConfig.DEBUG:
        logging.info("run on debug mode")

    RuntimeConfig.init_env()
    RuntimeConfig.init_config(JOB_SERVER_HOST=settings.HOST_IP, HTTP_PORT=settings.HOST_PORT)

    GlobalPluginManager.load_plugins()

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    def delayed_start_update_progress():
        logging.info("Starting update_progress thread (delayed)")
        t = threading.Thread(target=update_progress, daemon=True)
        t.start()

    if RuntimeConfig.DEBUG:
        if os.environ.get("WERKZEUG_RUN_MAIN") == "true":
            threading.Timer(1.0, delayed_start_update_progress).start()
    else:
        threading.Timer(1.0, delayed_start_update_progress).start()

    # start http server
    try:
        logging.info(f"RAGFlow server is ready after {time.time() - start_ts}s initialization.")
        app.run(host=settings.HOST_IP, port=settings.HOST_PORT)
    except Exception:
        traceback.print_exc()
        stop_event.set()
        stop_event.wait(1)
        os.kill(os.getpid(), signal.SIGKILL)
