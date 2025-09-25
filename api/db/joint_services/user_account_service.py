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
from api.db import FileType, UserTenantRole, ActiveEnum
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.langfuse_service import TenantLangfuseService
from api.db.services.llm_service import get_init_tenant_llm
from api.db.services.file_service import FileService
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.user_service import TenantService, UserService, UserTenantService
from rag.utils.storage_factory import STORAGE_IMPL
from rag.nlp import search


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
            TenantLLMService.delete_by_tenant_id(user_id)
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


def delete_user_data(user_id: str) -> dict:
    # use user_id to delete
    usr = UserService.filter_by_id(user_id)
    if not usr:
        return {"success": False, "message": "User not exist or already deleted."}
    # check is inactive and not admin
    if usr.is_active == ActiveEnum.ACTIVE.value:
        return {"success": False, "message": "User is active, cannot delete."}
    if usr.is_superuser:
        return {"success": False, "message": "Cannot delete admin account."}
    # tenant info
    tenants = UserTenantService.get_user_tenant_relation_by_user_id(usr.id)
    owned_tenant = [t for t in tenants if t["role"] == UserTenantRole.OWNER.value]

    done_msg = ''
    try:
        # step1. delete owned tenant info
        if owned_tenant:
            tenant_id = owned_tenant[0]["tenant_id"]
            kb_ids = KnowledgebaseService.get_kb_ids(usr.id)
            # step1.1 delete kb related file and info
            if kb_ids:
                # step1.1.1 delete files in storage, rm bucket
                for kb_id in kb_ids:
                    STORAGE_IMPL.remove_bucket(kb_id)
                done_msg += f"- Removed {len(kb_ids)} dataset's buckets.\n"
                # step1.1.2 delete file and document info in db
                doc_ids = DocumentService.get_all_doc_ids_by_kb_ids(kb_ids)
                if doc_ids:
                    doc_res = DocumentService.delete_by_ids(doc_ids)
                    done_msg += f"- Deleted {doc_res} document records.\n"
                file_ids = FileService.get_all_file_ids_by_tenant_id(usr.id)
                if file_ids:
                    file_res = FileService.delete_by_ids(file_ids)
                    done_msg += f"- Deleted {file_res} file records.\n"
                if doc_ids or file_ids:
                    rl_res = File2DocumentService.delete_by_document_ids_or_file_ids(doc_ids, file_ids)
                    done_msg += f"- Deleted {rl_res} document-file relation records.\n"
                # step1.1.3 delete dataset info
                r = settings.docStoreConn.delete({"kb_id": kb_ids},
                                         search.index_name(tenant_id), kb_ids)
                done_msg += f"- Deleted {r} doc store records.\n"
                kb_res = KnowledgebaseService.delete_by_ids(kb_ids)
                done_msg += f"- Deleted {kb_res} knowledgebase records.\n"
            # step1.2 delete tenant_llm and tenant_langfuse
            llm_res = TenantLLMService.delete_by_tenant_id(tenant_id)
            done_msg += f"- Deleted {llm_res} tenant-LLM records.\n"
            langfuse_res = TenantLangfuseService.delete_ty_tenant_id(tenant_id)
            done_msg += f"- Deleted {langfuse_res} langfuse records.\n"
            # step1.3 delete own tenant
            t_res = TenantService.delete_by_id(tenant_id)
            done_msg += f"- Deleted {t_res} tenant.\n"
        # step2 delete user-tenant relation
        if tenants:
            ut_res = TenantService.delete_by_ids([t["id"] for t in tenants])
            done_msg += f"- Deleted {ut_res} user-tenant records.\n"
        # step3 finally delete user
        u_res = UserService.delete_by_id(usr.id)
        done_msg += f"- Deleted {u_res} user.\n"

        return {"success": True, "message": f"Successfully deleted user. Details:\n{done_msg}"}

    except Exception as e:
        logging.exception(e)
        return {"success": False, "message": f"Fail to delete user, error: {str(e)}. Already done:\n{done_msg}"}
