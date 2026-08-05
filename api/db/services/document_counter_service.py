#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import logging

from api.db.db_models import DB, Document, Knowledgebase


def release_reparse_counters(doc_id):
    """Roll back a document's chunk, token, and duration counters and the owning
    knowledgebase's chunk/token totals so a re-parse starts from zero.

    The counters are re-read under a ``FOR UPDATE`` row lock in the same
    transaction as the decrement, so the release subtracts the row's committed
    value at release time rather than a request-time snapshot a concurrent worker
    may have already moved past. ``increment_chunk_num`` updates both ledgers
    together. This does not serialize a worker that writes its final counts in a
    separate transaction after the release commits; fully closing the
    stop-parse-during-parse race needs a worker-side cancel check.

    Raises ``LookupError`` if the document row no longer exists so callers can
    surface a not-found result.
    """
    with DB.atomic():
        fresh = Document.select().where(Document.id == doc_id).for_update().first()
        if fresh is None:
            raise LookupError(doc_id)
        if not (fresh.token_num or fresh.chunk_num or fresh.process_duration):
            logging.debug("release_reparse_counters: nothing to release for document %s", doc_id)
            return
        # Decrement directly inside the outer transaction. Do NOT call
        # DocumentService.increment_chunk_num here: it is decorated with
        # @DB.connection_context(), which closes the shared connection on exit.
        # That would raise "Attempting to close database while transaction is
        # open." because the outer DB.atomic() block is still open.
        num = (
            Document.update(
                token_num=Document.token_num - fresh.token_num,
                chunk_num=Document.chunk_num - fresh.chunk_num,
                process_duration=Document.process_duration - fresh.process_duration,
            )
            .where((Document.id == fresh.id) & (Document.kb_id == fresh.kb_id))
            .execute()
        )
        if num == 0:
            raise LookupError("Document not found which is supposed to be there")
        # Fetch and lock the knowledgebase row so existence is checked reliably.
        # A bare UPDATE's num == 0 is ambiguous in MySQL (missing row vs. an
        # unchanged update both report 0), so rely on the locked fetch instead.
        kb = Knowledgebase.select().where(Knowledgebase.id == fresh.kb_id).for_update().first()
        if kb is None:
            raise LookupError("Knowledgebase not found which is supposed to be there")
        # Only touch the knowledgebase aggregate when a token/chunk delta exists;
        # otherwise the update would be a no-op that still reports num == 0.
        if fresh.token_num or fresh.chunk_num:
            Knowledgebase.update(
                token_num=Knowledgebase.token_num - fresh.token_num,
                chunk_num=Knowledgebase.chunk_num - fresh.chunk_num,
            ).where(Knowledgebase.id == fresh.kb_id).execute()
        logging.debug(
            "release_reparse_counters: released document %s (token=%s chunk=%s duration=%s)",
            doc_id,
            fresh.token_num,
            fresh.chunk_num,
            fresh.process_duration,
        )
