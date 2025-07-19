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
from flask import g, request
from flask_login import current_user
from functools import wraps

from api.db.services.user_service import UserTenantService


class TenantMiddleware:
    """
    Middleware for handling tenant context in requests.
    
    This middleware extracts tenant information from requests and makes it available
    throughout the request lifecycle. It supports multiple ways of tenant identification:
    1. HTTP headers: X-Tenant-ID
    2. Query parameters: tenant_id
    3. User's default tenant (if authenticated)
    """
    
    TENANT_HEADER = 'X-Tenant-ID'
    TENANT_PARAM = 'tenant_id'
    
    @classmethod
    def get_current_tenant(cls):
        """
        Get the current tenant ID for the request.
        
        Returns:
            str: The current tenant ID or None if not specified
        """
        # Check request context first
        if hasattr(g, 'tenant_id'):
            return g.tenant_id
            
        # Check HTTP header
        tenant_id = request.headers.get(cls.TENANT_HEADER)
        if tenant_id:
            return tenant_id
            
        # Check query parameter
        tenant_id = request.args.get(cls.TENANT_PARAM)
        if tenant_id:
            return tenant_id
            
        # Check form data
        tenant_id = request.form.get(cls.TENANT_PARAM)
        if tenant_id:
            return tenant_id
            
        # Check JSON body
        if request.is_json:
            json_data = request.get_json(silent=True)
            if json_data and 'tenant_id' in json_data:
                return json_data['tenant_id']
                
        # If user is authenticated, use their default tenant
        if hasattr(current_user, 'id') and current_user.is_authenticated:
            tenants = UserTenantService.query(user_id=current_user.id)
            if tenants:
                return tenants[0].tenant_id
                
        # During migration period, return default tenant for existing data
        # This allows gradual migration without breaking existing functionality
        return 'default_tenant_001'  # Default fallback during migration
    
    @classmethod
    def set_current_tenant(cls, tenant_id):
        """
        Set the current tenant ID in the request context.
        
        Args:
            tenant_id (str): The tenant ID to set
        """
        g.tenant_id = tenant_id
    
    @classmethod
    def init_app(cls, app):
        """
        Initialize the middleware with a Flask app.
        
        Args:
            app: The Flask application instance
        """
        
        @app.before_request
        def before_request():
            """Set up tenant context before each request."""
            tenant_id = cls.get_current_tenant()
            cls.set_current_tenant(tenant_id)
            
        @app.after_request
        def after_request(response):
            """Clean up tenant context after each request."""
            if hasattr(g, 'tenant_id'):
                del g.tenant_id
            return response


def require_tenant(f):
    """
    Decorator to require a valid tenant context.
    
    Usage:
        @require_tenant
        @login_required
        def my_endpoint():
            tenant_id = g.tenant_id
            # ... rest of endpoint
    """
    @wraps(f)
    def decorated_function(*args, **kwargs):
        from api.utils.api_utils import get_data_error_result
        from api import settings
        
        if not hasattr(g, 'tenant_id') or not g.tenant_id:
            return get_data_error_result(
                message="Tenant context is required",
                code=settings.RetCode.AUTHENTICATION_ERROR
            )
        return f(*args, **kwargs)
    return decorated_function


def tenant_aware(f):
    """
    Decorator to automatically filter queries by tenant.
    
    This decorator ensures that database queries automatically include
    tenant filtering when a tenant context is available.
    """
    @wraps(f)
    def decorated_function(*args, **kwargs):
        # Inject tenant_id into kwargs if not explicitly provided
        if hasattr(g, 'tenant_id') and g.tenant_id and 'tenant_id' not in kwargs:
            kwargs['tenant_id'] = g.tenant_id
        return f(*args, **kwargs)
    return decorated_function