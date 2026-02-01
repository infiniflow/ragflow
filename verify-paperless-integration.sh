#!/bin/bash

# Paperless-ngx Integration Verification Script
# This script checks if Paperless-ngx integration is available in your RAGFlow installation

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Paperless-ngx Integration Verification"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if running from correct directory
if [ ! -f "web/src/pages/user-setting/data-source/constant/index.tsx" ]; then
    echo -e "${RED}✗${NC} Error: Please run this script from the RAGFlow repository root"
    exit 1
fi

echo "Checking Paperless-ngx integration status..."
echo ""

# 1. Check backend files
echo "1. Backend Integration:"
if [ -f "common/data_source/paperless_ngx_connector.py" ]; then
    echo -e "   ${GREEN}✓${NC} Connector file exists"
else
    echo -e "   ${RED}✗${NC} Connector file missing"
fi

if grep -q "PAPERLESS_NGX" common/constants.py 2>/dev/null; then
    echo -e "   ${GREEN}✓${NC} Added to constants"
else
    echo -e "   ${RED}✗${NC} Not in constants"
fi

if grep -q "PaperlessNgxConnector" common/data_source/__init__.py 2>/dev/null; then
    echo -e "   ${GREEN}✓${NC} Exported in __init__.py"
else
    echo -e "   ${RED}✗${NC} Not exported"
fi

echo ""

# 2. Check frontend files
echo "2. Frontend Integration:"
if grep -q "PAPERLESS_NGX = 'paperless_ngx'" web/src/pages/user-setting/data-source/constant/index.tsx 2>/dev/null; then
    echo -e "   ${GREEN}✓${NC} Added to DataSourceKey enum"
    
    # Check position
    LINE=$(grep -n "PAPERLESS_NGX = 'paperless_ngx'" web/src/pages/user-setting/data-source/constant/index.tsx | cut -d: -f1)
    S3_LINE=$(grep -n "S3 = 's3'" web/src/pages/user-setting/data-source/constant/index.tsx | cut -d: -f1)
    NOTION_LINE=$(grep -n "NOTION = 'notion'" web/src/pages/user-setting/data-source/constant/index.tsx | cut -d: -f1)
    
    if [ $LINE -gt $S3_LINE ] && [ $LINE -lt $NOTION_LINE ]; then
        echo -e "   ${GREEN}✓${NC} Positioned between S3 and Notion (line $LINE)"
    else
        echo -e "   ${YELLOW}⚠${NC} Position may be incorrect (line $LINE)"
    fi
else
    echo -e "   ${RED}✗${NC} Not in DataSourceKey enum"
fi

if [ -f "web/src/assets/svg/data-source/paperless-ngx.svg" ]; then
    echo -e "   ${GREEN}✓${NC} Icon file exists"
else
    echo -e "   ${RED}✗${NC} Icon file missing"
fi

if grep -q "paperless_ngxDescription" web/src/locales/en.ts 2>/dev/null; then
    echo -e "   ${GREEN}✓${NC} Translations added"
else
    echo -e "   ${RED}✗${NC} Translations missing"
fi

echo ""

# 3. Check Docker status
echo "3. Docker Status:"
if command -v docker &> /dev/null; then
    if docker ps | grep -q ragflow; then
        echo -e "   ${GREEN}✓${NC} RAGFlow container is running"
        
        # Get image info
        IMAGE=$(docker inspect ragflow-cpu 2>/dev/null | grep -o '"Image": "[^"]*"' | cut -d'"' -f4 | head -1)
        if [ ! -z "$IMAGE" ]; then
            echo -e "   ${YELLOW}ℹ${NC} Using image: $IMAGE"
        fi
        
        # Check if it's a custom build
        if echo "$IMAGE" | grep -q "dev-paperless"; then
            echo -e "   ${GREEN}✓${NC} Using custom-built image with Paperless-ngx"
        elif echo "$IMAGE" | grep -q "nightly\|latest"; then
            echo -e "   ${YELLOW}⚠${NC} Using nightly/latest - may or may not include changes"
        else
            echo -e "   ${RED}⚠${NC} Using pre-built image - likely missing Paperless-ngx"
            echo -e "   ${YELLOW}→${NC} You need to rebuild the Docker image locally"
        fi
    else
        echo -e "   ${YELLOW}⚠${NC} RAGFlow container not running"
    fi
else
    echo -e "   ${YELLOW}⚠${NC} Docker not found or not in PATH"
fi

echo ""

# 4. Summary
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  Summary"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [ -f "common/data_source/paperless_ngx_connector.py" ] && \
   grep -q "PAPERLESS_NGX = 'paperless_ngx'" web/src/pages/user-setting/data-source/constant/index.tsx 2>/dev/null; then
    echo -e "${GREEN}✓ Code Integration Complete${NC}"
    echo ""
    echo "Next steps:"
    echo "1. Build Docker image locally:"
    echo "   docker build --platform linux/amd64 -f Dockerfile -t infiniflow/ragflow:dev-paperless ."
    echo ""
    echo "2. Update docker/.env:"
    echo "   RAGFLOW_IMAGE=infiniflow/ragflow:dev-paperless"
    echo ""
    echo "3. Restart services:"
    echo "   cd docker && docker compose down && docker compose up -d"
    echo ""
    echo "4. Access RAGFlow and check Settings → Data Sources"
    echo ""
    echo "See BUILD_WITH_PAPERLESS.md for detailed instructions."
else
    echo -e "${RED}✗ Code Integration Incomplete${NC}"
    echo ""
    echo "Some files are missing. This might not be the correct branch."
    echo "Please check out the 'copilot/implement-paperless-ngx-integration' branch."
fi

echo ""
