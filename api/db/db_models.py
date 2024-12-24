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
import inspect
import os
import sys
import typing
import operator
from enum import Enum
from functools import wraps
from itsdangerous.url_safe import URLSafeTimedSerializer as Serializer
from flask_login import UserMixin
from playhouse.migrate import MySQLMigrator, PostgresqlMigrator, migrate
from peewee import (
    BigIntegerField, BooleanField, CharField,
    CompositeKey, IntegerField, TextField, FloatField, DateTimeField,
    Field, Model, Metadata
)
from playhouse.pool import PooledMySQLDatabase, PooledPostgresqlDatabase

from api.db import SerializedType, ParserType
from api import settings
from api import utils


def singleton(cls, *args, **kw):
    instances = {}

    def _singleton():
        key = str(cls) + str(os.getpid())
        if key not in instances:
            instances[key] = cls(*args, **kw)
        return instances[key]

    return _singleton


CONTINUOUS_FIELD_TYPE = {IntegerField, FloatField, DateTimeField}
AUTO_DATE_TIMESTAMP_FIELD_PREFIX = {
    "create",
    "start",
    "end",
    "update",
    "read_access",
    "write_access"}


class TextFieldType(Enum):
    MYSQL = 'LONGTEXT'
    POSTGRES = 'TEXT'


class LongTextField(TextField):
    field_type = TextFieldType[settings.DATABASE_TYPE.upper()].value


class JSONField(LongTextField):
    default_value = {}

    def __init__(self, object_hook=None, object_pairs_hook=None, **kwargs):
        self._object_hook = object_hook
        self._object_pairs_hook = object_pairs_hook
        super().__init__(**kwargs)

    def db_value(self, value):
        if value is None:
            value = self.default_value
        return utils.json_dumps(value)

    def python_value(self, value):
        if not value:
            return self.default_value
        return utils.json_loads(
            value, object_hook=self._object_hook, object_pairs_hook=self._object_pairs_hook)


class ListField(JSONField):
    default_value = []


class SerializedField(LongTextField):
    def __init__(self, serialized_type=SerializedType.PICKLE,
                 object_hook=None, object_pairs_hook=None, **kwargs):
        self._serialized_type = serialized_type
        self._object_hook = object_hook
        self._object_pairs_hook = object_pairs_hook
        super().__init__(**kwargs)

    def db_value(self, value):
        if self._serialized_type == SerializedType.PICKLE:
            return utils.serialize_b64(value, to_str=True)
        elif self._serialized_type == SerializedType.JSON:
            if value is None:
                return None
            return utils.json_dumps(value, with_type=True)
        else:
            raise ValueError(
                f"the serialized type {self._serialized_type} is not supported")

    def python_value(self, value):
        if self._serialized_type == SerializedType.PICKLE:
            return utils.deserialize_b64(value)
        elif self._serialized_type == SerializedType.JSON:
            if value is None:
                return {}
            return utils.json_loads(
                value, object_hook=self._object_hook, object_pairs_hook=self._object_pairs_hook)
        else:
            raise ValueError(
                f"the serialized type {self._serialized_type} is not supported")


def is_continuous_field(cls: typing.Type) -> bool:
    if cls in CONTINUOUS_FIELD_TYPE:
        return True
    for p in cls.__bases__:
        if p in CONTINUOUS_FIELD_TYPE:
            return True
        elif p is not Field and p is not object:
            if is_continuous_field(p):
                return True
    else:
        return False


def auto_date_timestamp_field():
    return {f"{f}_time" for f in AUTO_DATE_TIMESTAMP_FIELD_PREFIX}


def auto_date_timestamp_db_field():
    return {f"f_{f}_time" for f in AUTO_DATE_TIMESTAMP_FIELD_PREFIX}


def remove_field_name_prefix(field_name):
    return field_name[2:] if field_name.startswith('f_') else field_name


