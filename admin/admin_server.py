
import os
import signal
import logging
import time
import threading
import traceback
from werkzeug.serving import run_simple
from flask import Flask
from routes import admin_bp
from api.utils.log_utils import init_root_logger
from api.constants import SERVICE_CONF
from api import settings
from config import load_configurations, SERVICE_CONFIGS

stop_event = threading.Event()

if __name__ == '__main__':
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
    settings.init_settings()
    SERVICE_CONFIGS.configs = load_configurations(SERVICE_CONF)

    try:
        logging.info("RAGFlow Admin service start...")
        run_simple(
            hostname="0.0.0.0",
            port=9381,
            application=app,
            threaded=True,
            use_reloader=True,
            use_debugger=True,
        )
    except Exception:
        traceback.print_exc()
        stop_event.set()
        time.sleep(1)
        os.kill(os.getpid(), signal.SIGKILL)
