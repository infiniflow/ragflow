# RAGFlow ES to OceanBase Migration Tool

A CLI tool for migrating RAGFlow data from Elasticsearch to OceanBase. This tool is specifically designed for RAGFlow's data structure and handles schema conversion, vector data mapping, batch import, and resume capability.

## Features

- **RAGFlow-Specific**: Designed for RAGFlow's fixed data schema
- **ES 8+ Support**: Uses `search_after` API for efficient data scrolling
- **Vector Support**: Auto-detects vector field dimensions from ES mapping
- **Batch Processing**: Configurable batch size for optimal performance
- **Resume Capability**: Save and resume migration progress
- **Data Consistency Validation**: Compare document counts and sample data
- **Migration Report Generation**: Generate detailed migration reports

## Quick Start

This section provides a complete guide to verify the migration works correctly with a real RAGFlow deployment.

### Prerequisites

- RAGFlow source code cloned
- Docker and Docker Compose installed
- This migration tool installed (`uv pip install -e .`)

### Step 1: Start RAGFlow with Elasticsearch Backend

First, start RAGFlow using Elasticsearch as the document storage backend (default configuration).

```bash
# Navigate to RAGFlow docker directory
cd /path/to/ragflow/docker

# Ensure DOC_ENGINE=elasticsearch in .env (this is the default)
# DOC_ENGINE=elasticsearch

# Start RAGFlow with Elasticsearch (--profile cpu for CPU, --profile gpu for GPU)
docker compose --profile elasticsearch --profile cpu up -d

# Wait for services to be ready (this may take a few minutes)
docker compose ps

# Check ES is running
curl -X GET "http://localhost:9200/_cluster/health?pretty"
```

### Step 2: Create Test Data in RAGFlow

1. Open RAGFlow Web UI: http://localhost:9380
2. Create a new Knowledge Base
3. Upload some test documents (PDF, TXT, DOCX, etc.)
4. Wait for the documents to be parsed and indexed
5. Test the knowledge base with some queries to ensure it works

### Step 3: Verify ES Data (Optional)

Before migration, verify the data exists in Elasticsearch. This step is important to ensure you have a baseline for comparison after migration.

```bash
# Navigate to migration tool directory (from ragflow root)
cd tools/es-to-oceanbase-migration

# Activate the virtual environment if not already done
source .venv/bin/activate

# Check connection and list indices
es-ob-migrate status --es-host localhost --es-port 9200

# First, find your actual index name (pattern: ragflow_{tenant_id})
curl -X GET "http://localhost:9200/_cat/indices/ragflow_*?v"

# List all knowledge bases in the index
# Replace ragflow_{tenant_id} with your actual index from the curl output above
es-ob-migrate list-kb --es-host localhost --es-port 9200 --index ragflow_{tenant_id}

# View sample documents
es-ob-migrate sample --es-host localhost --es-port 9200 --index ragflow_{tenant_id} --size 5

# Check schema
es-ob-migrate schema --es-host localhost --es-port 9200 --index ragflow_{tenant_id}
```

### Step 4: Start OceanBase for Migration

Start RAGFlow's OceanBase service as the migration target:

```bash
# Navigate to ragflow docker directory (from ragflow root)
cd ../docker

# Start only OceanBase service from RAGFlow docker compose
docker compose --profile oceanbase up -d

# Wait for OceanBase to be ready
docker compose logs -f oceanbase
```

### Step 5: Run Migration

Execute the migration from Elasticsearch to OceanBase:

```bash
cd ../tools/es-to-oceanbase-migration

# Option A: Migrate ALL ragflow_* indices (Recommended)
# If --index and --table are omitted, the tool auto-discovers all ragflow_* indices
es-ob-migrate migrate \
  --es-host localhost --es-port 9200 \
  --ob-host localhost --ob-port 2881 \
  --ob-user "root@ragflow" --ob-password "infini_rag_flow" \
  --ob-database ragflow_doc \
  --batch-size 1000 \
  --verify

# Option B: Migrate a specific index
# Use the SAME name for both --index and --table
# The index name pattern is: ragflow_{tenant_id}
# Find your tenant_id from Step 3's curl output
es-ob-migrate migrate \
  --es-host localhost --es-port 9200 \
  --ob-host localhost --ob-port 2881 \
  --ob-user "root@ragflow" --ob-password "infini_rag_flow" \
  --ob-database ragflow_doc \
  --index ragflow_{tenant_id} \
  --table ragflow_{tenant_id} \
  --batch-size 1000 \
  --verify
```

Expected output:
```
RAGFlow ES to OceanBase Migration
Source: localhost:9200/ragflow_{tenant_id}
Target: localhost:2881/ragflow_doc.ragflow_{tenant_id}

Step 1: Checking connections...
  ES cluster status: green
  OceanBase connection: OK (version: 4.3.5.1)

Step 2: Analyzing ES index...
  Auto-detected vector dimension: 1024
  Known RAGFlow fields: 25
  Total documents: 1,234

Step 3: Creating OceanBase table...
  Created table 'ragflow_{tenant_id}' with RAGFlow schema

Step 4: Migrating data...
Migrating... ━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100% 1,234/1,234

Step 5: Verifying migration...
✓ Document counts match: 1,234
✓ Sample verification: 100/100 matched

Migration completed successfully!
  Total: 1,234 documents
  Migrated: 1,234 documents
  Failed: 0 documents
  Duration: 45.2 seconds
```