class BaseModel(Model):
    create_time = BigIntegerField(null=True, index=True)
    create_date = DateTimeField(null=True, index=True)
    update_time = BigIntegerField(null=True, index=True)
    update_date = DateTimeField(null=True, index=True)

    def to_json(self):
        # This function is obsolete
        return self.to_dict()

    def to_dict(self):
        return self.__dict__['__data__']

    def to_human_model_dict(self, only_primary_with: list = None):
        model_dict = self.__dict__['__data__']

        if not only_primary_with:
            return {remove_field_name_prefix(
                k): v for k, v in model_dict.items()}

        human_model_dict = {}
        for k in self._meta.primary_key.field_names:
            human_model_dict[remove_field_name_prefix(k)] = model_dict[k]
        for k in only_primary_with:
            human_model_dict[k] = model_dict[f'f_{k}']
        return human_model_dict

    @property
    def meta(self) -> Metadata:
        return self._meta

    @classmethod
    def get_primary_keys_name(cls):
        return cls._meta.primary_key.field_names if isinstance(cls._meta.primary_key, CompositeKey) else [
            cls._meta.primary_key.name]

    @classmethod
    def getter_by(cls, attr):
        return operator.attrgetter(attr)(cls)

    @classmethod
    def query(cls, reverse=None, order_by=None, **kwargs):
        filters = []
        for f_n, f_v in kwargs.items():
            attr_name = '%s' % f_n
            if not hasattr(cls, attr_name) or f_v is None:
                continue
            if type(f_v) in {list, set}:
                f_v = list(f_v)
                if is_continuous_field(type(getattr(cls, attr_name))):
                    if len(f_v) == 2:
                        for i, v in enumerate(f_v):
                            if isinstance(
                                    v, str) and f_n in auto_date_timestamp_field():
                                # time type: %Y-%m-%d %H:%M:%S
                                f_v[i] = utils.date_string_to_timestamp(v)
                        lt_value = f_v[0]
                        gt_value = f_v[1]
                        if lt_value is not None and gt_value is not None:
                            filters.append(
                                cls.getter_by(attr_name).between(
                                    lt_value, gt_value))
                        elif lt_value is not None:
                            filters.append(
                                operator.attrgetter(attr_name)(cls) >= lt_value)
                        elif gt_value is not None:
                            filters.append(
                                operator.attrgetter(attr_name)(cls) <= gt_value)
                else:
                    filters.append(operator.attrgetter(attr_name)(cls) << f_v)
            else:
                filters.append(operator.attrgetter(attr_name)(cls) == f_v)
        if filters:
            query_records = cls.select().where(*filters)
            if reverse is not None:
                if not order_by or not hasattr(cls, f"{order_by}"):
                    order_by = "create_time"
                if reverse is True:
                    query_records = query_records.order_by(
                        cls.getter_by(f"{order_by}").desc())
                elif reverse is False:
                    query_records = query_records.order_by(
                        cls.getter_by(f"{order_by}").asc())
            return [query_record for query_record in query_records]
        else:
            return []

    @classmethod
    def insert(cls, __data=None, **insert):
        if isinstance(__data, dict) and __data:
            __data[cls._meta.combined["create_time"]
            ] = utils.current_timestamp()
        if insert:
            insert["create_time"] = utils.current_timestamp()

        return super().insert(__data, **insert)

    # update and insert will call this method
    @classmethod
    def _normalize_data(cls, data, kwargs):
        normalized = super()._normalize_data(data, kwargs)
        if not normalized:
            return {}

        normalized[cls._meta.combined["update_time"]
        ] = utils.current_timestamp()

        for f_n in AUTO_DATE_TIMESTAMP_FIELD_PREFIX:
            if {f"{f_n}_time", f"{f_n}_date"}.issubset(cls._meta.combined.keys()) and \
                    cls._meta.combined[f"{f_n}_time"] in normalized and \
                    normalized[cls._meta.combined[f"{f_n}_time"]] is not None:
                normalized[cls._meta.combined[f"{f_n}_date"]] = utils.timestamp_to_date(
                    normalized[cls._meta.combined[f"{f_n}_time"]])

        return normalized


class JsonSerializedField(SerializedField):
    def __init__(self, object_hook=utils.from_dict_hook,
                 object_pairs_hook=None, **kwargs):
        super(JsonSerializedField, self).__init__(serialized_type=SerializedType.JSON, object_hook=object_hook,
                                                  object_pairs_hook=object_pairs_hook, **kwargs)


class PooledDatabase(Enum):
    MYSQL = PooledMySQLDatabase
    POSTGRES = PooledPostgresqlDatabase


class DatabaseMigrator(Enum):
    MYSQL = MySQLMigrator
    POSTGRES = PostgresqlMigrator


