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

export const enum DocumentApiAction {
  UploadDocument = 'uploadDocument',
  FetchDocumentList = 'fetchDocumentList',
  UpdateDocumentStatus = 'updateDocumentStatus',
  RunDocumentByIds = 'runDocumentByIds',
  RemoveDocument = 'removeDocument',
  SaveDocumentName = 'saveDocumentName',
  SetDocumentParser = 'setDocumentParser',
  FetchDocumentFilter = 'fetchDocumentFilter',
  FetchDocumentStatusCounts = 'fetchDocumentStatusCounts',
  CreateDocument = 'createDocument',
  FetchDocumentThumbnails = 'fetchDocumentThumbnails',
  ParseDocument = 'parseDocument',
  StopAllDocuments = 'stopAllDocuments',
}

export const DocumentKeys = {
  all: () => [DocumentApiAction.FetchDocumentList] as const,
  // The dataset id scopes the list: without it two datasets with the same
  // search/pagination/filter share one cache entry, and entering dataset B
  // briefly renders dataset A's documents (firing their per-row requests).
  list: (
    knowledgeId: string | undefined,
    searchString: string,
    pagination: unknown,
    filter: unknown,
  ) =>
    [
      ...DocumentKeys.all(),
      knowledgeId,
      searchString,
      pagination,
      filter,
    ] as const,
  allFilters: () => [DocumentApiAction.FetchDocumentFilter] as const,
  // The counts cover the whole dataset and ignore the list's search text.
  filter: (datasetId?: string) =>
    [...DocumentKeys.allFilters(), datasetId] as const,
  // Per-status counts for the whole dataset, kept apart from the filter's key
  // so refreshing them never refetches the filter's facets.
  allStatusCounts: () => [DocumentApiAction.FetchDocumentStatusCounts] as const,
  statusCounts: (datasetId?: string) =>
    [...DocumentKeys.allStatusCounts(), datasetId] as const,
  thumbnails: (ids: string[]) =>
    [DocumentApiAction.FetchDocumentThumbnails, ids] as const,
  byIds: (ids: string[]) =>
    [DocumentApiAction.FetchDocumentList, 'byIds', ids] as const,
};
