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
"""
Migration script to add tenant_id to existing documents and conversations.

This script handles the migration of existing data to the new multi-tenant architecture.
It provides three migration strategies:
1. Single tenant: All existing data assigned to a default tenant
2. User-based: Each user's data assigned to their own tenant
3. Knowledgebase-based: Each knowledgebase becomes a separate tenant
"""

import os
import sys
import argparse
from datetime import datetime

# Add project root to path
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from api.db.db_models import DB, Document, Conversation, Knowledgebase, Tenant, UserTenant
from api.db.db_models import init_database_tables
from api.utils import get_uuid


def create_default_tenant():
    """Create a default tenant for migration purposes."""
    default_tenant = {
        'id': 'default_tenant_001',
        'name': 'Default Tenant',
        'description': 'Default tenant for existing data migration'
    }
    
    try:
        tenant = Tenant.create(**default_tenant)
        print(f"✓ Created default tenant: {tenant.id}")
        return tenant.id
    except Exception as e:
        print(f"✗ Failed to create default tenant: {e}")
        return None


def migrate_documents_strategy_1(default_tenant_id):
    """
    Strategy 1: Assign all existing documents to a single default tenant.
    """
    print("\n=== Strategy 1: Single Default Tenant Migration ===")
    
    try:
        # Count documents without tenant_id
        count = Document.select().where(Document.tenant_id.is_null()).count()
        print(f"Found {count} documents without tenant_id")
        
        if count == 0:
            print("✓ No documents need migration")
            return True
            
        # Update documents
        updated = Document.update(tenant_id=default_tenant_id).where(
            Document.tenant_id.is_null()
        ).execute()
        
        print(f"✓ Updated {updated} documents with tenant_id={default_tenant_id}")
        return True
        
    except Exception as e:
        print(f"✗ Failed to migrate documents: {e}")
        return False


def migrate_conversations_strategy_1(default_tenant_id):
    """
    Strategy 1: Assign all existing conversations to a single default tenant.
    """
    print("\n=== Conversations Migration ===")
    
    try:
        # Count conversations without tenant_id
        count = Conversation.select().where(Conversation.tenant_id.is_null()).count()
        print(f"Found {count} conversations without tenant_id")
        
        if count == 0:
            print("✓ No conversations need migration")
            return True
            
        # Update conversations
        updated = Conversation.update(tenant_id=default_tenant_id).where(
            Conversation.tenant_id.is_null()
        ).execute()
        
        print(f"✓ Updated {updated} conversations with tenant_id={default_tenant_id}")
        return True
        
    except Exception as e:
        print(f"✗ Failed to migrate conversations: {e}")
        return False


def migrate_documents_strategy_2():
    """
    Strategy 2: Create tenant per knowledgebase and assign documents accordingly.
    """
    print("\n=== Strategy 2: Knowledgebase-based Tenant Migration ===")
    
    try:
        # Get all knowledgebases
        knowledgebases = Knowledgebase.select()
        total_docs_migrated = 0
        
        for kb in knowledgebases:
            # Create tenant for this knowledgebase
            tenant_id = get_uuid()
            tenant_name = f"Tenant for {kb.name}"
            
            tenant = Tenant.create(
                id=tenant_id,
                name=tenant_name,
                description=f"Auto-created tenant for KB: {kb.name}"
            )
            
            # Update documents for this knowledgebase
            updated = Document.update(tenant_id=tenant_id).where(
                (Document.kb_id == kb.id) & 
                (Document.tenant_id.is_null())
            ).execute()
            
            # Update knowledgebase tenant_id
            Knowledgebase.update(tenant_id=tenant_id).where(
                Knowledgebase.id == kb.id
            ).execute()
            
            total_docs_migrated += updated
            print(f"✓ Created tenant {tenant_id} for KB {kb.name}, migrated {updated} documents")
        
        print(f"✓ Total documents migrated: {total_docs_migrated}")
        return True
        
    except Exception as e:
        print(f"✗ Failed in knowledgebase-based migration: {e}")
        return False


def validate_migration():
    """Validate the migration results."""
    print("\n=== Migration Validation ===")
    
    try:
        # Check for remaining documents without tenant_id
        docs_without_tenant = Document.select().where(Document.tenant_id.is_null()).count()
        convs_without_tenant = Conversation.select().where(Conversation.tenant_id.is_null()).count()
        
        print(f"Documents still without tenant_id: {docs_without_tenant}")
        print(f"Conversations still without tenant_id: {convs_without_tenant}")
        
        # Check tenant distribution
        tenant_counts = {}
        for doc in Document.select(Document.tenant_id, fn.COUNT(Document.id).alias('count')).group_by(Document.tenant_id):
            tenant_counts[doc.tenant_id] = doc.count
        
        print("\nTenant distribution:")
        for tenant_id, count in tenant_counts.items():
            print(f"  Tenant {tenant_id}: {count} documents")
            
        return docs_without_tenant == 0 and convs_without_tenant == 0
        
    except Exception as e:
        print(f"✗ Failed to validate migration: {e}")
        return False


def rollback_migration():
    """Rollback migration by setting tenant_id to NULL."""
    print("\n=== Rollback Migration ===")
    
    try:
        # Rollback documents
        Document.update(tenant_id=None).execute()
        print("✓ Rolled back documents")
        
        # Rollback conversations
        Conversation.update(tenant_id=None).execute()
        print("✓ Rolled back conversations")
        
        # Delete created tenants
        Tenant.delete().where(Tenant.name.contains("Tenant for")).execute()
        print("✓ Deleted auto-created tenants")
        
        return True
        
    except Exception as e:
        print(f"✗ Failed to rollback: {e}")
        return False


def main():
    parser = argparse.ArgumentParser(description='Migrate existing data to multi-tenant architecture')
    parser.add_argument('--strategy', choices=['1', '2'], default='1',
                        help='Migration strategy: 1=single tenant, 2=knowledgebase-based')
    parser.add_argument('--dry-run', action='store_true',
                        help='Show what would be migrated without making changes')
    parser.add_argument('--validate', action='store_true',
                        help='Validate migration results')
    parser.add_argument('--rollback', action='store_true',
                        help='Rollback migration')
    
    args = parser.parse_args()
    
    print("=" * 60)
    print("Multi-Tenant Migration Script")
    print("=" * 60)
    print(f"Started at: {datetime.now()}")
    
    try:
        # Initialize database
        init_database_tables()
        
        if args.rollback:
            rollback_migration()
            return
            
        if args.validate:
            validate_migration()
            return
            
        if args.dry_run:
            print("\n=== DRY RUN MODE ===")
            docs_count = Document.select().where(Document.tenant_id.is_null()).count()
            convs_count = Conversation.select().where(Conversation.tenant_id.is_null()).count()
            print(f"Would migrate {docs_count} documents and {convs_count} conversations")
            return
        
        # Perform migration based on strategy
        if args.strategy == '1':
            default_tenant_id = create_default_tenant()
            if default_tenant_id:
                migrate_documents_strategy_1(default_tenant_id)
                migrate_conversations_strategy_1(default_tenant_id)
                
        elif args.strategy == '2':
            migrate_documents_strategy_2()
            migrate_conversations_strategy_1('default_tenant_001')  # Use default for conversations
            
        # Validate results
        validate_migration()
        
    except Exception as e:
        print(f"✗ Migration failed: {e}")
        sys.exit(1)
    
    print(f"\nMigration completed at: {datetime.now()}")


if __name__ == "__main__":
    main()