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
import inspect
import os
import sys
import typing
import operator
from functools import wraps
from itsdangerous.url_safe import URLSafeTimedSerializer as Serializer
from flask_login import UserMixin

from peewee import (
    BigAutoField, BigIntegerField, BooleanField, CharField,
    CompositeKey, Insert, IntegerField, TextField, FloatField, DateTimeField,
    Field, Model, Metadata
)
from playhouse.pool import PooledMySQLDatabase

from api.db import SerializedType, ParserType
from api.settings import DATABASE, stat_logger, SECRET_KEY
from api.utils.log_utils import getLogger
from api import utils

LOGGER = getLogger()


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


class LongTextField(TextField):
    field_type = 'LONGTEXT'


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
        elif p != Field and p != object:
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
    create_time = BigIntegerField(null=True)
    create_date = DateTimeField(null=True)
    update_time = BigIntegerField(null=True)
    update_date = DateTimeField(null=True)

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


@singleton
class BaseDataBase:
    def __init__(self):
        database_config = DATABASE.copy()
        db_name = database_config.pop("name")
        self.database_connection = PooledMySQLDatabase(
            db_name, **database_config)
        stat_logger.info('init mysql database on cluster mode successfully')


class DatabaseLock:
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


DB = BaseDataBase().database_connection
DB.lock = DatabaseLock


def close_connection():
    try:
        if DB:
            DB.close()
    except Exception as e:
        LOGGER.exception(e)


class DataBaseModel(BaseModel):
    class Meta:
        database = DB


@DB.connection_context()
def init_database_tables():
    members = inspect.getmembers(sys.modules[__name__], inspect.isclass)
    table_objs = []
    create_failed_list = []
    for name, obj in members:
        if obj != DataBaseModel and issubclass(obj, DataBaseModel):
            table_objs.append(obj)
            LOGGER.info(f"start create table {obj.__name__}")
            try:
                obj.create_table()
                LOGGER.info(f"create table success: {obj.__name__}")
            except Exception as e:
                LOGGER.exception(e)
                create_failed_list.append(obj.__name__)
    if create_failed_list:
        LOGGER.info(f"create tables failed: {create_failed_list}")
        raise Exception(f"create tables failed: {create_failed_list}")


def fill_db_model_object(model_object, human_model_dict):
    for k, v in human_model_dict.items():
        attr_name = '%s' % k
        if hasattr(model_object.__class__, attr_name):
            setattr(model_object, attr_name, v)
    return model_object


class User(DataBaseModel, UserMixin):
    id = CharField(max_length=32, primary_key=True)
    access_token = CharField(max_length=255, null=True)
    nickname = CharField(max_length=100, null=False, help_text="nicky name")
    password = CharField(max_length=255, null=True, help_text="password")
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
        default="Chinese")
    color_schema = CharField(
        max_length=32,
        null=True,
        help_text="Bright|Dark",
        default="Bright")
    timezone = CharField(
        max_length=64,
        null=True,
        help_text="Timezone",
        default="UTC+8\tAsia/Shanghai")
    last_login_time = DateTimeField(null=True)
    is_authenticated = CharField(max_length=1, null=False, default="1")
    is_active = CharField(max_length=1, null=False, default="1")
    is_anonymous = CharField(max_length=1, null=False, default="0")
    login_channel = CharField(null=True, help_text="from which user login")
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted，1: validate)",
        default="1")
    is_superuser = BooleanField(null=True, help_text="is root", default=False)

    def __str__(self):
        return self.email

    def get_id(self):
        jwt = Serializer(secret_key=SECRET_KEY)
        return jwt.dumps(str(self.access_token))

    class Meta:
        db_table = "user"


class Tenant(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    name = CharField(max_length=100, null=True, help_text="Tenant name")
    public_key = CharField(max_length=255, null=True)
    llm_id = CharField(max_length=128, null=False, help_text="default llm ID")
    embd_id = CharField(
        max_length=128,
        null=False,
        help_text="default embedding model ID")
    asr_id = CharField(
        max_length=128,
        null=False,
        help_text="default ASR model ID")
    img2txt_id = CharField(
        max_length=128,
        null=False,
        help_text="default image to text model ID")
    parser_ids = CharField(
        max_length=256,
        null=False,
        help_text="document processors")
    credit = IntegerField(default=512)
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted，1: validate)",
        default="1")

    class Meta:
        db_table = "tenant"


class UserTenant(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    user_id = CharField(max_length=32, null=False)
    tenant_id = CharField(max_length=32, null=False)
    role = CharField(max_length=32, null=False, help_text="UserTenantRole")
    invited_by = CharField(max_length=32, null=False)
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted，1: validate)",
        default="1")

    class Meta:
        db_table = "user_tenant"


class InvitationCode(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    code = CharField(max_length=32, null=False)
    visit_time = DateTimeField(null=True)
    user_id = CharField(max_length=32, null=True)
    tenant_id = CharField(max_length=32, null=True)
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted，1: validate)",
        default="1")

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
        help_text="LLM, Text Embedding, Image2Text, ASR")
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted，1: validate)",
        default="1")

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
        index=True,
        primary_key=True)
    model_type = CharField(
        max_length=128,
        null=False,
        help_text="LLM, Text Embedding, Image2Text, ASR")
    fid = CharField(max_length=128, null=False, help_text="LLM factory id")
    max_tokens = IntegerField(default=0)
    tags = CharField(
        max_length=255,
        null=False,
        help_text="LLM, Text Embedding, Image2Text, Chat, 32k...")
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted，1: validate)",
        default="1")

    def __str__(self):
        return self.llm_name

    class Meta:
        db_table = "llm"


