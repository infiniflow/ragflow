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

from api.db import TenantPermission, FileType, TaskStatus
from api.db.db_models import DB, Document, Tenant
from api.db.db_models import Folder
from api.db.services.common_service import CommonService


class FolderService(CommonService):
    model = Folder

    @classmethod
    @DB.connection_context()
    def get_by_pf_id(cls, pf_id, page_number, items_per_page,
                     orderby, desc, keywords):
        if keywords:
            files = cls.model.select().where(
                cls.model.pf_id == pf_id,
                cls.model.name.like(f"%%{keywords}%%"))
        else:
            files = cls.model.select().where(cls.model.pf_id == pf_id)
        count = files.count()
        if desc:
            files = files.order_by(cls.model.getter_by(orderby).desc())
        else:
            files = files.order_by(cls.model.getter_by(orderby).asc())

        files = files.paginate(page_number, items_per_page)

        return list(files.dicts()), count

    @classmethod
    @DB.connection_context()
    def insert(cls, file):
        if not cls.save(**file):
            raise RuntimeError("Database error (File)!")
        e, file = cls.get_by_id(file["id"])
        if not e:
            raise RuntimeError("Database error (File retrieval)!")
        return file

    @classmethod
    @DB.connection_context()
    def delete(cls, folder):
        return cls.delete_by_id(folder.id)
