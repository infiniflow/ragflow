//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

// Package serenedb implements the doc-store DocEngine backed by SereneDB, a
// PostgreSQL-wire engine whose single inverted index carries both a scored
// text column (@@, BM25) and an IVF vector column (<#>, inner product), so
// hybrid search is one SQL statement. It connects with database/sql + lib/pq
// and emits SQL directly; the query shapes mirror the Python
// SereneDBConnection (rag/utils/serenedb_conn.py) that reached Elasticsearch
// retrieval parity.
//
// Table layout follows the Elasticsearch/OceanBase model: one chunk table per
// tenant (the index name) with kb_id as a filter column, so BM25 statistics are
// computed over the whole tenant corpus. Metadata is one table per tenant
// (ragflow_doc_meta_{tenantID}).
//
// Minimum engine version: SereneDB 26.07.4. The vector-branch and fusion
// queries use the natural forms that rely on the 26.07.4 fixes for the
// vector-op predicate in an ANN scan's WHERE and the multi-reference index
// CTE; on earlier builds both silently returned empty.
package serenedb