class TenantLLM(DataBaseModel):
    tenant_id = CharField(max_length=32, null=False)
    llm_factory = CharField(
        max_length=128,
        null=False,
        help_text="LLM factory name")
    model_type = CharField(
        max_length=128,
        null=True,
        help_text="LLM, Text Embedding, Image2Text, ASR")
    llm_name = CharField(
        max_length=128,
        null=True,
        help_text="LLM name",
        default="")
    api_key = CharField(max_length=255, null=True, help_text="API KEY")
    api_base = CharField(max_length=255, null=True, help_text="API Base")
    used_tokens = IntegerField(default=0)

    def __str__(self):
        return self.llm_name

    class Meta:
        db_table = "tenant_llm"
        primary_key = CompositeKey('tenant_id', 'llm_factory', 'llm_name')


class Knowledgebase(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    avatar = TextField(null=True, help_text="avatar base64 string")
    tenant_id = CharField(max_length=32, null=False)
    name = CharField(
        max_length=128,
        null=False,
        help_text="KB name",
        index=True)
    language = CharField(
        max_length=32,
        null=True,
        default="English",
        help_text="English|Chinese")
    description = TextField(null=True, help_text="KB description")
    embd_id = CharField(
        max_length=128,
        null=False,
        help_text="default embedding model ID")
    permission = CharField(
        max_length=16,
        null=False,
        help_text="me|team",
        default="me")
    created_by = CharField(max_length=32, null=False)
    doc_num = IntegerField(default=0)
    token_num = IntegerField(default=0)
    chunk_num = IntegerField(default=0)
    similarity_threshold = FloatField(default=0.2)
    vector_similarity_weight = FloatField(default=0.3)

    parser_id = CharField(
        max_length=32,
        null=False,
        help_text="default parser ID",
        default=ParserType.NAIVE.value)
    parser_config = JSONField(null=False, default={"pages": [[1, 1000000]]})
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted，1: validate)",
        default="1")

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
        help_text="default parser ID")
    parser_config = JSONField(null=False, default={"pages": [[1, 1000000]]})
    source_type = CharField(
        max_length=128,
        null=False,
        default="local",
        help_text="where dose this document from")
    type = CharField(max_length=32, null=False, help_text="file extension")
    created_by = CharField(
        max_length=32,
        null=False,
        help_text="who created it")
    name = CharField(
        max_length=255,
        null=True,
        help_text="file name",
        index=True)
    location = CharField(
        max_length=255,
        null=True,
        help_text="where dose it store")
    size = IntegerField(default=0)
    token_num = IntegerField(default=0)
    chunk_num = IntegerField(default=0)
    progress = FloatField(default=0)
    progress_msg = TextField(
        null=True,
        help_text="process message",
        default="")
    process_begin_at = DateTimeField(null=True)
    process_duation = FloatField(default=0)
    run = CharField(
        max_length=1,
        null=True,
        help_text="start to run processing or cancel.(1: run it; 2: cancel)",
        default="0")
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted，1: validate)",
        default="1")

    class Meta:
        db_table = "document"


class Task(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    doc_id = CharField(max_length=32, null=False, index=True)
    from_page = IntegerField(default=0)
    to_page = IntegerField(default=-1)
    begin_at = DateTimeField(null=True)
    process_duation = FloatField(default=0)
    progress = FloatField(default=0)
    progress_msg = TextField(
        null=True,
        help_text="process message",
        default="")


class Dialog(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    tenant_id = CharField(max_length=32, null=False)
    name = CharField(
        max_length=255,
        null=True,
        help_text="dialog application name")
    description = TextField(null=True, help_text="Dialog description")
    icon = TextField(null=True, help_text="icon base64 string")
    language = CharField(
        max_length=32,
        null=True,
        default="Chinese",
        help_text="English|Chinese")
    llm_id = CharField(max_length=32, null=False, help_text="default llm ID")
    llm_setting = JSONField(null=False, default={"temperature": 0.1, "top_p": 0.3, "frequency_penalty": 0.7,
                                                 "presence_penalty": 0.4, "max_tokens": 215})
    prompt_type = CharField(
        max_length=16,
        null=False,
        default="simple",
        help_text="simple|advanced")
    prompt_config = JSONField(null=False, default={"system": "", "prologue": "您好，我是您的助手小樱，长得可爱又善良，can I help you?",
                                                   "parameters": [], "empty_response": "Sorry! 知识库中未找到相关内容！"})

    similarity_threshold = FloatField(default=0.2)
    vector_similarity_weight = FloatField(default=0.3)
    top_n = IntegerField(default=6)
    do_refer = CharField(
        max_length=1,
        null=False,
        help_text="it needs to insert reference index into answer or not",
        default="1")

    kb_ids = JSONField(null=False, default=[])
    status = CharField(
        max_length=1,
        null=True,
        help_text="is it validate(0: wasted，1: validate)",
        default="1")

    class Meta:
        db_table = "dialog"


# class DialogKb(DataBaseModel):
#     dialog_id = CharField(max_length=32, null=False, index=True)
#     kb_id = CharField(max_length=32, null=False)
#
#     class Meta:
#         db_table = "dialog_kb"
#         primary_key = CompositeKey('dialog_id', 'kb_id')


class Conversation(DataBaseModel):
    id = CharField(max_length=32, primary_key=True)
    dialog_id = CharField(max_length=32, null=False, index=True)
    name = CharField(max_length=255, null=True, help_text="converastion name")
    message = JSONField(null=True)
    reference = JSONField(null=True, default=[])

    class Meta:
        db_table = "conversation"


"""

    class Meta:
        db_table = 't_pipeline_component_meta'
        indexes = (
            (('f_model_id', 'f_model_version', 'f_role', 'f_party_id', 'f_component_name'), True),
        )


"""
