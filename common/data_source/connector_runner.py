import re
import sys
import time
import logging
from collections.abc import Generator
from datetime import datetime
from typing import Generic
from typing import TypeVar
from common.data_source.interfaces import (
    BaseConnector,
    CheckpointedConnector,
    CheckpointedConnectorWithPermSync,
    CheckpointOutput,
    LoadConnector,
    PollConnector,
)
from common.data_source.models import ConnectorCheckpoint, ConnectorFailure, Document


TimeRange = tuple[datetime, datetime]

CT = TypeVar("CT", bound=ConnectorCheckpoint)

# Substring patterns (lowercased) that mark a ``f_locals`` key as
# credential-bearing. ``ConnectorRunner.run`` redacts any variable whose
# name matches one of these patterns before logging, so that an OAuth
# access_token / refresh_token / client_secret / api_key / etc. that a
# connector holds in memory cannot be written to the log file on any
# transient connector error.
#
# The match is case-insensitive substring on the key name. This is
# intentionally a small, explicit allowlist -- we are not trying to do
# perfect secrets detection on variable values, only to drop the
# well-known credential field names from the auto-dump of locals. Pure
# password strings inside a non-credential variable name are out of
# scope.
_SENSITIVE_KEY_PATTERNS = (
    "access_token",
    "refresh_token",
    "id_token",
    "client_secret",
    "client_id",
    "api_key",
    "apikey",
    "secret",
    "password",
    "token",
    "credentials",
    "credential",
    "authorization",
    "auth",
    "cookie",
    "session",
    # Provider/SDK credential field names that don't already substring-match
    # any of the patterns above. ``secret`` already catches ``secret_key`` /
    # ``aws_secret_access_key`` etc., so we only add the ones that would
    # otherwise leak.
    "private_key",  # google_service_account_private_key, privateKey, ...
    "access_key",  # aws_access_key_id, access_key, AccessKey, ...
    "creds",  # creds, creds_dict, service_account_creds, ...
)


def _is_sensitive_key(name: object) -> bool:
    """Return True if ``name`` looks like a credential / token / secret field.

    Used by :meth:`ConnectorRunner.run` to redact ``f_locals`` entries
    before logging them. Matching is case-insensitive substring on a
    small allowlist of well-known credential-related field names -- see
    ``_SENSITIVE_KEY_PATTERNS`` for the full list. Non-string keys
    (e.g. tuple indices from ``f_locals``) are never sensitive.

    Separators (``_``, ``-``) are stripped from the input before
    matching so that ``privateKey``, ``private_key``, ``Private-Key``
    etc. all collapse to the same canonical form and a single pattern
    like ``private_key`` catches them all.
    """
    if not isinstance(name, str):
        return False
    lowered = re.sub(r"[_\-]", "", name.lower())
    return any(pat.replace("_", "").replace("-", "") in lowered for pat in _SENSITIVE_KEY_PATTERNS)


_REDACTED = "<redacted>"


def batched_doc_ids(
    checkpoint_connector_generator: CheckpointOutput[CT],
    batch_size: int,
) -> Generator[set[str], None, None]:
    batch: set[str] = set()
    for document, failure, next_checkpoint in CheckpointOutputWrapper[CT]()(checkpoint_connector_generator):
        if document is not None:
            batch.add(document.id)
        elif failure and failure.failed_document and failure.failed_document.document_id:
            batch.add(failure.failed_document.document_id)

        if len(batch) >= batch_size:
            yield batch
            batch = set()
    if len(batch) > 0:
        yield batch


class CheckpointOutputWrapper(Generic[CT]):
    """
    Wraps a CheckpointOutput generator to give things back in a more digestible format,
    specifically for Document outputs.
    The connector format is easier for the connector implementor (e.g. it enforces exactly
    one new checkpoint is returned AND that the checkpoint is at the end), thus the different
    formats.
    """

    def __init__(self) -> None:
        self.next_checkpoint: CT | None = None

    def __call__(
        self,
        checkpoint_connector_generator: CheckpointOutput[CT],
    ) -> Generator[
        tuple[Document | None, ConnectorFailure | None, CT | None],
        None,
        None,
    ]:
        # grabs the final return value and stores it in the `next_checkpoint` variable
        def _inner_wrapper(
            checkpoint_connector_generator: CheckpointOutput[CT],
        ) -> CheckpointOutput[CT]:
            self.next_checkpoint = yield from checkpoint_connector_generator
            return self.next_checkpoint  # not used

        for document_or_failure in _inner_wrapper(checkpoint_connector_generator):
            if isinstance(document_or_failure, Document):
                yield document_or_failure, None, None
            elif isinstance(document_or_failure, ConnectorFailure):
                yield None, document_or_failure, None
            else:
                raise ValueError(f"Invalid document_or_failure type: {type(document_or_failure)}")

        if self.next_checkpoint is None:
            raise RuntimeError("Checkpoint is None. This should never happen - the connector should always return a checkpoint.")

        yield None, None, self.next_checkpoint


