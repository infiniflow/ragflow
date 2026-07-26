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

from types import SimpleNamespace

from common.constants import ActiveStatusEnum
from api.db.joint_services import tenant_model_service as module


def test_resolve_instance_for_model_falls_back_from_default_to_single_active_instance(monkeypatch):
    provider = SimpleNamespace(id="provider-1", provider_name="SILICONFLOW")
    resolved = SimpleNamespace(
        id="instance-1",
        instance_name="yy2",
        status=ActiveStatusEnum.ACTIVE.value,
    )

    monkeypatch.setattr(
        module.TenantModelInstanceService,
        "get_by_provider_id_and_instance_name",
        lambda provider_id, instance_name: None,
    )
    monkeypatch.setattr(
        module.TenantModelInstanceService,
        "get_all_by_provider_id",
        lambda provider_id: [resolved],
    )

    got = module._resolve_instance_for_model(
        provider,
        "default",
        "Qwen/Qwen3-8B@default@SILICONFLOW",
    )

    assert got is resolved
