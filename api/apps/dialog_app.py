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

from quart import request
from api.db.services import duplicate_name
from api.db.services.dialog_service import DialogService
from common.constants import StatusEnum
from api.db.services.tenant_llm_service import TenantLLMService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.user_service import TenantService, UserTenantService
from api.utils.api_utils import get_data_error_result, get_json_result, get_request_json, server_error_response, validate_request
from common.misc_utils import get_uuid
from common.constants import RetCode
from api.apps import login_required, current_user
from peewee import fn
from api.db.db_models import Dialog


@manager.route('/set', methods=['POST'])  # noqa: F821
@validate_request("prompt_config")
@login_required
async def set_dialog():
    req = await get_request_json()
    dialog_id = req.get("dialog_id", "")
    is_create = not dialog_id
    name = req.get("name", "New Dialog")
    if not isinstance(name, str):
        return get_data_error_result(message="Dialog name must be string.")
    if name.strip() == "":
        return get_data_error_result(message="Dialog name can't be empty.")
    if len(name.encode("utf-8")) > 255:
        return get_data_error_result(message=f"Dialog name length is {len(name)} which is larger than 255")

    if is_create and DialogService.query(tenant_id=current_user.id, name=name.strip()):
        name = name.strip()
        name = duplicate_name(
            DialogService.query,
            name=name,
            tenant_id=current_user.id,
            status=StatusEnum.VALID.value)

    description = req.get("description", "A helpful dialog")
    icon = req.get("icon", "")
    top_n = req.get("top_n", 6)
    top_k = req.get("top_k", 1024)
    rerank_id = req.get("rerank_id", "")
    if not rerank_id:
        req["rerank_id"] = ""
    similarity_threshold = req.get("similarity_threshold", 0.1)
    vector_similarity_weight = req.get("vector_similarity_weight", 0.3)
    llm_setting = req.get("llm_setting", {})
    meta_data_filter = req.get("meta_data_filter", {})
    prompt_config = req["prompt_config"]

    if not is_create:
        if not req.get("kb_ids", []) and not prompt_config.get("tavily_api_key") and "{knowledge}" in prompt_config['system']:
            return get_data_error_result(message="Please remove `{knowledge}` in system prompt since no dataset / Tavily used here.")

        for p in prompt_config["parameters"]:
            if p["optional"]:
                continue
            if prompt_config["system"].find("{%s}" % p["key"]) < 0:
                return get_data_error_result(
                    message="Parameter '{}' is not used".format(p["key"]))

    try:
        e, tenant = TenantService.get_by_id(current_user.id)
        if not e:
            return get_data_error_result(message="Tenant not found!")
        kbs = KnowledgebaseService.get_by_ids(req.get("kb_ids", []))
        embd_ids = [TenantLLMService.split_model_name_and_factory(kb.embd_id)[0] for kb in kbs]  # remove vendor suffix for comparison
        embd_count = len(set(embd_ids))
        if embd_count > 1:
            return get_data_error_result(message=f'Datasets use different embedding models: {[kb.embd_id for kb in kbs]}"')

        llm_id = req.get("llm_id", tenant.llm_id)
        if not dialog_id:
            dia = {
                "id": get_uuid(),
                "tenant_id": current_user.id,
                "name": name,
                "kb_ids": req.get("kb_ids", []),
                "description": description,
                "llm_id": llm_id,
                "llm_setting": llm_setting,
                "prompt_config": prompt_config,
                "meta_data_filter": meta_data_filter,
                "top_n": top_n,
                "top_k": top_k,
                "rerank_id": rerank_id,
                "similarity_threshold": similarity_threshold,
                "vector_similarity_weight": vector_similarity_weight,
                "icon": icon
            }
            if not DialogService.save(**dia):
                return get_data_error_result(message="Fail to new a dialog!")
            return get_json_result(data=dia)
        else:
            del req["dialog_id"]
            if "kb_names" in req:
                del req["kb_names"]
            if not DialogService.update_by_id(dialog_id, req):
                return get_data_error_result(message="Dialog not found!")
            e, dia = DialogService.get_by_id(dialog_id)
            if not e:
                return get_data_error_result(message="Fail to update a dialog!")
            dia = dia.to_dict()
            dia.update(req)
            dia["kb_ids"], dia["kb_names"] = get_kb_names(dia["kb_ids"])
            return get_json_result(data=dia)
    except Exception as e:
        return server_error_response(e)


