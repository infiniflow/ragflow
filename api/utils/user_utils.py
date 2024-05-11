

from api.db.db_models import TenantLLM
from api.db.services.llm_service import TenantLLMService, LLMService
from api.utils import get_uuid
from api.db import UserTenantRole, FileType
from api.settings import CHAT_MDL, EMBEDDING_MDL, ASR_MDL, IMAGE2TEXT_MDL, PARSERS, API_KEY, \
    LLM_FACTORY, LLM_BASE_URL
from api.db.services.user_service import UserService, TenantService, UserTenantService
from api.db.services.file_service import FileService

def rollback_user_registration(user_id):
    try:
        UserService.delete_by_id(user_id)
    except Exception as e:
        pass
    try:
        TenantService.delete_by_id(user_id)
    except Exception as e:
        pass
    try:
        u = UserTenantService.query(tenant_id=user_id)
        if u:
            UserTenantService.delete_by_id(u[0].id)
    except Exception as e:
        pass
    try:
        TenantLLM.delete().where(TenantLLM.tenant_id == user_id).execute()
    except Exception as e:
        pass


def user_register(user_id, user):
    user["id"] = user_id
    tenant = {
        "id": user_id,
        "name": user["nickname"] + "‘s Kingdom",
        "llm_id": CHAT_MDL,
        "embd_id": EMBEDDING_MDL,
        "asr_id": ASR_MDL,
        "parser_ids": PARSERS,
        "img2txt_id": IMAGE2TEXT_MDL
    }
    usr_tenant = {
        "tenant_id": user_id,
        "user_id": user_id,
        "invited_by": user_id,
        "role": UserTenantRole.OWNER
    }
    file_id = get_uuid()
    file = {
        "id": file_id,
        "parent_id": file_id,
        "tenant_id": user_id,
        "created_by": user_id,
        "name": "/",
        "type": FileType.FOLDER.value,
        "size": 0,
        "location": "",
    }
    tenant_llm = []
    for llm in LLMService.query(fid=LLM_FACTORY):
        tenant_llm.append({"tenant_id": user_id,
                           "llm_factory": LLM_FACTORY,
                           "llm_name": llm.llm_name,
                           "model_type": llm.model_type,
                           "api_key": API_KEY,
                           "api_base": LLM_BASE_URL
                           })

    if not UserService.save(**user):
        return
    TenantService.insert(**tenant)
    UserTenantService.insert(**usr_tenant)
    TenantLLMService.insert_many(tenant_llm)
    FileService.insert(file)
    return UserService.query(email=user["email"])