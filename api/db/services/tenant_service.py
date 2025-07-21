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

from peewee import fn

from api.db.db_models import DB, Tenant, UserTenant
from api.db.services.common_service import CommonService


class TenantService(CommonService):
    model = Tenant

    @classmethod
    @DB.connection_context()
    def create_tenant(cls, **kwargs):
        """Create a new tenant."""
        return cls.model.create(**kwargs)

    @classmethod
    @DB.connection_context()
    def get_by_id(cls, tenant_id):
        """Get tenant by ID."""
        try:
            return cls.model.get(cls.model.id == tenant_id)
        except cls.model.DoesNotExist:
            return None

    @classmethod
    @DB.connection_context()
    def query(cls, tenant_id=None, name=None, status=None, page_number=1, items_per_page=20, orderby="create_time", desc=True):
        """Query tenants with filtering."""
        tenants = cls.model.select()
        if tenant_id:
            tenants = tenants.where(cls.model.id == tenant_id)
        if name:
            tenants = tenants.where(cls.model.name.contains(name))
        if status:
            tenants = tenants.where(cls.model.status == status)
        
        if desc:
            tenants = tenants.order_by(getattr(cls.model, orderby).desc())
        else:
            tenants = tenants.order_by(getattr(cls.model, orderby).asc())
            
        return tenants.paginate(page_number, items_per_page)

    @classmethod
    @DB.connection_context()
    def count(cls, tenant_id=None, name=None, status=None):
        """Count tenants with filtering."""
        tenants = cls.model.select()
        if tenant_id:
            tenants = tenants.where(cls.model.id == tenant_id)
        if name:
            tenants = tenants.where(cls.model.name.contains(name))
        if status:
            tenants = tenants.where(cls.model.status == status)
        return tenants.count()

    @classmethod
    @DB.connection_context()
    def update_tenant(cls, tenant_id, **kwargs):
        """Update tenant details."""
        return cls.model.update(**kwargs).where(cls.model.id == tenant_id).execute()

    @classmethod
    @DB.connection_context()
    def delete_tenant(cls, tenant_id):
        """Soft delete a tenant."""
        return cls.model.update(status='0').where(cls.model.id == tenant_id).execute()

    @classmethod
    @DB.connection_context()
    def get_user_tenants(cls, user_id, page=1, size=10, keywords="", orderby="create_time", desc=True):
        """Get all tenants for a specific user."""
        query = cls.model.select(
            cls.model,
            UserTenant.role,
            UserTenant.status.label('user_status')
        ).join(UserTenant, on=cls.model.id == UserTenant.tenant_id).where(
            UserTenant.user_id == user_id
        )
        
        if keywords:
            query = query.where(cls.model.name.contains(keywords))
            
        if desc:
            query = query.order_by(getattr(cls.model, orderby).desc())
        else:
            query = query.order_by(getattr(cls.model, orderby).asc())
            
        total = query.count()
        tenants = query.paginate(page, size)
        return tenants, total

    @classmethod
    @DB.connection_context()
    def get_tenant_users(cls, tenant_id, page=1, size=10, role=None, status=None, keywords=""):
        """Get all users in a tenant."""
        from api.db.db_models import User
        from api.db import UserTenantRole
        
        query = User.select(
            User.id,
            User.email,
            User.nickname,
            User.avatar,
            UserTenant.role,
            UserTenant.status,
            UserTenant.create_time,
            UserTenant.update_time
        ).join(UserTenant, on=User.id == UserTenant.user_id).where(
            UserTenant.tenant_id == tenant_id
        )
        
        if role:
            query = query.where(UserTenant.role == role)
        if status:
            query = query.where(UserTenant.status == status)
        if keywords:
            query = query.where(
                (User.email.contains(keywords)) | (User.nickname.contains(keywords))
            )
            
        total = query.count()
        users = query.paginate(page, size)
        return users, total

    @classmethod
    @DB.connection_context()
    def is_user_owner(cls, user_id, tenant_id):
        """Check if user is owner of the tenant."""
        try:
            user_tenant = UserTenant.get(
                (UserTenant.user_id == user_id) & (UserTenant.tenant_id == tenant_id)
            )
            return user_tenant.role == 'owner'
        except UserTenant.DoesNotExist:
            return False

    @classmethod
    @DB.connection_context()
    def check_user_access(cls, user_id, tenant_id):
        """Check if user has access to the tenant."""
        try:
            UserTenant.get(
                (UserTenant.user_id == user_id) & (UserTenant.tenant_id == tenant_id)
            )
            return True
        except UserTenant.DoesNotExist:
            return False

    @classmethod
    @DB.connection_context()
    def get_user_role(cls, user_id, tenant_id):
        """Get user's role in a specific tenant."""
        try:
            user_tenant = UserTenant.get(
                (UserTenant.user_id == user_id) & (UserTenant.tenant_id == tenant_id)
            )
            return user_tenant.role
        except UserTenant.DoesNotExist:
            return None

    @classmethod
    @DB.connection_context()
    def count_active_tenants(cls, user_id=None):
        """Count active tenants."""
        query = cls.model.select().where(cls.model.status == '1')
        if user_id:
            query = query.join(UserTenant).where(UserTenant.user_id == user_id)
        return query.count()

    @classmethod
    @DB.connection_context()
    def search_tenants(cls, keywords, page=1, size=10):
        """Search tenants by name or description."""
        tenants = cls.model.select().where(
            (cls.model.name.contains(keywords)) | (cls.model.description.contains(keywords))
        ).where(cls.model.status == '1')
        
        total = tenants.count()
        tenants = tenants.paginate(page, size)
        return tenants, total