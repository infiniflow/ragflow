---
sidebar_position: 3
title: Database Change Log
sidebar_label: Database Change Log
slug: /database_change_log
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Database Change Log

A version-by-version record of the schema and data changes the Go backend applies to a RAGFlow database, and of the version marker that decides which of them still have to run.

## How the database version is recorded

- The marker is a row in `system_settings` with the key `mysql_migration.database.version`.
- The same marker is written by the Python script [run_migrations.sh](https://github.com/infiniflow/ragflow/blob/main/tools/scripts/run_migrations.sh), so a step already performed by Python is skipped by Go, and vice versa.
- Go defines the versions as constants: `modelMigrationBaseVersion` and `modelMigrationTargetVersion` in [migration_version.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/migration_version.go), and `conversationHistoryTargetVersion` in [conversation_history_migration.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/conversation_history_migration.go).
- A step runs when the stored version is lower than the step's version. Versions are compared with Python PEP 440 semantics, where a `dev` prerelease sorts *before* the release it precedes (`v1.0.0-rc1.dev1` < `v1.0.0-rc1`). A missing or unparsable marker never skips a step.
- `checkDatabaseVersion` in [cmd/ragflow_server.go](https://github.com/infiniflow/ragflow/blob/main/cmd/ragflow_server.go) refuses to start when the code version is older than the stored database version, so a database migrated by a newer release cannot be opened by an older one.

Startup order in `InitDB` ([database.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/database.go)):

1. `RunMigrations` — manual, idempotent steps plus the two versioned tenant model steps. Runs *before* `AutoMigrate` because it has to observe the legacy schema.
2. `migrateIngestionLogRunIdentity` — ingestion log columns and indexes.
3. `AutoMigrate` — converges every ORM model.
4. `migrateConversationHistory` — runs *after* `AutoMigrate`, which creates the child tables it backfills.

## Version overview

| Database version | Scope | Gate constant | Minimum compatible RAGFlow version |
|---|---|---|---|
| `v0.26.0` | Rebuild the tenant model tables from `tenant_llm` and normalize stored model ids | `modelMigrationBaseVersion` | `_TBD_` |
| `v0.27.2` | Seed factory-declared models, merge `model_type` into an integer bitmask, populate the `tenant_*_id` columns | `modelMigrationTargetVersion` | `_TBD_` |
| `v1.0.0-rc1.dev1` | Split conversation message and reference payloads into child tables | `conversationHistoryTargetVersion` | `_TBD_` |

The two tenant model steps are cumulative: a database at `v0.26.0` still runs the `v0.27.2` step, and a database at or above `v0.27.2` runs neither.

---

## v0.26.0

**Minimum compatible RAGFlow version:** `_TBD_`

### Schema changes

| Object | Change |
|---|---|
| `system_settings` | Created if missing, so the version marker can be written before `AutoMigrate` reaches the table. |
| `tenant_model_provider` | Created if missing. Columns: `id` (PK, varchar 32), `provider_name`, `tenant_id`, base timestamps. Unique index `idx_tenant_provider_unique (tenant_id, provider_name)`. |
| `tenant_model_instance` | Created if missing. Columns: `id` (PK), `instance_name`, `provider_id`, `api_key`, `status`, `extra`, base timestamps. |
| `tenant_model` | Created if missing. Columns: `id` (PK), `model_name`, `provider_id`, `instance_id`, `model_type`, `status`, `extra`, base timestamps. |

The three tables are created only when the legacy `tenant_llm` table exists and the target table does not, mirroring the `CREATE TABLE IF NOT EXISTS` the Python stages run. Existing tables are never altered by this step: `tenant_model.model_type` deliberately keeps the text shape the Python migration left behind so the `v0.27.2` merge can still read the legacy model names.

### Data migration

| Table | Change |
|---|---|
| `tenant_model_provider` | One row per distinct `(tenant_id, llm_factory)` in `tenant_llm`; `provider_name` takes the factory name. |
| `tenant_model_instance` | One row per `(tenant_id, llm_factory)`, carrying the first `api_key` after deduplication; `instance_name` defaults to `default`. |
| `tenant_model` | Rows derived from `tenant_llm`: `model_type` is written as an integer bitmask, only groups whose merged status is `active` are kept, and `provider_id` / `instance_id` point at the rows created above. Bits: `chat=1`, `embedding=2`, `asr`/`speech2text=4`, `vision`/`image2text=8`, `rerank=16`, `tts=32`, `ocr=64`. |
| `tenant`, `knowledgebase`, `dialog`, `memory` | Stored model ids are rewritten from `<model>@<provider>` to `<model>@default@<provider>`: `tenant.llm_id / embd_id / asr_id / img2txt_id / rerank_id / tts_id / ocr_id`, `knowledgebase.embd_id`, `dialog.llm_id / rerank_id`, `memory.embd_id / llm_id`, plus the same keys nested inside `parser_config` and `prompt_config` JSON (`llm_id`, `embd_id`, `embedding_model`, `rerank_id`, `asr_id`, `img2txt_id`, `tts_id`, `ocr_id`). |

On success the step writes `v0.26.0` into the version marker.

---

## v0.27.2

**Minimum compatible RAGFlow version:** `_TBD_`

### Schema changes

| Table | Change |
|---|---|
| `tenant_model` | `model_type` is converted from the legacy text representation to the integer bitmask (see the data migration below). Once the column is an integer the conversion is a no-op. |
| `tenant` | Adds `tenant_llm_id`, `tenant_embd_id`, `tenant_asr_id`, `tenant_img2txt_id`, `tenant_rerank_id`, `tenant_tts_id`, `tenant_ocr_id` — each `varchar(32) NULL`. |
| `knowledgebase` | Adds `tenant_embd_id` — `varchar(32) NULL`. |
| `dialog` | Adds `tenant_llm_id`, `tenant_rerank_id` — `varchar(32) NULL`. |
| `memory` | Adds `tenant_embd_id`, `tenant_llm_id` — `varchar(32) NULL`. |

The `tenant_*_id` columns are added only when missing, and are skipped entirely when any of `tenant_model`, `tenant_model_provider`, `tenant_model_instance` is absent.

### Data migration

| Table | Change |
|---|---|
| `tenant_model` | **Seeding:** the models declared in `conf/llm_factories.json` are inserted for every provider/instance created by the `v0.26.0` step. When a `(provider_id, instance_id, model_name)` row already exists, the declared factory bits are OR-ed into its `model_type` instead of a second row being inserted. |
| `tenant_model` | **Type merge:** rows sharing `(provider_id, instance_id, model_name)` collapse into a single row whose `model_type` is the OR of the contributing bits. Bits that only ever appeared on `unsupported` rows are dropped, and groups whose merged status is not `active` are dropped. |
| `tenant`, `knowledgebase`, `dialog`, `memory` | **Id backfill:** each `tenant_*_id` column is filled from its legacy counterpart (`llm_id`, `embd_id`, …) by resolving the referenced `tenant_model.id` through a `(tenant_id, model_name, provider, model_type)` lookup over active models, with the `model_type` bitmask expanded back into individual types. |

On success the step writes `v0.27.2` into the version marker.

---

## v1.0.0-rc1.dev1

**Minimum compatible RAGFlow version:** `_TBD_`

This step only prepares the schema owned by `v1.0.0-rc1`; dropping the legacy payload columns is out of its scope.

### Schema changes

| Table | Change |
|---|---|
| `conversation_message` | Created by `AutoMigrate`. Composite PK `(conversation_id, position)`; columns `message_id`, `role`, `content`, `content_type`, `status`, `thumb_up`, `feedback`, `created_at`, `metadata`; composite index `message_lookup (conversation_id, message_id, role, position)`; foreign key `conversation_id → conversation.id` with `ON DELETE CASCADE`. |
| `conversation_reference` | Created by `AutoMigrate`. Composite PK `(conversation_id, position)`; columns `reference` (longtext), `message_position`; foreign key `conversation_id → conversation.id` with `ON DELETE CASCADE`. |
| `api_4_conversation_message` | Same shape as `conversation_message`, parented by `api_4_conversation`. |
| `api_4_conversation_reference` | Same shape as `conversation_reference`, parented by `api_4_conversation`. |
| `conversation`, `api_4_conversation` | Unchanged by this step: the legacy `message` and `reference` payload columns stay in place. The Go schema marks both `gorm:"-"`, so only a probe model can still read them. |

### Data migration

| Table | Change |
|---|---|
| `conversation_message`, `api_4_conversation_message` | Each element of the parent row's `message` JSON array becomes one row, `position` being its index in the array. |
| `conversation_reference`, `api_4_conversation_reference` | Each entry of the parent row's `reference` JSON array becomes one row, `position` being its index in the array. `message_position` stays `NULL`: the payload carries no message binding, and only references written together with a message by the current code path set it. |

Execution details:

- Conversations are processed in cursor-ordered batches of 100, each batch in its own transaction, so an interrupted run resumes from the last committed batch.
- A conversation that already has rows in either child table is left untouched: rows written after the split took effect win over the payload, so a rerun neither duplicates nor clobbers them.
- Parent tables without `message` and `reference` columns are skipped.

On success the step writes `v1.0.0-rc1.dev1` into the version marker.

---

## Version-less steps

These steps are not gated by the version marker. They are idempotent and run on every startup, either through `RunMigrations` or through the ingestion helpers in `InitDB`.

### Schema changes

| Table | Change |
|---|---|
| `tenant_llm` | Composite primary key `(tenant_id, llm_factory, llm_name)` is replaced by a surrogate auto-increment `id`; the legacy uniqueness is preserved by a unique index on the old key columns. |
| `task`, `document` | `process_duation` renamed to `process_duration`. |
| `user` | Unique index on `email`. |
| `ingestion_task` | Unique index on `document_id`. |
| `knowledgebase` | Adds the `VIRTUAL` generated column `name_ci` (`LOWER(name)` for `status='1'` rows, `NULL` otherwise) and the unique index `idx_kb_tenant_name_ci (tenant_id, name_ci)`, so soft-deleted rows never block name reuse. |
| `user_canvas` | Unique index `(user_id, canvas_category, title)`. |
| `pipeline_operation_log` | Adds `run_count int NULL` and the index `idx_pipeline_operation_log_document_run`. |
| `ingestion_task` | Adds `pipeline_log_id varchar(32) NULL`. |
| `ingestion_task_log` | Adds `pipeline_log_id varchar(32) NULL`, `event_type tinyint NOT NULL DEFAULT 4`, and the index `idx_ingestion_task_log_pipeline_id`. |
| `dialog`, `tenant_llm`, `api_token`, `canvas_template`, `system_settings`, `knowledgebase` | Column types converged to the Peewee models, for example `dialog.top_k int NOT NULL DEFAULT 1024`, `tenant_llm.api_key text`, `api_token.dialog_id varchar(32)`, `canvas_template.title` / `description` nullable longtext, `system_settings.value text NOT NULL`, `knowledgebase.raptor_task_finish_at` / `mindmap_task_finish_at` datetime. |
| `conversation`, `api_4_conversation` | Index `idx_<table>_dialog_updated (dialog_id, update_time, id)` for conversation list queries. |

### Data changes

| Table | Change |
|---|---|
| `knowledgebase`, `dialog` | `parser_config` entries stored under the legacy key `TokenChunker:SixApplesFall` are rewritten to `GeneralChunker:SixApplesFall`. Values already stored under the new key win; `TokenChunker`-only fields (`delimiter_mode`, `outputs`) are dropped, and a legacy `delimiter` becomes `delimiters`. `delimiter` is discarded when `delimiters` already exists. |
