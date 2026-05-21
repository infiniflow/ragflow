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
    handling the fixed schema and vector embeddings correctly.
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
        self.schema_converter = RAGFlowSchemaConverter()

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

        try:
            # Step 1: Check connections
            console.print("[bold blue]Step 1: Checking connections...[/]")
            self._check_connections()

            # Step 2: Analyze ES index
            console.print("\n[bold blue]Step 2: Analyzing ES index...[/]")
            analysis = self._analyze_es_index(es_index)
            
            # Auto-detect vector size from ES mapping
            vector_size = 768  # Default fallback
            if analysis["vector_fields"]:
                vector_size = analysis["vector_fields"][0]["dimension"]
                console.print(f"  [green]Auto-detected vector dimension: {vector_size}[/]")
            else:
                console.print(f"  [yellow]No vector fields found, using default: {vector_size}[/]")
            console.print(f"  Known RAGFlow fields: {len(analysis['known_fields'])}")
            if analysis["unknown_fields"]:
                console.print(f"  [yellow]Unknown fields (will be stored in 'extra'): {analysis['unknown_fields']}[/]")

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
                if not self.ob_client.table_exists(ob_table):
                    self.ob_client.create_ragflow_table(
                        table_name=ob_table,
                        vector_size=vector_size,
                        create_indexes=True,
                        create_fts_indexes=True,
                    )
                    console.print(f"  Created table '{ob_table}' with RAGFlow schema")
                else:
                    console.print(f"  Table '{ob_table}' already exists")
                    # Check and add vector column if needed
                    self.ob_client.add_vector_column(ob_table, vector_size)
                
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
                verification = verifier.verify(
                    es_index, ob_table, 
                    primary_key="id"
                )
                result["verification"] = {
                    "passed": verification.passed,
                    "message": verification.message,
                    "es_count": verification.es_count,
                    "ob_count": verification.ob_count,
                    "sample_match_rate": verification.sample_match_rate,
                }
                console.print(verifier.generate_report(verification))

            result["success"] = True
            result["duration_seconds"] = time.time() - start_time

            console.print(
                f"\n[bold green]Migration completed successfully![/]"
                f"\n  Total: {result['total_documents']:,} documents"
                f"\n  Migrated: {result['migrated_documents']:,} documents"
                f"\n  Failed: {result['failed_documents']:,} documents"
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

    def _analyze_es_index(self, es_index: str) -> dict[str, Any]:
        """Analyze ES index structure for RAGFlow compatibility."""
        es_mapping = self.es_client.get_index_mapping(es_index)
        return self.schema_converter.analyze_es_mapping(es_mapping)

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

    def get_schema_preview(self, es_index: str) -> dict[str, Any]:
        """
        Get a preview of schema analysis without executing migration.

        Args:
            es_index: Elasticsearch index name

        Returns:
            Schema analysis information
        """
        es_mapping = self.es_client.get_index_mapping(es_index)
        analysis = self.schema_converter.analyze_es_mapping(es_mapping)
        column_defs = self.schema_converter.get_column_definitions()

        return {
            "es_index": es_index,
            "es_mapping": es_mapping,
            "analysis": analysis,
            "ob_columns": column_defs,
            "vector_fields": self.schema_converter.get_vector_fields(),
            "total_columns": len(column_defs),
        }

    def get_data_preview(
        self,
        es_index: str,
        sample_size: int = 5,
        kb_id: str | None = None,
    ) -> list[dict[str, Any]]:
        """
        Get sample documents from ES for preview.
        
        Args:
            es_index: ES index name
            sample_size: Number of samples
            kb_id: Optional KB filter
        """
        query = None
        if kb_id:
            query = {"term": {"kb_id": kb_id}}
        return self.es_client.get_sample_documents(es_index, sample_size, query=query)

    def list_knowledge_bases(self, es_index: str) -> list[str]:
        """
        List all knowledge base IDs in an ES index.
        
        Args:
            es_index: ES index name
            
        Returns:
            List of kb_id values
        """
        try:
            agg_result = self.es_client.aggregate_field(es_index, "kb_id")
            return [bucket["key"] for bucket in agg_result.get("buckets", [])]
        except Exception as e:
            logger.warning(f"Failed to list knowledge bases: {e}")
            return []
