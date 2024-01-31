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
from peewee import Expression
from api.db.db_models import DB
from api.db import StatusEnum, FileType
from api.db.db_models import Task, Document, Knowledgebase, Tenant
from api.db.services.common_service import CommonService


class TaskService(CommonService):
    model = Task

    @classmethod
    @DB.connection_context()
    def get_tasks(cls, tm, mod=0, comm=1, items_per_page=64):
        fields = [cls.model.id, cls.model.doc_id, cls.model.from_page,cls.model.to_page, Document.kb_id, Document.parser_id, Document.name, Document.type, Document.location, Document.size, Knowledgebase.tenant_id, Tenant.embd_id, Tenant.img2txt_id, Tenant.asr_id, cls.model.update_time]
        docs = cls.model.select(*fields) \
            .join(Document, on=(cls.model.doc_id == Document.id)) \
            .join(Knowledgebase, on=(Document.kb_id == Knowledgebase.id)) \
            .join(Tenant, on=(Knowledgebase.tenant_id == Tenant.id))\
            .where(
                Document.status == StatusEnum.VALID.value,
                ~(Document.type == FileType.VIRTUAL.value),
                cls.model.progress == 0,
                cls.model.update_time >= tm,
                (Expression(cls.model.create_time, "%%", comm) == mod))\
            .order_by(cls.model.update_time.asc())\
            .paginate(1, items_per_page)
        return list(docs.dicts())


    @classmethod
    @DB.connection_context()
    def do_cancel(cls, id):
        try:
            cls.model.get_by_id(id)
            return False
        except Exception as e:
            pass
        return True