@manager.route('/get', methods=['GET'])  # noqa: F821
@login_required
def get():
    dialog_id = request.args["dialog_id"]
    try:
        e, dia = DialogService.get_by_id(dialog_id)
        if not e:
            return get_data_error_result(message="Dialog not found!")
        dia = dia.to_dict()
        dia["kb_ids"], dia["kb_names"] = get_kb_names(dia["kb_ids"])
        return get_json_result(data=dia)
    except Exception as e:
        return server_error_response(e)


def get_kb_names(kb_ids):
    ids, nms = [], []
    for kid in kb_ids:
        e, kb = KnowledgebaseService.get_by_id(kid)
        if not e or kb.status != StatusEnum.VALID.value:
            continue
        ids.append(kid)
        nms.append(kb.name)
    return ids, nms


@manager.route('/list', methods=['GET'])  # noqa: F821
@login_required
def list_dialogs():
    try:
        conversations = DialogService.query(
            tenant_id=current_user.id,
            status=StatusEnum.VALID.value,
            reverse=True,
            order_by=DialogService.model.create_time)
        conversations = [d.to_dict() for d in conversations]
        for conversation in conversations:
            conversation["kb_ids"], conversation["kb_names"] = get_kb_names(conversation["kb_ids"])
        return get_json_result(data=conversations)
    except Exception as e:
        return server_error_response(e)


@manager.route('/next', methods=['POST'])  # noqa: F821
@login_required
async def list_dialogs_next():
    args = request.args
    keywords = args.get("keywords", "")
    page_number = int(args.get("page", 0))
    items_per_page = int(args.get("page_size", 0))
    parser_id = args.get("parser_id")
    orderby = args.get("orderby", "create_time")
    if args.get("desc", "true").lower() == "false":
        desc = False
    else:
        desc = True

    req = await get_request_json()
    owner_ids = req.get("owner_ids", [])
    try:
        if not owner_ids:
            # tenants = TenantService.get_joined_tenants_by_user_id(current_user.id)
            # tenants = [tenant["tenant_id"] for tenant in tenants]
            tenants = [] # keep it here
            dialogs, total = DialogService.get_by_tenant_ids(
                tenants, current_user.id, page_number,
                items_per_page, orderby, desc, keywords, parser_id)
        else:
            tenants = owner_ids
            dialogs, total = DialogService.get_by_tenant_ids(
                tenants, current_user.id, 0,
                0, orderby, desc, keywords, parser_id)
            dialogs = [dialog for dialog in dialogs if dialog["tenant_id"] in tenants]
            total = len(dialogs)
            if page_number and items_per_page:
                dialogs = dialogs[(page_number-1)*items_per_page:page_number*items_per_page]
        return get_json_result(data={"dialogs": dialogs, "total": total})
    except Exception as e:
        return server_error_response(e)


@manager.route('/rm', methods=['POST'])  # noqa: F821
@login_required
@validate_request("dialog_ids")
async def rm():
    req = await get_request_json()
    dialog_list=[]
    tenants = UserTenantService.query(user_id=current_user.id)
    try:
        for id in req["dialog_ids"]:
            for tenant in tenants:
                if DialogService.query(tenant_id=tenant.tenant_id, id=id):
                    break
            else:
                return get_json_result(
                    data=False, message='Only owner of dialog authorized for this operation.',
                    code=RetCode.OPERATING_ERROR)
            dialog_list.append({"id": id,"status":StatusEnum.INVALID.value})
        DialogService.update_many_by_id(dialog_list)
        return get_json_result(data=True)
    except Exception as e:
        return server_error_response(e)


