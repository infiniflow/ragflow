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

// The per-event message stream is no longer persisted. The hook keeps its
// signature so call sites (and the collapsed "view full log" disclosure)
// remain, but it never fetches and always yields an empty stream.
export const useIngestionMessages = (
  _datasetId: string | undefined,
  _logId: string | undefined,
  _enabled: boolean,
) => {
  return {
    data: undefined,
    items: [] as never[],
    fetchPreviousPage: async () => {},
    hasPreviousPage: false,
    isFetchingPreviousPage: false,
  };
};
