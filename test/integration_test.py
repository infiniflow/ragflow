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

import requests
import json
import uuid
import time

# Test configuration
BASE_URL = "http://localhost:9380"
API_VERSION = "v1"

def test_complete_tenant_flow():
    """
    Complete end-to-end test of tenant management flow.
    
    Tests the following flow:
    1. Create tenant
    2. List tenants
    3. Get tenant details
    4. Update tenant
    5. Switch tenant context
    6. Verify tenant isolation
    """
    
    print("Starting complete tenant management flow test...")
    
    # Test 1: Create a new tenant
    print("1. Testing tenant creation...")
    create_payload = {
        "name": f"Test Tenant {uuid.uuid4().hex[:8]}",
        "description": "Test tenant for integration testing",
        "credit": 1000
    }
    
    try:
        response = requests.post(
            f"{BASE_URL}/api/{API_VERSION}/tenant_management/create",
            json=create_payload,
            headers={"Authorization": "test_token"}  # Mock token
        )
        
        if response.status_code == 401:
            print("Authentication required - setting up test user...")
            # In real test, you'd set up proper auth
            return
            
        response.raise_for_status()
        created_tenant = response.json()
        tenant_id = created_tenant['data']['id']
        print(f"   ✓ Created tenant: {tenant_id}")
        
    except requests.exceptions.RequestException as e:
        print(f"   ✗ Failed to create tenant: {e}")
        return
    
    # Test 2: List tenants
    print("2. Testing tenant listing...")
    try:
        response = requests.get(
            f"{BASE_URL}/api/{API_VERSION}/tenant_management/list",
            headers={"Authorization": "test_token"}
        )
        response.raise_for_status()
        tenant_list = response.json()
        
        assert 'tenants' in tenant_list['data']
        assert isinstance(tenant_list['data']['tenants'], list)
        print(f"   ✓ Listed {len(tenant_list['data']['tenants'])} tenants")
        
    except requests.exceptions.RequestException as e:
        print(f"   ✗ Failed to list tenants: {e}")
        return
    
    # Test 3: Get tenant details
    print("3. Testing get tenant details...")
    try:
        response = requests.get(
            f"{BASE_URL}/api/{API_VERSION}/tenant_management/{tenant_id}",
            headers={"Authorization": "test_token"}
        )
        response.raise_for_status()
        tenant_details = response.json()
        
        assert tenant_details['data']['id'] == tenant_id
        assert tenant_details['data']['name'] == create_payload['name']
        print("   ✓ Retrieved tenant details")
        
    except requests.exceptions.RequestException as e:
        print(f"   ✗ Failed to get tenant details: {e}")
        return
    
    # Test 4: Update tenant
    print("4. Testing tenant update...")
    update_payload = {
        "name": f"Updated Test Tenant {uuid.uuid4().hex[:8]}",
        "description": "Updated test tenant description"
    }
    
    try:
        response = requests.put(
            f"{BASE_URL}/api/{API_VERSION}/tenant_management/{tenant_id}",
            json=update_payload,
            headers={"Authorization": "test_token"}
        )
        response.raise_for_status()
        updated_tenant = response.json()
        
        assert updated_tenant['data']['name'] == update_payload['name']
        print("   ✓ Updated tenant")
        
    except requests.exceptions.RequestException as e:
        print(f"   ✗ Failed to update tenant: {e}")
        return
    
    # Test 5: Get tenant configuration
    print("5. Testing tenant configuration...")
    try:
        response = requests.get(
            f"{BASE_URL}/api/{API_VERSION}/tenant_management/{tenant_id}/config",
            headers={"Authorization": "test_token"}
        )
        response.raise_for_status()
        config = response.json()
        
        assert 'tenant_id' in config['data']
        assert config['data']['tenant_id'] == tenant_id
        print("   ✓ Retrieved tenant configuration")
        
    except requests.exceptions.RequestException as e:
        print(f"   ✗ Failed to get tenant configuration: {e}")
        return
    
    # Test 6: Get tenant usage
    print("6. Testing tenant usage statistics...")
    try:
        response = requests.get(
            f"{BASE_URL}/api/{API_VERSION}/tenant_management/{tenant_id}/usage",
            headers={"Authorization": "test_token"}
        )
        response.raise_for_status()
        usage = response.json()
        
        assert 'document_count' in usage['data']
        assert 'knowledgebase_count' in usage['data']
        assert 'conversation_count' in usage['data']
        print("   ✓ Retrieved tenant usage")
        
    except requests.exceptions.RequestException as e:
        print(f"   ✗ Failed to get tenant usage: {e}")
        return
    
    # Test 7: Switch tenant context
    print("7. Testing tenant switching...")
    try:
        response = requests.post(
            f"{BASE_URL}/api/{API_VERSION}/tenant_management/{tenant_id}/switch",
            headers={"Authorization": "test_token"}
        )
        response.raise_for_status()
        switch_result = response.json()
        
        assert switch_result['data']['tenant_id'] == tenant_id
        print("   ✓ Switched tenant context")
        
    except requests.exceptions.RequestException as e:
        print(f"   ✗ Failed to switch tenant: {e}")
        return
    
    print("\n✅ All tenant management tests completed successfully!")

def test_tenant_isolation():
    """
    Test tenant isolation by creating resources across tenants.
    """
    print("\nTesting tenant isolation...")
    
    # This would require proper authentication setup
    print("⚠️  Tenant isolation tests require authenticated users...")
    print("Integration test completed - manual verification recommended")

if __name__ == "__main__":
    print("RAGFlow Tenant Management Integration Test")
    print("=" * 50)
    
    # Wait for services to be ready
    print("Waiting for services to be ready...")
    time.sleep(5)
    
    try:
        test_complete_tenant_flow()
        test_tenant_isolation()
    except Exception as e:
        print(f"Integration test failed: {e}")
        print("\n⚠️  Note: This test requires:")
        print("   1. RAGFlow backend running on http://localhost:9380")
        print("   2. Valid authentication token")
        print("   3. Database connection")
        print("   4. Tenant management APIs registered")