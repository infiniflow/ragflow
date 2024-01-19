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

from api.db import TenantPermission
from api.db.db_models import DB, Tenant
from api.db.db_models import Knowledgebase
from api.db.services.common_service import CommonService
from api.db import StatusEnum


class KnowledgebaseService(CommonService):
    model = Knowledgebase

    @classmethod
    @DB.connection_context()
    def get_by_tenant_ids(cls, joined_tenant_ids, user_id,
                          page_number, items_per_page, orderby, desc):
        kbs = cls.model.select().where(
            ((cls.model.tenant_id.in_(joined_tenant_ids) & (cls.model.permission ==
             TenantPermission.TEAM.value)) | (cls.model.tenant_id == user_id))
            & (cls.model.status == StatusEnum.VALID.value)
        )
        if desc:
            kbs = kbs.order_by(cls.model.getter_by(orderby).desc())
        else:
            kbs = kbs.order_by(cls.model.getter_by(orderby).asc())

        kbs = kbs.paginate(page_number, items_per_page)

        return list(kbs.dicts())

    @classmethod
    @DB.connection_context()
    def get_detail(cls, kb_id):
        fields = [
            cls.model.id,
            Tenant.embd_id,
            cls.model.avatar,
            cls.model.name,
            cls.model.description,
            cls.model.permission,
            cls.model.doc_num,
            cls.model.token_num,
            cls.model.chunk_num,
            cls.model.parser_id]
        kbs = cls.model.select(*fields).join(Tenant, on=((Tenant.id == cls.model.tenant_id)&(Tenant.status== StatusEnum.VALID.value))).where(
            (cls.model.id == kb_id),
            (cls.model.status == StatusEnum.VALID.value)
        )
        if not kbs:
            return
        d = kbs[0].to_dict()
        d["embd_id"] = kbs[0].tenant.embd_id
        return d
