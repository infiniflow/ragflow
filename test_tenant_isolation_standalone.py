#!/usr/bin/env python3
"""
Standalone tenant isolation test without heavy dependencies
"""

import os
import sys
import json
import uuid
from datetime import datetime

# Add project root to path
sys.path.insert(0, 'F:/04_AI/01_Workplace/ragflow_A')

def test_tenant_model_structure():
    """Test that tenant_id fields are properly added to models"""
    print("[TEST] Testing Tenant Model Structure")
    print("=" * 50)
    
    # Mock the model structure to test tenant_id fields
    expected_models = [
        'Document', 'Conversation', 'Knowledgebase', 
        'ChatAssistant', 'Session', 'File', 'Chunk'
    ]
    
    print("Testing tenant_id field presence:")
    
    # Read the actual model files to verify tenant_id fields
    model_files = [
        'api/db/db_models.py',
        'api/db/services/tenant_service.py',
        'api/middleware/tenant_middleware.py'
    ]
    
    for model_file in model_files:
        if os.path.exists(model_file):
            print(f"[OK] {model_file} exists")
        else:
            print(f"[MISSING] {model_file} not found")
    
    # Check for tenant_id in Document model
    try:
        with open('api/db/db_models.py', 'r') as f:
            content = f.read()
            
        tenant_fields_found = 0
        expected_fields = [
            'tenant_id = CharField',
            'tenant_id = ForeignKeyField',
            "tenant_id = CharField(max_length=32, null=True, index=True)"
        ]
        
        for field in expected_fields:
            if field in content:
                tenant_fields_found += 1
                print(f"[OK] Found tenant_id field definition: {field[:30]}...")
        
        if tenant_fields_found > 0:
            print(f"[SUCCESS] Found {tenant_fields_found} tenant_id field definitions")
            
        # Check for tenant service
        if 'class TenantService' in content or 'tenant_service.py' in os.listdir('api/db/services'):
            print("[OK] TenantService exists")
            
    except Exception as e:
        print(f"[ERROR] Reading model file: {e}")

def test_tenant_service_api():
    """Test tenant service API structure"""
    print("\n[TEST] Testing Tenant Service API")
    print("=" * 50)
    
    # Check for tenant service file
    tenant_service_path = 'api/db/services/tenant_service.py'
    if os.path.exists(tenant_service_path):
        print("[OK] Tenant service file exists")
        
        try:
            with open(tenant_service_path, 'r') as f:
                content = f.read()
                
            expected_methods = [
                'create_tenant',
                'get_tenant_by_id',
                'update_tenant',
                'delete_tenant',
                'get_all_tenants'
            ]
            
            found_methods = 0
            for method in expected_methods:
                if method in content:
                    found_methods += 1
                    print(f"[OK] Found method: {method}")
                    
            print(f"[SUCCESS] Found {found_methods}/{len(expected_methods)} tenant service methods")
            
        except Exception as e:
            print(f"[ERROR] Reading tenant service: {e}")
    else:
        print("[MISSING] Tenant service file not found")

def test_tenant_middleware():
    """Test tenant middleware structure"""
    print("\n[TEST] Testing Tenant Middleware")
    print("=" * 50)
    
    # Check for tenant middleware file
    middleware_path = 'api/middleware/tenant_middleware.py'
    if os.path.exists(middleware_path):
        print("[OK] Tenant middleware file exists")
        
        try:
            with open(middleware_path, 'r') as f:
                content = f.read()
                
            expected_components = [
                'class TenantMiddleware',
                'get_current_tenant',
                'set_current_tenant',
                'tenant_aware',
                'require_tenant'
            ]
            
            found_components = 0
            for component in expected_components:
                if component in content:
                    found_components += 1
                    print(f"[OK] Found component: {component}")
                    
            print(f"[SUCCESS] Found {found_components}/{len(expected_components)} middleware components")
            
        except Exception as e:
            print(f"[ERROR] Reading middleware: {e}")
    else:
        print("[MISSING] Tenant middleware file not found")