@singleton
class BaseDataBase:
    def __init__(self):
        database_config = settings.DATABASE.copy()
        db_name = database_config.pop("name")
        self.database_connection = PooledDatabase[settings.DATABASE_TYPE.upper()].value(db_name, **database_config)
        logging.info('init database on cluster mode successfully')


class PostgresDatabaseLock:
    def __init__(self, lock_name, timeout=10, db=None):
        self.lock_name = lock_name
        self.timeout = int(timeout)
        self.db = db if db else DB

    def lock(self):
        cursor = self.db.execute_sql("SELECT pg_try_advisory_lock(%s)", self.timeout)
        ret = cursor.fetchone()
        if ret[0] == 0:
            raise Exception(f'acquire postgres lock {self.lock_name} timeout')
        elif ret[0] == 1:
            return True
        else:
            raise Exception(f'failed to acquire lock {self.lock_name}')

    def unlock(self):
        cursor = self.db.execute_sql("SELECT pg_advisory_unlock(%s)", self.timeout)
        ret = cursor.fetchone()
        if ret[0] == 0:
            raise Exception(
                f'postgres lock {self.lock_name} was not established by this thread')
        elif ret[0] == 1:
            return True
        else:
            raise Exception(f'postgres lock {self.lock_name} does not exist')

    def __enter__(self):
        if isinstance(self.db, PostgresDatabaseLock):
            self.lock()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        if isinstance(self.db, PostgresDatabaseLock):
            self.unlock()

    def __call__(self, func):
        @wraps(func)
        def magic(*args, **kwargs):
            with self:
                return func(*args, **kwargs)

        return magic


class MysqlDatabaseLock:
    def __init__(self, lock_name, timeout=10, db=None):
        self.lock_name = lock_name
        self.timeout = int(timeout)
        self.db = db if db else DB

    def lock(self):
        # SQL parameters only support %s format placeholders
        cursor = self.db.execute_sql(
            "SELECT GET_LOCK(%s, %s)", (self.lock_name, self.timeout))
        ret = cursor.fetchone()
        if ret[0] == 0:
            raise Exception(f'acquire mysql lock {self.lock_name} timeout')
        elif ret[0] == 1:
            return True
        else:
            raise Exception(f'failed to acquire lock {self.lock_name}')

    def unlock(self):
        cursor = self.db.execute_sql(
            "SELECT RELEASE_LOCK(%s)", (self.lock_name,))
        ret = cursor.fetchone()
        if ret[0] == 0:
            raise Exception(
                f'mysql lock {self.lock_name} was not established by this thread')
        elif ret[0] == 1:
            return True
        else:
            raise Exception(f'mysql lock {self.lock_name} does not exist')

    def __enter__(self):
        if isinstance(self.db, PooledMySQLDatabase):
            self.lock()
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        if isinstance(self.db, PooledMySQLDatabase):
            self.unlock()

    def __call__(self, func):
        @wraps(func)
        def magic(*args, **kwargs):
            with self:
                return func(*args, **kwargs)

        return magic


class DatabaseLock(Enum):
    MYSQL = MysqlDatabaseLock
    POSTGRES = PostgresDatabaseLock


DB = BaseDataBase().database_connection
DB.lock = DatabaseLock[settings.DATABASE_TYPE.upper()].value


def close_connection():
    try:
        if DB:
            DB.close_stale(age=30)
    except Exception as e:
        logging.exception(e)


class DataBaseModel(BaseModel):
    class Meta:
        database = DB


@DB.connection_context()
def init_database_tables(alter_fields=[]):
    members = inspect.getmembers(sys.modules[__name__], inspect.isclass)
    table_objs = []
    create_failed_list = []
    for name, obj in members:
        if obj != DataBaseModel and issubclass(obj, DataBaseModel):
            table_objs.append(obj)
            logging.debug(f"start create table {obj.__name__}")
            try:
                obj.create_table()
                logging.debug(f"create table success: {obj.__name__}")
            except Exception as e:
                logging.exception(e)
                create_failed_list.append(obj.__name__)
    if create_failed_list:
        logging.error(f"create tables failed: {create_failed_list}")
        raise Exception(f"create tables failed: {create_failed_list}")
    migrate_db()


def fill_db_model_object(model_object, human_model_dict):
    for k, v in human_model_dict.items():
        attr_name = '%s' % k
        if hasattr(model_object.__class__, attr_name):
            setattr(model_object, attr_name, v)
    return model_object


