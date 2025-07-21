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

import pytest
import uuid
from unittest.mock import patch

from api.db.db_models import Tenant, UserTenant, Document, Knowledgebase, Conversation
from api.db.services.tenant_service import TenantService
from api.db.services.user_service import UserTenantService, UserService
from api.db.services.document_service import DocumentService
from api.db.services.knowledgebase_service import KnowledgebaseService
from api.db.services.conversation_service import ConversationService
from api.db import UserTenantRole, StatusEnum


class TestTenantIsolation:
    """
    Comprehensive test suite for tenant isolation validation.
    
    This test suite ensures that tenant data is properly isolated
    and users can only access resources within their assigned tenants.
    """
    
    @pytest.fixture
    def setup_tenants(self):
        """Set up test tenants and users."""
        # Create test tenants
        tenant1 = TenantService.create_tenant(
            id=str(uuid.uuid4()),
            name="Test Tenant 1",
            description="Test tenant 1",
            status=StatusEnum.VALID.value,
            credit=1000
        )
        
        tenant2 = TenantService.create_tenant(
            id=str(uuid.uuid4()),
            name="Test Tenant 2", 
            description="Test tenant 2",
            status=StatusEnum.VALID.value,
            credit=1000
        )
        
        # Create test users
        user1 = UserService.save(
            id=str(uuid.uuid4()),
            email="user1@test.com",
            nickname="User One",
            status=StatusEnum.VALID.value
        )
        
        user2 = UserService.save(
            id=str(uuid.uuid4()),
            email="user2@test.com", 
            nickname="User Two",
            status=StatusEnum.VALID.value
        )
        
        # Assign users to tenants
        UserTenantService.save(
            id=str(uuid.uuid4()),
            user_id=user1.id,
            tenant_id=tenant1.id,
            role=UserTenantRole.OWNER,
            status=StatusEnum.VALID.value
        )
        
        UserTenantService.save(
            id=str(uuid.uuid4()),
            user_id=user2.id,
            tenant_id=tenant2.id,
            role=UserTenantRole.OWNER,
            status=StatusEnum.VALID.value
        )
        
        return {
            'tenant1': tenant1,
            'tenant2': tenant2,
            'user1': user1,
            'user2': user2
        }
    
    def test_tenant_document_isolation(self, setup_tenants):
        """Test that documents are properly isolated by tenant."""
        tenant1 = setup_tenants['tenant1']
        tenant2 = setup_tenants['tenant2']
        
        # Create documents for tenant 1
        doc1 = DocumentService.save(
            id=str(uuid.uuid4()),
            name="Tenant 1 Document",
            tenant_id=tenant1.id,
            kb_id=str(uuid.uuid4()),
            type="pdf",
            size=1000,
            status="1"
        )
        
        # Create documents for tenant 2
        doc2 = DocumentService.save(
            id=str(uuid.uuid4()),
            name="Tenant 2 Document",
            tenant_id=tenant2.id,
            kb_id=str(uuid.uuid4()),
            type="pdf",
            size=1000,
            status="1"
        )
        
        # Test tenant 1 can only see their documents
        tenant1_docs = DocumentService.get_by_tenant_id(tenant1.id)
        assert len(tenant1_docs) == 1
        assert tenant1_docs[0].name == "Tenant 1 Document"
        
        # Test tenant 2 can only see their documents
        tenant2_docs = DocumentService.get_by_tenant_id(tenant2.id)
        assert len(tenant2_docs) == 1
        assert tenant2_docs[0].name == "Tenant 2 Document"
    
    def test_tenant_knowledgebase_isolation(self, setup_tenants):
        """Test that knowledgebases are properly isolated by tenant."""
        tenant1 = setup_tenants['tenant1']
        tenant2 = setup_tenants['tenant2']
        
        # Create knowledgebases for tenant 1
        kb1 = KnowledgebaseService.save(
            id=str(uuid.uuid4()),
            name="Tenant 1 KB",
            tenant_id=tenant1.id,
            embd_id="default_embd",
            description="Test KB for tenant 1",
            status="1"
        )
        
        # Create knowledgebases for tenant 2
        kb2 = KnowledgebaseService.save(
            id=str(uuid.uuid4()),
            name="Tenant 2 KB",
            tenant_id=tenant2.id,
            embd_id="default_embd",
            description="Test KB for tenant 2",
            status="1"
        )
        
        # Test tenant 1 can only see their knowledgebases
        tenant1_kbs = KnowledgebaseService.query(tenant_id=tenant1.id)
        assert len(tenant1_kbs) == 1
        assert tenant1_kbs[0].name == "Tenant 1 KB"
        
        # Test tenant 2 can only see their knowledgebases
        tenant2_kbs = KnowledgebaseService.query(tenant_id=tenant2.id)
        assert len(tenant2_kbs) == 1
        assert tenant2_kbs[0].name == "Tenant 2 KB"
    
    def test_tenant_conversation_isolation(self, setup_tenants):
        """Test that conversations are properly isolated by tenant."""
        tenant1 = setup_tenants['tenant1']
        tenant2 = setup_tenants['tenant2']
        
        # Create conversations for tenant 1
        conv1 = ConversationService.save(
            id=str(uuid.uuid4()),
            name="Tenant 1 Conversation",
            tenant_id=tenant1.id,
            user_id=setup_tenants['user1'].id,
            kb_ids=[],
            status="1"
        )
        
        # Create conversations for tenant 2
        conv2 = ConversationService.save(
            id=str(uuid.uuid4()),
            name="Tenant 2 Conversation",
            tenant_id=tenant2.id,
            user_id=setup_tenants['user2'].id,
            kb_ids=[],
            status="1"
        )
        
        # Test tenant 1 can only see their conversations
        tenant1_convs = ConversationService.query(tenant_id=tenant1.id)
        assert len(tenant1_convs) == 1
        assert tenant1_convs[0].name == "Tenant 1 Conversation"
        
        # Test tenant 2 can only see their conversations
        tenant2_convs = ConversationService.query(tenant_id=tenant2.id)
        assert len(tenant2_convs) == 1
        assert tenant2_convs[0].name == "Tenant 2 Conversation"
    
    def test_user_tenant_access_control(self, setup_tenants):
        """Test that users can only access their assigned tenants."""
        tenant1 = setup_tenants['tenant1']
        tenant2 = setup_tenants['tenant2']
        user1 = setup_tenants['user1']
        user2 = setup_tenants['user2']
        
        # Test user 1 has access to tenant 1
        user1_tenants = UserTenantService.get_tenants_by_user_id(user1.id)
        tenant_ids = [t['tenant_id'] for t in user1_tenants]
        assert tenant1.id in tenant_ids
        assert tenant2.id not in tenant_ids
        
        # Test user 2 has access to tenant 2
        user2_tenants = UserTenantService.get_tenants_by_user_id(user2.id)
        tenant_ids = [t['tenant_id'] for t in user2_tenants]
        assert tenant2.id in tenant_ids
        assert tenant1.id not in tenant_ids
    
    def test_cross_tenant_access_denied(self, setup_tenants):
        """Test that cross-tenant access is properly denied."""
        tenant1 = setup_tenants['tenant1']
        tenant2 = setup_tenants['tenant2']
        
        # Create a document for tenant 1
        doc1 = DocumentService.save(
            id=str(uuid.uuid4()),
            name="Tenant 1 Document",
            tenant_id=tenant1.id,
            kb_id=str(uuid.uuid4()),
            type="pdf",
            size=1000,
            status="1"
        )
        
        # Verify tenant 2 cannot access tenant 1's documents
        tenant2_docs = DocumentService.get_by_tenant_id(tenant2.id)
        doc_ids = [d.id for d in tenant2_docs]
        assert doc1.id not in doc_ids
    
    def test_tenant_counting_isolation(self, setup_tenants):
        """Test that counting operations respect tenant isolation."""
        tenant1 = setup_tenants['tenant1']
        tenant2 = setup_tenants['tenant2']
        
        # Create multiple documents for each tenant
        for i in range(3):
            DocumentService.save(
                id=str(uuid.uuid4()),
                name=f"Tenant 1 Doc {i}",
                tenant_id=tenant1.id,
                kb_id=str(uuid.uuid4()),
                type="pdf",
                size=1000,
                status="1"
            )
            
            DocumentService.save(
                id=str(uuid.uuid4()),
                name=f"Tenant 2 Doc {i}",
                tenant_id=tenant2.id,
                kb_id=str(uuid.uuid4()),
                type="pdf",
                size=1000,
                status="1"
            )
        
        # Test counts are isolated by tenant
        tenant1_count = DocumentService.count_documents(tenant_id=tenant1.id)
        tenant2_count = DocumentService.count_documents(tenant_id=tenant2.id)
        
        assert tenant1_count == 3
        assert tenant2_count == 3
    
    def test_tenant_filtering_in_queries(self, setup_tenants):
        """Test that queries automatically filter by tenant when tenant_id is provided."""
        tenant1 = setup_tenants['tenant1']
        tenant2 = setup_tenants['tenant2']
        
        # Create documents with similar names but different tenants
        DocumentService.save(
            id=str(uuid.uuid4()),
            name="Shared Document",
            tenant_id=tenant1.id,
            kb_id=str(uuid.uuid4()),
            type="pdf",
            size=1000,
            status="1"
        )
        
        DocumentService.save(
            id=str(uuid.uuid4()),
            name="Shared Document",
            tenant_id=tenant2.id,
            kb_id=str(uuid.uuid4()),
            type="pdf",
            size=1000,
            status="1"
        )
        
        # Test that queries respect tenant filtering
        tenant1_docs = DocumentService.query(name="Shared Document", tenant_id=tenant1.id)
        tenant2_docs = DocumentService.query(name="Shared Document", tenant_id=tenant2.id)
        
        assert len(tenant1_docs) == 1
        assert len(tenant2_docs) == 1
        assert tenant1_docs[0].tenant_id == tenant1.id
        assert tenant2_docs[0].tenant_id == tenant2.id
    
    def test_soft_deleted_tenant_isolation(self, setup_tenants):
        """Test that soft-deleted tenants properly isolate data."""
        tenant1 = setup_tenants['tenant1']
        
        # Create a document
        doc1 = DocumentService.save(
            id=str(uuid.uuid4()),
            name="Test Document",
            tenant_id=tenant1.id,
            kb_id=str(uuid.uuid4()),
            type="pdf",
            size=1000,
            status="1"
        )
        
        # Soft delete the tenant
        TenantService.update_tenant(tenant1.id, status=StatusEnum.INVALID.value)
        
        # Test that documents from deleted tenant are not accessible
        active_docs = DocumentService.get_by_tenant_id(tenant1.id)
        # Note: This depends on implementation - may return all or filter by tenant status
        assert len(active_docs) == 0 or all(d.status != "1" for d in active_docs)


