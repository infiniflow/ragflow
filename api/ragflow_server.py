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

import logging
import os
import signal
import sys
import traceback
import faulthandler
import argparse

from api.apps import app
from api.db.init_data import init_superuser
from api.db.runtime_config import RuntimeConfig
from api.ragflow_init import init_ragflow, stop_event, start_update_progress_thread
from common.mcp_tool_call_conn import shutdown_all_mcp_sessions
from common import settings
from common.versions import get_ragflow_version


def signal_handler(sig, frame):
    logging.info("Received interrupt signal, shutting down...")
    shutdown_all_mcp_sessions()
    stop_event.set()
    stop_event.wait(1)
    sys.exit(0)


if __name__ == "__main__":
    faulthandler.enable()

    # Parse command line arguments
    parser = argparse.ArgumentParser()
    parser.add_argument("--version", default=False, help="RAGFlow version", action="store_true")
    parser.add_argument("--debug", default=False, help="debug mode", action="store_true")
    parser.add_argument("--init-superuser", default=False, help="init superuser", action="store_true")
    args = parser.parse_args()

    if args.version:
        print(get_ragflow_version())
        sys.exit(0)

    if args.init_superuser:
        init_superuser()

    # Initialize RAGFlow application with debug mode
    init_ragflow(debug_mode=args.debug)

    # Setup signal handlers
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    # Start background task with delay
    # In debug mode, only start if WERKZEUG_RUN_MAIN is true (to avoid duplicate threads)
    if RuntimeConfig.DEBUG:
        if os.environ.get("WERKZEUG_RUN_MAIN") == "true":
            start_update_progress_thread(delayed=True, delay_seconds=1.0)
    else:
        start_update_progress_thread(delayed=True, delay_seconds=1.0)

    # start http server
    try:
        logging.info("RAGFlow HTTP server start...")
        app.run(host=settings.HOST_IP, port=settings.HOST_PORT)
    except Exception:
        traceback.print_exc()
        stop_event.set()
        stop_event.wait(1)
        os.kill(os.getpid(), signal.SIGKILL)
