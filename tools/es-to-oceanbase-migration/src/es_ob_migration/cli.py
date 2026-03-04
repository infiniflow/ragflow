"""
CLI entry point for RAGFlow ES to OceanBase migration tool.
"""

import json
import logging
import sys

import click
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
    level = logging.DEBUG if verbose else logging.INFO
    logging.basicConfig(
        level=level,
        format="%(message)s",
        datefmt="[%X]",
        handlers=[RichHandler(rich_tracebacks=True, console=console)],
    )


@click.group()
@click.option("-v", "--verbose", is_flag=True, help="Enable verbose logging")
@click.pass_context
def main(ctx, verbose):
    """RAGFlow ES to OceanBase Migration Tool.

    Migrate RAGFlow data from Elasticsearch 8+ to OceanBase with schema conversion,
    vector data mapping, batch import, and resume capability.
    
    This tool is specifically designed for RAGFlow's data structure.
    """
    ctx.ensure_object(dict)
    ctx.obj["verbose"] = verbose
    setup_logging(verbose)


@main.command()
@click.option("--es-host", default="localhost", help="Elasticsearch host")
@click.option("--es-port", default=9200, type=int, help="Elasticsearch port")
@click.option("--es-user", default=None, help="Elasticsearch username")
@click.option("--es-password", default=None, help="Elasticsearch password")
@click.option("--es-api-key", default=None, help="Elasticsearch API key")
@click.option("--ob-host", default="localhost", help="OceanBase host")
@click.option("--ob-port", default=2881, type=int, help="OceanBase port")
@click.option("--ob-user", default="root@test", help="OceanBase user (format: user@tenant)")
@click.option("--ob-password", default="", help="OceanBase password")
@click.option("--ob-database", default="test", help="OceanBase database")
@click.option("--index", "-i", default=None, help="Source ES index name (omit to migrate all ragflow_* indices)")
@click.option("--table", "-t", default=None, help="Target OceanBase table name (omit to use same name as index)")
@click.option("--batch-size", default=1000, type=int, help="Batch size for migration")
@click.option("--resume", is_flag=True, help="Resume from previous progress")
@click.option("--verify/--no-verify", default=True, help="Verify after migration")
@click.option("--progress-dir", default=".migration_progress", help="Progress file directory")
@click.pass_context
def migrate(
    ctx,
    es_host,
    es_port,
    es_user,
    es_password,
    es_api_key,
    ob_host,
    ob_port,
    ob_user,
    ob_password,
    ob_database,
    index,
    table,
    batch_size,
    resume,
    verify,
    progress_dir,
):
    """Run RAGFlow data migration from Elasticsearch to OceanBase.
    
    If --index is omitted, all indices starting with 'ragflow_' will be migrated.
    If --table is omitted, the same name as the source index will be used.
    """
    console.print("[bold]RAGFlow ES to OceanBase Migration[/]")

    try:
        # Initialize ES client first to discover indices if needed
        es_client = ESClient(
            host=es_host,
            port=es_port,
            username=es_user,
            password=es_password,
            api_key=es_api_key,
        )

        ob_client = OBClient(
            host=ob_host,
            port=ob_port,
            user=ob_user,
            password=ob_password,
            database=ob_database,
        )

        # Determine indices to migrate
        if index:
            # Single index specified
            indices_to_migrate = [(index, table if table else index)]
        else:
            # Auto-discover all ragflow_* indices
            console.print("\n[cyan]Discovering RAGFlow indices...[/]")
            ragflow_indices = es_client.list_ragflow_indices()
            
            if not ragflow_indices:
                console.print("[yellow]No ragflow_* indices found in Elasticsearch[/]")
                sys.exit(0)
            
            # Each index maps to a table with the same name
            indices_to_migrate = [(idx, idx) for idx in ragflow_indices]
            
            console.print(f"[green]Found {len(indices_to_migrate)} RAGFlow indices:[/]")
            for idx, _ in indices_to_migrate:
                doc_count = es_client.count_documents(idx)
                console.print(f"  - {idx} ({doc_count:,} documents)")
            console.print()

        # Initialize migrator
        migrator = ESToOceanBaseMigrator(
            es_client=es_client,
            ob_client=ob_client,
            progress_dir=progress_dir,
        )

        # Track overall results
        total_success = 0
        total_failed = 0
        results = []

        # Migrate each index
        for es_index, ob_table in indices_to_migrate:
            console.print(f"\n[bold blue]{'='*60}[/]")
            console.print(f"[bold]Migrating: {es_index} -> {ob_database}.{ob_table}[/]")
            console.print(f"[bold blue]{'='*60}[/]")

            result = migrator.migrate(
                es_index=es_index,
                ob_table=ob_table,
                batch_size=batch_size,
                resume=resume,
                verify_after=verify,
            )
            
            results.append(result)
            if result["success"]:
                total_success += 1
            else:
                total_failed += 1

        # Summary for multiple indices
        if len(indices_to_migrate) > 1:
            console.print(f"\n[bold]{'='*60}[/]")
            console.print("[bold]Migration Summary[/]")
            console.print(f"[bold]{'='*60}[/]")
            console.print(f"  Total indices: {len(indices_to_migrate)}")
            console.print(f"  [green]Successful: {total_success}[/]")
            if total_failed > 0:
                console.print(f"  [red]Failed: {total_failed}[/]")

        # Exit code based on results
        if total_failed == 0:
            console.print("\n[bold green]All migrations completed successfully![/]")
            sys.exit(0)
        else:
            console.print(f"\n[bold red]{total_failed} migration(s) failed[/]")
            sys.exit(1)

    except Exception as e:
        console.print(f"[bold red]Error: {e}[/]")
        if ctx.obj.get("verbose"):
            console.print_exception()
        sys.exit(1)
    finally:
        # Cleanup
        if "es_client" in locals():
            es_client.close()
        if "ob_client" in locals():
            ob_client.close()


