#
#  Copyright 2019 The FATE Authors. All Rights Reserved.
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
from web_server.db import TenantPermission, FileType
from web_server.db.db_models import DB, Knowledgebase
from web_server.db.db_models import Document
from web_server.db.services.common_service import CommonService
from web_server.db.services.kb_service import KnowledgebaseService
from web_server.utils import get_uuid, get_format_time
from web_server.db.db_utils import StatusEnum


class DocumentService(CommonService):
    model = Document

    @classmethod
    @DB.connection_context()
    def get_by_kb_id(cls, kb_id, page_number, items_per_page,
                     orderby, desc, keywords):
        if keywords:
            docs = cls.model.select().where(
                cls.model.kb_id == kb_id,
                cls.model.name.like(f"%%{keywords}%%"))
        else:
            docs = cls.model.select().where(cls.model.kb_id == kb_id)
        if desc:
            docs = docs.order_by(cls.model.getter_by(orderby).desc())
        else:
            docs = docs.order_by(cls.model.getter_by(orderby).asc())

        docs = docs.paginate(page_number, items_per_page)

        return list(docs.dicts())

    @classmethod
    @DB.connection_context()
    def insert(cls, doc):
        if not cls.save(**doc):
            raise RuntimeError("Database error (Document)!")
        e, doc = cls.get_by_id(doc["id"])
        if not e:
            raise RuntimeError("Database error (Document retrieval)!")
        e, kb = KnowledgebaseService.get_by_id(doc.kb_id)
        if not KnowledgebaseService.update_by_id(
                kb.id, {"doc_num": kb.doc_num + 1}):
            raise RuntimeError("Database error (Knowledgebase)!")
        return doc

    @classmethod
    @DB.connection_context()
    def get_newly_uploaded(cls, tm, mod, comm, items_per_page=64):
        fields = [cls.model.id, cls.model.kb_id, cls.model.parser_id, cls.model.name, cls.model.location, Knowledgebase.tenant_id]
        docs = cls.model.select(fields).join(Knowledgebase, on=(cls.model.kb_id == Knowledgebase.id)).where(
            cls.model.status == StatusEnum.VALID.value,
            cls.model.type != FileType.VIRTUAL,
            cls.model.progress == 0,
            cls.model.update_time >= tm,
            cls.model.create_time %
            comm == mod).order_by(
            cls.model.update_time.asc()).paginate(
            1,
            items_per_page)
        return list(docs.dicts())