class TestRoleBasedAccess:
    """
    Test suite for role-based access control.
    """
    
    @pytest.fixture
    def setup_roles(self):
        """Set up test users with different roles."""
        tenant = TenantService.create_tenant(
            id=str(uuid.uuid4()),
            name="Role Test Tenant",
            description="Test tenant for role testing",
            status=StatusEnum.VALID.value,
            credit=1000
        )
        
        # Create users with different roles
        owner = UserService.save(
            id=str(uuid.uuid4()),
            email="owner@test.com",
            nickname="Owner User",
            status=StatusEnum.VALID.value
        )
        
        normal = UserService.save(
            id=str(uuid.uuid4()),
            email="normal@test.com",
            nickname="Normal User",
            status=StatusEnum.VALID.value
        )
        
        invite = UserService.save(
            id=str(uuid.uuid4()),
            email="invite@test.com",
            nickname="Invite User",
            status=StatusEnum.VALID.value
        )
        
        # Assign roles
        UserTenantService.save(
            id=str(uuid.uuid4()),
            user_id=owner.id,
            tenant_id=tenant.id,
            role=UserTenantRole.OWNER,
            status=StatusEnum.VALID.value
        )
        
        UserTenantService.save(
            id=str(uuid.uuid4()),
            user_id=normal.id,
            tenant_id=tenant.id,
            role=UserTenantRole.NORMAL,
            status=StatusEnum.VALID.value
        )
        
        UserTenantService.save(
            id=str(uuid.uuid4()),
            user_id=invite.id,
            tenant_id=tenant.id,
            role=UserTenantRole.INVITE,
            status=StatusEnum.VALID.value
        )
        
        return {
            'tenant': tenant,
            'owner': owner,
            'normal': normal,
            'invite': invite
        }
    
    def test_owner_role_access(self, setup_roles):
        """Test owner role has full access."""
        tenant = setup_roles['tenant']
        owner = setup_roles['owner']
        
        has_access, role = RoleBasedAccessControl.check_user_role(tenant.id, owner.id)
        assert has_access is True
        assert role == UserTenantRole.OWNER
    
    def test_normal_role_access(self, setup_roles):
        """Test normal role has limited access."""
        tenant = setup_roles['tenant']
        normal = setup_roles['normal']
        
        has_access, role = RoleBasedAccessControl.check_user_role(tenant.id, normal.id)
        assert has_access is True
        assert role == UserTenantRole.NORMAL
    
    def test_invite_role_access(self, setup_roles):
        """Test invite role has minimal access."""
        tenant = setup_roles['tenant']
        invite = setup_roles['invite']
        
        has_access, role = RoleBasedAccessControl.check_user_role(tenant.id, invite.id)
        assert has_access is True
        assert role == UserTenantRole.INVITE
    
    def test_no_access_for_unauthorized_user(self, setup_roles):
        """Test unauthorized users have no access."""
        tenant = setup_roles['tenant']
        
        # Create unauthorized user
        unauthorized = UserService.save(
            id=str(uuid.uuid4()),
            email="unauthorized@test.com",
            nickname="Unauthorized User",
            status=StatusEnum.VALID.value
        )
        
        has_access, role = RoleBasedAccessControl.check_user_role(tenant.id, unauthorized.id)
        assert has_access is False
        assert role is None


if __name__ == "__main__":
    pytest.main([__file__, "-v"])