class User(DataBaseModel, UserMixin):
    id = CharField(max_length=32, primary_key=True)
    access_token = CharField(max_length=255, null=True, index=True)
    nickname = CharField(max_length=100, null=False, help_text="nicky name", index=True)
    password = CharField(max_length=255, null=True, help_text="password", index=True)
    email = CharField(
        max_length=255,
        null=False,
        help_text="email",
        index=True)
    avatar = TextField(null=True, help_text="avatar base64 string")
    language = CharField(
        max_length=32,
        null=True,
        help_text="English|Chinese",
        default="Chinese" if "zh_CN" in os.getenv("LANG", "") else "English",
        index=True)
    color_schema = CharField(
        max_length=32,
        null=True,
        help_text="Bright|Dark",
        default="Bright",
        index=True)
    timezone = CharField(
        max_length=64,
        null=True,
        help_text="Timezone",
        default="UTC+8\tAsia/Shanghai",
        index=True)
    last_login_time = DateTimeField(null=True, index=True)
    is_authenticated = CharField(max_length=1, null=False, default="1", index=True)
    is_active = CharField(max_length=1, null=False, default="1", index=True)
    is_anonymous = CharField(max_length=1, null=False, default="0", index=True)
    login_channel = CharField(null=True, help_text="from which user login", index=True)
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted, 1: validate)",
        default="1",
        index=True)
    is_superuser = BooleanField(null=True, help_text="is root", default=False, index=True)

    def __str__(self):
        return self.email

    def get_id(self):
        jwt = Serializer(secret_key=settings.SECRET_KEY)
        return jwt.dumps(str(self.access_token))

    class Meta:
        db_table = "user"


class Tenant(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    name = CharField(max_length=100, null=True, help_text="Tenant name", index=True)
    public_key = CharField(max_length=255, null=True, index=True)
    llm_id = CharField(max_length=128, null=False, help_text="default llm ID", index=True)
    embd_id = CharField(
        max_length=128,
        null=False,
        help_text="default embedding model ID",
        index=True)
    asr_id = CharField(
        max_length=128,
        null=False,
        help_text="default ASR model ID",
        index=True)
    img2txt_id = CharField(
        max_length=128,
        null=False,
        help_text="default image to text model ID",
        index=True)
    rerank_id = CharField(
        max_length=128,
        null=False,
        help_text="default rerank model ID",
        index=True)
    tts_id = CharField(
        max_length=256,
        null=True,
        help_text="default tts model ID",
        index=True)
    parser_ids = CharField(
        max_length=256,
        null=False,
        help_text="document processors",
        index=True)
    credit = IntegerField(default=512, index=True)
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted, 1: validate)",
        default="1",
        index=True)

    class Meta:
        db_table = "tenant"


class UserTenant(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    user_id = CharField(max_length=32, null=False, index=True)
    tenant_id = CharField(max_length=32, null=False, index=True)
    role = CharField(max_length=32, null=False, help_text="UserTenantRole", index=True)
    invited_by = CharField(max_length=32, null=False, index=True)
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted, 1: validate)",
        default="1",
        index=True)

    class Meta:
        db_table = "user_tenant"


class InvitationCode(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    code = CharField(max_length=32, null=False, index=True)
    visit_time = DateTimeField(null=True, index=True)
    user_id = CharField(max_length=32, null=True, index=True)
    tenant_id = CharField(max_length=32, null=True, index=True)
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted, 1: validate)",
        default="1",
        index=True)

    class Meta:
        db_table = "invitation_code"


class LLMFactories(DataBaseModel):
    name = CharField(
        max_length=128,
        null=False,
        help_text="LLM factory name",
        primary_key=True)
    logo = TextField(null=True, help_text="llm logo base64")
    tags = CharField(
        max_length=255,
        null=False,
        help_text="LLM, Text Embedding, Image2Text, ASR",
        index=True)
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted, 1: validate)",
        default="1",
        index=True)

    def __str__(self):
        return self.name

    class Meta:
        db_table = "llm_factories"


