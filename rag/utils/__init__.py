import os
import re
import tiktoken


def singleton(cls, *args, **kw):
    instances = {}

    def _singleton():
        key = str(cls) + str(os.getpid())
        if key not in instances:
            instances[key] = cls(*args, **kw)
        return instances[key]

    return _singleton


from .minio_conn import MINIO
from .es_conn import ELASTICSEARCH

def rmSpace(txt):
    txt = re.sub(r"([^a-z0-9.,]) +([^ ])", r"\1\2", txt, flags=re.IGNORECASE)
    return re.sub(r"([^ ]) +([^a-z0-9.,])", r"\1\2", txt, flags=re.IGNORECASE)


def findMaxDt(fnm):
    m = "1970-01-01 00:00:00"
    try:
        with open(fnm, "r") as f:
            while True:
                l = f.readline()
                if not l:
                    break
                l = l.strip("\n")
                if l == 'nan':
                    continue
                if l > m:
                    m = l
    except Exception as e:
        pass
    return m

  
def findMaxTm(fnm):
    m = 0
    try:
        with open(fnm, "r") as f:
            while True:
                l = f.readline()
                if not l:
                    break
                l = l.strip("\n")
                if l == 'nan':
                    continue
                if int(l) > m:
                    m = int(l)
    except Exception as e:
        pass
    return m


encoder = tiktoken.encoding_for_model("gpt-3.5-turbo")

def num_tokens_from_string(string: str) -> int:
    """Returns the number of tokens in a text string."""
    num_tokens = len(encoder.encode(string))
    return num_tokens

