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

from api.utils.api_utils import group_by
from api.db import FileType, UserTenantRole
from api.db.services.api_service import APITokenService, API4ConversationService
from api.db.services.canvas_service import UserCanvasService
from api.db.services.conversation_service import ConversationService
from api.db.services.dialog_service import DialogService
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.langfuse_service import TenantLangfuseService
from api.db.services.llm_service import get_init_tenant_llm
from api.db.services.file_service import FileService
from api.db.services.mcp_server_service import MCPServerService
from api.db.services.search_service import SearchService
from api.db.services.task_service import TaskService
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.user_canvas_version import UserCanvasVersionService
from api.db.services.user_service import TenantService, UserService, UserTenantService
from rag.nlp import search
from common.constants import ActiveEnum
from common import settings

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
        return {"success": False, "message": f"{user_id} can't be found."}
    # check is inactive and not admin
    if usr.is_active == ActiveEnum.ACTIVE.value:
        return {"success": False, "message": f"{user_id} is active and can't be deleted."}
    if usr.is_superuser:
        return {"success": False, "message": "Can't delete the super user."}
    # tenant info
    tenants = UserTenantService.get_user_tenant_relation_by_user_id(usr.id)
    owned_tenant = [t for t in tenants if t["role"] == UserTenantRole.OWNER.value]

    done_msg = ''
    try:
        # step1. delete owned tenant info
        if owned_tenant:
            done_msg += "Start to delete owned tenant.\n"
            tenant_id = owned_tenant[0]["tenant_id"]
            kb_ids = KnowledgebaseService.get_kb_ids(usr.id)
            # step1.1 delete knowledgebase related file and info
            if kb_ids:
                # step1.1.1 delete files in storage, remove bucket
                for kb_id in kb_ids:
                    if settings.STORAGE_IMPL.bucket_exists(kb_id):
                        settings.STORAGE_IMPL.remove_bucket(kb_id)
                done_msg += f"- Removed {len(kb_ids)} dataset's buckets.\n"
                # step1.1.2 delete file and document info in db
                doc_ids = DocumentService.get_all_doc_ids_by_kb_ids(kb_ids)
                if doc_ids:
                    doc_delete_res = DocumentService.delete_by_ids([i["id"] for i in doc_ids])
                    done_msg += f"- Deleted {doc_delete_res} document records.\n"
                    task_delete_res = TaskService.delete_by_doc_ids([i["id"] for i in doc_ids])
                    done_msg += f"- Deleted {task_delete_res} task records.\n"
                file_ids = FileService.get_all_file_ids_by_tenant_id(usr.id)
                if file_ids:
                    file_delete_res = FileService.delete_by_ids([f["id"] for f in file_ids])
                    done_msg += f"- Deleted {file_delete_res} file records.\n"
                if doc_ids or file_ids:
                    file2doc_delete_res = File2DocumentService.delete_by_document_ids_or_file_ids(
                        [i["id"] for i in doc_ids],
                        [f["id"] for f in file_ids]
                    )
                    done_msg += f"- Deleted {file2doc_delete_res} document-file relation records.\n"
                # step1.1.3 delete chunk in es
                r = settings.docStoreConn.delete({"kb_id": kb_ids},
                                         search.index_name(tenant_id), kb_ids)
                done_msg += f"- Deleted {r} chunk records.\n"
                kb_delete_res = KnowledgebaseService.delete_by_ids(kb_ids)
                done_msg += f"- Deleted {kb_delete_res} knowledgebase records.\n"
                # step1.1.4 delete agents
                agent_delete_res = delete_user_agents(usr.id)
                done_msg += f"- Deleted {agent_delete_res['agents_deleted_count']} agent, {agent_delete_res['version_deleted_count']} versions records.\n"
                # step1.1.5 delete dialogs
                dialog_delete_res = delete_user_dialogs(usr.id)
                done_msg += f"- Deleted {dialog_delete_res['dialogs_deleted_count']} dialogs, {dialog_delete_res['conversations_deleted_count']} conversations, {dialog_delete_res['api_token_deleted_count']} api tokens, {dialog_delete_res['api4conversation_deleted_count']} api4conversations.\n"
                # step1.1.6 delete mcp server
                mcp_delete_res = MCPServerService.delete_by_tenant_id(usr.id)
                done_msg += f"- Deleted {mcp_delete_res} MCP server.\n"
                # step1.1.7 delete search
                search_delete_res = SearchService.delete_by_tenant_id(usr.id)
                done_msg += f"- Deleted {search_delete_res} search records.\n"
            # step1.2 delete tenant_llm and tenant_langfuse
            llm_delete_res = TenantLLMService.delete_by_tenant_id(tenant_id)
            done_msg += f"- Deleted {llm_delete_res} tenant-LLM records.\n"
            langfuse_delete_res = TenantLangfuseService.delete_ty_tenant_id(tenant_id)
            done_msg += f"- Deleted {langfuse_delete_res} langfuse records.\n"
            # step1.3 delete own tenant
            tenant_delete_res = TenantService.delete_by_id(tenant_id)
            done_msg += f"- Deleted {tenant_delete_res} tenant.\n"
        # step2 delete user-tenant relation
        if tenants:
            # step2.1 delete docs and files in joined team
            joined_tenants = [t for t in tenants if t["role"] == UserTenantRole.NORMAL.value]
            if joined_tenants:
                done_msg += "Start to delete data in joined tenants.\n"
                created_documents = DocumentService.get_all_docs_by_creator_id(usr.id)
                if created_documents:
                    # step2.1.1 delete files
                    doc_file_info = File2DocumentService.get_by_document_ids([d['id'] for d in created_documents])
                    created_files = FileService.get_by_ids([f['file_id'] for f in doc_file_info])
                    if created_files:
                        # step2.1.1.1 delete file in storage
                        for f in created_files:
                            settings.STORAGE_IMPL.rm(f.parent_id, f.location)
                        done_msg += f"- Deleted {len(created_files)} uploaded file.\n"
                        # step2.1.1.2 delete file record
                        file_delete_res = FileService.delete_by_ids([f.id for f in created_files])
                        done_msg += f"- Deleted {file_delete_res} file records.\n"
                    # step2.1.2 delete document-file relation record
                    file2doc_delete_res = File2DocumentService.delete_by_document_ids_or_file_ids(
                        [d['id'] for d in created_documents],
                        [f.id for f in created_files]
                    )
                    done_msg += f"- Deleted {file2doc_delete_res} document-file relation records.\n"
                    # step2.1.3 delete chunks
                    doc_groups = group_by(created_documents, "tenant_id")
                    kb_grouped_doc = {k: group_by(v, "kb_id") for k, v in doc_groups.items()}
                    # chunks in {'tenant_id': {'kb_id': [{'id': doc_id}]}} structure
                    chunk_delete_res = 0
                    kb_doc_info = {}
                    for _tenant_id, kb_doc in kb_grouped_doc.items():
                        for _kb_id, docs in kb_doc.items():
                            chunk_delete_res += settings.docStoreConn.delete(
                                {"doc_id": [d["id"] for d in docs]},
                                search.index_name(_tenant_id), _kb_id
                            )
                            # record doc info
                            if _kb_id in kb_doc_info.keys():
                                kb_doc_info[_kb_id]['doc_num'] += 1
                                kb_doc_info[_kb_id]['token_num'] += sum([d["token_num"] for d in docs])
                                kb_doc_info[_kb_id]['chunk_num'] += sum([d["chunk_num"] for d in docs])
                            else:
                                kb_doc_info[_kb_id] = {
                                    'doc_num': 1,
                                    'token_num': sum([d["token_num"] for d in docs]),
                                    'chunk_num': sum([d["chunk_num"] for d in docs])
                                }
                    done_msg += f"- Deleted {chunk_delete_res} chunks.\n"
                    # step2.1.4 delete tasks
                    task_delete_res = TaskService.delete_by_doc_ids([d['id'] for d in created_documents])
                    done_msg += f"- Deleted {task_delete_res} tasks.\n"
                    # step2.1.5 delete document record
                    doc_delete_res = DocumentService.delete_by_ids([d['id'] for d in created_documents])
                    done_msg += f"- Deleted {doc_delete_res} documents.\n"
                    # step2.1.6 update knowledge base doc&chunk&token cnt
                    for kb_id, doc_num in kb_doc_info.items():
                        KnowledgebaseService.decrease_document_num_in_delete(kb_id, doc_num)

            # step2.2 delete relation
            user_tenant_delete_res = UserTenantService.delete_by_ids([t["id"] for t in tenants])
            done_msg += f"- Deleted {user_tenant_delete_res} user-tenant records.\n"
        # step3 finally delete user
        user_delete_res = UserService.delete_by_id(usr.id)
        done_msg += f"- Deleted {user_delete_res} user.\nDelete done!"

        return {"success": True, "message": f"Successfully deleted user. Details:\n{done_msg}"}

    except Exception as e:
        logging.exception(e)
        return {"success": False, "message": f"Error: {str(e)}. Already done:\n{done_msg}"}


