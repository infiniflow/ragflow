#!/usr/bin/env python3
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

from flask import request
from flask_login import login_required, current_user

from api import settings
from api.db import UserTenantRole, StatusEnum
from api.db.db_models import Tenant, UserTenant
from api.db.services.tenant_service import TenantService
from api.db.services.user_service import UserTenantService, UserService
from api.middleware.tenant_middleware import require_tenant, tenant_aware
from api.utils import get_uuid, delta_seconds
from api.utils.api_utils import get_json_result, validate_request, server_error_response, get_data_error_result

manager = Blueprint('tenant_management', __name__)

@manager.route("/create", methods=["POST"])
@login_required
@validate_request("name")
def create_tenant():
    """Create a new tenant."""
    try:
        req = request.json
        name = req["name"]
        description = req.get("description", "")
        
        # Check if tenant name already exists for this user
        existing_tenant = TenantService.query(name=name)
        if existing_tenant:
            return get_data_error_result(message="Tenant name already exists.")
        
        tenant_id = get_uuid()
        tenant_data = {
            'id': tenant_id,
            'name': name,
            'description': description,
            'llm_id': req.get('llm_id', 'default_llm'),
            'embd_id': req.get('embd_id', 'default_embedding'),
            'asr_id': req.get('asr_id', 'default_asr'),
            'img2txt_id': req.get('img2txt_id', 'default_img2txt'),
            'rerank_id': req.get('rerank_id', 'default_rerank'),
            'tts_id': req.get('tts_id', 'default_tts'),
            'parser_ids': req.get('parser_ids', 'default_parser'),
            'credit': req.get('credit', 512),
            'status': StatusEnum.VALID.value
        }
        
        tenant = TenantService.save(**tenant_data)
        
        # Add creator as owner
        UserTenantService.save(
            id=get_uuid(),
            user_id=current_user.id,
            tenant_id=tenant_id,
            role=UserTenantRole.OWNER,
            status=StatusEnum.VALID.value
        )
        
        return get_json_result(data=tenant.to_dict())
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/list", methods=["GET"])
@login_required
def list_tenants():
    """List all tenants for the current user."""
    try:
        page = int(request.args.get("page", 1))
        size = int(request.args.get("size", 10))
        keywords = request.args.get("keywords", "")
        orderby = request.args.get("orderby", "create_time")
        desc = request.args.get("desc", True, type=bool)
        
        tenants, total = TenantService.get_user_tenants(
            user_id=current_user.id,
            page=page,
            size=size,
            keywords=keywords,
            orderby=orderby,
            desc=desc
        )
        
        return get_json_result(data={
            "tenants": [t.to_dict() for t in tenants],
            "total": total
        })
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/<tenant_id>", methods=["GET"])
@login_required
@require_tenant
def get_tenant(tenant_id):
    """Get tenant details."""
    try:
        tenant = TenantService.get_by_id(tenant_id)
        if not tenant:
            return get_data_error_result(message="Tenant not found.")
            
        # Check user has access to this tenant
        user_tenant = UserTenantService.query(user_id=current_user.id, tenant_id=tenant_id)
        if not user_tenant:
            return get_data_error_result(message="Access denied.")
            
        return get_json_result(data=tenant.to_dict())
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/<tenant_id>", methods=["PUT"])
@login_required
@require_tenant
@validate_request("name")
def update_tenant(tenant_id):
    """Update tenant details."""
    try:
        tenant = TenantService.get_by_id(tenant_id)
        if not tenant:
            return get_data_error_result(message="Tenant not found.")
            
        # Check user has owner role
        user_tenant = UserTenantService.query(user_id=current_user.id, tenant_id=tenant_id)
        if not user_tenant or user_tenant[0].role != UserTenantRole.OWNER:
            return get_data_error_result(message="Only owner can update tenant.")
            
        req = request.json
        updatable_fields = ['name', 'description', 'llm_id', 'embd_id', 'asr_id', 'img2txt_id', 'rerank_id', 'tts_id', 'parser_ids']
        update_data = {field: req[field] for field in updatable_fields if field in req}
        
        TenantService.filter_update([Tenant.id == tenant_id], update_data)
        updated_tenant = TenantService.get_by_id(tenant_id)
        
        return get_json_result(data=updated_tenant.to_dict())
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/<tenant_id>", methods=["DELETE"])
@login_required
@require_tenant
def delete_tenant(tenant_id):
    """Delete a tenant (soft delete)."""
    try:
        tenant = TenantService.get_by_id(tenant_id)
        if not tenant:
            return get_data_error_result(message="Tenant not found.")
            
        # Check user has owner role
        user_tenant = UserTenantService.query(user_id=current_user.id, tenant_id=tenant_id)
        if not user_tenant or user_tenant[0].role != UserTenantRole.OWNER:
            return get_data_error_result(message="Only owner can delete tenant.")
            
        # Soft delete by setting status to invalid
        TenantService.filter_update([Tenant.id == tenant_id], {'status': StatusEnum.INVALID.value})
        
        return get_json_result(data=True)
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/<tenant_id>/users", methods=["GET"])
@login_required
@require_tenant
def list_tenant_users(tenant_id):
    """List all users in a tenant."""
    try:
        page = int(request.args.get("page", 1))
        size = int(request.args.get("size", 10))
        role = request.args.get("role", "")
        status = request.args.get("status", "")
        keywords = request.args.get("keywords", "")
        
        users, total = TenantService.get_tenant_users(
            tenant_id=tenant_id,
            page=page,
            size=size,
            role=role,
            status=status,
            keywords=keywords
        )
        
        return get_json_result(data={
            "users": users,
            "total": total
        })
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/<tenant_id>/users/<user_id>/role", methods=["PUT"])
@login_required
@require_tenant
@validate_request("role")
def update_user_role(tenant_id, user_id):
    """Update user role in tenant."""
    try:
        req = request.json
        new_role = req["role"]
        
        # Check user has owner role
        current_user_tenant = UserTenantService.query(user_id=current_user.id, tenant_id=tenant_id)
        if not current_user_tenant or current_user_tenant[0].role != UserTenantRole.OWNER:
            return get_data_error_result(message="Only owner can update user roles.")
            
        # Prevent owner demotion
        if new_role != UserTenantRole.OWNER and user_id == current_user.id:
            return get_data_error_result(message="Cannot change your own role.")
            
        UserTenantService.filter_update(
            [UserTenant.tenant_id == tenant_id, UserTenant.user_id == user_id],
            {'role': new_role}
        )
        
        return get_json_result(data=True)
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/<tenant_id>/switch", methods=["POST"])
@login_required
def switch_tenant(tenant_id):
    """Switch to a specific tenant."""
    try:
        user_tenant = UserTenantService.query(user_id=current_user.id, tenant_id=tenant_id)
        if not user_tenant:
            return get_data_error_result(message="Access denied or tenant not found.")
            
        tenant = TenantService.get_by_id(tenant_id)
        if not tenant or tenant.status == StatusEnum.INVALID.value:
            return get_data_error_result(message="Tenant not found or inactive.")
            
        return get_json_result(data={
            "tenant_id": tenant_id,
            "tenant_name": tenant.name,
            "user_role": user_tenant[0].role
        })
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/<tenant_id>/config", methods=["GET"])
@login_required
@require_tenant
def get_tenant_config(tenant_id):
    """Get tenant configuration."""
    try:
        tenant = TenantService.get_by_id(tenant_id)
        if not tenant:
            return get_data_error_result(message="Tenant not found.")
            
        config = {
            "tenant_id": tenant.id,
            "name": tenant.name,
            "description": tenant.description,
            "llm_id": tenant.llm_id,
            "embd_id": tenant.embd_id,
            "asr_id": tenant.asr_id,
            "img2txt_id": tenant.img2txt_id,
            "rerank_id": tenant.rerank_id,
            "tts_id": tenant.tts_id,
            "parser_ids": tenant.parser_ids,
            "credit": tenant.credit
        }
        
        return get_json_result(data=config)
        
    except Exception as e:
        return server_error_response(e)


@manager.route("/<tenant_id>/usage", methods=["GET"])
@login_required
@require_tenant
def get_tenant_usage(tenant_id):
    """Get tenant usage statistics."""
    try:
        from api.db.services.document_service import DocumentService
        from api.db.services.knowledgebase_service import KnowledgebaseService
        from api.db.services.conversation_service import ConversationService
        
        usage_stats = {
            "document_count": DocumentService.count_documents(tenant_id=tenant_id),
            "knowledgebase_count": KnowledgebaseService.count(tenant_id=tenant_id),
            "conversation_count": ConversationService.count(tenant_id=tenant_id),
            "tenant_id": tenant_id
        }
        
        return get_json_result(data=usage_stats)
        
    except Exception as e:
        return server_error_response(e)