@manager.route('/public/list', methods=['POST'])  # noqa: F821
async def list_public_dialogs():
    """
    Public API to get all shared dialogs (assistants).
    No authentication required.
    Supports pagination and keyword search.
    
    Query Parameters:
        - keywords: Search keywords (optional)
        - page: Page number (default: 1)
        - page_size: Items per page (default: 20)
        - orderby: Order by field (default: create_time)
        - desc: Descending order (default: true)
    
    Returns:
        {
            "code": 0,
            "data": {
                "dialogs": [...],
                "total": 100
            }
        }
    
    Logic:
        - Each tenant (admin) creates one APIToken (with dialog_id=NULL)
        - All dialogs created by this tenant use this APIToken's token and beta
        - Join: Dialog.tenant_id = APIToken.tenant_id (where APIToken.dialog_id IS NULL)
    """
    from api.db.services.api_service import APITokenService
    from api.db.db_models import APIToken, User
    
    try:
        # Get query parameters (same as /next endpoint)
        args = request.args
        keywords = args.get("keywords", "")
        page_number = int(args.get("page", 1))
        items_per_page = int(args.get("page_size", 20))
        orderby = args.get("orderby", "create_time")
        if args.get("desc", "true").lower() == "false":
            desc = False
        else:
            desc = True
        
        # Query dialogs with their tenant's APIToken
        # Each tenant has one APIToken (dialog_id IS NULL), all their dialogs use it
        fields = [
            DialogService.model.id,
            DialogService.model.tenant_id,
            DialogService.model.name,
            DialogService.model.description,
            DialogService.model.language,
            DialogService.model.llm_id,
            DialogService.model.llm_setting,
            DialogService.model.prompt_type,
            DialogService.model.prompt_config,
            DialogService.model.similarity_threshold,
            DialogService.model.vector_similarity_weight,
            DialogService.model.top_n,
            DialogService.model.top_k,
            DialogService.model.do_refer,
            DialogService.model.rerank_id,
            DialogService.model.kb_ids,
            DialogService.model.icon,
            DialogService.model.status,
            User.nickname,
            User.avatar.alias("tenant_avatar"),
            DialogService.model.update_time,
            DialogService.model.create_time,
            APIToken.token.alias("shared_id"),
            APIToken.beta.alias("auth_token"),
        ]
        
        # Build query: Dialog JOIN APIToken (on tenant_id, where dialog_id IS NULL) JOIN User
        query = (
            DialogService.model.select(*fields)
            .join(APIToken, on=(
                (DialogService.model.tenant_id == APIToken.tenant_id) &
                (APIToken.dialog_id.is_null(True))
            ))
            .join(User, on=(DialogService.model.tenant_id == User.id))
            .where(DialogService.model.status == StatusEnum.VALID.value)
        )
        
        # Apply keyword filter if provided
        if keywords:
            query = query.where(fn.LOWER(DialogService.model.name).contains(keywords.lower()))
        
        # Apply ordering
        if desc:
            query = query.order_by(DialogService.model.getter_by(orderby).desc())
        else:
            query = query.order_by(DialogService.model.getter_by(orderby).asc())
        
        # Get total count
        count = query.count()
        
        # Apply pagination
        if page_number and items_per_page:
            query = query.paginate(page_number, items_per_page)
        
        # Convert to list of dicts
        dialogs = list(query.dicts())
        
        return get_json_result(data={"dialogs": dialogs, "total": count})
        
    except Exception as e:
        return server_error_response(e)


