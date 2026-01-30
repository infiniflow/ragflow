# ✅ Paperless-ngx Integration - Successfully Deployed!

**Status: Integration is now visible in the UI** 🎉

## What You Should See

In your RAGFlow UI at **Settings → Data Sources**, you should now see:

1. **Confluence**
2. **S3**
3. **Paperless-ngx** ← ✅ **NEW!**
4. **Notion**
5. **Discord**
6. ... (other sources)

The Paperless-ngx card should display:
- 📄 Green document icon with "ngx" badge
- **Title:** Paperless-ngx
- **Description:** "Connect to Paperless-ngx to sync and index your document management system."

## Next Steps: Configure Your First Connection

### Step 1: Get Your Paperless-ngx API Token

1. Log into your Paperless-ngx instance
2. Navigate to **Settings → API Tokens**
3. Click **Create new token**
4. Give it a name (e.g., "RAGFlow Integration")
5. **Copy the token** (you won't see it again!)

### Step 2: Add Paperless-ngx in RAGFlow

1. In RAGFlow, go to **Settings → Data Sources**
2. Click on the **Paperless-ngx** card
3. Fill in the configuration form:

   **Required Fields:**
   - **Name:** A friendly name for this connection (e.g., "My Paperless Documents")
   - **Paperless-ngx URL:** Your Paperless-ngx server URL
     - Example: `https://paperless.example.com`
     - Or for local: `http://localhost:8000`
   - **API Token:** Paste the token you copied from Paperless-ngx

   **Optional Fields:**
   - **Verify SSL:** ✓ (checked by default)
     - Uncheck only if using self-signed certificates in development
   - **Batch Size:** 10 (default: 2)
     - Number of documents to process per batch
     - Increase for better performance, decrease if having issues

4. Click **Confirm** to save

### Step 3: Link to a Knowledge Base

1. Go to **Knowledge Bases** in RAGFlow
2. Create a new knowledge base OR select an existing one
3. In the knowledge base settings, find **Data Sources**
4. Click **Add Data Source**
5. Select your Paperless-ngx connection
6. Configure sync settings:
   - **Sync Frequency:** How often to check for new/updated documents (e.g., every 30 minutes)
   - **Prune Frequency:** How often to remove deleted documents (e.g., every 24 hours)
7. Click **Save**

### Step 4: Initial Sync

The first sync will import all documents from your Paperless-ngx instance:

1. Go to the knowledge base
2. Check the **Documents** tab
3. You should see documents being imported
4. Wait for the sync to complete

**Note:** Large document collections may take time. Monitor progress in the RAGFlow logs.

### Step 5: Use in Chat/Agents

Once documents are synced, you can use them in:

**Chat:**
1. Create a new conversation
2. Select the knowledge base containing Paperless-ngx documents
3. Ask questions about your documents

**Agents/Workflows:**
1. Create or edit an agent workflow
2. Add a **Retrieval** node
3. In the retrieval configuration:
   - **Retrieval From:** Select "Dataset"
   - **Knowledge Base:** Select your knowledge base with Paperless-ngx documents
4. The agent will now retrieve information from your Paperless documents

## Configuration Examples

### Basic Configuration
```json
{
  "name": "My Paperless Docs",
  "source": "paperless_ngx",
  "config": {
    "base_url": "https://paperless.myserver.com",
    "verify_ssl": true,
    "batch_size": 2,
    "credentials": {
      "api_token": "your-api-token-here"
    }
  }
}
```

### Local Development Setup
```json
{
  "name": "Local Paperless",
  "source": "paperless_ngx",
  "config": {
    "base_url": "http://localhost:8000",
    "verify_ssl": false,
    "batch_size": 5,
    "credentials": {
      "api_token": "your-local-dev-token"
    }
  }
}
```

### Production Setup with High Volume
```json
{
  "name": "Production Paperless Archive",
  "source": "paperless_ngx",
  "config": {
    "base_url": "https://paperless.company.com",
    "verify_ssl": true,
    "batch_size": 20,
    "credentials": {
      "api_token": "your-production-token"
    }
  },
  "refresh_freq": 15,
  "prune_freq": 360
}
```

## What Gets Synced

From each Paperless-ngx document, RAGFlow imports:

✓ **Document Content** (PDF, images, text)
✓ **Title**
✓ **Original Filename**
✓ **Correspondent** (sender/author)
✓ **Document Type**
✓ **Tags** (as IDs)
✓ **Creation Date**
✓ **Modification Date**
✓ **OCR Text** (first 500 characters as preview)

## Sync Behavior

### Full Sync (First Time)
- Imports all documents from Paperless-ngx
- Processes in batches (default: 2 documents at a time)
- May take time for large collections

### Incremental Sync (Subsequent)
- Only syncs new or modified documents
- Uses `modified__gte` filter for efficiency
- Much faster than full sync

### Prune (Document Cleanup)
- Removes documents deleted from Paperless-ngx
- Runs based on `prune_freq` setting
- Keeps RAGFlow in sync with Paperless-ngx

## Monitoring Sync Status

### Via UI
1. Go to knowledge base
2. Check **Documents** tab for newly imported files
3. Look at **Last Sync** timestamp

### Via Logs
```bash
# Check RAGFlow logs for sync activity
docker compose logs -f ragflow-cpu | grep paperless
```

Look for messages like:
- `Syncing from Paperless-ngx...`
- `Retrieved X documents`
- `Imported Y new documents`
- `Sync completed successfully`

## Troubleshooting

### Connection Issues

**Problem:** "Failed to connect to Paperless-ngx"

**Solutions:**
1. Verify Paperless-ngx URL is correct and accessible
2. Check if Paperless-ngx is running: `curl https://your-paperless-url/api/`
3. Verify API token is valid (try in Postman/curl)
4. Check firewall rules allow RAGFlow → Paperless-ngx connection
5. If using Docker, ensure both are on same network or accessible

**Test Connection:**
```bash
curl -H "Authorization: Token your-api-token" https://paperless.example.com/api/documents/
```

### Authentication Issues

**Problem:** "401 Unauthorized" or "Invalid token"

**Solutions:**
1. Generate a new API token in Paperless-ngx
2. Copy the FULL token (no spaces, no line breaks)
3. Paste exactly in RAGFlow configuration
4. Save and retry

### SSL Certificate Issues

**Problem:** "SSL certificate verify failed"

**Solutions:**
1. If using self-signed certificate, uncheck "Verify SSL"
2. Or add certificate to Docker container's trust store
3. For production, use a valid SSL certificate

### Sync Performance

**Problem:** Sync is very slow

**Solutions:**
1. Increase `batch_size` (try 10, 20, or higher)
2. Reduce `refresh_freq` if checking too often
3. Check network latency between RAGFlow and Paperless-ngx
4. Monitor Paperless-ngx performance
5. Check RAGFlow resources (CPU, memory)

### Missing Documents

**Problem:** Some documents not syncing

**Solutions:**
1. Check document size (default max: 20MB)
2. Verify document type is supported
3. Check RAGFlow logs for errors
4. Manually trigger sync
5. Check Paperless-ngx permissions for API token

## Advanced Features

### Filtering Documents

Currently, all accessible documents are synced. Future versions may support:
- Tag-based filtering
- Date range filtering
- Document type filtering
- Custom metadata filters

### Metadata Enhancement

The connector extracts rich metadata:
- Use document titles for better search results
- Tags help categorize and filter
- Correspondent info provides context
- OCR text preview aids discovery

### Integration with RAG

Documents from Paperless-ngx work seamlessly with RAG:
- Automatic chunking for better retrieval
- Embedding generation for semantic search
- Citation tracking back to original documents
- Multi-document synthesis

## Best Practices

1. **Start Small:** Test with a subset of documents first
2. **Monitor Resources:** Watch CPU/memory during initial sync
3. **Adjust Batch Size:** Find optimal balance for your setup
4. **Regular Sync:** Set appropriate `refresh_freq` for your needs
5. **Security:** Use SSL in production, store tokens securely
6. **Backups:** Keep Paperless-ngx and RAGFlow backups separate
7. **Access Control:** Use RAGFlow's permission system

## Support Resources

- **Connector Documentation:** `docs/paperless_ngx_connector.md`
- **UI Integration Guide:** `docs/paperless_ngx_ui_integration.md`
- **Configuration Examples:** `docs/examples/paperless_ngx_connector_config.md`
- **Build Instructions:** `BUILD_WITH_PAPERLESS.md`

## Feedback & Issues

If you encounter any problems:
1. Check the troubleshooting section above
2. Review RAGFlow logs for errors
3. Test Paperless-ngx API directly
4. Report issues with:
   - RAGFlow version
   - Paperless-ngx version
   - Error messages from logs
   - Steps to reproduce

---

**Congratulations! Your Paperless-ngx integration is now live!** 🎉

Start by connecting your Paperless-ngx instance and importing your first documents.
