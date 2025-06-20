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

# Gevent monkey patching - must be done before importing other modules
import os
if os.environ.get('GUNICORN_WORKER_CLASS') == 'gevent':
    from gevent import monkey
    monkey.patch_all()

    # Import gevent for greenlet spawning
    import gevent
    from gevent import spawn
    USE_GEVENT = True
else:
    USE_GEVENT = False

from api.utils.log_utils import initRootLogger
from plugin import GlobalPluginManager

# Initialize logging first
initRootLogger("ragflow_server")

import logging
import signal
import threading
import uuid
from concurrent.futures import ThreadPoolExecutor

from api import settings
from api.apps import app
from api.db.runtime_config import RuntimeConfig
from api.db.services.document_service import DocumentService
from api import utils

from api.db.db_models import init_database_tables as init_web_db
from api.db.init_data import init_web_data
from api.versions import get_ragflow_version
from api.utils import show_configs
from rag.settings import print_rag_settings
from rag.utils.redis_conn import RedisDistributedLock

# Global variables for background tasks
if USE_GEVENT:
    stop_event = None
    background_executor = None
    background_greenlet = None
else:
    stop_event = threading.Event()
    background_executor = None
    background_greenlet = None

RAGFLOW_DEBUGPY_LISTEN = int(os.environ.get('RAGFLOW_DEBUGPY_LISTEN', "0"))


def update_progress():
    """Background task to update document processing progress"""
    lock_value = str(uuid.uuid4())
    redis_lock = RedisDistributedLock("update_progress", lock_value=lock_value, timeout=60)
    logging.info(f"update_progress lock_value: {lock_value}")

    if USE_GEVENT:
        # Use gevent sleep and loop for greenlet compatibility
        while True:
            try:
                if redis_lock.acquire():
                    DocumentService.update_progress()
                    redis_lock.release()
                gevent.sleep(6)  # Use gevent.sleep instead of stop_event.wait
            except Exception:
                logging.exception("update_progress exception")
                redis_lock.release()
                break
    else:
        # Traditional threading approach
        while not stop_event.is_set():
            try:
                if redis_lock.acquire():
                    DocumentService.update_progress()
                    redis_lock.release()
                stop_event.wait(6)
            except Exception:
                logging.exception("update_progress exception")
            finally:
                redis_lock.release()


def signal_handler(sig, frame):
    """Handle shutdown signals gracefully"""
    logging.info("Received shutdown signal, stopping background tasks...")

    if USE_GEVENT:
        # Kill the background greenlet
        global background_greenlet
        if background_greenlet and not background_greenlet.dead:
            background_greenlet.kill()
    else:
        # Traditional threading approach
        stop_event.set()
        if hasattr(background_executor, 'shutdown'):
            background_executor.shutdown(wait=False)

    logging.info("Background tasks stopped")
    exit(0)


def initialize_ragflow():
    """Initialize RAGFlow application"""
    global background_executor

    logging.info(r"""
        ____   ___    ______ ______ __               
       / __ \ /   |  / ____// ____// /____  _      __
      / /_/ // /| | / / __ / /_   / // __ \| | /| / /
     / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ / 
    /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/                             

    """)
    logging.info(f'RAGFlow version: {get_ragflow_version()}')
    logging.info(f'project base: {utils.file_utils.get_project_base_directory()}')

    show_configs()
    settings.init_settings()
    print_rag_settings()

    if RAGFLOW_DEBUGPY_LISTEN > 0:
        logging.info(f"debugpy listen on {RAGFLOW_DEBUGPY_LISTEN}")
        import debugpy
        debugpy.listen(("0.0.0.0", RAGFLOW_DEBUGPY_LISTEN))

    # Initialize database
    init_web_db()
    init_web_data()

    # Initialize runtime config
    RuntimeConfig.DEBUG = False  # Force production mode for WSGI
    RuntimeConfig.init_env()
    RuntimeConfig.init_config(JOB_SERVER_HOST=settings.HOST_IP, HTTP_PORT=settings.HOST_PORT)

    # Load plugins
    GlobalPluginManager.load_plugins()

    # Set up signal handlers
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    # Start background progress update task
    if USE_GEVENT:
        # Use gevent spawn for greenlet-based execution
        global background_greenlet
        background_greenlet = spawn(update_progress)
        logging.info("Started document progress update task in gevent mode")
    else:
        # Use thread pool for traditional threading
        background_executor = ThreadPoolExecutor(max_workers=1)
        background_executor.submit(update_progress)
        logging.info("Started document progress update task in threading mode")

    logging.info("RAGFlow WSGI application initialized successfully in production mode")


# Initialize the application when module is imported
initialize_ragflow()

# Export the Flask app for WSGI
application = app

if __name__ == '__main__':
    # This should not be used in production
    logging.warning("Running WSGI module directly - this is not recommended for production")
    from werkzeug.serving import run_simple

    run_simple(
        hostname=settings.HOST_IP,
        port=settings.HOST_PORT,
        application=app,
        threaded=True,
        use_reloader=False,
        use_debugger=False,
    )