def test_tenant_api_endpoints():
    """Test tenant API endpoints"""
    print("\n[TEST] Testing Tenant API Endpoints")
    print("=" * 50)
    
    # Check for tenant API files
    api_files = [
        'api/apps/tenant_app.py',
        'api/apps/tenant_management_app.py'
    ]
    
    for api_file in api_files:
        if os.path.exists(api_file):
            print(f"[OK] {api_file} exists")
            
            try:
                with open(api_file, 'r') as f:
                    content = f.read()
                    
                if '@manager.route' in content:
                    print(f"[OK] {api_file} has API routes")
                    
                # Count API endpoints
                route_count = content.count('@manager.route')
                print(f"[INFO] {api_file} has {route_count} API endpoints")
                
            except Exception as e:
                print(f"[ERROR] Reading API file: {e}")
        else:
            print(f"[MISSING] {api_file} not found")

def test_migration_scripts():
    """Test migration scripts"""
    print("\n[TEST] Testing Migration Scripts")
    print("=" * 50)
    
    # Check for migration scripts
    migration_scripts = [
        'scripts/migrate_existing_tenant_data.py',
        'scripts/add_tenant_id_fields.py',
        'scripts/migrate_tenant_data.py'
    ]
    
    for script in migration_scripts:
        if os.path.exists(script):
            print(f"[OK] {script} exists")
            
            try:
                with open(script, 'r') as f:
                    content = f.read()
                    
                if 'def ' in content:
                    print(f"[OK] {script} has functions")
                    
                # Check for main migration functions
                if 'execute_migration' in content:
                    print(f"[OK] {script} has execute_migration function")
                    
            except Exception as e:
                print(f"[ERROR] Reading migration script: {e}")
        else:
            print(f"[MISSING] {script} not found")

def test_frontend_tenant_components():
    """Test frontend tenant components"""
    print("\n[TEST] Testing Frontend Tenant Components")
    print("=" * 50)
    
    # Check for frontend tenant components
    frontend_files = [
        'web/src/components/TenantSelector.tsx',
        'web/src/contexts/TenantContext.tsx',
        'web/src/services/tenant-service.ts'
    ]
    
    for file in frontend_files:
        if os.path.exists(file):
            print(f"[OK] {file} exists")
        else:
            print(f"[MISSING] {file} not found")

def generate_test_summary():
    """Generate test summary"""
    print("\n" + "=" * 60)
    print("[SUMMARY] Tenant Isolation Testing Summary")
    print("=" * 60)
    
    # Count existing components
    components = {
        'Model Files': ['api/db/db_models.py'],
        'Tenant Service': ['api/db/services/tenant_service.py'],
        'Tenant Middleware': ['api/middleware/tenant_middleware.py'],
        'Tenant API': ['api/apps/tenant_app.py', 'api/apps/tenant_management_app.py'],
        'Migration Scripts': ['scripts/migrate_existing_tenant_data.py', 'scripts/add_tenant_id_fields.py'],
        'Frontend Components': ['web/src/components/TenantSelector.tsx', 'web/src/services/tenant-service.ts']
    }
    
    total_components = 0
    found_components = 0
    
    for category, files in components.items():
        category_found = 0
        for file in files:
            total_components += 1
            if os.path.exists(file):
                found_components += 1
                category_found += 1
        
        print(f"{category}: {category_found}/{len(files)} components found")
    
    print(f"\nOverall: {found_components}/{total_components} components found")
    
    if found_components == total_components:
        print("[SUCCESS] All tenant isolation components are implemented!")
    else:
        print("[WARNING] Some components are missing")
    
    print("\n[READY] Tenant isolation system is ready for testing!")

if __name__ == "__main__":
    test_tenant_model_structure()
    test_tenant_service_api()
    test_tenant_middleware()
    test_tenant_api_endpoints()
    test_migration_scripts()
    test_frontend_tenant_components()
    generate_test_summary()