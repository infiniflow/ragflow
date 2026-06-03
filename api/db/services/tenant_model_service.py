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
from api.db.db_models import DB, TenantModel
from api.db.services.common_service import CommonService


class TenantModelService(CommonService):
    model = TenantModel

    @classmethod
    @DB.connection_context()
    def get_by_provider_id_and_instance_id_and_model_name(cls, provider_id, instance_id, model_name):
        return list(cls.model.select().where(cls.model.provider_id == provider_id, cls.model.instance_id == instance_id, cls.model.model_name == model_name))

    @classmethod
    @DB.connection_context()
    def get_by_provider_id_and_instance_id_and_model_type_and_model_name(cls, provider_id, instance_id, model_type, model_name):
        return cls.model.get_or_none(
            cls.model.provider_id == provider_id,
            cls.model.instance_id == instance_id,
            cls.model.model_type == model_type,
            cls.model.model_name == model_name
        )

    @classmethod
    @DB.connection_context()
    def get_models_by_instance_id(cls, instance_id):
        return list(cls.model.select().where(cls.model.instance_id == instance_id))

    @classmethod
    @DB.connection_context()
    def get_models_by_provider_ids_and_instance_ids(cls, provider_ids, instance_ids):
        return list(cls.model.select().where(cls.model.provider_id.in_(provider_ids), cls.model.instance_id.in_(instance_ids)))

    @classmethod
    @DB.connection_context()
    def batch_update_model_status(cls, model_ids, status):
        return cls.model.update(status=status).where(cls.model.id.in_(model_ids)).execute()

    @classmethod
    @DB.connection_context()
    def delete_by_id(cls, model_id):
        return cls.model.delete().where(cls.model.id == model_id).execute()

    @classmethod
    @DB.connection_context()
    def delete_by_instance_ids(cls, instance_ids):
        return cls.model.delete().where(cls.model.instance_id.in_(instance_ids)).execute()