### Step 6: Stop RAGFlow and Switch to OceanBase Backend

```bash
# Navigate to ragflow docker directory
cd ../../docker

# Stop only Elasticsearch and RAGFlow (but keep OceanBase running)
docker compose --profile elasticsearch --profile cpu down

# Edit .env file, change:
#   DOC_ENGINE=elasticsearch  ->  DOC_ENGINE=oceanbase
#
# The OceanBase connection settings are already configured by default in .env
```

### Step 7: Start RAGFlow with OceanBase Backend

```bash
# OceanBase should still be running from Step 4
# Start RAGFlow with OceanBase profile (OceanBase is already running)
docker compose --profile oceanbase --profile cpu up -d

# Wait for services to start
docker compose ps

# Check logs for any errors
docker compose logs -f ragflow-cpu
```

### Step 8: Data Integrity Verification (Optional)

Run the verification command to compare ES and OceanBase data:

```bash
es-ob-migrate verify \
  --es-host localhost --es-port 9200 \
  --ob-host localhost --ob-port 2881 \
  --ob-user "root@ragflow" --ob-password "infini_rag_flow" \
  --ob-database ragflow_doc \
  --index ragflow_{tenant_id} \
  --table ragflow_{tenant_id} \
  --sample-size 100
```

Expected output:
```
╭─────────────────────────────────────────────────────────────╮
│                   Migration Verification Report             │
├─────────────────────────────────────────────────────────────┤
│ ES Index:  ragflow_{tenant_id}                              │
│ OB Table:  ragflow_{tenant_id}                              │
├─────────────────────────────────────────────────────────────┤
│ Document Counts                                             │
│   ES:      1,234                                            │
│   OB:      1,234                                            │
│   Match:   ✓ Yes                                            │
├─────────────────────────────────────────────────────────────┤
│ Sample Verification (100 documents)                         │
│   Matched:     100                                          │
│   Match Rate:  100.0%                                       │
├─────────────────────────────────────────────────────────────┤
│ Result: ✓ PASSED                                            │
╰─────────────────────────────────────────────────────────────╯
```

### Step 9: Verify RAGFlow Works with OceanBase

1. Open RAGFlow Web UI: http://localhost:9380
2. Navigate to your Knowledge Base
3. Try the same queries you tested before migration

## CLI Reference

### `es-ob-migrate migrate`

Run data migration from Elasticsearch to OceanBase.

| Option | Default | Description |
|--------|---------|-------------|
| `--es-host` | localhost | Elasticsearch host |
| `--es-port` | 9200 | Elasticsearch port |
| `--es-user` | None | ES username (if auth required) |
| `--es-password` | None | ES password |
| `--ob-host` | localhost | OceanBase host |
| `--ob-port` | 2881 | OceanBase port |
| `--ob-user` | root@test | OceanBase user (format: user@tenant) |
| `--ob-password` | "" | OceanBase password |
| `--ob-database` | test | OceanBase database name |
| `-i, --index` | None | Source ES index (omit to migrate all ragflow_* indices) |
| `-t, --table` | None | Target OB table (omit to use same name as index) |
| `--batch-size` | 1000 | Documents per batch |
| `--resume` | False | Resume from previous progress |
| `--verify/--no-verify` | True | Verify after migration |

**Example:**

```bash
# Migrate all ragflow_* indices
es-ob-migrate migrate \
  --es-host localhost --es-port 9200 \
  --ob-host localhost --ob-port 2881 \
  --ob-user "root@ragflow" --ob-password "infini_rag_flow" \
  --ob-database ragflow_doc

# Migrate a specific index
es-ob-migrate migrate \
  --es-host localhost --es-port 9200 \
  --ob-host localhost --ob-port 2881 \
  --ob-user "root@ragflow" --ob-password "infini_rag_flow" \
  --ob-database ragflow_doc \
  --index ragflow_abc123 --table ragflow_abc123

# Resume interrupted migration
es-ob-migrate migrate \
  --es-host localhost --es-port 9200 \
  --ob-host localhost --ob-port 2881 \
  --ob-user "root@ragflow" --ob-password "infini_rag_flow" \
  --ob-database ragflow_doc \
  --index ragflow_abc123 --table ragflow_abc123 \
  --resume
```

**Resume Feature:**

Migration progress is automatically saved to `.migration_progress/` directory. If migration is interrupted (network error, timeout, etc.), use `--resume` to continue from where it stopped:

- Progress file: `.migration_progress/{index_name}_progress.json`
- Contains: total count, migrated count, last document ID, timestamp
- On resume: skips already migrated documents, continues from last position

