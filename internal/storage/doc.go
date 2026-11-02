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

// Package storage abstracts object storage behind the Storage interface.
//
// Layout: s3base.go holds the shared implementation for aws-sdk-go-v2
// backends (s3.go = AWS S3, oss.go = Aliyun OSS); storage_common.go holds
// the backend-agnostic retry/move helpers used by every backend including
// minio.go (minio-go SDK), gcs.go (Google SDK) and memory.go (test double).
//
// # Deliberately non-uniform path folding
//
// resolveBucketAndPath intentionally differs per backend. The pre-migration
// Python connectors (rag/utils/minio_conn.py and rag/utils/s3_conn.py, both
// removed in #20289) folded object keys differently — minio only inserted
// the logical bucket as a path segment in single-bucket mode, while s3
// inserted it whenever a prefix was configured — and the Go ports preserved
// each vendor's semantics so that keys written before the migration keep
// resolving after it. The table below is the contract; do not "unify" it.
//
//	                      | minio              | s3                  | oss
//	----------------------+--------------------+---------------------+---------------------
//	bucket=="" prefix=="" | fnm                | fnm                 | fnm
//	bucket=="" prefix!="" | prefix/fnm         | prefix/bucket/fnm   | prefix/fnm
//	bucket!="" prefix=="" | bucket/fnm         | bucket/fnm          | fnm
//	bucket!="" prefix!="" | prefix/bucket/fnm  | prefix/bucket/fnm   | prefix/fnm
//
// GCS and the in-memory backend perform no folding at all.
//
// Consequences to be aware of:
//
//   - Switching STORAGE_ENGINE does not migrate existing object keys; the
//     same (bucket, fnm) argument resolves to different keys on different
//     backends. Treat the engine as fixed per deployment.
//   - In shared-bucket mode OSS cannot scope a removal to a logical bucket,
//     so RemoveBucket/RemoveEmptyBucket refuse rather than guess.
//
// # Deliberately non-uniform health checks
//
// MinioStorage.Health is a lightweight liveness probe (client HealthCheck +
// IsOnline, no writes). The s3Base health check uploads a probe object to
// <bucket or "health-check-bucket">/<prefix>/txtxtxtxt1, matching the
// pre-migration Python S3 connector. The difference is intentional: a
// write-based probe also verifies write permissions, but it requires them,
// so read-only credentials make MinIO-style and S3-style deployments report
// health differently. Callers must not treat a false from one engine as
// comparable to a false from the other.
package storage
