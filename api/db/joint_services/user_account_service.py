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
import logging
import uuid

from api import settings
from api.db import FileType, UserTenantRole
from api.db.db_models import TenantLLM
from api.db.services.llm_service import get_init_tenant_llm
from api.db.services.file_service import FileService
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.user_service import TenantService, UserService, UserTenantService



def create_new_user(user_info: dict) -> dict:
    """
    Add a new user, and create tenant, tenant llm, file folder for new user.
    :param user_info: {
        "email": <example@example.com>,
        "nickname": <str, "name">,
        "password": <decrypted password>,
        "login_channel": <enum, "password">,
        "is_superuser": <bool, role == "admin">,
    }
    :return: {
        "success": <bool>,
        "user_info": <dict>, # if true, return user_info
    }
    """
    # generate user_id and access_token for user
    user_id = uuid.uuid1().hex
    user_info['id'] = user_id
    user_info['access_token'] = uuid.uuid1().hex
    # construct tenant info
    tenant = {
        "id": user_id,
        "name": user_info["nickname"] + "‘s Kingdom",
        "llm_id": settings.CHAT_MDL,
        "embd_id": settings.EMBEDDING_MDL,
        "asr_id": settings.ASR_MDL,
        "parser_ids": settings.PARSERS,
        "img2txt_id": settings.IMAGE2TEXT_MDL,
        "rerank_id": settings.RERANK_MDL,
    }
    usr_tenant = {
        "tenant_id": user_id,
        "user_id": user_id,
        "invited_by": user_id,
        "role": UserTenantRole.OWNER,
    }
    # construct file folder info
    file_id = uuid.uuid1().hex
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
    try:
        tenant_llm = get_init_tenant_llm(user_id)

        if not UserService.save(**user_info):
            return {"success": False}

        TenantService.insert(**tenant)
        UserTenantService.insert(**usr_tenant)
        TenantLLMService.insert_many(tenant_llm)
        FileService.insert(file)

        return {
            "success": True,
            "user_info": user_info,
        }

    except Exception as create_error:
        logging.exception(create_error)
        # rollback
        try:
            TenantService.delete_by_id(user_id)
        except Exception as e:
            logging.exception(e)
        try:
            u = UserTenantService.query(tenant_id=user_id)
            if u:
                UserTenantService.delete_by_id(u[0].id)
        except Exception as e:
            logging.exception(e)
        try:
            TenantLLM.delete().where(TenantLLM.tenant_id == user_id).execute()
        except Exception as e:
            logging.exception(e)
        try:
            FileService.delete_by_id(file["id"])
        except Exception as e:
            logging.exception(e)
        # delete user row finally
        try:
            UserService.delete_by_id(user_id)
        except Exception as e:
            logging.exception(e)
        # reraise
        raise create_error
