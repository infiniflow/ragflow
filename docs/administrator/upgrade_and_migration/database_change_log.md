---
sidebar_position: 6
title: Database Change Log
sidebar_label: Database Change Log
slug: /database_change_log
sidebar_custom_props: {
  categoryIcon: LucideLocateFixed
}
---

# Database Change Log

A record of the schema and data changes the Go backend applies to a RAGFlow database, and of the version or schema checks that decide which of them still have to run.

## How the database version is recorded

- The marker is a row in `system_settings` with the key `mysql_migration.database.version`.
- For version-gated steps, the marker prevents a migration step from running again after that step has completed successfully.
- Go defines the versions as constants: `modelMigrationBaseVersion` and `modelMigrationTargetVersion` in [migration_version.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/migration_version.go), `conversationHistoryTargetVersion` in [conversation_history_migration.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/conversation_history_migration.go), `knowledgebaseParserConfigTargetVersion` in [migration.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/migration.go), and `ingestionParserConfigTargetVersion` in [ingestion_parser_config_migration.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/ingestion_parser_config_migration.go). The two parser-config constants share `v1.0.0-rc2`. The DSL multi-version backfill is gated by `dslVersionTargetVersion` (`v1.0.0-rc4`) in [dsl_version_migration.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/dsl_version_migration.go).
- The current permission migration targets `permissionMigrationTargetVersion` (`v1.0.0-rc3.dev4`) in [permission_migration.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/permission_migration.go). Tenant default-model migration targets `tenantDefaultModelsTargetVersion` (`v1.0.0-rc3.dev5`) in [tenant_default_models_migration.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/tenant_default_models_migration.go).
- The compilation queue split records `knowledgeCompileQueueTargetVersion` (`v1.0.0-rc3.dev5`), defined in [knowledge_compile_queue_migration.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/knowledge_compile_queue_migration.go), after both queues finish. Source-column presence controls the queue copy even when the marker already contains this or a newer version.
- A version-gated step runs when the stored version is lower than the step's version. Versions are compared with Python PEP 440 semantics, where a `dev` prerelease sorts *before* the release it precedes (`v1.0.0-rc1.dev1` < `v1.0.0-rc1`). A missing or unparsable marker never skips a step.
- `checkDatabaseVersion` in [cmd/ragflow_server.go](https://github.com/infiniflow/ragflow/blob/main/cmd/ragflow_server.go) refuses to start when the code version is older than the stored database version, so a database migrated by a newer release cannot be opened by an older one.

Startup order in `InitDB` for `ragflow_server --migrate` ([database.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/database.go)):

1. `RunMigrations` — manual, idempotent steps plus the two versioned tenant model steps. Runs *before* `AutoMigrate` because it has to observe the legacy schema.
2. `migrateIngestionLogRunIdentity` — ingestion log columns and indexes.
3. `migrateUserTenantDuplicates` — deduplicates memberships before `AutoMigrate` creates their unique index.
4. `AutoMigrate` — converges every ORM model, including the compilation queue tables.
5. `migrateConversationHistory` — runs *after* `AutoMigrate`, which creates the child tables it backfills.
6. `migrateIngestionParserConfigs` — converts ingestion settings on datasets and documents, then invokes the existing `migrateKnowledgebaseParserConfig` cleanup. Both run after the conversation split so the shared marker cannot skip that split.
7. `migrateTenantModelMaxTokens` — backfills model context windows before advancing the marker to its target.
8. `migratePermissions` — applies the cumulative permission migration through `v1.0.0-rc3.dev4`. The enterprise hook runs before the shared version check.
9. `migrateTenantDefaultModels` — moves tenant-wide model selections to owner memberships, drops the tenant columns, and records `v1.0.0-rc3.dev5`.
10. `migrateTenantModelProviderUserUniqueIndex` — reconciles provider ownership indexes.
11. `migrateKnowledgeCompileQueues` — copies the existing compilation queues and drops their source columns, then records `v1.0.0-rc3.dev5` if the marker is older. It runs after tenant default-model migration so the queue split cannot make that step skip its data changes. Queue copying remains gated on source-column presence even when the preceding step has already recorded `dev5`.

## Version overview

| Database version  | Scope                                                                                                        | Gate                               |
|-------------------|--------------------------------------------------------------------------------------------------------------|------------------------------------|
| `v0.26.0`         | Rebuild the tenant model tables from `tenant_llm` and normalize stored model ids                             | `modelMigrationBaseVersion`        |
| `v1.0.0-rc1`      | Seed factory-declared models, merge `model_type` into an integer bitmask, populate the `tenant_*_id` columns | `modelMigrationTargetVersion`      |
| `v1.0.0-rc1`      | Split conversation message and reference payloads into child tables                                          | `conversationHistoryTargetVersion` |
| `v1.0.0-rc2`      | Convert Python ingestion settings in dataset/document `parser_config` onto pipeline components, then remove retired dataset `raptor` / `graphrag` sections | `ingestionParserConfigTargetVersion` / `knowledgebaseParserConfigTargetVersion` |
| `v1.0.0-rc3.dev1` | Permission ownership columns, nullable inviters, and unique tenant memberships | Historical permission target; included in the current cumulative permission migration |
| `v1.0.0-rc3.dev2` | No standalone community migration is defined in the checked-in code or its history | No separate community gate |
| `v1.0.0-rc3.dev3` | Tenant role support, user-scoped model providers, and enterprise migration-marker separation | Historical permission target; provider index reconciliation uses schema checks |
| `v1.0.0-rc3.dev4` | Normalize legacy tenant invitation roles and statuses | `permissionMigrationTargetVersion` |
| `v1.0.0-rc3.dev5` | Move tenant default-model selections to owner memberships and drop the tenant columns | `tenantDefaultModelsTargetVersion` |
| `v1.0.0-rc3.dev5` | Split compilation queues into `knowledge_compile_docs_backlog` and `knowledge_compile_docs_inflight`, then drop their source columns | `knowledgeCompileQueueTargetVersion`; source-column presence controls copying |
| `v1.0.0-rc4`      | Lock existing dataflow-canvas dataset/document bindings to the canvas latest version (the `pipeline_version_id` columns are added by the version-less step below) | `dslVersionTargetVersion` |

The two tenant model steps are cumulative: a database at `v0.26.0` still runs the `v1.0.0-rc1` step, and a database at or above `v1.0.0-rc1` runs neither.

The `dev1`–`dev4` permission entries below describe the development history. They are consolidated into the current `migratePermissions` body targeting `dev4`; they are not four independent migration functions. The two `dev5` changes run in the order listed above and share the database version marker. A `dev5` marker alone does not prove that the compilation queue columns have been removed.

---

## v0.26.0

### Schema changes

| Object                  | Change                                                                                                                                                                   |
|-------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `system_settings`       | Created if missing, so the version marker can be written before `AutoMigrate` reaches the table.                                                                         |
| `tenant_model_provider` | Created if missing. Columns: `id` (PK, varchar 32), `provider_name`, `tenant_id`, base timestamps. Unique index `idx_tenant_provider_unique (tenant_id, provider_name)`. |
| `tenant_model_instance` | Created if missing. Columns: `id` (PK), `instance_name`, `provider_id`, `api_key`, `status`, `extra`, base timestamps.                                                   |
| `tenant_model`          | Created if missing. Columns: `id` (PK), `model_name`, `provider_id`, `instance_id`, `model_type`, `status`, `extra`, base timestamps.                                    |

The three tables are created only when the legacy `tenant_llm` table exists and the target table does not, mirroring the `CREATE TABLE IF NOT EXISTS` the Python stages run. Existing tables are never altered by this step: `tenant_model.model_type` deliberately keeps the text shape the Python migration left behind so the `v1.0.0-rc1` merge can still read the legacy model names.

### Data migration

| Table                                         | Change                                                                                                                                                                                                                                                                                                                                                                                                                                 |
|-----------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `tenant_model_provider`                       | One row per distinct `(tenant_id, llm_factory)` in `tenant_llm`; `provider_name` takes the factory name.                                                                                                                                                                                                                                                                                                                               |
| `tenant_model_instance`                       | One row per `(tenant_id, llm_factory)`, carrying the first `api_key` after deduplication; `instance_name` defaults to `default`.                                                                                                                                                                                                                                                                                                       |
| `tenant_model`                                | Rows derived from `tenant_llm`: `model_type` is written as an integer bitmask, only groups whose merged status is `active` are kept, and `provider_id` / `instance_id` point at the rows created above. Bits: `chat=1`, `embedding=2`, `asr`/`speech2text=4`, `vision`/`image2text=8`, `rerank=16`, `tts=32`, `ocr=64`.                                                                                                                |
| `tenant`, `knowledgebase`, `dialog`, `memory` | Stored model ids are rewritten from `<model>@<provider>` to `<model>@default@<provider>`: `tenant.llm_id / embd_id / asr_id / img2txt_id / rerank_id / tts_id / ocr_id`, `knowledgebase.embd_id`, `dialog.llm_id / rerank_id`, `memory.embd_id / llm_id`, plus the same keys nested inside `parser_config` and `prompt_config` JSON (`llm_id`, `embd_id`, `embedding_model`, `rerank_id`, `asr_id`, `img2txt_id`, `tts_id`, `ocr_id`). |

On success the step writes `v0.26.0` into the version marker.

---

## v0.27.2

### Schema changes

| Table           | Change                                                                                                                                                                        |
|-----------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `tenant_model`  | `model_type` is converted from the legacy text representation to the integer bitmask (see the data migration below). Once the column is an integer the conversion is a no-op. |
| `tenant`        | Adds `tenant_llm_id`, `tenant_embd_id`, `tenant_asr_id`, `tenant_img2txt_id`, `tenant_rerank_id`, `tenant_tts_id`, `tenant_ocr_id` — each `varchar(32) NULL`.                 |
| `knowledgebase` | Adds `tenant_embd_id` — `varchar(32) NULL`.                                                                                                                                   |
| `dialog`        | Adds `tenant_llm_id`, `tenant_rerank_id` — `varchar(32) NULL`.                                                                                                                |
| `memory`        | Adds `tenant_embd_id`, `tenant_llm_id` — `varchar(32) NULL`.                                                                                                                  |

The `tenant_*_id` columns are added only when missing, and are skipped entirely when any of `tenant_model`, `tenant_model_provider`, `tenant_model_instance` is absent.

### Data migration

| Table                                         | Change                                                                                                                                                                                                                                                                                                          |
|-----------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `tenant_model`                                | **Seeding:** the models declared in `conf/llm_factories.json` are inserted for every provider/instance created by the `v0.26.0` step. When a `(provider_id, instance_id, model_name)` row already exists, the declared factory bits are OR-ed into its `model_type` instead of a second row being inserted.     |
| `tenant_model`                                | **Type merge:** rows sharing `(provider_id, instance_id, model_name)` collapse into a single row whose `model_type` is the OR of the contributing bits. Bits that only ever appeared on `unsupported` rows are dropped, and groups whose merged status is not `active` are dropped.                             |
| `tenant`, `knowledgebase`, `dialog`, `memory` | **Id backfill:** each `tenant_*_id` column is filled from its legacy counterpart (`llm_id`, `embd_id`, …) by resolving the referenced `tenant_model.id` through a `(tenant_id, model_name, provider, model_type)` lookup over active models, with the `model_type` bitmask expanded back into individual types. |

On success the step writes `v0.27.2` into the version marker.

---

## v1.0.0-rc1

This step only prepares the schema owned by `v1.0.0-rc1`; dropping the legacy payload columns is out of its scope.

### Schema changes

| Table                                | Change                                                                                                                                                                                                                                                                                                                                               |
|--------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `conversation_message`               | Created by `AutoMigrate`. Composite PK `(conversation_id, position)`; columns `message_id`, `role`, `content`, `content_type`, `status`, `thumb_up`, `feedback`, `created_at`, `metadata`; composite index `message_lookup (conversation_id, message_id, role, position)`; foreign key `conversation_id → conversation.id` with `ON DELETE CASCADE`. |
| `conversation_reference`             | Created by `AutoMigrate`. Composite PK `(conversation_id, position)`; columns `reference` (longtext), `message_position`; foreign key `conversation_id → conversation.id` with `ON DELETE CASCADE`.                                                                                                                                                  |
| `api_4_conversation_message`         | Same shape as `conversation_message`, parented by `api_4_conversation`.                                                                                                                                                                                                                                                                              |
| `api_4_conversation_reference`       | Same shape as `conversation_reference`, parented by `api_4_conversation`.                                                                                                                                                                                                                                                                            |
| `conversation`, `api_4_conversation` | Unchanged by this step: the legacy `message` and `reference` payload columns stay in place. The Go schema marks both `gorm:"-"`, so only a probe model can still read them.                                                                                                                                                                          |

### Data migration

| Table                                                    | Change                                                                                                                                                                                                                                                                        |
|----------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `conversation_message`, `api_4_conversation_message`     | Each element of the parent row's `message` JSON array becomes one row, `position` being its index in the array.                                                                                                                                                               |
| `conversation_reference`, `api_4_conversation_reference` | Each entry of the parent row's `reference` JSON array becomes one row, `position` being its index in the array. `message_position` stays `NULL`: the payload carries no message binding, and only references written together with a message by the current code path set it. |

Execution details:

- Conversations are processed in cursor-ordered batches of 100, each batch in its own transaction, so an interrupted run resumes from the last committed batch.
- A conversation that already has rows in either child table is left untouched: rows written after the split took effect win over the payload, so a rerun neither duplicates nor clobbers them.
- Parent tables without `message` and `reference` columns are skipped.

On success the step writes `v1.0.0-rc1` into the version marker.

---

## v1.0.0-rc2

Converts saved Python v0.27.2 ingestion settings into the component-scoped configuration consumed by Go. After conversion, the existing dataset cleanup removes the retired `raptor` and `graphrag` sections.

### Data migration

| Table | Change |
|-------|--------|
| `knowledgebase`, `document` | Move supported top-level Parser, Chunker, Tokenizer and Extractor settings onto the corresponding pipeline component nodes. |
| `knowledgebase` | The existing cleanup deletes top-level `raptor` and `graphrag` sections, preserving other settings. |

The conversion covers parser engines and model references, PDF pages, MinerU method/language, TCADP options, spreadsheet column configuration, supported chunk sizes and delimiters, overlap, media context, parent/child settings, filename embedding weight, keywords, questions and metadata. Fields are converted only where the selected component supports them. Settings without a Go consumer are dropped with table/row/field warnings; tags and early Go node formats are left untouched.

Execution details:

- Component conversion reads each table in id-ordered batches of 256, each batch in its own transaction. Changed rows are updated once by the conversion; unchanged rows retain their original bytes. The existing dataset cleanup performs its own subsequent scan.
- Each row's `parser_id` determines the builtin component targets. Parser/Chunker/Tokenizer conversion leaves custom pipeline rows and unsupported parser IDs untouched. Extractor conversion can also target existing Extractor nodes in custom pipeline configs.
- Existing component values win, except that dataset-level modular metadata transport remains authoritative under the current service contract.
- SQL `NULL`, empty strings and JSON `null` are left untouched. Invalid JSON, unsupported metadata schemas and unsupported engine selections stop migration with table/row context. A failed batch rolls back; earlier committed batches can be rerun safely. See [Parser-config conversion failures](./upgrade_from_v1_to_later_releases.md#parser-config-conversion-failures) for recovery instructions.
- The marker advances only after both table conversions and the existing dataset cleanup succeed. If neither table exists, the marker is left unchanged.

On success the migration records `v1.0.0-rc2`. Databases already marked `v1.0.0-rc2` or later skip conversion. This includes databases marked by the earlier cleanup-only implementation: that marker does not prove their ingestion settings were converted.

---

## v1.0.0-rc3.dev1

### Permission schema and ownership

The initial permission migration introduced these changes, now retained in the cumulative `migratePermissions` implementation:

| Table | Schema and data change |
|-------|------------------------|
| `user_tenant` | Makes `invited_by` nullable, widens `status` to `varchar(16)`, and adds unique index `idx_user_tenant_user_tenant (user_id, tenant_id)`. |
| `tenant_model_provider` | Adds `user_id varchar(32)` and fills missing or empty owners from `tenant_id`. |
| `api_token` | Adds nullable `user_id varchar(32)` and fills missing or empty owners from `tenant_id`. |
| `user_canvas` | Adds nullable `tenant_id varchar(32)` and fills missing or empty tenant IDs from `user_id`. |
| `tenant` | Adds `invite_enabled` with default `false` and nullable `role_id`. |

Before `AutoMigrate` creates the membership unique index, `migrateUserTenantDuplicates` merges duplicate `(user_id, tenant_id)` memberships. It retains the preferred joined membership using role and timestamp ordering, remaps references in existing `group_info`, `group_member`, `department_member`, and `permission` tables, and deletes the duplicates. This schema-checked preparation runs independently of the version marker.

The ownership backfills preserve non-empty values. They run after schema creation through `migratePermissions`, which currently records `dev4` after its cumulative work succeeds.

---

## v1.0.0-rc3.dev2

No standalone community schema or data migration for `dev2` is defined in this checkout. The community permission target moved directly from `dev1` to `dev3`; `dev2` is exercised as an existing database version in the permission migration tests. Upgrading from a `dev2` marker runs the cumulative permission migration targeting `dev4`.

Enterprise permission work is supplied through `permissionDataModelsEE` and `migratePermissionDataEE`. This community checkout provides empty/no-op implementations, so it does not define enterprise-only `dev2` changes.

---

## v1.0.0-rc3.dev3

### Tenant roles and provider ownership

| Table or migration | Change |
|--------------------|--------|
| `user` | Adds nullable, indexed `role_id` for user role assignment. |
| `tenant_model_provider` | Replaces uniqueness on `(provider_name, tenant_id)` with `idx_tenant_provider_user_unique (provider_name, tenant_id, user_id)`, allowing different users in one tenant to own providers with the same name. |
| `migratePermissions` | Invokes the enterprise permission hook before checking the shared version marker, so a completed community migration cannot suppress enterprise data work. Enterprise implementations can use their own marker through the marker-specific read/write helpers. |

Provider index reconciliation is implemented in [tenant_model_provider_migration.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/tenant_model_provider_migration.go). It checks existing indexes by column set, creates the replacement before dropping old two-column unique indexes, and runs independently of the permission version gate. Enterprise system-setting initialization also runs after the migration/server schema setup; it is a no-op in this community checkout.

---

## v1.0.0-rc3.dev4

### Legacy tenant invitations

`migrateLegacyTenantInvitationStatuses`, called by `migratePermissions`, rewrites `user_tenant` rows whose role is `invite` and whose status is joined (`'1'`) or SQL `NULL`. The resulting role is an empty string and status is invited (`'2'`). Other memberships remain unchanged.

This step adds no separate schema object. `permissionMigrationTargetVersion` is now `v1.0.0-rc3.dev4`; success records that value after the invitation repair and earlier permission backfills finish. A database already at `dev4` or later skips the shared permission body, while the enterprise hook still runs before that check.

---

## v1.0.0-rc3.dev5

### Tenant default models

Implemented by [tenant_default_models_migration.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/tenant_default_models_migration.go), with target `tenantDefaultModelsTargetVersion` (`v1.0.0-rc3.dev5`).

| Table | Schema and data change |
|-------|------------------------|
| `user_tenant` | Receives the model-selection columns through `AutoMigrate`. Missing or empty selections are filled from the matching `tenant` row only for memberships with role `owner` or `user_id = tenant_id`. Existing selections and other members' settings are preserved. |
| `tenant` | **Destructive:** drops the old model-selection columns after the transactional copy succeeds. |

The columns are `llm_id`, `embd_id`, `asr_id`, `img2txt_id`, `rerank_id`, `tts_id`, `ocr_id`, and `decision_id`, together with their corresponding `tenant_*_id` columns. Copying all columns is transactional; column drops follow and check whether each column still exists, allowing an interrupted run to resume before the marker is advanced.

This version-gated step skips databases already marked `dev5` or later. When either required table is absent, there is no copy or drop, but the step still records its target version. It runs before the queue split, which also targets `dev5`.

### Knowledge compilation queues

Introduced in `v1.0.0-rc3.dev5` and implemented by `migrateKnowledgeCompileQueues` in [knowledge_compile_queue_migration.go](https://github.com/infiniflow/ragflow/blob/main/internal/dao/knowledge_compile_queue_migration.go). Each queue is migrated when its source column exists, regardless of `mysql_migration.database.version`. After both queues finish, the step records `knowledgeCompileQueueTargetVersion` (`v1.0.0-rc3.dev5`) in that marker if its current value is older. A newer version is preserved.

| Table | Schema and data change |
|-------|------------------------|
| `knowledge_compile_docs_backlog` | Created by `AutoMigrate`. Each event in `knowledge_compile_docs.backlog_doc_ids` becomes a row with `dataset_id`, `doc_id`, `queue_position`, `event_type`, `variants`, and `task_types`. |
| `knowledge_compile_docs_inflight` | Created by `AutoMigrate` with the same columns; populated from `knowledge_compile_docs.inflight_doc_ids`. |
| `knowledge_compile_docs` | **Destructive:** drops `backlog_doc_ids` and `inflight_doc_ids`, each after its queue has been copied successfully. Dataset scheduling state, leases, progress, and diagnostics remain in this table. |

Both new tables have no `id`, primary key, or unique constraint. A non-unique index on `(dataset_id, queue_position)` supports ordered reads. Migration preserves duplicate events, array order, and routing metadata; `variants` and `task_types` remain JSON arrays stored in longtext columns.

Datasets are read in batches of 100, with each dataset copied in its own transaction. Info-level progress logs identify the queue and batch range, report committed dataset and entry counts and remaining datasets, and record each source-column drop. Entry counts include duplicate events and reset for each queue and migration run. Failed dataset copies roll back and log the dataset and committed progress. Rerunning while services remain stopped replaces copied rows for any source column that still exists, so retries do not duplicate events.

The queue step advances the version marker only after both queue copies and source-column drops succeed. If either queue fails, it leaves the marker at its value on entry; the preceding tenant default-model step may already have recorded `dev5`. The source-column checks and server startup guard therefore remain authoritative for queue migration completion. Databases whose queues were already split also advance to `v1.0.0-rc3.dev5` without copying entries again when their marker is older. The completion log includes the database version and target version.

This migration requires a coordinated service stop:

1. Stop all services that publish or consume compilation events.
2. Run `ragflow_server --migrate --log-level info` to see progress and wait for successful completion of both queues.
3. Restart the services.

Starting a server while either source column remains fails with `knowledge compilation queues need migration`. A database version marker at or above the current release does not bypass this requirement. If migration fails, keep services stopped, resolve the reported error, and rerun `ragflow_server --migrate`.

## v1.0.0-rc4

Locks every pre-existing dataflow-canvas binding to the latest version of that canvas, so already-configured datasets and documents keep parsing against the version that was current when the upgrade ran (design section 5/B4 sticky snapshot). The `pipeline_version_id` columns it writes are added by the version-less step below; this step only backfills their values and is gated by the `dslVersionTargetVersion` marker.

### Data migration

| Table | Change |
|-------|--------|
| `knowledgebase`, `document` | Rows whose `pipeline_id` points at a `dataflow_canvas` are bound to that canvas latest version (`user_canvas_version` ordered by `create_time DESC`). Bindings to a built-in parser id (no backing `user_canvas`), to a non-dataflow canvas, or to a canvas with no versions are left unbound; a dangling version id is never written. |

Execution details:

- Rows are read in id-ordered batches of 256, each batch in its own transaction, so an interrupted run resumes cleanly.
- The marker advances to `v1.0.0-rc4` only after both tables are processed. Databases already marked `v1.0.0-rc4` or later skip the backfill.

On success the step writes `v1.0.0-rc4` into the version marker.

---

## Version-less steps

These steps are not gated by the version marker. They use schema or data checks to make repeated execution safe. Steps in `RunMigrations` run during `ragflow_server --migrate`; ingestion helpers also run during server startup.

### Schema changes

| Table                                                                                      | Change                                                                                                                                                                                                                                                                                                                                        |
|--------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `tenant_llm`                                                                               | Composite primary key `(tenant_id, llm_factory, llm_name)` is replaced by a surrogate auto-increment `id`; the legacy uniqueness is preserved by a unique index on the old key columns.                                                                                                                                                       |
| `task`, `document`                                                                         | `process_duation` renamed to `process_duration`.                                                                                                                                                                                                                                                                                              |
| `user`                                                                                     | Unique index on `email`.                                                                                                                                                                                                                                                                                                                      |
| `ingestion_task`                                                                           | Unique index on `document_id`.                                                                                                                                                                                                                                                                                                                |
| `knowledgebase`                                                                            | Adds the `VIRTUAL` generated column `name_ci` (`LOWER(name)` for `status='1'` rows, `NULL` otherwise) and the unique index `idx_kb_tenant_name_ci (tenant_id, name_ci)`, so soft-deleted rows never block name reuse.                                                                                                                         |
| `user_canvas`                                                                              | Unique index `(user_id, canvas_category, title)`.                                                                                                                                                                                                                                                                                             |
| `pipeline_operation_log`                                                                   | Adds `run_count int NULL` and the index `idx_pipeline_operation_log_document_run`.                                                                                                                                                                                                                                                            |
| `ingestion_task`                                                                           | Adds `pipeline_log_id varchar(32) NULL`.                                                                                                                                                                                                                                                                                                      |
| `ingestion_task_log`                                                                       | Adds `pipeline_log_id varchar(32) NULL`, `event_type tinyint NOT NULL DEFAULT 4`, and the index `idx_ingestion_task_log_pipeline_id`.                                                                                                                                                                                                         |
| `dialog`, `tenant_llm`, `api_token`, `canvas_template`, `system_settings`, `knowledgebase` | Column types converged to the Peewee models, for example `dialog.top_k int NOT NULL DEFAULT 1024`, `tenant_llm.api_key text`, `api_token.dialog_id varchar(32)`, `canvas_template.title` / `description` nullable longtext, `system_settings.value text NOT NULL`, `knowledgebase.raptor_task_finish_at` / `mindmap_task_finish_at` datetime. |
| `conversation`, `api_4_conversation`                                                       | Index `idx_<table>_dialog_updated (dialog_id, update_time, id)` for conversation list queries.                                                                                                                                                                                                                                                |
| `knowledgebase` | Adds `pipeline_version_id varchar(32) NULL` (dataflow canvas version binding; design section 5/B4). |
| `document` | Adds `pipeline_version_id varchar(32) NULL` (inherits the dataset pinned version at upload; design section 5). |
| `user_canvas_version` | Adds `alias varchar(255) NULL` (free-text version alias for the version list filter; design A2/F1). |

### Data changes

| Table                     | Change                                                                                                                                                                                                                                                                                                                                                         |
|---------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `knowledgebase`, `dialog` | `parser_config` entries stored under the legacy key `TokenChunker:SixApplesFall` are rewritten to `GeneralChunker:SixApplesFall`. Values already stored under the new key win; `TokenChunker`-only fields (`delimiter_mode`, `outputs`) are dropped, and a legacy `delimiter` becomes `delimiters`. `delimiter` is discarded when `delimiters` already exists. |
