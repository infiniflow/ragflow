#!/bin/bash

# Production Tenant Testing Script
echo "=== Production Tenant Testing ==="

# 1. Environment Check
echo "1. Checking production environment..."
docker-compose -f docker/docker-compose.yml ps

# 2. Database Connectivity
echo "2. Testing database connectivity..."
docker exec ragflow-mysql mysql -u root -p${MYSQL_PASSWORD} -e "SELECT COUNT(*) FROM tenant;" ragflow

# 3. API Health Check
echo "3. Testing API health..."
curl -f http://localhost:9380/api/v1/health || echo "API health check failed"

# 4. Tenant Creation Test
echo "4. Testing tenant creation..."
curl -X POST http://localhost:9380/api/v1/tenant/create \
  -H "Content-Type: application/json" \
  -d '{"name": "prod_test_tenant", "description": "Production test"}' \
  -w "%{http_code}" -o /dev/null

# 5. Tenant Isolation Test
echo "5. Testing tenant isolation..."
curl -X GET http://localhost:9380/api/v1/tenant/list \
  -H "X-Tenant-ID: prod_test_tenant" \
  -w "%{http_code}" -o /dev/null

echo "=== Production Testing Complete ==="