"""
RAGFlow-specific migration orchestrator from Elasticsearch to OceanBase.
"""

import logging
import time
from typing import Any, Callable

from rich.console import Console
from rich.progress import (
    Progress,
    SpinnerColumn,
    TextColumn,
    BarColumn,
    TaskProgressColumn,
    TimeRemainingColumn,
)

from .es_client import ESClient
from .ob_client import OBClient
from .schema import RAGFlowSchemaConverter, RAGFlowDataConverter
from .progress import ProgressManager, MigrationProgress
from .verify import MigrationVerifier

logger = logging.getLogger(__name__)
console = Console()


class ESToOceanBaseMigrator:
    """
    RAGFlow-specific migration orchestrator.
    
    This migrator is designed specifically for RAGFlow's data structure,
    handling the dynamic schema and vector embeddings correctly.
    """

    def __init__(
        self,
        es_client: ESClient,
        ob_client: OBClient,
        progress_dir: str = ".migration_progress",
    ):
        """
        Initialize migrator.

        Args:
            es_client: Elasticsearch client
            ob_client: OceanBase client
            progress_dir: Directory for progress files
        """
        self.es_client = es_client
        self.ob_client = ob_client
        self.progress_manager = ProgressManager(progress_dir)

    def migrate(
        self,
        es_index: str,
        ob_table: str,
        batch_size: int = 1000,
        resume: bool = False,
        verify_after: bool = True,
        on_progress: Callable[[int, int], None] | None = None,
    ) -> dict[str, Any]:
        """
        Execute full migration from ES to OceanBase for RAGFlow data.

        Args:
            es_index: Source Elasticsearch index
            ob_table: Target OceanBase table
            batch_size: Documents per batch
            resume: Resume from previous progress
            verify_after: Run verification after migration
            on_progress: Progress callback (migrated, total)

        Returns:
            Migration result dictionary
        """
        start_time = time.time()
        result = {
            "success": False,
            "es_index": es_index,
            "ob_table": ob_table,
            "total_documents": 0,
            "migrated_documents": 0,
            "failed_documents": 0,
            "duration_seconds": 0,
            "verification": None,
            "error": None,
        }

        progress: MigrationProgress | None = None
        schema_converter = RAGFlowSchemaConverter()

        try:
            # Step 1: Check connections
            console.print("[bold blue]Step 1: Checking connections...[/]")
            self._check_connections()

            # Step 2: Analyze ES index
            console.print("\n[bold blue]Step 2: Analyzing ES index...[/]")
            es_mapping = self.es_client.get_index_mapping(es_index)
            schema_converter.analyze_es_mapping(es_mapping)
            
            # Discover vector fields from actual data (may not be in ES mapping)
            sample_docs = self.es_client.get_sample_documents(es_index, size=100)
            if sample_docs:
                schema_converter.discover_vector_fields(sample_docs)
            
            console.print(f"  Detected fields: {len(schema_converter.fields)}")
            if schema_converter.vector_fields:
                for vf in schema_converter.vector_fields:
                    console.print(f"  Vector field: {vf['name']} (dim={vf['dimension']})")

            # Step 3: Get total document count
            total_docs = self.es_client.count_documents(es_index)
            console.print(f"  Total documents: {total_docs:,}")
            
            result["total_documents"] = total_docs

            if total_docs == 0:
                console.print("[yellow]No documents to migrate[/]")
                result["success"] = True
                return result

            # Step 4: Handle resume or fresh start
            if resume and self.progress_manager.can_resume(es_index, ob_table):
                console.print("\n[bold yellow]Resuming from previous progress...[/]")
                progress = self.progress_manager.load_progress(es_index, ob_table)
                console.print(
                    f"  Previously migrated: {progress.migrated_documents:,} documents"
                )
            else:
                # Fresh start - check if table already exists
                if self.ob_client.table_exists(ob_table):
                    raise RuntimeError(
                        f"Table '{ob_table}' already exists in OceanBase. "
                        f"Migration aborted to prevent data conflicts. "
                        f"Please drop the table manually or use a different table name."
                    )

                progress = self.progress_manager.create_progress(
                    es_index, ob_table, total_docs
                )

            # Step 5: Create table if needed
            if not progress.table_created:
                console.print("\n[bold blue]Step 3: Creating OceanBase table...[/]")
                self.ob_client.create_table_from_schema(
                    table_name=ob_table,
                    schema_converter=schema_converter,
                    create_indexes=True,
                    create_fts_indexes=True,
                )
                console.print(f"  Created table '{ob_table}' with {len(schema_converter.fields)} columns")
                
                progress.table_created = True
                progress.indexes_created = True
                progress.schema_converted = True
                self.progress_manager.save_progress(progress)

            # Step 6: Migrate data
            console.print("\n[bold blue]Step 4: Migrating data...[/]")
            data_converter = RAGFlowDataConverter()

            migrated = self._migrate_data(
                es_index=es_index,
                ob_table=ob_table,
                data_converter=data_converter,
                progress=progress,
                batch_size=batch_size,
                on_progress=on_progress,
            )

            result["migrated_documents"] = migrated
            result["failed_documents"] = progress.failed_documents

            # Step 7: Mark completed
            self.progress_manager.mark_completed(progress)

            # Step 8: Verify (optional)
            if verify_after:
                console.print("\n[bold blue]Step 5: Verifying migration...[/]")
                verifier = MigrationVerifier(self.es_client, self.ob_client)
                verification = verifier.verify(es_index, ob_table)
                result["verification"] = {
                    "passed": verification.passed,
                    "message": verification.message,
                    "es_count": verification.es_count,
                    "ob_count": verification.ob_count,
                    "sample_match_rate": verification.sample_match_rate,
                }
                console.print(verifier.generate_report(verification))

            result["duration_seconds"] = time.time() - start_time
            
            # Success only if we migrated documents and failed count is acceptable
            migrated = result['migrated_documents']
            failed = result['failed_documents']
            total = result['total_documents']
            
            if failed == 0 and migrated > 0:
                result["success"] = True
                console.print(
                    f"\n[bold green]Migration completed successfully![/]"
                    f"\n  Total: {total:,} documents"
                    f"\n  Migrated: {migrated:,} documents"
                    f"\n  Duration: {result['duration_seconds']:.1f} seconds"
                )
            elif migrated > 0:
                result["success"] = True  # Partial success
                console.print(
                    f"\n[bold yellow]Migration completed with errors![/]"
                    f"\n  Total: {total:,} documents"
                    f"\n  Migrated: {migrated:,} documents"
                    f"\n  [red]Failed: {failed:,} documents[/]"
                    f"\n  Duration: {result['duration_seconds']:.1f} seconds"
                )
            else:
                result["success"] = False
                console.print(
                    f"\n[bold red]Migration failed![/]"
                    f"\n  Total: {total:,} documents"
                    f"\n  Migrated: 0 documents"
                    f"\n  [red]Failed: {failed:,} documents[/]"
                    f"\n  Duration: {result['duration_seconds']:.1f} seconds"
                )

        except KeyboardInterrupt:
            console.print("\n[bold yellow]Migration interrupted by user[/]")
            if progress:
                self.progress_manager.mark_paused(progress)
            result["error"] = "Interrupted by user"

        except Exception as e:
            logger.exception("Migration failed")
            if progress:
                self.progress_manager.mark_failed(progress, str(e))
            result["error"] = str(e)
            console.print(f"\n[bold red]Migration failed: {e}[/]")

        return result

    def _check_connections(self):
        """Verify connections to both databases."""
        # Check ES
        es_health = self.es_client.health_check()
        if es_health.get("status") not in ("green", "yellow"):
            raise RuntimeError(f"ES cluster unhealthy: {es_health}")
        console.print(f"  ES cluster status: {es_health.get('status')}")

        # Check OceanBase
        if not self.ob_client.health_check():
            raise RuntimeError("OceanBase connection failed")
        
        ob_version = self.ob_client.get_version()
        console.print(f"  OceanBase connection: OK (version: {ob_version})")

    def _migrate_data(
        self,
        es_index: str,
        ob_table: str,
        data_converter: RAGFlowDataConverter,
        progress: MigrationProgress,
        batch_size: int,
        on_progress: Callable[[int, int], None] | None,
    ) -> int:
        """Migrate data in batches."""
        total = progress.total_documents
        migrated = progress.migrated_documents

        with Progress(
            SpinnerColumn(),
            TextColumn("[progress.description]{task.description}"),
            BarColumn(),
            TaskProgressColumn(),
            TimeRemainingColumn(),
            console=console,
        ) as pbar:
            task = pbar.add_task(
                "Migrating...",
                total=total,
                completed=migrated,
            )

            batch_count = 0
            for batch in self.es_client.scroll_documents(es_index, batch_size):
                batch_count += 1
                
                # Convert batch to OceanBase format
                ob_rows = data_converter.convert_batch(batch)

                # Insert batch
                try:
                    inserted = self.ob_client.insert_batch(ob_table, ob_rows)
                    migrated += inserted

                    # Update progress
                    last_ids = [doc.get("_id", doc.get("id", "")) for doc in batch]
                    self.progress_manager.update_progress(
                        progress,
                        migrated_count=inserted,
                        last_batch_ids=last_ids,
                    )

                    # Update progress bar
                    pbar.update(task, completed=migrated)

                    # Callback
                    if on_progress:
                        on_progress(migrated, total)

                    # Log periodically
                    if batch_count % 10 == 0:
                        logger.info(f"Migrated {migrated:,}/{total:,} documents")

                except Exception as e:
                    logger.error(f"Batch insert failed: {e}")
                    progress.failed_documents += len(batch)
                    # Continue with next batch

        return migrated