**Output:**

```
RAGFlow ES to OceanBase Migration
Source: localhost:9200/ragflow_abc123
Target: localhost:2881/ragflow_doc.ragflow_abc123

Step 1: Checking connections...
  ES cluster status: green
  OceanBase connection: OK

Step 2: Analyzing ES index...
  Auto-detected vector dimension: 1024
  Total documents: 1,234

Step 3: Creating OceanBase table...
  Created table 'ragflow_abc123' with RAGFlow schema

Step 4: Migrating data...
Migrating... ━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100% 1,234/1,234

Migration completed successfully!
  Total: 1,234 documents
  Duration: 45.2 seconds
```

---

### `es-ob-migrate list-indices`

List all RAGFlow indices (`ragflow_*`) in Elasticsearch.

**Example:**

```bash
es-ob-migrate list-indices --es-host localhost --es-port 9200
```

**Output:**

```
RAGFlow Indices in Elasticsearch:

  Index Name                          Documents    Type
  ragflow_abc123def456789             1234         Document Chunks
  ragflow_doc_meta_abc123def456789    56           Document Metadata

Total: 2 ragflow_* indices found
```

---

### `es-ob-migrate schema`

Preview schema analysis from ES mapping.

**Example:**

```bash
es-ob-migrate schema --es-host localhost --es-port 9200 --index ragflow_abc123
```

**Output:**

```
RAGFlow Schema Analysis for index: ragflow_abc123

Vector Fields:
  q_1024_vec: dense_vector (dim=1024)

Known RAGFlow Fields (25):
  id, kb_id, doc_id, docnm_kwd, content_with_weight, content_ltks,
  available_int, important_kwd, question_kwd, tag_kwd, page_num_int...

Unknown Fields (stored in 'extra' column):
  custom_field_1, custom_field_2
```

---

### `es-ob-migrate verify`

Verify migration data consistency between ES and OceanBase.

**Example:**

```bash
es-ob-migrate verify \
  --es-host localhost --es-port 9200 \
  --ob-host localhost --ob-port 2881 \
  --ob-user "root@ragflow" --ob-password "infini_rag_flow" \
  --ob-database ragflow_doc \
  --index ragflow_abc123 --table ragflow_abc123 \
  --sample-size 100
```

**Output:**

```
╭─────────────────────────────────────────────────────────────╮
│                   Migration Verification Report             │
├─────────────────────────────────────────────────────────────┤
│ ES Index:  ragflow_abc123                                   │
│ OB Table:  ragflow_abc123                                   │
├─────────────────────────────────────────────────────────────┤
│ Document Counts                                             │
│   ES:      1,234                                            │
│   OB:      1,234                                            │
│   Match:   ✓ Yes                                            │
├─────────────────────────────────────────────────────────────┤
│ Sample Verification (100 documents)                         │
│   Matched:     100                                          │
│   Match Rate:  100.0%                                       │
├─────────────────────────────────────────────────────────────┤
│ Result: ✓ PASSED                                            │
╰─────────────────────────────────────────────────────────────╯
```

---

### `es-ob-migrate list-kb`

List all knowledge bases in an ES index.

**Example:**

```bash
es-ob-migrate list-kb --es-host localhost --es-port 9200 --index ragflow_abc123
```

**Output:**

```
Knowledge Bases in index 'ragflow_abc123':

  KB ID                                 Documents
  kb_001_finance_docs                   456
  kb_002_technical_manual               321
  kb_003_product_faq                    457

Total: 3 knowledge bases, 1234 documents
```

---

### `es-ob-migrate sample`

Show sample documents from ES index.

**Example:**

```bash
es-ob-migrate sample --es-host localhost --es-port 9200 --index ragflow_abc123 --size 2
```

**Output:**

```
Sample Documents from 'ragflow_abc123':

Document 1:
  id: chunk_001_abc123
  kb_id: kb_001_finance_docs
  doc_id: doc_001
  docnm_kwd: quarterly_report.pdf
  content_with_weight: The company reported Q3 revenue of $1.2B...
  available_int: 1

Document 2:
  id: chunk_002_def456
  kb_id: kb_001_finance_docs
  doc_id: doc_001
  docnm_kwd: quarterly_report.pdf
  content_with_weight: Operating expenses decreased by 5%...
  available_int: 1
```

---

### `es-ob-migrate status`

Check connection status to ES and OceanBase.

**Example:**

```bash
es-ob-migrate status \
  --es-host localhost --es-port 9200 \
  --ob-host localhost --ob-port 2881 \
  --ob-user "root@ragflow" --ob-password "infini_rag_flow"
```

**Output:**

```
Connection Status:

Elasticsearch:
  Host: localhost:9200
  Status: ✓ Connected
  Cluster: ragflow-cluster
  Version: 8.11.0
  Indices: 5

OceanBase:
  Host: localhost:2881
  Status: ✓ Connected
  Version: 4.3.5.1
```
