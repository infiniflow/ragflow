"""
CLI entry point for RAGFlow ES to OceanBase migration tool.
Using Google Fire for flexible argument positioning.
"""

import json
import logging
import fire
from rich.console import Console
from rich.table import Table
from rich.logging import RichHandler

from .es_client import ESClient
from .ob_client import OBClient
from .migrator import ESToOceanBaseMigrator
from .verify import MigrationVerifier

console = Console()


def setup_logging(verbose: bool = False):
    """Setup logging configuration."""
    level = logging.DEBUG if verbose else logging.WARNING
    logging.basicConfig(
        level=level,
        format="%(message)s",
        datefmt="[%X]",
        handlers=[RichHandler(rich_tracebacks=True, console=console)],
    )


class ESOBMigrate:
    """RAGFlow ES to OceanBase Migration Tool.
    
    Migrate RAGFlow data from Elasticsearch 8+ to OceanBase with schema conversion,
    vector data mapping, batch import, and resume capability.
    
    Connection options (RAGFlow defaults):
        --es_host: Elasticsearch host (default: localhost)
        --es_port: Elasticsearch port (default: 1200)
        --es_user: Elasticsearch username (default: elastic)
        --es_password: Elasticsearch password (default: infini_rag_flow)
        --ob_host: OceanBase host (default: localhost)
        --ob_port: OceanBase port (default: 2881)
        --ob_user: OceanBase user (default: root@ragflow)
        --ob_password: OceanBase password (default: infini_rag_flow)
        --ob_database: OceanBase database (default: ragflow_doc)
    """
    
    def __init__(
        self,
        es_host: str = "localhost",
        es_port: int = 1200,
        es_user: str = "elastic",
        es_password: str = "infini_rag_flow",
        ob_host: str = "localhost",
        ob_port: int = 2881,
        ob_user: str = "root@ragflow",
        ob_password: str = "infini_rag_flow",
        ob_database: str = "ragflow_doc",
        verbose: bool = False,
    ):
        """Initialize with connection settings."""
        self.es_host = es_host
        self.es_port = es_port
        self.es_user = es_user
        self.es_password = es_password
        self.ob_host = ob_host
        self.ob_port = ob_port
        self.ob_user = ob_user
        self.ob_password = ob_password
        self.ob_database = ob_database
        self.verbose = verbose
        
        setup_logging(verbose)
        
        # Lazy-loaded clients
        self._es_client = None
        self._ob_client = None
    
    def _get_es_client(self) -> ESClient:
        """Get or create ES client."""
        if self._es_client is None:
            self._es_client = ESClient(
                host=self.es_host,
                port=self.es_port,
                username=self.es_user,
                password=self.es_password,
            )
        return self._es_client
    
    def _get_ob_client(self) -> OBClient:
        """Get or create OB client."""
        if self._ob_client is None:
            self._ob_client = OBClient(
                host=self.ob_host,
                port=self.ob_port,
                user=self.ob_user,
                password=self.ob_password,
                database=self.ob_database,
            )
        return self._ob_client
    
    def _close_clients(self):
        """Close all clients."""
        if self._es_client:
            self._es_client.close()
        if self._ob_client:
            self._ob_client.close()

    def migrate(
        self,
        index: str = None,
        table: str = None,
        batch_size: int = 1000,
        resume: bool = False,
        verify: bool = True,
        progress_dir: str = ".migration_progress",
    ):
        """Run RAGFlow data migration from Elasticsearch to OceanBase.
        
        Args:
            index: ES index name (omit to migrate all ragflow_* indices)
            table: OceanBase table name (omit to use same name as index)
            batch_size: Batch size for migration (default: 1000)
            resume: Resume from previous progress (default: False)
            verify: Verify after migration (default: True)
            progress_dir: Progress file directory (default: .migration_progress)
        """
        console.print("[bold]RAGFlow ES to OceanBase Migration[/]")

        try:
            es_client = self._get_es_client()
            ob_client = self._get_ob_client()
            
            # Determine indices to migrate
            if index:
                indices_to_migrate = [(index, table if table else index)]
            else:
                console.print(f"\n[cyan]Discovering RAGFlow indices...[/]")
                ragflow_indices = es_client.list_ragflow_indices()
                
                if not ragflow_indices:
                    console.print("[yellow]No ragflow_* indices found in Elasticsearch[/]")
                    return
                
                indices_to_migrate = [(idx, idx) for idx in ragflow_indices]
                
                console.print(f"[green]Found {len(indices_to_migrate)} RAGFlow indices:[/]")
                for idx, _ in indices_to_migrate:
                    doc_count = es_client.count_documents(idx)
                    console.print(f"  - {idx} ({doc_count:,} documents)")
                console.print()

            migrator = ESToOceanBaseMigrator(
                es_client=es_client,
                ob_client=ob_client,
                progress_dir=progress_dir,
            )

            total_success = 0
            total_failed = 0

            for es_index, ob_table in indices_to_migrate:
                console.print(f"\n[bold blue]{'='*60}[/]")
                console.print(f"[bold]Migrating: {es_index} -> {self.ob_database}.{ob_table}[/]")
                console.print(f"[bold blue]{'='*60}[/]")

                result = migrator.migrate(
                    es_index=es_index,
                    ob_table=ob_table,
                    batch_size=batch_size,
                    resume=resume,
                    verify_after=verify,
                )
                
                if result["success"]:
                    total_success += 1
                else:
                    total_failed += 1

            if len(indices_to_migrate) > 1:
                console.print(f"\n[bold]{'='*60}[/]")
                console.print(f"[bold]Migration Summary[/]")
                console.print(f"[bold]{'='*60}[/]")
                console.print(f"  Total indices: {len(indices_to_migrate)}")
                console.print(f"  [green]Successful: {total_success}[/]")
                if total_failed > 0:
                    console.print(f"  [red]Failed: {total_failed}[/]")

            if total_failed == 0:
                console.print("\n[bold green]All migrations completed successfully![/]")
            else:
                console.print(f"\n[bold red]{total_failed} migration(s) failed[/]")

        except Exception as e:
            console.print(f"[bold red]Error: {e}[/]")
            if self.verbose:
                console.print_exception()
        finally:
            self._close_clients()

    def verify(self, index: str = None, table: str = None, sample_size: int = 100):
        """Verify migration data consistency.
        
        Args:
            index: ES index name (optional, auto-discover if not specified)
            table: OceanBase table name (optional, same as index if not specified)
            sample_size: Sample size for verification (default: 100)
        """
        try:
            es_client = self._get_es_client()
            ob_client = self._get_ob_client()
            
            verifier = MigrationVerifier(es_client, ob_client)
            
            # Determine indices to verify
            if index:
                # Verify single index
                indices_to_verify = [(index, table or index)]
            else:
                # Auto-discover all ragflow_* indices
                console.print("[bold]Discovering RAGFlow indices...[/]")
                ragflow_indices = es_client.list_ragflow_indices()
                
                if not ragflow_indices:
                    console.print("[yellow]No ragflow_* indices found[/]")
                    return
                
                indices_to_verify = [(idx, idx) for idx in ragflow_indices]
                console.print(f"[green]Found {len(indices_to_verify)} indices to verify[/]\n")
            
            # Verify each index
            total_passed = 0
            total_failed = 0
            
            for es_index, ob_table in indices_to_verify:
                console.print(f"[bold blue]{'='*60}[/]")
                console.print(f"[bold]Verifying: {es_index} <-> {ob_table}[/]")
                console.print(f"[bold blue]{'='*60}[/]")
                
                try:
                    result = verifier.verify(es_index, ob_table, sample_size=sample_size)
                    console.print(verifier.generate_report(result))
                    
                    if result.passed:
                        total_passed += 1
                    else:
                        total_failed += 1
                except Exception as e:
                    console.print(f"[red]Error verifying {es_index}: {e}[/]")
                    total_failed += 1
            
            # Summary
            if len(indices_to_verify) > 1:
                console.print(f"\n[bold]{'='*60}[/]")
                console.print(f"[bold]Verification Summary[/]")
                console.print(f"[bold]{'='*60}[/]")
                console.print(f"  Total: {len(indices_to_verify)}")
                console.print(f"  [green]Passed: {total_passed}[/]")
                if total_failed > 0:
                    console.print(f"  [red]Failed: {total_failed}[/]")

        except Exception as e:
            console.print(f"[bold red]Error: {e}[/]")
            if self.verbose:
                console.print_exception()
        finally:
            self._close_clients()

    def list_indices(self):
        """List all RAGFlow indices (ragflow_*) in Elasticsearch."""
        try:
            es_client = self._get_es_client()
            console.print(f"\n[bold]RAGFlow Indices in Elasticsearch ({self.es_host}:{self.es_port})[/]\n")

            indices = es_client.list_ragflow_indices()

            if not indices:
                console.print("[yellow]No ragflow_* indices found[/]")
                return

            tbl = Table(title="RAGFlow Indices")
            tbl.add_column("Index Name", style="cyan")
            tbl.add_column("Document Count", style="green", justify="right")
            tbl.add_column("Type", style="yellow")

            total_docs = 0
            for idx in indices:
                doc_count = es_client.count_documents(idx)
                total_docs += doc_count
                
                if idx.startswith("ragflow_doc_meta_"):
                    idx_type = "Metadata"
                elif idx.startswith("ragflow_"):
                    idx_type = "Document Chunks"
                else:
                    idx_type = "Unknown"
                
                tbl.add_row(idx, f"{doc_count:,}", idx_type)

            tbl.add_row("", "", "")
            tbl.add_row("[bold]Total[/]", f"[bold]{total_docs:,}[/]", f"[bold]{len(indices)} indices[/]")

            console.print(tbl)

        except Exception as e:
            console.print(f"[bold red]Error: {e}[/]")
            if self.verbose:
                console.print_exception()
        finally:
            self._close_clients()

    def list_kb(self, index: str):
        """List all knowledge bases in an ES index.
        
        Args:
            index: ES index name
        """
        try:
            es_client = self._get_es_client()
            console.print(f"\n[bold]Knowledge Bases in index: {index}[/]\n")

            agg_result = es_client.aggregate_field(index, "kb_id")
            buckets = agg_result.get("buckets", [])

            if not buckets:
                console.print("[yellow]No knowledge bases found[/]")
                return

            tbl = Table(title="Knowledge Bases")
            tbl.add_column("KB ID", style="cyan")
            tbl.add_column("Document Count", style="green", justify="right")

            total_docs = 0
            for bucket in buckets:
                tbl.add_row(bucket["key"], f"{bucket['doc_count']:,}")
                total_docs += bucket["doc_count"]

            tbl.add_row("", "")
            tbl.add_row("[bold]Total[/]", f"[bold]{total_docs:,}[/]")

            console.print(tbl)
            console.print(f"\nTotal knowledge bases: {len(buckets)}")

        except Exception as e:
            console.print(f"[bold red]Error: {e}[/]")
            if self.verbose:
                console.print_exception()
        finally:
            self._close_clients()

    def sample(self, index: str, size: int = 5):
        """Show sample documents from ES index.
        
        Args:
            index: ES index name
            size: Number of samples (default: 5)
        """
        try:
            es_client = self._get_es_client()
            docs = es_client.get_sample_documents(index, size)

            console.print(f"\n[bold]Sample documents from {index}[/]")
            console.print()

            for i, doc in enumerate(docs, 1):
                console.print(f"[bold cyan]Document {i}[/]")
                console.print(f"  _id: {doc.get('_id')}")
                console.print(f"  kb_id: {doc.get('kb_id')}")
                console.print(f"  doc_id: {doc.get('doc_id')}")
                console.print(f"  docnm_kwd: {doc.get('docnm_kwd')}")
                
                vector_fields = [k for k in doc.keys() if k.startswith("q_") and k.endswith("_vec")]
                if vector_fields:
                    for vf in vector_fields:
                        vec = doc.get(vf)
                        if vec:
                            console.print(f"  {vf}: [{len(vec)} dimensions]")
                
                content = doc.get("content_with_weight", "")
                if content:
                    if isinstance(content, dict):
                        content = json.dumps(content, ensure_ascii=False)
                    preview = content[:100] + "..." if len(str(content)) > 100 else content
                    console.print(f"  content: {preview}")
                
                console.print()

        except Exception as e:
            console.print(f"[bold red]Error: {e}[/]")
            if self.verbose:
                console.print_exception()
        finally:
            self._close_clients()


def main_cli():
    """Entry point for the CLI."""
    fire.Fire(ESOBMigrate)


if __name__ == "__main__":
    main_cli()