class LLM(DataBaseModel):
    # LLMs dictionary
    llm_name = CharField(
        max_length=128,
        null=False,
        help_text="LLM name",
        index=True)
    model_type = CharField(
        max_length=128,
        null=False,
        help_text="LLM, Text Embedding, Image2Text, ASR",
        index=True)
    fid = CharField(max_length=128, null=False, help_text="LLM factory id", index=True)
    max_tokens = IntegerField(default=0)

    tags = CharField(
        max_length=255,
        null=False,
        help_text="LLM, Text Embedding, Image2Text, Chat, 32k...",
        index=True)
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted, 1: validate)",
        default="1",
        index=True)

    def __str__(self):
        return self.llm_name

    class Meta:
        primary_key = CompositeKey('fid', 'llm_name')
        db_table = "llm"


class TenantLLM(DataBaseModel):
    tenant_id = CharField(max_length=32, null=False, index=True)
    llm_factory = CharField(
        max_length=128,
        null=False,
        help_text="LLM factory name",
        index=True)
    model_type = CharField(
        max_length=128,
        null=True,
        help_text="LLM, Text Embedding, Image2Text, ASR",
        index=True)
    llm_name = CharField(
        max_length=128,
        null=True,
        help_text="LLM name",
        default="",
        index=True)
    api_key = CharField(max_length=1024, null=True, help_text="API KEY", index=True)
    api_base = CharField(max_length=255, null=True, help_text="API Base")
    max_tokens = IntegerField(default=8192, index=True)
    used_tokens = IntegerField(default=0, index=True)

    def __str__(self):
        return self.llm_name

    class Meta:
        db_table = "tenant_llm"
        primary_key = CompositeKey('tenant_id', 'llm_factory', 'llm_name')


class Knowledgebase(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    avatar = TextField(null=True, help_text="avatar base64 string")
    tenant_id = CharField(max_length=32, null=False, index=True)
    name = CharField(
        max_length=128,
        null=False,
        help_text="KB name",
        index=True)
    language = CharField(
        max_length=32,
        null=True,
        default="Chinese" if "zh_CN" in os.getenv("LANG", "") else "English",
        help_text="English|Chinese",
        index=True)
    description = TextField(null=True, help_text="KB description")
    embd_id = CharField(
        max_length=128,
        null=False,
        help_text="default embedding model ID",
        index=True)
    permission = CharField(
        max_length=16,
        null=False,
        help_text="me|team",
        default="me",
        index=True)
    created_by = CharField(max_length=32, null=False, index=True)
    doc_num = IntegerField(default=0, index=True)
    token_num = IntegerField(default=0, index=True)
    chunk_num = IntegerField(default=0, index=True)
    similarity_threshold = FloatField(default=0.2, index=True)
    vector_similarity_weight = FloatField(default=0.3, index=True)

    parser_id = CharField(
        max_length=32,
        null=False,
        help_text="default parser ID",
        default=ParserType.NAIVE.value,
        index=True)
    parser_config = JSONField(null=False, default={"pages": [[1, 1000000]]})
    pagerank = IntegerField(default=0, index=False)
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted, 1: validate)",
        default="1",
        index=True)

    def __str__(self):
        return self.name

    class Meta:
        db_table = "knowledgebase"


class Document(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    thumbnail = TextField(null=True, help_text="thumbnail base64 string")
    kb_id = CharField(max_length=256, null=False, index=True)
    parser_id = CharField(
        max_length=32,
        null=False,
        help_text="default parser ID",
        index=True)
    parser_config = JSONField(null=False, default={"pages": [[1, 1000000]]})
    source_type = CharField(
        max_length=128,
        null=False,
        default="local",
        help_text="where dose this document come from",
        index=True)
    type = CharField(max_length=32, null=False, help_text="file extension",
                     index=True)
    created_by = CharField(
        max_length=32,
        null=False,
        help_text="who created it",
        index=True)
    name = CharField(
        max_length=255,
        null=True,
        help_text="file name",
        index=True)
    location = CharField(
        max_length=255,
        null=True,
        help_text="where dose it store",
        index=True)
    size = IntegerField(default=0, index=True)
    token_num = IntegerField(default=0, index=True)
    chunk_num = IntegerField(default=0, index=True)
    progress = FloatField(default=0, index=True)
    progress_msg = TextField(
        null=True,
        help_text="process message",
        default="")
    process_begin_at = DateTimeField(null=True, index=True)
    process_duation = FloatField(default=0)

    run = CharField(
        max_length=1,
        null=True,
        help_text="start to run processing or cancel.(1: run it; 2: cancel)",
        default="0",
        index=True)
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted, 1: validate)",
        default="1",
        index=True)

    class Meta:
        db_table = "document"


