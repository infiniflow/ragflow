# Paperless-ngx Integration Status

## Current Status: ✅ Code Complete, ⚠️ Deployment Required

### What's Been Implemented

All code for Paperless-ngx integration is **complete and committed**:

✅ **Backend Integration** (Python)
- REST API connector in `common/data_source/paperless_ngx_connector.py`
- Integration with RAGFlow sync service
- Comprehensive tests
- Full documentation

✅ **Frontend Integration** (TypeScript/React)  
- UI configuration in `web/src/pages/user-setting/data-source/constant/index.tsx`
- Paperless-ngx appears in position 3 (between S3 and Notion)
- Configuration form with all fields
- Translations and tooltips
- Custom SVG icon

✅ **Documentation**
- `docs/paperless_ngx_connector.md` - Connector documentation
- `docs/paperless_ngx_ui_integration.md` - UI integration guide
- `BUILD_WITH_PAPERLESS.md` - Build instructions
- Configuration examples

### Why It's Not Showing in Your UI

Looking at your screenshot, Paperless-ngx is not visible because you're running a **pre-built Docker image** from Docker Hub. This image was built before these changes were committed.

### Solution: Rebuild Docker Image

You have two options:

#### Option 1: Build Locally (Recommended for Testing)

```bash
# 1. Navigate to repository root
cd /path/to/ragflow

# 2. Build Docker image with your changes
docker build --platform linux/amd64 -f Dockerfile -t infiniflow/ragflow:dev-paperless .

# 3. Update docker/.env to use your local image
# Edit docker/.env and change:
RAGFLOW_IMAGE=infiniflow/ragflow:dev-paperless

# 4. Restart Docker services
cd docker
docker compose down
docker compose up -d

# 5. Wait for services to start
docker compose logs -f ragflow-cpu
```

#### Option 2: Wait for Official Release

Wait for the RAGFlow team to:
1. Merge this PR
2. Build and publish a new Docker image
3. Update the default `RAGFLOW_IMAGE` version

Then you can run:
```bash
cd docker
docker compose pull
docker compose up -d
```

### Verification Steps

After rebuilding, you should see:

1. **Data Sources Page** (`Settings → Data Sources`)
   - Position 1: Confluence
   - Position 2: S3
   - **Position 3: Paperless-ngx** ✅ (NEW!)
   - Position 4: Notion
   - Position 5: Discord
   - ... (other sources)

2. **Paperless-ngx Card**
   - Green document icon with "ngx" badge
   - Description: "Connect to Paperless-ngx to sync and index your document management system."
   - Clicking opens configuration modal

3. **Configuration Modal**
   - Paperless-ngx URL (required)
   - API Token (required, password field)
   - Verify SSL (optional checkbox, default: true)
   - Batch Size (optional number, default: 2)

### Understanding the Architecture

**Important:** Paperless-ngx is a **data source**, not a direct retrieval option.

```
┌─────────────────────┐
│  Paperless-ngx      │  ← Data Source (imports documents)
│  (External System)  │
└──────────┬──────────┘
           │ sync
           ↓
┌─────────────────────┐
│  Knowledge Base     │  ← Document storage/indexing
│  (RAGFlow)          │
└──────────┬──────────┘
           │ retrieve
           ↓
┌─────────────────────┐
│  Agent/Chat         │  ← Uses knowledge base for RAG
│  (Retrieval Node)   │
└─────────────────────┘
```

**Workflow:**
1. **Add Data Source**: Settings → Data Sources → Paperless-ngx
2. **Configure Connection**: Enter Paperless-ngx URL and API token
3. **Create Knowledge Base**: Create or select a knowledge base
4. **Link Data Source**: Connect Paperless-ngx to the knowledge base
5. **Sync Documents**: Documents are imported and indexed
6. **Use in Agents**: Select the knowledge base (not Paperless-ngx directly) in agent retrieval nodes

### Paperless-ngx Configuration

To get your API token from Paperless-ngx:

1. Log into your Paperless-ngx instance
2. Go to **Settings → API Tokens**
3. Create a new token
4. Copy the token value

In RAGFlow configuration:
```json
{
  "name": "My Paperless Documents",
  "source": "paperless_ngx",
  "config": {
    "base_url": "https://paperless.example.com",
    "verify_ssl": true,
    "batch_size": 10,
    "credentials": {
      "api_token": "your-token-here"
    }
  }
}
```

### Troubleshooting

**Problem:** Paperless-ngx still not showing after rebuild

**Solutions:**
1. Clear browser cache: `Ctrl+Shift+Delete`
2. Hard refresh: `Ctrl+F5`
3. Check image version: `docker inspect ragflow-cpu | grep Image`
4. Verify build completed: `docker images | grep ragflow`
5. Check logs: `docker compose logs ragflow-cpu`

**Problem:** Build fails

**Solutions:**
1. Free up disk space (need ~5GB)
2. Increase Docker memory (8GB+ recommended)
3. Check Docker daemon is running
4. Retry with `--no-cache`: `docker build --no-cache ...`

**Problem:** Paperless-ngx shows but won't connect

**Solutions:**
1. Verify Paperless-ngx is accessible from Docker network
2. Check API token is valid
3. Try with `verify_ssl: false` if using self-signed cert
4. Check firewall/network settings
5. Review connector logs in RAGFlow

### Files in This PR

```
Backend (9 files):
  common/data_source/paperless_ngx_connector.py
  common/data_source/config.py
  common/constants.py
  rag/svr/sync_data_source.py
  common/data_source/__init__.py
  test/unit/test_paperless_ngx_connector.py
  docs/paperless_ngx_connector.md
  docs/examples/paperless_ngx_connector_config.md
  PAPERLESS_NGX_INTEGRATION.md

Frontend (5 files):
  web/src/pages/user-setting/data-source/constant/index.tsx
  web/src/locales/en.ts
  web/src/assets/svg/data-source/paperless-ngx.svg
  docs/paperless_ngx_ui_integration.md
  web/validate-paperless-ui.cjs

Documentation (3 files):
  BUILD_WITH_PAPERLESS.md
  PAPERLESS_NGX_UI_FIX.md
  IMPLEMENTATION_COMPLETE.md
```

### Next Steps

1. ✅ **Code Complete** - All changes committed
2. ⚠️ **Your Action Required** - Rebuild Docker image locally
3. ⏳ **Future** - Wait for official release with changes

### Questions?

- **Q:** Why can't I use Paperless-ngx in agent flows?
  **A:** Data sources populate knowledge bases. Agents retrieve from knowledge bases, not data sources directly.

- **Q:** Do I need to rebuild every time?
  **A:** Only when code changes. Once official image is released, just pull updates.

- **Q:** Can I use development mode?
  **A:** Yes! Run `cd web && npm run dev` for frontend development with hot-reload.

- **Q:** Will my data be preserved?
  **A:** Yes, data is in Docker volumes. `docker compose down` preserves volumes. Use `-v` flag only if you want to reset.

---

**Status:** Ready for use after Docker rebuild ✅