@main.command()
@click.option("--es-host", default="localhost", help="Elasticsearch host")
@click.option("--es-port", default=9200, type=int, help="Elasticsearch port")
@click.option("--es-user", default=None, help="Elasticsearch username")
@click.option("--es-password", default=None, help="Elasticsearch password")
@click.option("--index", "-i", required=True, help="ES index name")
@click.option("--output", "-o", default=None, help="Output file (JSON)")
@click.pass_context
def schema(ctx, es_host, es_port, es_user, es_password, index, output):
    """Preview RAGFlow schema analysis from ES mapping."""
    try:
        es_client = ESClient(
            host=es_host,
            port=es_port,
            username=es_user,
            password=es_password,
        )

        # Dummy OB client for schema preview
        ob_client = None

        migrator = ESToOceanBaseMigrator(es_client, ob_client if ob_client else OBClient.__new__(OBClient))
        # Directly use schema converter
        from .schema import RAGFlowSchemaConverter
        converter = RAGFlowSchemaConverter()
        
        es_mapping = es_client.get_index_mapping(index)
        analysis = converter.analyze_es_mapping(es_mapping)
        column_defs = converter.get_column_definitions()

        # Display analysis
        console.print(f"\n[bold]ES Index Analysis: {index}[/]\n")
        
        # Known RAGFlow fields
        console.print(f"[green]Known RAGFlow fields:[/] {len(analysis['known_fields'])}")
        
        # Vector fields
        if analysis['vector_fields']:
            console.print("\n[cyan]Vector fields detected:[/]")
            for vf in analysis['vector_fields']:
                console.print(f"  - {vf['name']} (dimension: {vf['dimension']})")
        
        # Unknown fields
        if analysis['unknown_fields']:
            console.print("\n[yellow]Unknown fields (will be stored in 'extra'):[/]")
            for uf in analysis['unknown_fields']:
                console.print(f"  - {uf}")

        # Display RAGFlow column schema
        console.print(f"\n[bold]RAGFlow OceanBase Schema ({len(column_defs)} columns):[/]\n")
        
        table = Table(title="Column Definitions")
        table.add_column("Column Name", style="cyan")
        table.add_column("OB Type", style="green")
        table.add_column("Nullable", style="yellow")
        table.add_column("Special", style="magenta")

        for col in column_defs[:20]:  # Show first 20
            special = []
            if col.get("is_primary"):
                special.append("PK")
            if col.get("index"):
                special.append("IDX")
            if col.get("is_array"):
                special.append("ARRAY")
            if col.get("is_vector"):
                special.append("VECTOR")
            
            table.add_row(
                col["name"],
                col["ob_type"],
                "Yes" if col.get("nullable", True) else "No",
                ", ".join(special) if special else "-",
            )

        if len(column_defs) > 20:
            table.add_row("...", f"({len(column_defs) - 20} more)", "", "")

        console.print(table)

        # Save to file if requested
        if output:
            preview = {
                "es_index": index,
                "es_mapping": es_mapping,
                "analysis": analysis,
                "ob_columns": column_defs,
            }
            with open(output, "w") as f:
                json.dump(preview, f, indent=2, default=str)
            console.print(f"\nSchema saved to {output}")

    except Exception as e:
        console.print(f"[bold red]Error: {e}[/]")
        if ctx.obj.get("verbose"):
            console.print_exception()
        sys.exit(1)
    finally:
        if "es_client" in locals():
            es_client.close()


