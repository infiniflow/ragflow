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
from datetime import datetime

import peewee

from api.db.db_models import DB, API4Conversation, APIToken, Dialog
from api.db.services.common_service import CommonService
from api.utils import current_timestamp, datetime_format


class APITokenService(CommonService):
    model = APIToken

    @classmethod
    @DB.connection_context()
    def used(cls, token):
        return cls.model.update({
            "update_time": current_timestamp(),
            "update_date": datetime_format(datetime.now()),
        }).where(
            cls.model.token == token
        )


class API4ConversationService(CommonService):
    model = API4Conversation

    @classmethod
    @DB.connection_context()
    def get_list(cls, dialog_id, tenant_id,
                 page_number, items_per_page,
                 orderby, desc, id, user_id=None, include_dsl=True, keywords="",
                 from_date=None, to_date=None
                 ):
        if include_dsl:
            sessions = cls.model.select().where(cls.model.dialog_id == dialog_id)
        else:
            fields = [field for field in cls.model._meta.fields.values() if field.name != 'dsl']
            sessions = cls.model.select(*fields).where(cls.model.dialog_id == dialog_id)
        if id:
            sessions = sessions.where(cls.model.id == id)
        if user_id:
            sessions = sessions.where(cls.model.user_id == user_id)
        if keywords:
            sessions = sessions.where(peewee.fn.LOWER(cls.model.message).contains(keywords.lower()))
        if from_date:
            sessions = sessions.where(cls.model.create_date >= from_date)
        if to_date:
            sessions = sessions.where(cls.model.create_date <= to_date)
        if desc:
            sessions = sessions.order_by(cls.model.getter_by(orderby).desc())
        else:
            sessions = sessions.order_by(cls.model.getter_by(orderby).asc())
        count = sessions.count()
        sessions = sessions.paginate(page_number, items_per_page)

        return count, list(sessions.dicts())

    @classmethod
    @DB.connection_context()
    def append_message(cls, id, conversation):
        cls.update_by_id(id, conversation)
        return cls.model.update(round=cls.model.round + 1).where(cls.model.id == id).execute()

    @classmethod
    @DB.connection_context()
    def stats(cls, tenant_id, from_date, to_date, source=None):
        if len(to_date) == 10:
            to_date += " 23:59:59"
        return cls.model.select(
            cls.model.create_date.truncate("day").alias("dt"),
            peewee.fn.COUNT(
                cls.model.id).alias("pv"),
            peewee.fn.COUNT(
                cls.model.user_id.distinct()).alias("uv"),
            peewee.fn.SUM(
                cls.model.tokens).alias("tokens"),
            peewee.fn.SUM(
                cls.model.duration).alias("duration"),
            peewee.fn.AVG(
                cls.model.round).alias("round"),
            peewee.fn.SUM(
                cls.model.thumb_up).alias("thumb_up")
        ).join(Dialog, on=((cls.model.dialog_id == Dialog.id) & (Dialog.tenant_id == tenant_id))).where(
            cls.model.create_date >= from_date,
            cls.model.create_date <= to_date,
            cls.model.source == source
        ).group_by(cls.model.create_date.truncate("day")).dicts()
