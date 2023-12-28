from configparser import ConfigParser
import os
import inspect

CF = ConfigParser()
__fnm = os.path.join(os.path.dirname(__file__), '../conf/sys.cnf')
if not os.path.exists(__fnm):
    __fnm = os.path.join(os.path.dirname(__file__), '../../conf/sys.cnf')
assert os.path.exists(
    __fnm), f"【EXCEPTION】can't find {__fnm}." + os.path.dirname(__file__)
if not os.path.exists(__fnm):
    __fnm = "./sys.cnf"

CF.read(__fnm)


class Config:
    def __init__(self, env):
        self.env = env
        if env == "spark":
            CF.read("./cv.cnf")

    def get(self, key, default=None):
        global CF
        return os.environ.get(key.upper(),
                              CF[self.env].get(key, default)
                              )


def init(env):
    return Config(env)
