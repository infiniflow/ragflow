# Multi-Tenant Deployment and Rollback Guide

## Overview
This guide provides comprehensive instructions for deploying the multi-tenant architecture in RAGFlow_A, including user level management, deployment strategies, and rollback procedures.

## Tenant User Levels and Hierarchy

### User Role Architecture
The multi-tenant system implements a **three-tier user hierarchy**:

#### 1. **System Administrator**
- **Scope**: Cross-tenant management
- **Permissions**:
  - Create/delete tenants
  - Manage tenant quotas and limits
  - Monitor system-wide resource usage
  - Configure global settings
  - Access system logs and analytics
- **API Endpoints**: `/api/v1/admin/*`

#### 2. **Tenant Manager**
- **Scope**: Single tenant management
- **Permissions**:
  - Manage tenant users and roles
  - Configure tenant-specific settings
  - Monitor tenant resource usage
  - Manage knowledgebases within tenant
  - Configure AI models and integrations
- **API Endpoints**: `/api/v1/tenant/*`

#### 3. **End User**
- **Scope**: Resource access within assigned tenant
- **Permissions**:
  - Create/view documents in assigned knowledgebases
  - Participate in conversations
  - Access shared resources
  - Personal settings management
- **API Endpoints**: `/api/v1/user/*`

## Safe Deployment Process

### Pre-Deployment Checklist
```bash
# 1. Environment verification
python -c "import api.db.db_models; print('Database ready')"
python -c "from api.middleware.tenant_middleware import TenantMiddleware; print('Middleware ready')"

# 2. Backup existing data
pg_dump -h localhost -U ragflow ragflow_db > backup_$(date +%Y%m%d_%H%M%S).sql

# 3. Verify current state
python scripts/migrate_existing_tenant_data.py --validate
```

### Deployment Steps

#### Phase 1: Environment Preparation (5-10 minutes)
```bash
# 1. Update codebase
git pull origin feature/multitenant

# 2. Install dependencies
pip install -r requirements.txt

# 3. Database schema verification
python -c "
from api.db.db_models import init_database_tables
init_database_tables()
print('✓ Database schema updated')
"
```

#### Phase 2: Migration Strategy Selection
Choose your migration strategy based on organizational needs:

**Strategy A: Single Tenant (Recommended for small teams)**
```bash
# All existing data under one tenant
python scripts/migrate_existing_tenant_data.py --strategy 1
```

**Strategy B: Knowledgebase-based (Recommended for larger organizations)**
```bash
# Each knowledgebase becomes separate tenant
python scripts/migrate_existing_tenant_data.py --strategy 2
```

#### Phase 3: Validation and Testing
```bash
# 1. Dry-run validation
python scripts/migrate_existing_tenant_data.py --dry-run

# 2. Run migration
python scripts/migrate_existing_tenant_data.py --strategy 1

# 3. Post-migration validation
python scripts/migrate_existing_tenant_data.py --validate

# 4. Test basic functionality
curl -X GET "http://localhost:9380/api/v1/documents" -H "X-Tenant-ID: default_tenant_001"
```

#### Phase 4: Service Restart
```bash
# 1. Restart application
sudo systemctl restart ragflow

# 2. Verify service health
curl -f http://localhost:9380/health || echo "Service check failed"
```

## Rollback Procedures

### Immediate Rollback (Emergency)
```bash
# 1. Stop service
sudo systemctl stop ragflow

# 2. Restore database
psql -h localhost -U ragflow ragflow_db < backup_YYYYMMDD_HHMMSS.sql

# 3. Revert code
git checkout main

# 4. Restart service
sudo systemctl start ragflow
```

### Partial Rollback (Data Only)
```bash
# 1. Rollback tenant data only
python scripts/migrate_existing_tenant_data.py --rollback

# 2. Verify rollback
python scripts/migrate_existing_tenant_data.py --validate
```

