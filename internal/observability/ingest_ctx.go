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

package observability

import "context"

// Ingest identity is execution/routing metadata that is not derivable from the
// task, document, or pipeline-run entities: which ingestor produced the event,
// which NATS subject it traveled on, which worker claimed it, and the
// user/tenant names. These values vary per execution in the EE build (multiple
// ingestors, per-task worker assignment), so they are carried on the
// context.Context rather than passed as positional parameters. The OSS build
// shares the exact same accessors; an absent key simply yields an empty string,
// which serializes as an omitted (omitempty) or zero field in common.IngestionLog.
type ingestKey int

const (
	keyIngestorName ingestKey = iota
	keyNatsQueue
	keyWorkerName
	keyUserName
	keyTenantName
)

// IngestIdentity bundles the full ingest identity. It is filled once at the
// ingestor worker entry point and read back by BuildIngestLog.
type IngestIdentity struct {
	IngestorName string
	NatsQueue    string
	WorkerName   string
	UserName     string
	TenantName   string
}

func withString(ctx context.Context, key ingestKey, value string) context.Context {
	if value == "" {
		return ctx
	}
	return context.WithValue(ctx, key, value)
}

func stringFrom(ctx context.Context, key ingestKey) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(key).(string); ok {
		return v
	}
	return ""
}

// WithIngestorName returns a context carrying the ingestor name. An empty value
// is a no-op so a higher-priority injector is not clobbered.
func WithIngestorName(ctx context.Context, name string) context.Context {
	return withString(ctx, keyIngestorName, name)
}

// WithNatsQueue returns a context carrying the NATS subject the task traveled on.
func WithNatsQueue(ctx context.Context, subject string) context.Context {
	return withString(ctx, keyNatsQueue, subject)
}

// WithWorkerName returns a context carrying the worker that claimed the task. It
// maps onto common.IngestionLog.Worker.
func WithWorkerName(ctx context.Context, worker string) context.Context {
	return withString(ctx, keyWorkerName, worker)
}

// WithUserName returns a context carrying the owning user's display name.
func WithUserName(ctx context.Context, name string) context.Context {
	return withString(ctx, keyUserName, name)
}

// WithTenantName returns a context carrying the owning tenant's display name.
func WithTenantName(ctx context.Context, name string) context.Context {
	return withString(ctx, keyTenantName, name)
}

// IngestorName reads the ingestor name from ctx, or "" if absent.
func IngestorName(ctx context.Context) string { return stringFrom(ctx, keyIngestorName) }

// NatsQueue reads the NATS subject from ctx, or "" if absent.
func NatsQueue(ctx context.Context) string { return stringFrom(ctx, keyNatsQueue) }

// WorkerName reads the worker name from ctx, or "" if absent.
func WorkerName(ctx context.Context) string { return stringFrom(ctx, keyWorkerName) }

// UserName reads the user display name from ctx, or "" if absent.
func UserName(ctx context.Context) string { return stringFrom(ctx, keyUserName) }

// TenantName reads the tenant display name from ctx, or "" if absent.
func TenantName(ctx context.Context) string { return stringFrom(ctx, keyTenantName) }

// WithIngestIdentity injects every non-empty field of id into ctx in one call.
// Empty fields are skipped so a value already present on ctx (from a more
// specific injector) is preserved.
func WithIngestIdentity(ctx context.Context, id IngestIdentity) context.Context {
	ctx = WithIngestorName(ctx, id.IngestorName)
	ctx = WithNatsQueue(ctx, id.NatsQueue)
	ctx = WithWorkerName(ctx, id.WorkerName)
	ctx = WithUserName(ctx, id.UserName)
	ctx = WithTenantName(ctx, id.TenantName)
	return ctx
}
