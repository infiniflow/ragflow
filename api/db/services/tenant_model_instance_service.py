#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
from common.misc_utils import get_uuid
from api.db.db_models import DB, TenantModelInstance
from api.db.services.common_service import CommonService
from api.db.services import duplicate_name

class TenantModelInstanceService(CommonService):
    model = TenantModelInstance

    @classmethod
    @DB.connection_context()
    def create_instance(cls, provider_id: str, instance_name: str, api_key: str, extra: str):
        unique_instance_name = duplicate_name(cls.query, name_field="instance_name", provider_id=provider_id, instance_name=instance_name)
        return cls.insert(id=get_uuid(), provider_id=provider_id, instance_name=unique_instance_name, api_key=api_key, extra=extra)

    @classmethod
    @DB.connection_context()
    def get_all_by_provider_id(cls, provider_id):
        return list(cls.model.select().where(cls.model.provider_id == provider_id))

    @classmethod
    @DB.connection_context()
    def get_by_provider_ids(cls, provider_ids):
        return list(cls.model.select().where(cls.model.provider_id.in_(provider_ids)))

    @classmethod
    @DB.connection_context()
    def get_by_provider_id_and_instance_name(cls, provider_id, instance_name):
        return cls.model.get_or_none(
            cls.model.provider_id == provider_id,
            cls.model.instance_name == instance_name,
        )

    @classmethod
    @DB.connection_context()
    def get_by_provider_id_and_api_key(cls, provider_id, api_key):
        return cls.model.get_or_none(
            cls.model.provider_id == provider_id,
            cls.model.api_key == api_key
        )

    @classmethod
    @DB.connection_context()
    def delete_by_provider_id_and_instance_name(cls, provider_id, instance_name):
        return cls.model.delete().where(
            cls.model.provider_id == provider_id,
            cls.model.instance_name == instance_name,
        ).execute()

    @classmethod
    @DB.connection_context()
    def delete_by_provider_ids(cls, provider_ids):
        return cls.model.delete().where(
            cls.model.provider_id.in_(provider_ids)
        ).execute()
