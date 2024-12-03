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
import operator
from functools import reduce

from playhouse.pool import PooledMySQLDatabase

from api.utils import current_timestamp, timestamp_to_date

from api.db.db_models import DB, DataBaseModel


@DB.connection_context()
def bulk_insert_into_db(model, data_source, replace_on_conflict=False):
    DB.create_tables([model])

    for i, data in enumerate(data_source):
        current_time = current_timestamp() + i
        current_date = timestamp_to_date(current_time)
        if 'create_time' not in data:
            data['create_time'] = current_time
        data['create_date'] = timestamp_to_date(data['create_time'])
        data['update_time'] = current_time
        data['update_date'] = current_date

    preserve = tuple(data_source[0].keys() - {'create_time', 'create_date'})

    batch_size = 1000

    for i in range(0, len(data_source), batch_size):
        with DB.atomic():
            query = model.insert_many(data_source[i:i + batch_size])
            if replace_on_conflict:
                if isinstance(DB, PooledMySQLDatabase):
                    query = query.on_conflict(preserve=preserve)
                else:
                    query = query.on_conflict(conflict_target="id", preserve=preserve)
            query.execute()


def get_dynamic_db_model(base, job_id):
    return type(base.model(
        table_index=get_dynamic_tracking_table_index(job_id=job_id)))


def get_dynamic_tracking_table_index(job_id):
    return job_id[:8]


def fill_db_model_object(model_object, human_model_dict):
    for k, v in human_model_dict.items():
        attr_name = 'f_%s' % k
        if hasattr(model_object.__class__, attr_name):
            setattr(model_object, attr_name, v)
    return model_object


# https://docs.peewee-orm.com/en/latest/peewee/query_operators.html
supported_operators = {
    '==': operator.eq,
    '<': operator.lt,
    '<=': operator.le,
    '>': operator.gt,
    '>=': operator.ge,
    '!=': operator.ne,
    '<<': operator.lshift,
    '>>': operator.rshift,
    '%': operator.mod,
    '**': operator.pow,
    '^': operator.xor,
    '~': operator.inv,
}


def query_dict2expression(
        model: type[DataBaseModel], query: dict[str, bool | int | str | list | tuple]):
    expression = []

    for field, value in query.items():
        if not isinstance(value, (list, tuple)):
            value = ('==', value)
        op, *val = value

        field = getattr(model, f'f_{field}')
        value = supported_operators[op](
            field, val[0]) if op in supported_operators else getattr(
            field, op)(
            *val)
        expression.append(value)

    return reduce(operator.iand, expression)


def query_db(model: type[DataBaseModel], limit: int = 0, offset: int = 0,
             query: dict = None, order_by: str | list | tuple | None = None):
    data = model.select()
    if query:
        data = data.where(query_dict2expression(model, query))
    count = data.count()

    if not order_by:
        order_by = 'create_time'
    if not isinstance(order_by, (list, tuple)):
        order_by = (order_by, 'asc')
    order_by, order = order_by
    order_by = getattr(model, f'f_{order_by}')
    order_by = getattr(order_by, order)()
    data = data.order_by(order_by)

    if limit > 0:
        data = data.limit(limit)
    if offset > 0:
        data = data.offset(offset)

    return list(data), count
