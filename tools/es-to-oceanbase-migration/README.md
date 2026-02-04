# RAGFlow ES to OceanBase Migration Tool

A CLI tool for migrating RAGFlow data from Elasticsearch to OceanBase. This tool is specifically designed for RAGFlow's data structure and handles schema conversion, vector data mapping, batch import, and resume capability.

## Features

- **Batch Processing**: Configurable batch size for optimal performance.
- **Resume Capability**: Save and resume migration progress.
- **Data Consistency Validation**: Compare document counts and sample data after migration.

## RAGFlow Data Model

Understanding how RAGFlow stores data in Elasticsearch:

| RAGFlow Concept | ES/OB Concept | Description |
|-----------------|---------------|-------------|
| **Tenant** | Index/Table (`ragflow_{tenant_id}`) | One tenant = one ES index = one OB table |
| **Dataset (Knowledge Base)** | `kb_id` field | Multiple datasets share the same index, distinguished by `kb_id` |
| **Document** | `doc_id` field | A single uploaded file (PDF, DOCX, etc.) |
| **Chunk** | ES Document / OB Row | The actual storage unit, one document is split into multiple chunks |

**Example Structure:**

```
Tenant: user_abc123
└── ES Index: ragflow_abc123
    │
    ├── Dataset: kb_id="product_manual"
    │   ├── Document: doc_id="manual.pdf"
    │   │   ├── Chunk (_id: "chunk_001", kb_id: "product_manual", doc_id: "manual.pdf")
    │   │   └── Chunk (_id: "chunk_002", kb_id: "product_manual", doc_id: "manual.pdf")
    │   └── Document: doc_id="guide.docx"
    │       └── Chunk (_id: "chunk_003", kb_id: "product_manual", doc_id: "guide.docx")
    │
    └── Dataset: kb_id="tech_docs"
        └── Document: doc_id="api.md"
            ├── Chunk (_id: "chunk_004", kb_id: "tech_docs", doc_id: "api.md")
            └── Chunk (_id: "chunk_005", kb_id: "tech_docs", doc_id: "api.md")
```

**Key Points:**
- All datasets (knowledge bases) of a tenant are stored in a **single index**
- The migration tool migrates **entire indices**, preserving all datasets within

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
curl -u elastic:infini_rag_flow -X GET "http://localhost:1200/_cluster/health?pretty"
```

### Step 2: Create Test Data in RAGFlow

1. Open RAGFlow Web UI: http://localhost
2. Create a new Knowledge Base
3. Upload some test documents (PDF, TXT, DOCX, etc.)
4. Wait for the documents to be parsed and indexed
5. Test the knowledge base with some queries to ensure it works

### Step 3: Verify ES Data (Optional)

Before migration, verify the data exists in Elasticsearch. This step is important to ensure you have a baseline for comparison after migration.

```bash
# Navigate to migration tool directory (from ragflow root)
cd /path/to/tools/es-to-oceanbase-migration

# Activate the virtual environment if not already done
source .venv/bin/activate

# List all RAGFlow indices to find your index name (pattern: ragflow_{tenant_id})
es-ob-migrate list-indices

# List all knowledge bases in a specific index
# Replace ragflow_{tenant_id} with your actual index from the output above
es-ob-migrate list-kb --index ragflow_{tenant_id}

# View sample documents
es-ob-migrate sample --index ragflow_{tenant_id} --size 5
```

### Step 4: Start OceanBase for Migration

Start RAGFlow's OceanBase service as the migration target:

```bash
# Navigate to RAGFlow docker directory
cd /path/to/ragflow/docker

# Start only OceanBase service from RAGFlow docker compose
docker compose --profile oceanbase up -d

# Wait for OceanBase to be ready
docker compose logs -f oceanbase
```

### Step 5: Run Migration

Execute the migration from Elasticsearch to OceanBase:

```bash
cd /path/to/tools/es-to-oceanbase-migration

# Migrate all ragflow_* indices
es-ob-migrate migrate
```

Expected output:
```
es-ob-migrate migrate
RAGFlow ES to OceanBase Migration

Discovering RAGFlow indices...
Found 1 RAGFlow indices:
  - ragflow_20a72220ff7011f099e112baf51a40f8 (12 documents)


============================================================
Migrating: ragflow_20a72220ff7011f099e112baf51a40f8 -> ragflow_doc.ragflow_20a72220ff7011f099e112baf51a40f8
============================================================
Step 1: Checking connections...
  ES cluster status: green
  OceanBase connection: OK (version: 4.4.1.0)

Step 2: Analyzing ES index...
  Detected fields: 18
  Vector field: q_1024_vec (dim=1024)
  Total documents: 12

Step 3: Creating OceanBase table...
  Created table 'ragflow_20a72220ff7011f099e112baf51a40f8' with 18 columns

Step 4: Migrating data...
  Migrating... ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 100% 0:00:00

Step 5: Verifying migration...

