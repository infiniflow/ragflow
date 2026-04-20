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
from api.db.db_models import DB, TenantModelProvider
from api.db.services.common_service import CommonService


class TenantModelProviderService(CommonService):
    model = TenantModelProvider

    @classmethod
    @DB.connection_context()
    def get_by_tenant_id_and_provider_name(cls, tenant_id, provider_name):
        return cls.model.get_or_none(
            cls.model.tenant_id == tenant_id,
            cls.model.provider_name == provider_name,
        )

    @classmethod
    @DB.connection_context()
    def delete_by_tenant_id(cls, tenant_id):
        return cls.model.delete().where(cls.model.tenant_id == tenant_id).execute()

    @classmethod
    @DB.connection_context()
    def delete_by_tenant_id_and_provider_name(cls, tenant_id, provider_name):
        return cls.model.delete().where(
            cls.model.tenant_id == tenant_id,
            cls.model.provider_name == provider_name,
        ).execute()

    @classmethod
    @DB.connection_context()
    def list_provider_names_by_tenant_id(cls, tenant_id):
        return [row.provider_name for row in cls.model.select(cls.model.provider_name).where(cls.model.tenant_id == tenant_id)]