@main.command()
@click.option("--es-host", default="localhost", help="Elasticsearch host")
@click.option("--es-port", default=9200, type=int, help="Elasticsearch port")
@click.option("--ob-host", default="localhost", help="OceanBase host")
@click.option("--ob-port", default=2881, type=int, help="OceanBase port")
@click.option("--ob-user", default="root@test", help="OceanBase user")
@click.option("--ob-password", default="", help="OceanBase password")
@click.option("--ob-database", default="test", help="OceanBase database")
@click.option("--index", "-i", required=True, help="Source ES index name")
@click.option("--table", "-t", required=True, help="Target OceanBase table name")
@click.option("--sample-size", default=100, type=int, help="Sample size for verification")
@click.pass_context
def verify(
    ctx,
    es_host,
    es_port,
    ob_host,
    ob_port,
    ob_user,
    ob_password,
    ob_database,
    index,
    table,
    sample_size,
):
    """Verify migration data consistency."""
    try:
        es_client = ESClient(host=es_host, port=es_port)
        ob_client = OBClient(
            host=ob_host,
            port=ob_port,
            user=ob_user,
            password=ob_password,
            database=ob_database,
        )

        verifier = MigrationVerifier(es_client, ob_client)
        result = verifier.verify(
            index, table, 
            sample_size=sample_size,
        )

        console.print(verifier.generate_report(result))

        sys.exit(0 if result.passed else 1)

    except Exception as e:
        console.print(f"[bold red]Error: {e}[/]")
        if ctx.obj.get("verbose"):
            console.print_exception()
        sys.exit(1)
    finally:
        if "es_client" in locals():
            es_client.close()
        if "ob_client" in locals():
            ob_client.close()


@main.command("list-indices")
@click.option("--es-host", default="localhost", help="Elasticsearch host")
@click.option("--es-port", default=9200, type=int, help="Elasticsearch port")
@click.option("--es-user", default=None, help="Elasticsearch username")
@click.option("--es-password", default=None, help="Elasticsearch password")
@click.pass_context
def list_indices(ctx, es_host, es_port, es_user, es_password):
    """List all RAGFlow indices (ragflow_*) in Elasticsearch."""
    try:
        es_client = ESClient(
            host=es_host,
            port=es_port,
            username=es_user,
            password=es_password,
        )

        console.print(f"\n[bold]RAGFlow Indices in Elasticsearch ({es_host}:{es_port})[/]\n")

        indices = es_client.list_ragflow_indices()

        if not indices:
            console.print("[yellow]No ragflow_* indices found[/]")
            return

        table = Table(title="RAGFlow Indices")
        table.add_column("Index Name", style="cyan")
        table.add_column("Document Count", style="green", justify="right")
        table.add_column("Type", style="yellow")

        total_docs = 0
        for idx in indices:
            doc_count = es_client.count_documents(idx)
            total_docs += doc_count
            
            # Determine index type
            if idx.startswith("ragflow_doc_meta_"):
                idx_type = "Metadata"
            elif idx.startswith("ragflow_"):
                idx_type = "Document Chunks"
            else:
                idx_type = "Unknown"
            
            table.add_row(idx, f"{doc_count:,}", idx_type)

        table.add_row("", "", "")
        table.add_row("[bold]Total[/]", f"[bold]{total_docs:,}[/]", f"[bold]{len(indices)} indices[/]")

        console.print(table)

    except Exception as e:
        console.print(f"[bold red]Error: {e}[/]")
        if ctx.obj.get("verbose"):
            console.print_exception()
        sys.exit(1)
    finally:
        if "es_client" in locals():
            es_client.close()