============================================================
Migration Verification Report
============================================================
ES Index:  ragflow_20a72220ff7011f099e112baf51a40f8
OB Table:  ragflow_20a72220ff7011f099e112baf51a40f8

Document Counts:
  Elasticsearch: 12
  OceanBase:     12
  Difference:    0
  Match:         Yes

Sample Verification:
  Sample Size:   12
  Verified:      12
  Matched:       12
  Match Rate:    100.00%

============================================================
Result: PASSED
Verification PASSED. ES: 12, OB: 12. Sample match rate: 100.00%
============================================================


Migration completed successfully!
  Total: 12 documents
  Migrated: 12 documents
  Duration: 2.8 seconds

All migrations completed successfully!
```

### Step 6: Start RAGFlow with OceanBase Backend

```bash
# Edit .env file, change:
#   DOC_ENGINE=elasticsearch  ->  DOC_ENGINE=oceanbase
#
# The OceanBase connection settings are already configured by default in .env
# Start RAGFlow with OceanBase profile (OceanBase is already running)
docker compose --profile oceanbase --profile cpu up -d

# Wait for services to start
docker compose ps

# Check logs for any errors
docker compose logs -f ragflow-cpu
```

### Step 7: Data Migration Verification (Optional)

Run the verification command to compare ES and OceanBase data:

```bash
# Verify all indices (auto-discover)
es-ob-migrate verify

# Or verify a specific index
es-ob-migrate verify --index ragflow_xxx --table ragflow_xxx
```

Expected output:
```
Discovering RAGFlow indices...
Found 1 indices to verify

============================================================
Verifying: ragflow_be3a54c701a111f19c7dbe0f2700305e <-> ragflow_be3a54c701a111f19c7dbe0f2700305e
============================================================

============================================================
Migration Verification Report
============================================================
ES Index:  ragflow_be3a54c701a111f19c7dbe0f2700305e
OB Table:  ragflow_be3a54c701a111f19c7dbe0f2700305e

Document Counts:
  Elasticsearch: 38
  OceanBase:     38
  Difference:    0
  Match:         Yes

Sample Verification:
  Sample Size:   38
  Verified:      38
  Matched:       38
  Match Rate:    100.00%

============================================================
Result: PASSED
Verification PASSED. ES: 38, OB: 38. Sample match rate: 100.00%
============================================================
```

### Step 8: Verify RAGFlow Works with OceanBase

1. Open RAGFlow Web UI: http://localhost
2. Navigate to your Knowledge Base
3. Try the same queries you tested before migration

## CLI Reference

### Connection Options (All Commands)

All commands share the following connection options with RAGFlow-compatible defaults:

| Option | Default | Description |
|--------|---------|-------------|
| `--es-host` | localhost | Elasticsearch host |
| `--es-port` | 1200 | Elasticsearch port (RAGFlow default) |
| `--es-user` | elastic | ES username |
| `--es-password` | infini_rag_flow | ES password |
| `--ob-host` | localhost | OceanBase host |
| `--ob-port` | 2881 | OceanBase port |
| `--ob-user` | root@ragflow | OceanBase user (format: user@tenant) |
| `--ob-password` | infini_rag_flow | OceanBase password |
| `--ob-database` | test | OceanBase database name |

> **Note**: These defaults match RAGFlow's standard configuration. When connecting to a standard RAGFlow deployment, you can omit these options.

### `es-ob-migrate migrate`

Run data migration from Elasticsearch to OceanBase.
| `-i, --index` | None | Source ES index (omit to migrate all ragflow_* indices) |
| `-t, --table` | None | Target OB table (omit to use same name as index) |
| `--batch-size` | 1000 | Documents per batch |
| `--resume` | False | Resume from previous progress |
| `--verify/--no-verify` | True | Verify after migration |

**Example:**

```bash
# Migrate all ragflow_* indices
es-ob-migrate migrate

# Migrate a specific index
es-ob-migrate migrate --index ragflow_abc123 --table ragflow_abc123

# Resume interrupted migration
es-ob-migrate migrate --index ragflow_abc123 --table ragflow_abc123 --resume
```

**Resume Feature:**

Migration progress is automatically saved to `.migration_progress/` directory. If migration is interrupted (network error, timeout, etc.), use `--resume` to continue from where it stopped:

- Progress file: `.migration_progress/{index_name}_progress.json`
- Contains: total count, migrated count, last document ID, timestamp
- On resume: skips already migrated documents, continues from last position

**Output:**

```
RAGFlow ES to OceanBase Migration
Source: localhost:1200/ragflow_abc123
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
es-ob-migrate list-indices
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

### `es-ob-migrate verify`

Verify migration data consistency between ES and OceanBase.

**Example:**

```bash
# Verify all indices (auto-discover)
es-ob-migrate verify

# Verify a specific index
es-ob-migrate verify --index ragflow_abc123 --table ragflow_abc123
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
es-ob-migrate list-kb --index ragflow_abc123
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
es-ob-migrate sample --index ragflow_abc123 --size 2
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