class File(DataBaseModel):
    id = CharField(
        max_length=32,
        primary_key=True)
    parent_id = CharField(
        max_length=32,
        null=False,
        help_text="parent folder id",
        index=True)
    tenant_id = CharField(
        max_length=32,
        null=False,
        help_text="tenant id",
        index=True)
    created_by = CharField(
        max_length=32,
        null=False,
        help_text="who created it",
        index=True)
    name = CharField(
        max_length=255,
        null=False,
        help_text="file name or folder name",
        index=True)
    location = CharField(
        max_length=255,
        null=True,
        help_text="where dose it store",
        index=True)
    size = IntegerField(default=0, index=True)
    type = CharField(max_length=32, null=False, help_text="file extension", index=True)
    source_type = CharField(
        max_length=128,
        null=False,
        default="",
        help_text="where dose this document come from", index=True)

    class Meta:
        db_table = "file"


class File2Document(DataBaseModel):
    id = CharField(
        max_length=32,
        primary_key=True)
    file_id = CharField(
        max_length=32,
        null=True,
        help_text="file id",
        index=True)
    document_id = CharField(
        max_length=32,
        null=True,
        help_text="document id",
        index=True)

    class Meta:
        db_table = "file2document"


class Task(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    doc_id = CharField(max_length=32, null=False, index=True)
    from_page = IntegerField(default=0)

    to_page = IntegerField(default=100000000)

    begin_at = DateTimeField(null=True, index=True)
    process_duation = FloatField(default=0)

    progress = FloatField(default=0, index=True)
    progress_msg = TextField(
        null=True,
        help_text="process message",
        default="")
    retry_count = IntegerField(default=0)
    digest = TextField(null=True, help_text="task digest", default="")
    chunk_ids = LongTextField(null=True, help_text="chunk ids", default="")


class Dialog(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    tenant_id = CharField(max_length=32, null=False, index=True)
    name = CharField(
        max_length=255,
        null=True,
        help_text="dialog application name",
        index=True)
    description = TextField(null=True, help_text="Dialog description")
    icon = TextField(null=True, help_text="icon base64 string")
    language = CharField(
        max_length=32,
        null=True,
        default="Chinese" if "zh_CN" in os.getenv("LANG", "") else "English",
        help_text="English|Chinese",
        index=True)
    llm_id = CharField(max_length=128, null=False, help_text="default llm ID")

    llm_setting = JSONField(null=False, default={"temperature": 0.1, "top_p": 0.3, "frequency_penalty": 0.7,
                                                 "presence_penalty": 0.4, "max_tokens": 512})
    prompt_type = CharField(
        max_length=16,
        null=False,
        default="simple",
        help_text="simple|advanced",
        index=True)
    prompt_config = JSONField(null=False,
                              default={"system": "", "prologue": "Hi! I'm your assistant, what can I do for you?",
                                       "parameters": [],
                                       "empty_response": "Sorry! No relevant content was found in the knowledge base!"})

    similarity_threshold = FloatField(default=0.2)
    vector_similarity_weight = FloatField(default=0.3)

    top_n = IntegerField(default=6)

    top_k = IntegerField(default=1024)

    do_refer = CharField(
        max_length=1,
        null=False,
        default="1",
        help_text="it needs to insert reference index into answer or not")

    rerank_id = CharField(
        max_length=128,
        null=False,
        help_text="default rerank model ID")

    kb_ids = JSONField(null=False, default=[])
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted, 1: validate)",
        default="1",
        index=True)

    class Meta:
        db_table = "dialog"


