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
import signal
import sys
import traceback
from werkzeug.serving import run_simple
from api.apps import app
from api.db.runtime_config import RuntimeConfig
from api.settings import (
    HOST, HTTP_PORT, access_logger, database_logger, stat_logger,
)
from api import utils

from api.db.db_models import init_database_tables as init_web_db
from api.db.init_data import init_web_data
from api.versions import get_versions

if __name__ == '__main__':
    print("""
    ____                 ______ __               
   / __ \ ____ _ ____ _ / ____// /____  _      __
  / /_/ // __ `// __ `// /_   / // __ \| | /| / /
 / _, _// /_/ // /_/ // __/  / // /_/ /| |/ |/ / 
/_/ |_| \__,_/ \__, //_/    /_/ \____/ |__/|__/  
              /____/                             

    """, flush=True)
    stat_logger.info(
        f'project base: {utils.file_utils.get_project_base_directory()}'
    )

    # init db
    init_web_db()
    init_web_data()
    # init runtime config
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('--version', default=False, help="rag flow version", action='store_true')
    parser.add_argument('--debug', default=False, help="debug mode", action='store_true')
    args = parser.parse_args()
    if args.version:
        print(get_versions())
        sys.exit(0)

    RuntimeConfig.DEBUG = args.debug
    if RuntimeConfig.DEBUG:
        stat_logger.info("run on debug mode")

    RuntimeConfig.init_env()
    RuntimeConfig.init_config(JOB_SERVER_HOST=HOST, HTTP_PORT=HTTP_PORT)

    peewee_logger = logging.getLogger('peewee')
    peewee_logger.propagate = False
    # rag_arch.common.log.ROpenHandler
    peewee_logger.addHandler(database_logger.handlers[0])
    peewee_logger.setLevel(database_logger.level)

    # start http server
    try:
        stat_logger.info("RAG Flow http server start...")
        werkzeug_logger = logging.getLogger("werkzeug")
        for h in access_logger.handlers:
            werkzeug_logger.addHandler(h)
        run_simple(hostname=HOST, port=HTTP_PORT, application=app, threaded=True, use_reloader=RuntimeConfig.DEBUG, use_debugger=RuntimeConfig.DEBUG)
    except Exception:
        traceback.print_exc()
        os.kill(os.getpid(), signal.SIGKILL)