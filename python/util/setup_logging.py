import json
import logging.config
import os


def log_dir():
    fnm = os.path.join(os.path.dirname(__file__), '../log/')
    if not os.path.exists(fnm):
        fnm = os.path.join(os.path.dirname(__file__), '../../log/')
    assert os.path.exists(fnm), f"Can't locate log dir: {fnm}"
    return fnm


def setup_logging(default_path="conf/logging.json",
                  default_level=logging.INFO,
                  env_key="LOG_CFG"):
    path = default_path
    value = os.getenv(env_key, None)
    if value:
        path = value
    if os.path.exists(path):
        with open(path, "r") as f:
            config = json.load(f)
            fnm = log_dir()

            config["handlers"]["info_file_handler"]["filename"] = fnm + "info.log"
            config["handlers"]["error_file_handler"]["filename"] = fnm + "error.log"
            logging.config.dictConfig(config)
    else:
        logging.basicConfig(level=default_level)


__fnm = os.path.join(os.path.dirname(__file__), 'conf/logging.json')
if not os.path.exists(__fnm):
    __fnm = os.path.join(os.path.dirname(__file__), '../../conf/logging.json')
setup_logging(__fnm)