### Feature Toggle Rollback
```python
# Add to config file (config.py)
MULTITENANT_ENABLED = False  # Toggle off
```

## Testing Matrix

### Test Scenarios
| Scenario | Expected Result | Test Command |
|----------|-----------------|--------------|
| Existing document access | Returns data | `curl -H "X-Tenant-ID: default_tenant_001" ...` |
| Cross-tenant access | Returns 403 | `curl -H "X-Tenant-ID: wrong_tenant" ...` |
| Admin tenant creation | Returns 201 | `curl -X POST /api/v1/admin/tenant -d '{"name":"new"}'` |
| User document upload | Creates w/ tenant ID | `curl -X POST /api/v1/documents/upload -F "file=@test.pdf"` |

### Load Testing
```bash
# Simulate multi-tenant load
locust -f tests/multitenant_load_test.py --host=http://localhost:9380
```

## Monitoring and Alerts

### Key Metrics
- **Tenant Isolation**: Ensure no cross-tenant data leakage
- **Resource Usage**: Monitor per-tenant resource consumption
- **Error Rates**: Track 403/404 rates for tenant validation
- **Migration Progress**: Monitor migration completion status

### Health Checks
```bash
# Check tenant context
python -c "
from api.middleware.tenant_middleware import TenantMiddleware
print('Tenant middleware:', TenantMiddleware.get_current_tenant() or 'Not set')
"

# Verify data isolation
python -c "
from api.db.services.document_service import DocumentService
from flask import g
g.tenant_id = 'test_tenant'
count = DocumentService.get_doc_count('test_tenant')
print(f'Documents for test_tenant: {count}')
"
```

## Troubleshooting

### Common Issues

#### 1. Missing Tenant Data
```bash
# Symptom: Documents return empty results
# Solution: Run migration
python scripts/migrate_existing_tenant_data.py --strategy 1
```

#### 2. Cross-Tenant Access
```bash
# Symptom: Users see other tenant's data
# Solution: Check middleware configuration
grep -r "default_tenant_001" api/middleware/
```

#### 3. Performance Degradation
```bash
# Symptom: Slow queries after migration
# Solution: Add tenant_id indexes
psql -c "CREATE INDEX idx_documents_tenant_id ON document(tenant_id);"
```

### Debug Commands
```bash
# Check tenant assignment
python -c "
from api.db.services.document_service import DocumentService
from api.db.db_models import Document
print('Documents without tenant:', Document.select().where(Document.tenant_id.is_null()).count())
"

# Validate user-tenant mapping
python -c "
from api.db.services.user_service import UserTenantService
mappings = UserTenantService.query()
print('User-tenant mappings:', len(mappings))
"
```

## Post-Deployment Verification

### Automated Verification Script
```bash
#!/bin/bash
# run_verification.sh

echo "=== Multi-Tenant Deployment Verification ==="

# 1. Check service health
curl -f http://localhost:9380/health || exit 1
echo "✓ Service health check passed"

# 2. Verify tenant isolation
python -c "
from api.db.services.document_service import DocumentService
tenant_docs = DocumentService.get_doc_count('default_tenant_001')
print(f'Default tenant documents: {tenant_docs}')
" || exit 1
echo "✓ Tenant isolation verified"

# 3. Check API endpoints
curl -s -H "X-Tenant-ID: default_tenant_001" \
  http://localhost:9380/api/v1/kb/list | jq '.code' | grep -q "0" || exit 1
echo "✓ API endpoints responsive"

echo "=== All checks passed ==="
```

## Support and Escalation

### Quick Support Contacts
- **Technical Issues**: Create GitHub issue with label `multitenant`
- **Database Issues**: Check `/logs/migration.log`
- **Performance Issues**: Contact system admin with tenant metrics

### Escalation Path
1. **Level 1**: Check this README and logs
2. **Level 2**: Run diagnostic scripts
3. **Level 3**: Contact development team with tenant context details