@main.command("list-kb")
@click.option("--es-host", default="localhost", help="Elasticsearch host")
@click.option("--es-port", default=9200, type=int, help="Elasticsearch port")
@click.option("--es-user", default=None, help="Elasticsearch username")
@click.option("--es-password", default=None, help="Elasticsearch password")
@click.option("--index", "-i", required=True, help="ES index name")
@click.pass_context
def list_kb(ctx, es_host, es_port, es_user, es_password, index):
    """List all knowledge bases in an ES index."""
    try:
        es_client = ESClient(
            host=es_host,
            port=es_port,
            username=es_user,
            password=es_password,
        )

        console.print(f"\n[bold]Knowledge Bases in index: {index}[/]\n")

        # Get kb_id aggregation
        agg_result = es_client.aggregate_field(index, "kb_id")
        buckets = agg_result.get("buckets", [])

        if not buckets:
            console.print("[yellow]No knowledge bases found[/]")
            return

        table = Table(title="Knowledge Bases")
        table.add_column("KB ID", style="cyan")
        table.add_column("Document Count", style="green", justify="right")

        total_docs = 0
        for bucket in buckets:
            table.add_row(
                bucket["key"],
                f"{bucket['doc_count']:,}",
            )
            total_docs += bucket["doc_count"]

        table.add_row("", "")
        table.add_row("[bold]Total[/]", f"[bold]{total_docs:,}[/]")

        console.print(table)
        console.print(f"\nTotal knowledge bases: {len(buckets)}")

    except Exception as e:
        console.print(f"[bold red]Error: {e}[/]")
        if ctx.obj.get("verbose"):
            console.print_exception()
        sys.exit(1)
    finally:
        if "es_client" in locals():
            es_client.close()


@main.command()
@click.option("--es-host", default="localhost", help="Elasticsearch host")
@click.option("--es-port", default=9200, type=int, help="Elasticsearch port")
@click.option("--ob-host", default="localhost", help="OceanBase host")
@click.option("--ob-port", default=2881, type=int, help="OceanBase port")
@click.option("--ob-user", default="root@test", help="OceanBase user")
@click.option("--ob-password", default="", help="OceanBase password")
@click.pass_context
def status(ctx, es_host, es_port, ob_host, ob_port, ob_user, ob_password):
    """Check connection status to ES and OceanBase."""
    console.print("[bold]Connection Status[/]\n")

    # Check ES
    try:
        es_client = ESClient(host=es_host, port=es_port)
        health = es_client.health_check()
        info = es_client.get_cluster_info()
        console.print(f"[green]Elasticsearch ({es_host}:{es_port}): Connected[/]")
        console.print(f"  Cluster: {health.get('cluster_name')}")
        console.print(f"  Status:  {health.get('status')}")
        console.print(f"  Version: {info.get('version', {}).get('number', 'unknown')}")
        
        # List indices
        indices = es_client.list_indices("*")
        console.print(f"  Indices: {len(indices)}")
        
        es_client.close()
    except Exception as e:
        console.print(f"[red]Elasticsearch ({es_host}:{es_port}): Failed[/]")
        console.print(f"  Error: {e}")

    console.print()

    # Check OceanBase
    try:
        ob_client = OBClient(
            host=ob_host,
            port=ob_port,
            user=ob_user,
            password=ob_password,
        )
        if ob_client.health_check():
            version = ob_client.get_version()
            console.print(f"[green]OceanBase ({ob_host}:{ob_port}): Connected[/]")
            console.print(f"  Version: {version}")
        else:
            console.print(f"[red]OceanBase ({ob_host}:{ob_port}): Health check failed[/]")
        ob_client.close()
    except Exception as e:
        console.print(f"[red]OceanBase ({ob_host}:{ob_port}): Failed[/]")
        console.print(f"  Error: {e}")


@main.command()
@click.option("--es-host", default="localhost", help="Elasticsearch host")
@click.option("--es-port", default=9200, type=int, help="Elasticsearch port")
@click.option("--index", "-i", required=True, help="ES index name")
@click.option("--size", "-n", default=5, type=int, help="Number of samples")
@click.pass_context
def sample(ctx, es_host, es_port, index, size):
    """Show sample documents from ES index."""
    try:
        es_client = ESClient(host=es_host, port=es_port)

        docs = es_client.get_sample_documents(index, size)

        console.print(f"\n[bold]Sample documents from {index}[/]")
        console.print()

        for i, doc in enumerate(docs, 1):
            console.print(f"[bold cyan]Document {i}[/]")
            console.print(f"  _id: {doc.get('_id')}")
            console.print(f"  kb_id: {doc.get('kb_id')}")
            console.print(f"  doc_id: {doc.get('doc_id')}")
            console.print(f"  docnm_kwd: {doc.get('docnm_kwd')}")
            
            # Check for vector fields
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

        es_client.close()

    except Exception as e:
        console.print(f"[bold red]Error: {e}[/]")
        if ctx.obj.get("verbose"):
            console.print_exception()
        sys.exit(1)


if __name__ == "__main__":
    main()
