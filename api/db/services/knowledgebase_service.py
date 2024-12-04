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
from api.db import StatusEnum, TenantPermission
from api.db.db_models import Knowledgebase, DB, Tenant, User, UserTenant,Document
from api.db.services.common_service import CommonService
from peewee import fn


class KnowledgebaseService(CommonService):
    model = Knowledgebase

    @classmethod
    @DB.connection_context()
    def list_documents_by_ids(cls,kb_ids):
        doc_ids=cls.model.select(Document.id.alias("document_id")).join(Document,on=(cls.model.id == Document.kb_id)).where(
            cls.model.id.in_(kb_ids)
        )
        doc_ids =list(doc_ids.dicts())
        doc_ids = [doc["document_id"] for doc in doc_ids]
        return doc_ids

    @classmethod
    @DB.connection_context()
    def get_by_tenant_ids(cls, joined_tenant_ids, user_id,
                          page_number, items_per_page, orderby, desc, keywords):
        fields = [
            cls.model.id,
            cls.model.avatar,
            cls.model.name,
            cls.model.language,
            cls.model.description,
            cls.model.permission,
            cls.model.doc_num,
            cls.model.token_num,
            cls.model.chunk_num,
            cls.model.parser_id,
            cls.model.embd_id,
            User.nickname,
            User.avatar.alias('tenant_avatar'),
            cls.model.update_time
        ]
        if keywords:
            kbs = cls.model.select(*fields).join(User, on=(cls.model.tenant_id == User.id)).where(
                ((cls.model.tenant_id.in_(joined_tenant_ids) & (cls.model.permission ==
                                                                TenantPermission.TEAM.value)) | (
                    cls.model.tenant_id == user_id))
                & (cls.model.status == StatusEnum.VALID.value),
                (fn.LOWER(cls.model.name).contains(keywords.lower()))
            )
        else:
            kbs = cls.model.select(*fields).join(User, on=(cls.model.tenant_id == User.id)).where(
                ((cls.model.tenant_id.in_(joined_tenant_ids) & (cls.model.permission ==
                                                                TenantPermission.TEAM.value)) | (
                    cls.model.tenant_id == user_id))
                & (cls.model.status == StatusEnum.VALID.value)
            )
        if desc:
            kbs = kbs.order_by(cls.model.getter_by(orderby).desc())
        else:
            kbs = kbs.order_by(cls.model.getter_by(orderby).asc())

        count = kbs.count()

        kbs = kbs.paginate(page_number, items_per_page)

        return list(kbs.dicts()), count

    @classmethod
    @DB.connection_context()
    def get_kb_ids(cls, tenant_id):
        fields = [
            cls.model.id,
        ]
        kbs = cls.model.select(*fields).where(cls.model.tenant_id == tenant_id)
        kb_ids = [kb.id for kb in kbs]
        return kb_ids

    @classmethod
    @DB.connection_context()
    def get_detail(cls, kb_id):
        fields = [
            cls.model.id,
            # Tenant.embd_id,
            cls.model.embd_id,
            cls.model.avatar,
            cls.model.name,
            cls.model.language,
            cls.model.description,
            cls.model.permission,
            cls.model.doc_num,
            cls.model.token_num,
            cls.model.chunk_num,
            cls.model.parser_id,
            cls.model.parser_config,
            cls.model.pagerank]
        kbs = cls.model.select(*fields).join(Tenant, on=(
                (Tenant.id == cls.model.tenant_id) & (Tenant.status == StatusEnum.VALID.value))).where(
            (cls.model.id == kb_id),
            (cls.model.status == StatusEnum.VALID.value)
        )
        if not kbs:
            return
        d = kbs[0].to_dict()
        # d["embd_id"] = kbs[0].tenant.embd_id
        return d

    @classmethod
    @DB.connection_context()
    def update_parser_config(cls, id, config):
        e, m = cls.get_by_id(id)
        if not e:
            raise LookupError(f"knowledgebase({id}) not found.")

        def dfs_update(old, new):
            for k, v in new.items():
                if k not in old:
                    old[k] = v
                    continue
                if isinstance(v, dict):
                    assert isinstance(old[k], dict)
                    dfs_update(old[k], v)
                elif isinstance(v, list):
                    assert isinstance(old[k], list)
                    old[k] = list(set(old[k] + v))
                else:
                    old[k] = v

        dfs_update(m.parser_config, config)
        cls.update_by_id(id, {"parser_config": m.parser_config})

    @classmethod
    @DB.connection_context()
    def get_field_map(cls, ids):
        conf = {}
        for k in cls.get_by_ids(ids):
            if k.parser_config and "field_map" in k.parser_config:
                conf.update(k.parser_config["field_map"])
        return conf

    @classmethod
    @DB.connection_context()
    def get_by_name(cls, kb_name, tenant_id):
        kb = cls.model.select().where(
            (cls.model.name == kb_name)
            & (cls.model.tenant_id == tenant_id)
            & (cls.model.status == StatusEnum.VALID.value)
        )
        if kb:
            return True, kb[0]
        return False, None

    @classmethod
    @DB.connection_context()
    def get_all_ids(cls):
        return [m["id"] for m in cls.model.select(cls.model.id).dicts()]

    @classmethod
    @DB.connection_context()
    def get_list(cls, joined_tenant_ids, user_id,
                 page_number, items_per_page, orderby, desc, id, name):
        kbs = cls.model.select()
        if id:
            kbs = kbs.where(cls.model.id == id)
        if name:
            kbs = kbs.where(cls.model.name == name)
        kbs = kbs.where(
            ((cls.model.tenant_id.in_(joined_tenant_ids) & (cls.model.permission ==
                                                            TenantPermission.TEAM.value)) | (
                     cls.model.tenant_id == user_id))
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
    def accessible(cls, kb_id, user_id):
        docs = cls.model.select(
            cls.model.id).join(UserTenant, on=(UserTenant.tenant_id == Knowledgebase.tenant_id)
            ).where(cls.model.id == kb_id, UserTenant.user_id == user_id).paginate(0, 1)
        docs = docs.dicts()
        if not docs:
            return False
        return True

    @classmethod
    @DB.connection_context()
    def get_kb_by_id(cls, kb_id, user_id):
        kbs = cls.model.select().join(UserTenant, on=(UserTenant.tenant_id == Knowledgebase.tenant_id)
            ).where(cls.model.id == kb_id, UserTenant.user_id == user_id).paginate(0, 1)
        kbs = kbs.dicts()
        return list(kbs)

    @classmethod
    @DB.connection_context()
    def get_kb_by_name(cls, kb_name, user_id):
        kbs = cls.model.select().join(UserTenant, on=(UserTenant.tenant_id == Knowledgebase.tenant_id)
            ).where(cls.model.name == kb_name, UserTenant.user_id == user_id).paginate(0, 1)
        kbs = kbs.dicts()
        return list(kbs)

    @classmethod
    @DB.connection_context()
    def accessible4deletion(cls, kb_id, user_id):
        docs = cls.model.select(
            cls.model.id).where(cls.model.id == kb_id, cls.model.created_by == user_id).paginate(0, 1)
        docs = docs.dicts()
        if not docs:
            return False
        return True

