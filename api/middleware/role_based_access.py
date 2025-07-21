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

from functools import wraps
from flask import g, request
from flask_login import current_user

from api.db import UserTenantRole
from api.db.services.user_service import UserTenantService
from api.utils.api_utils import get_data_error_result
from api import settings


class RoleBasedAccessControl:
    """
    Role-based access control system for multi-tenant RAGFlow.
    
    Supports three levels of access control:
    1. OWNER: Full access to tenant resources and management
    2. NORMAL: Regular access to tenant resources
    3. INVITE: Limited access, pending invitation acceptance
    """
    
    @staticmethod
    def check_user_role(tenant_id, user_id=None):
        """
        Check user's role in a specific tenant.
        
        Args:
            tenant_id (str): The tenant ID to check
            user_id (str, optional): User ID. Defaults to current_user.id
            
        Returns:
            tuple: (has_access, role) - has_access is bool, role is string or None
        """
        if user_id is None:
            if not hasattr(current_user, 'id') or not current_user.is_authenticated:
                return False, None
            user_id = current_user.id
            
        user_tenant = UserTenantService.query(user_id=user_id, tenant_id=tenant_id)
        if not user_tenant:
            return False, None
            
        return True, user_tenant[0].role
    
    @staticmethod
    def require_role(required_role, allow_owner=True):
        """
        Decorator to require specific role for accessing tenant resources.
        
        Args:
            required_role (str): Required role (OWNER, NORMAL, INVITE)
            allow_owner (bool): Whether to allow owner access regardless of required_role
            
        Returns:
            decorator: Flask route decorator
        """
        def decorator(f):
            @wraps(f)
            def decorated_function(*args, **kwargs):
                tenant_id = kwargs.get('tenant_id') or g.get('tenant_id')
                if not tenant_id:
                    return get_data_error_result(
                        message="Tenant ID is required",
                        code=settings.RetCode.AUTHENTICATION_ERROR
                    )
                
                has_access, user_role = RoleBasedAccessControl.check_user_role(tenant_id)
                if not has_access:
                    return get_data_error_result(
                        message="Access denied to tenant",
                        code=settings.RetCode.AUTHENTICATION_ERROR
                    )
                
                # Always allow owner access
                if allow_owner and user_role == UserTenantRole.OWNER:
                    return f(*args, **kwargs)
                
                # Check if user has required role
                role_hierarchy = {
                    UserTenantRole.OWNER: 3,
                    UserTenantRole.NORMAL: 2,
                    UserTenantRole.INVITE: 1
                }
                
                required_level = role_hierarchy.get(required_role, 0)
                user_level = role_hierarchy.get(user_role, 0)
                
                if user_level < required_level:
                    return get_data_error_result(
                        message=f"Insufficient permissions. Required role: {required_role}",
                        code=settings.RetCode.AUTHENTICATION_ERROR
                    )
                
                # Add role information to request context
                g.user_role = user_role
                return f(*args, **kwargs)
            return decorated_function
        return decorator
    
    @staticmethod
    def require_owner(f):
        """
        Decorator to require owner role for tenant management operations.
        """
        return RoleBasedAccessControl.require_role(UserTenantRole.OWNER)(f)
    
    @staticmethod
    def require_normal_user(f):
        """
        Decorator to require normal user role or higher.
        """
        return RoleBasedAccessControl.require_role(UserTenantRole.NORMAL)(f)
    
    @staticmethod
    def require_any_user(f):
        """
        Decorator to require any valid user role (including INVITE).
        """
        return RoleBasedAccessControl.require_role(UserTenantRole.INVITE)(f)


# Convenience decorators
def require_owner(f):
    """Decorator for requiring owner role."""
    return RoleBasedAccessControl.require_owner(f)


def require_normal_user(f):
    """Decorator for requiring normal user role."""
    return RoleBasedAccessControl.require_normal_user(f)


def require_any_user(f):
    """Decorator for requiring any valid user role."""
    return RoleBasedAccessControl.require_any_user(f)


def check_tenant_access(tenant_id, required_role=None):
    """
    Function to check tenant access programmatically.
    
    Args:
        tenant_id (str): Tenant ID to check
        required_role (str, optional): Required role level
        
    Returns:
        tuple: (has_access, user_role)
    """
    return RoleBasedAccessControl.check_user_role(tenant_id)