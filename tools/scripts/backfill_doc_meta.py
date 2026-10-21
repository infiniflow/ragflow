#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
"""Copy legacy ``document.meta_fields`` into the per-tenant doc metadata stores.

The server does this at startup (see
``api.db.db_models.migrate_document_meta_fields``); run this instead to preview
it, to retry a failed automatic run, or to restrict it to one tenant.

Read-only unless ``--execute`` is given. Re-running is safe: the default
``--on-conflict skip`` leaves documents that already have metadata alone. The
legacy column is never dropped.

Run it inside the container so ``conf/service_conf.yaml`` and ``DOC_ENGINE``
are available.
"""

import argparse
import logging
import os
import sys

# Add project root to path for imports
PROJECT_BASE = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.path.insert(0, PROJECT_BASE)

logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s")
logger = logging.getLogger("backfill_doc_meta")


def build_parser():
    # Local so --help works without pulling in the DB layer.
    from api.db.document_meta_backfill import DEFAULT_BATCH_SIZE

    parser = argparse.ArgumentParser(
        description="Backfill document metadata from the legacy document.meta_fields column into the document engine.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=(
            "Examples:\n"
            "  python tools/scripts/backfill_doc_meta.py\n"
            "  python tools/scripts/backfill_doc_meta.py --execute\n"
            "  python tools/scripts/backfill_doc_meta.py --execute --on-conflict overwrite\n"
        ),
    )
    parser.add_argument(
        "--execute",
        "-e",
        action="store_true",
        default=False,
        help="actually write; without it the script only reports what it would do",
    )
    parser.add_argument("--batch-size", type=int, default=DEFAULT_BATCH_SIZE, help=f"documents per read page and per write batch (default: {DEFAULT_BATCH_SIZE})")
    parser.add_argument("--tenant-id", type=str, default="", help="restrict the backfill to one tenant")
    parser.add_argument(
        "--on-conflict",
        choices=["skip", "overwrite"],
        default="skip",
        help="what to do when a document already has metadata in the document engine (default: skip)",
    )
    return parser


def main():
    args = build_parser().parse_args()
    if args.batch_size < 1:
        logger.error("--batch-size must be >= 1")
        return 2

    # Import order mirrors api/ragflow_server.py: importing db_models binds the
    # pooled connection, then init_settings() resolves DOC_ENGINE and builds the
    # document engine client.
    from api.db.db_models import DB
    from common import settings

    settings.init_settings()

    if settings.docStoreConn is None:
        logger.error("document engine client is not configured; check DOC_ENGINE and conf/service_conf.yaml")
        return 2

    from api.db.document_meta_backfill import backfill_document_meta_fields, has_legacy_column

    logger.info("document engine: %s", settings.DOC_ENGINE)
    try:
        with DB.connection_context():
            if not has_legacy_column(DB):
                logger.info("nothing to do: the document table has no meta_fields column; this database was created on v0.24.0 or later")
                return 0
            stats = backfill_document_meta_fields(
                DB,
                settings.docStoreConn,
                batch_size=args.batch_size,
                tenant_id=args.tenant_id,
                on_conflict=args.on_conflict,
                execute=args.execute,
            )
    except Exception:
        logger.exception("backfill failed")
        return 1

    if not args.execute:
        logger.info("dry run: re-run with --execute to write")
    return 1 if stats["failed"] else 0


if __name__ == "__main__":
    sys.exit(main())