class Conversation(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    dialog_id = CharField(max_length=32, null=False, index=True)
    name = CharField(max_length=255, null=True, help_text="converastion name", index=True)
    message = JSONField(null=True)
    reference = JSONField(null=True, default=[])
    user_id = CharField(max_length=255, null=True, help_text="user_id", index=True)

    class Meta:
        db_table = "conversation"


class APIToken(DataBaseModel):
    tenant_id = CharField(max_length=32, null=False, index=True)
    token = CharField(max_length=255, null=False, index=True)
    dialog_id = CharField(max_length=32, null=False, index=True)
    source = CharField(max_length=16, null=True, help_text="none|agent|dialog", index=True)
    beta = CharField(max_length=255, null=True, index=True)

    class Meta:
        db_table = "api_token"
        primary_key = CompositeKey('tenant_id', 'token')


class API4Conversation(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    dialog_id = CharField(max_length=32, null=False, index=True)
    user_id = CharField(max_length=255, null=False, help_text="user_id", index=True)
    message = JSONField(null=True)
    reference = JSONField(null=True, default=[])
    tokens = IntegerField(default=0)
    source = CharField(max_length=16, null=True, help_text="none|agent|dialog", index=True)
    dsl = JSONField(null=True, default={})
    duration = FloatField(default=0, index=True)
    round = IntegerField(default=0, index=True)
    thumb_up = IntegerField(default=0, index=True)

    class Meta:
        db_table = "api_4_conversation"


class UserCanvas(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    avatar = TextField(null=True, help_text="avatar base64 string")
    user_id = CharField(max_length=255, null=False, help_text="user_id", index=True)
    title = CharField(max_length=255, null=True, help_text="Canvas title")

    description = TextField(null=True, help_text="Canvas description")
    canvas_type = CharField(max_length=32, null=True, help_text="Canvas type", index=True)
    dsl = JSONField(null=True, default={})

    class Meta:
        db_table = "user_canvas"


class CanvasTemplate(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    avatar = TextField(null=True, help_text="avatar base64 string")
    title = CharField(max_length=255, null=True, help_text="Canvas title")

    description = TextField(null=True, help_text="Canvas description")
    canvas_type = CharField(max_length=32, null=True, help_text="Canvas type", index=True)
    dsl = JSONField(null=True, default={})

    class Meta:
        db_table = "canvas_template"


def migrate_db():
    with DB.transaction():
        migrator = DatabaseMigrator[settings.DATABASE_TYPE.upper()].value(DB)
        try:
            migrate(
                migrator.add_column('file', 'source_type', CharField(max_length=128, null=False, default="",
                                                                     help_text="where dose this document come from",
                                                                     index=True))
            )
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column('tenant', 'rerank_id',
                                    CharField(max_length=128, null=False, default="BAAI/bge-reranker-v2-m3",
                                              help_text="default rerank model ID"))

            )
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column('dialog', 'rerank_id', CharField(max_length=128, null=False, default="",
                                                                     help_text="default rerank model ID"))

            )
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column('dialog', 'top_k', IntegerField(default=1024))

            )
        except Exception:
            pass
        try:
            migrate(
                migrator.alter_column_type('tenant_llm', 'api_key',
                                           CharField(max_length=1024, null=True, help_text="API KEY", index=True))
            )
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column('api_token', 'source',
                                    CharField(max_length=16, null=True, help_text="none|agent|dialog", index=True))
            )
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column("tenant", "tts_id",
                                    CharField(max_length=256, null=True, help_text="default tts model ID", index=True))
            )
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column('api_4_conversation', 'source',
                                    CharField(max_length=16, null=True, help_text="none|agent|dialog", index=True))
            )
        except Exception:
            pass
        try:
            DB.execute_sql('ALTER TABLE llm DROP PRIMARY KEY;')
            DB.execute_sql('ALTER TABLE llm ADD PRIMARY KEY (llm_name,fid);')
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column('task', 'retry_count', IntegerField(default=0))
            )
        except Exception:
            pass
        try:
            migrate(
                migrator.alter_column_type('api_token', 'dialog_id',
                                           CharField(max_length=32, null=True, index=True))
            )
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column("tenant_llm", "max_tokens", IntegerField(default=8192, index=True))
            )
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column("api_4_conversation", "dsl", JSONField(null=True, default={}))
            )
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column("knowledgebase", "pagerank", IntegerField(default=0, index=False))
            )
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column("api_token", "beta", CharField(max_length=255, null=True, index=True))
            )
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column("task", "digest", TextField(null=True, help_text="task digest", default=""))
            )
        except Exception:
            pass

        try:
            migrate(
                migrator.add_column("task", "chunk_ids", LongTextField(null=True, help_text="chunk ids", default=""))
            )
        except Exception:
            pass
        try:
            migrate(
                migrator.add_column("conversation", "user_id",
                                    CharField(max_length=255, null=True, help_text="user_id", index=True))
            )
        except Exception:
            pass