def delete_user_agents(user_id: str) -> dict:
    """
    use user_id to delete
    :return: {
        "agents_deleted_count": 1,
        "version_deleted_count": 2
    }
    """
    agents_deleted_count, agents_version_deleted_count = 0, 0
    user_agents = UserCanvasService.get_all_agents_by_tenant_ids([user_id], user_id)
    if user_agents:
        agents_version = UserCanvasVersionService.get_all_canvas_version_by_canvas_ids([a['id'] for a in user_agents])
        agents_version_deleted_count = UserCanvasVersionService.delete_by_ids([v['id'] for v in agents_version])
        agents_deleted_count = UserCanvasService.delete_by_ids([a['id'] for a in user_agents])
    return {
        "agents_deleted_count": agents_deleted_count,
        "version_deleted_count": agents_version_deleted_count
    }


def delete_user_dialogs(user_id: str) -> dict:
    """
    use user_id to delete
    :return: {
        "dialogs_deleted_count": 1,
        "conversations_deleted_count": 1,
        "api_token_deleted_count": 2,
        "api4conversation_deleted_count": 2
    }
    """
    dialog_deleted_count, conversations_deleted_count, api_token_deleted_count, api4conversation_deleted_count = 0, 0, 0, 0
    user_dialogs = DialogService.get_all_dialogs_by_tenant_id(user_id)
    if user_dialogs:
        # delete conversation
        conversations = ConversationService.get_all_conversation_by_dialog_ids([ud['id'] for ud in user_dialogs])
        conversations_deleted_count = ConversationService.delete_by_ids([c['id'] for c in conversations])
        # delete api token
        api_token_deleted_count = APITokenService.delete_by_tenant_id(user_id)
        # delete api for conversation
        api4conversation_deleted_count = API4ConversationService.delete_by_dialog_ids([ud['id'] for ud in user_dialogs])
        # delete dialog at last
        dialog_deleted_count = DialogService.delete_by_ids([ud['id'] for ud in user_dialogs])
    return {
        "dialogs_deleted_count": dialog_deleted_count,
        "conversations_deleted_count": conversations_deleted_count,
        "api_token_deleted_count": api_token_deleted_count,
        "api4conversation_deleted_count": api4conversation_deleted_count
    }