class ConnectorRunner(Generic[CT]):
    """
    Handles:
        - Batching
        - Additional exception logging
        - Combining different connector types to a single interface
    """

    def __init__(
        self,
        connector: BaseConnector,
        batch_size: int,
        # cannot be True for non-checkpointed connectors
        include_permissions: bool,
        time_range: TimeRange | None = None,
    ):
        if not isinstance(connector, CheckpointedConnector) and include_permissions:
            raise ValueError("include_permissions cannot be True for non-checkpointed connectors")

        self.connector = connector
        self.time_range = time_range
        self.batch_size = batch_size
        self.include_permissions = include_permissions

        self.doc_batch: list[Document] = []

    def run(
        self, checkpoint: CT
    ) -> Generator[
        tuple[list[Document] | None, ConnectorFailure | None, CT | None],
        None,
        None,
    ]:
        """Adds additional exception logging to the connector."""
        try:
            if isinstance(self.connector, CheckpointedConnector):
                if self.time_range is None:
                    raise ValueError("time_range is required for CheckpointedConnector")

                start = time.monotonic()
                if self.include_permissions:
                    if not isinstance(self.connector, CheckpointedConnectorWithPermSync):
                        raise ValueError("Connector does not support permission syncing")
                    load_from_checkpoint = self.connector.load_from_checkpoint_with_perm_sync
                else:
                    load_from_checkpoint = self.connector.load_from_checkpoint
                checkpoint_connector_generator = load_from_checkpoint(
                    start=self.time_range[0].timestamp(),
                    end=self.time_range[1].timestamp(),
                    checkpoint=checkpoint,
                )
                next_checkpoint: CT | None = None
                # this is guaranteed to always run at least once with next_checkpoint being non-None
                for document, failure, next_checkpoint in CheckpointOutputWrapper[CT]()(checkpoint_connector_generator):
                    if document is not None and isinstance(document, Document):
                        self.doc_batch.append(document)

                    if failure is not None:
                        yield None, failure, None

                    if len(self.doc_batch) >= self.batch_size:
                        yield self.doc_batch, None, None
                        self.doc_batch = []

                # yield remaining documents
                if len(self.doc_batch) > 0:
                    yield self.doc_batch, None, None
                    self.doc_batch = []

                yield None, None, next_checkpoint

                logging.debug(f"Connector took {time.monotonic() - start} seconds to get to the next checkpoint.")

            else:
                finished_checkpoint = self.connector.build_dummy_checkpoint()
                finished_checkpoint.has_more = False

                if isinstance(self.connector, PollConnector):
                    if self.time_range is None:
                        raise ValueError("time_range is required for PollConnector")

                    for document_batch in self.connector.poll_source(
                        start=self.time_range[0].timestamp(),
                        end=self.time_range[1].timestamp(),
                    ):
                        yield document_batch, None, None

                    yield None, None, finished_checkpoint
                elif isinstance(self.connector, LoadConnector):
                    for document_batch in self.connector.load_from_state():
                        yield document_batch, None, None

                    yield None, None, finished_checkpoint
                else:
                    raise ValueError(f"Invalid connector. type: {type(self.connector)}")
        except Exception:
            exc_type, _, exc_traceback = sys.exc_info()

            # Traverse the traceback to find the last frame where the exception was raised
            tb = exc_traceback
            if tb is None:
                logging.error("No traceback found for exception")
                raise

            while tb.tb_next:
                tb = tb.tb_next  # Move to the next frame in the traceback

            # Get the local variables from the frame where the exception occurred
            # and redact any credential-bearing entries before logging. See
            # ``_is_sensitive_key`` for the allowlist; this prevents OAuth
            # access_token / refresh_token / client_secret / api_key from being
            # written to the log file on any connector error. The redacted value
            # is the literal string ``<redacted>`` so an operator debugging the
            # log can see that a field was hidden, not that it just disappeared.
            local_vars = tb.tb_frame.f_locals
            redacted_vars = {key: (_REDACTED if _is_sensitive_key(key) else value) for key, value in local_vars.items()}
            local_vars_str = "\n".join(f"{key}: {value}" for key, value in redacted_vars.items())
            logging.error(f"Error in connector. type: {exc_type};\nlocal_vars below -> \n{local_vars_str[:1024]}")
            raise
