import pytest
import types, sys
from playhouse.pool import PooledMySQLDatabase
from peewee import InterfaceError

# Provide a minimal quart_auth stub to avoid import-time errors when importing db_models
if 'quart_auth' not in sys.modules:
    quart_auth_mod = types.ModuleType('quart_auth')
    class AuthUser:
        pass
    quart_auth_mod.AuthUser = AuthUser
    sys.modules['quart_auth'] = quart_auth_mod

# Provide a minimal pydantic stub used by some modules during import
if 'pydantic' not in sys.modules:
    pyd_mod = types.ModuleType('pydantic')
    class BaseModel:
        def __init__(self, *args, **kwargs):
            pass
    pyd_mod.BaseModel = BaseModel
    sys.modules['pydantic'] = pyd_mod

# Provide a minimal pymysql stub for converters used during import
if 'pymysql' not in sys.modules:
    pymod = types.ModuleType('pymysql')
    conv_mod = types.ModuleType('pymysql.converters')
    def escape_string(s):
        return str(s).replace("'", "\\'")
    conv_mod.escape_string = escape_string
    pymod.converters = conv_mod
    sys.modules['pymysql'] = pymod
    sys.modules['pymysql.converters'] = conv_mod

# Provide a minimal itsdangerous.url_safe stub used by db_models
if 'itsdangerous' not in sys.modules:
    its_mod = types.ModuleType('itsdangerous')
    url_s_mod = types.ModuleType('itsdangerous.url_safe')
    class URLSafeTimedSerializer:
        def __init__(self, *args, **kwargs):
            pass
    url_s_mod.URLSafeTimedSerializer = URLSafeTimedSerializer
    its_mod.url_safe = url_s_mod
    sys.modules['itsdangerous'] = its_mod
    sys.modules['itsdangerous.url_safe'] = url_s_mod

# Provide a minimal pyobvector stub used by rag utils
if 'pyobvector' not in sys.modules:
    py_mod = types.ModuleType('pyobvector')
    class ObVecClient:
        pass
    class FtsIndexParam:
        pass
    class FtsParser:
        pass
    ARRAY = None
    VECTOR = None
    py_mod.ObVecClient = ObVecClient
    py_mod.FtsIndexParam = FtsIndexParam
    py_mod.FtsParser = FtsParser
    py_mod.ARRAY = ARRAY
    py_mod.VECTOR = VECTOR
    sys.modules['pyobvector'] = py_mod

# Robust import helper: stub missing modules iteratively to allow importing db_models in test env
import importlib

def safe_import_db_models(max_retries=30):
    for _ in range(max_retries):
        try:
            mod = importlib.import_module('api.db.db_models')
            return mod
        except ModuleNotFoundError as e:
            missing = e.name
            # create a minimal stub module
            stub = types.ModuleType(missing)
            sys.modules[missing] = stub
        except ImportError as e:
            # Handle 'cannot import name' style ImportError by creating the missing attribute on the module
            msg = str(e)
            m = None
            name = None
            if "cannot import name" in msg:
                # Expected format: "cannot import name 'X' from 'module'"
                import re
                match = re.search(r"cannot import name '\\'?([A-Za-z0-9_]+)\\?' from '([^']+)'", msg)
                if match:
                    name = match.group(1)
                    m = match.group(2)
            if m and m in sys.modules:
                setattr(sys.modules[m], name, type(name, (), {}))
            else:
                # If module not present yet, create module and attribute
                if m:
                    mod_stub = types.ModuleType(m)
                    setattr(mod_stub, name, type(name, (), {}))
                    sys.modules[m] = mod_stub
    raise ImportError('Could not import api.db.db_models after stubbing missing modules')

db_models_mod = safe_import_db_models()
RetryingPooledMySQLDatabase = db_models_mod.RetryingPooledMySQLDatabase


class DummyDB(RetryingPooledMySQLDatabase):
    def __init__(self):
        # Avoid calling parent constructor to prevent real DB connections during tests
        # Manually set the attributes used by the retry logic
        self.max_retries = 3
        self.retry_delay = 0.01
        # Provide stubs for connection management used in the implementation
        self._conn = None
        self.get_conn = lambda : self._conn
        self.close = lambda : None
        self.connect = lambda : None


def test_execute_sql_retries(monkeypatch):
    calls = {'count': 0}

    def failing_execute(self, sql, params=None, commit=True):
        calls['count'] += 1
        if calls['count'] < 3:
            raise InterfaceError('Simulated lost connection')
        return 'OK'

    # Patch the parent class execute_sql to simulate failures then success
    monkeypatch.setattr(PooledMySQLDatabase, 'execute_sql', failing_execute, raising=False)

    db = DummyDB()
    res = db.execute_sql('SELECT 1')
    assert res == 'OK'
    assert calls['count'] == 3


def test_execute_sql_logs_reconnect(monkeypatch, caplog):
    import logging
    caplog.set_level(logging.INFO)

    # Fake connection that will fail ping and provides a stable thread_id
    class FakeConn:
        def __init__(self):
            self._id = 12345

        def ping(self, reconnect=False):
            raise Exception('Simulated ping failure')

        def thread_id(self):
            return self._id

    fake_conn = FakeConn()

    calls = {'count': 0}

    def execute_once(self, sql, params=None, commit=True):
        calls['count'] += 1
        # First attempt raises interface error, second attempt succeeds
        if calls['count'] == 1:
            raise InterfaceError('(0, "")')
        return 'OK'

    monkeypatch.setattr(PooledMySQLDatabase, 'execute_sql', execute_once, raising=False)

    db = DummyDB()
    # Ensure get_conn returns the fake connection
    monkeypatch.setattr(db, 'get_conn', lambda: fake_conn, raising=False)

    # Track connect calls
    connected = {'count': 0}
    def fake_connect():
        connected['count'] += 1
    monkeypatch.setattr(db, 'connect', fake_connect, raising=False)

    res = db.execute_sql('SELECT 1')
    assert res == 'OK'
    # Expect ping failure warning and reconnect logs
    messages = [r.message for r in caplog.records]
    assert any('DB ping failed' in m or 'Database reconnect' in m or 'Database connection issue' in m for m in messages)
