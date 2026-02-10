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
from api.db.services.tenant_llm_service import TenantLLMService

def ensure_tenant_model_id_for_params(tenant_id: str, param_dict: dict) -> dict:
    for key in ["llm_id", "embd_id", "asr_id", "img2txt_id", "rerank_id", "tts_id"]:
        if param_dict.get(key) and not param_dict.get(f"tenant_{key}"):
            tenant_model = TenantLLMService.get_api_key(tenant_id, param_dict[key])
            if tenant_model:
                param_dict.update({f"tenant_{key}": tenant_model.id})
    return param_dict