@manager.route('/public/conversations', methods=['GET'])  # noqa: F821
async def get_public_conversations():
    """
    Public API to get all conversation history from public dialogs.
    No authentication required.
    
    Query Parameters:
        - page: Page number (default: 1)
        - page_size: Items per page (default: 50)
    
    Returns:
        {
            "code": 0,
            "data": {
                "conversations": [
                    {
                        "id": "conversation_id",
                        "dialog_id": "dialog_id",
                        "dialog_name": "Assistant Name",
                        "dialog_icon": "icon_url",
                        "title": "Conversation title",
                        "last_message": "Last message preview",
                        "message_count": 5,
                        "create_time": "2024-01-01T00:00:00",
                        "update_time": "2024-01-01T00:00:00"
                    }
                ],
                "total": 100
            }
        }
    
    Logic:
        - Query all valid dialogs (public assistants)
        - Get all conversations for these dialogs
        - Order by update_time DESC (newest first)
    """
    from api.db.services.api_service import API4ConversationService
    from api.db.db_models import API4Conversation
    
    try:
        # Get pagination parameters
        args = request.args
        page_number = int(args.get("page", 1))
        items_per_page = int(args.get("page_size", 50))
        
        # Query conversations from all valid (public) dialogs
        # Join with Dialog to get dialog info and filter by status
        query = (
            API4Conversation.select(
                API4Conversation.id,
                API4Conversation.dialog_id,
                API4Conversation.message,
                API4Conversation.create_time,
                API4Conversation.update_time,
                Dialog.name.alias("dialog_name"),
                Dialog.icon.alias("dialog_icon"),
            )
            .join(Dialog, on=(API4Conversation.dialog_id == Dialog.id))
            .where(Dialog.status == StatusEnum.VALID.value)
            .order_by(API4Conversation.update_time.desc())
        )
        
        # Print SQL for debugging
        import logging
        logger = logging.getLogger(__name__)
        logger.info(f"[DEBUG] SQL Query: {query.sql()}")
        
        # Get total count
        total = query.count()
        
        # Apply pagination
        if page_number and items_per_page:
            query = query.paginate(page_number, items_per_page)
        
        conversations = []
        for conv in query.dicts():
            # Extract first user message as title
            messages = conv.get("message", [])
            first_user_msg = ""
            last_msg = ""
            msg_count = len(messages)
            
            for msg in messages:
                if msg.get("role") == "user" and not first_user_msg:
                    first_user_msg = msg.get("content", "")
                if msg.get("content"):
                    last_msg = msg.get("content", "")
            
            # Generate title (limit to 20 chars)
            title = first_user_msg or last_msg or "新对话"
            if len(title) > 20:
                title = title[:20] + "..."
            
            # Handle timestamp fields (they are integers, not datetime objects)
            create_time = conv.get("create_time", 0)
            update_time = conv.get("update_time", 0)
            
            conversations.append({
                "id": conv["id"],
                "dialog_id": conv["dialog_id"],
                "dialog_name": conv.get("dialog_name", ""),
                "dialog_icon": conv.get("dialog_icon", ""),
                "title": title,
                "last_message": last_msg[:50] if last_msg else "",
                "message_count": msg_count,
                "create_time": create_time,
                "update_time": update_time,
            })
        
        logger.info(f"[DEBUG] Found {len(conversations)} conversations out of {total} total")
        
        return get_json_result(data={"conversations": conversations, "total": total})
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/public/conversation/<conversation_id>/messages", methods=["GET"])
def get_conversation_messages(conversation_id):
    """
    Get messages from a specific conversation.
    
    Args:
        conversation_id: The conversation ID
    
    Returns:
        {
            "code": 0,
            "data": {
                "conversation_id": "xxx",
                "dialog_id": "xxx",
                "messages": [
                    {
                        "role": "user",
                        "content": "Hello",
                        "id": "msg_id"
                    },
                    {
                        "role": "assistant",
                        "content": "Hi there!",
                        "id": "msg_id"
                    }
                ]
            }
        }
    """
    from api.db.services.api_service import API4ConversationService
    from api.db.db_models import API4Conversation
    
    try:
        # Query the conversation
        conversation = API4Conversation.select().where(
            API4Conversation.id == conversation_id
        ).first()
        
        if not conversation:
            return get_json_result(
                data=False,
                message=f"Conversation {conversation_id} not found",
                code=RetCode.DATA_ERROR
            )
        
        # Get messages from the conversation
        messages = conversation.message or []
        
        return get_json_result(data={
            "conversation_id": conversation.id,
            "dialog_id": conversation.dialog_id,
            "messages": messages
        })
        
    except Exception as e:
        return server_error_response(e)


@manager.route('/public/conversation/<conversation_id>', methods=['DELETE'])  # noqa: F821
def delete_public_conversation(conversation_id):
    """
    Delete a public conversation by ID
    ---
    tags:
      - Dialog
    parameters:
      - name: conversation_id
        in: path
        required: true
        type: string
        description: The ID of the conversation to delete
    responses:
      200:
        description: Conversation deleted successfully
      404:
        description: Conversation not found
      500:
        description: Server error
    """
    try:
        from api.db.services.api_service import API4ConversationService
        
        # Check if conversation exists
        e, conversation = API4ConversationService.get_by_id(conversation_id)
        if not e:
            return get_json_result(
                data=False,
                message=f"Conversation {conversation_id} not found",
                code=RetCode.DATA_ERROR
            )
        
        # Delete the conversation
        API4ConversationService.delete_by_id(conversation_id)
        
        return get_json_result(data=True, message="Conversation deleted successfully")
        
    except Exception as e:
        return server_error_response(e)
