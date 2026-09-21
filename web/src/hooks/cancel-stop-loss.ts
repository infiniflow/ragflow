/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

// Stop-loss for cancels that never converge: a stuck STOPPING status must not
// keep the document list on the fast poll interval forever. Once a cancel has
// been pending for CANCEL_STOP_LOSS_MS the list falls back to the normal
// cadence and one cancel request is re-sent for the overdue documents.
export const CANCEL_STOP_LOSS_MS = 30_000;

// Window start per document: set when the user cancels, or at the first sight
// of a stopping document that predates this session (e.g. a page reload), so
// an orphaned cancel still downgrades once its window has elapsed.
const cancelRequestedAt = new Map<string, number>();
// Only cancels issued by this session are retried; adopted ones are not.
const userRequestedCancelIds = new Set<string>();
// Latch: a retry is sent at most once per cancel request.
const cancelRetrySent = new Set<string>();

// Re-arming an id resets its window and clears the retry latch, so a new
// cancel click starts from a clean slate.
export const markCancelRequested = (documentIds: string[]) => {
  const now = Date.now();
  documentIds.forEach((id) => {
    cancelRequestedAt.set(id, now);
    userRequestedCancelIds.add(id);
    cancelRetrySent.delete(id);
  });
};

export const getCancelRequestInterval = (stoppingIds: string[]) => {
  const now = Date.now();
  // An untracked id counts as freshly observed, so a stopping document that
  // first appears here still gets a full window of fast polling.
  return stoppingIds.some(
    (id) => now - (cancelRequestedAt.get(id) ?? now) < CANCEL_STOP_LOSS_MS,
  )
    ? 1000
    : 5000;
};

// Called on every list poll with the ids currently stopping: starts the
// window for ids first seen here, drops trackers for ids that left the
// stopping state, and returns the overdue cancels whose retry has not been
// sent yet.
export const observeStoppingDocuments = (stoppingIds: string[]) => {
  const now = Date.now();
  const stoppingIdSet = new Set(stoppingIds);

  cancelRequestedAt.forEach((_, id) => {
    if (!stoppingIdSet.has(id)) {
      cancelRequestedAt.delete(id);
      userRequestedCancelIds.delete(id);
      cancelRetrySent.delete(id);
    }
  });

  return stoppingIds.filter((id) => {
    const requestedAt = cancelRequestedAt.get(id);
    if (requestedAt === undefined) {
      cancelRequestedAt.set(id, now);
      return false;
    }
    if (
      !userRequestedCancelIds.has(id) ||
      cancelRetrySent.has(id) ||
      now - requestedAt < CANCEL_STOP_LOSS_MS
    ) {
      return false;
    }
    cancelRetrySent.add(id);
    return true;
  });
};
