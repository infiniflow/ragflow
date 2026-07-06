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
from common.constants import ActiveStatusEnum
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
        return cls.model.get_or_none(cls.model.provider_id == provider_id, cls.model.instance_id == instance_id, cls.model.model_type == model_type, cls.model.model_name == model_name)

    @classmethod
    @DB.connection_context()
    def get_by_provider_id_and_instance_id_and_model_type(cls, provider_id, instance_id, model_type):
        return cls.model.get_or_none(cls.model.provider_id == provider_id, cls.model.instance_id == instance_id, cls.model.model_type == model_type)

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
    def upsert_model_type(cls, provider_id: str, instance_id: str, model_name: str, operation: dict):
        model_type_records = cls.model.select().where(cls.model.provider_id == provider_id, cls.model.instance_id == instance_id, cls.model.model_name == model_name)
        if not model_type_records:
            for _type in operation.get("add", []):
                cls.insert(model_name=model_name, provider_id=provider_id, instance_id=instance_id, model_type=_type, extra="{}")
            for _type in operation.get("delete", []):
                cls.insert(model_name=model_name, provider_id=provider_id, instance_id=instance_id, model_type=_type, status=ActiveStatusEnum.UNSUPPORTED, extra="{}")
            return len(operation.get("add", [])) + len(operation.get("delete", []))
        model_record_example = [model_record for model_record in model_type_records if model_record.status != ActiveStatusEnum.UNSUPPORTED.value]
        extra_fields = model_record_example[0].extra if model_record_example else "{}"
        model_status = model_record_example[0].status if model_record_example else ActiveStatusEnum.ACTIVE.value
        type_record_map = {record.model_type: record for record in model_type_records}
        operated_cnt = 0
        for _type in operation.get("add", []):
            if type_record_map.get(_type):
                cls.update_by_id(type_record_map[_type].id, {"status": model_status})

            else:
                cls.insert(model_name=model_name, provider_id=provider_id, instance_id=instance_id, model_type=_type, status=model_status, extra=extra_fields)
            operated_cnt += 1
        for _type in operation.get("delete", []):
            if type_record_map.get(_type):
                cls.update_by_id(type_record_map[_type].id, {"status": ActiveStatusEnum.UNSUPPORTED.value})
            else:
                cls.insert(model_name=model_name, provider_id=provider_id, instance_id=instance_id, model_type=_type, status=ActiveStatusEnum.UNSUPPORTED.value, extra=extra_fields)
            operated_cnt += 1
        return operated_cnt

    @classmethod
    @DB.connection_context()
    def delete_by_id(cls, model_id):
        return cls.model.delete().where(cls.model.id == model_id).execute()

    @classmethod
    @DB.connection_context()
    def delete_by_instance_ids(cls, instance_ids):
        return cls.model.delete().where(cls.model.instance_id.in_(instance_ids)).execute()
