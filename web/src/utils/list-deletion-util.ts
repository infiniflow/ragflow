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

// A list page cannot tell "the current page became empty after a deletion"
// apart from "the user typed a search with no matches" just by looking at the
// list length. Delete mutations record a marker keyed by the list they feed;
// the list page consumes the marker when an empty page sends the user back to
// the first page of the unfiltered list, and only then clears the search
// keywords and the filter selection. Markers expire quickly so a deletion
// made on a surface that never renders the list afterwards cannot affect the
// list page later.

const MarkerTtlMs = 30_000;

const DeletionMarkers = new Map<string, number>();

export function markListItemsDeleted(key: string) {
  DeletionMarkers.set(key, Date.now());
}

export function discardListDeletionMarker(key: string) {
  DeletionMarkers.delete(key);
}

export function consumeListDeletionMarker(key: string): boolean {
  const markedAt = DeletionMarkers.get(key);
  if (markedAt === undefined) {
    return false;
  }
  DeletionMarkers.delete(key);
  return Date.now() - markedAt < MarkerTtlMs;
}
