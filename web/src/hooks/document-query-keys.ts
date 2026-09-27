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
  SetDocumentMeta = 'setDocumentMeta',
  FetchDocumentFilter = 'fetchDocumentFilter',
  CreateDocument = 'createDocument',
  FetchDocumentThumbnails = 'fetchDocumentThumbnails',
  ParseDocument = 'parseDocument',
}

export const DocumentKeys = {
  all: () => [DocumentApiAction.FetchDocumentList] as const,
  list: (searchString: string, pagination: unknown, filter: unknown) =>
    [...DocumentKeys.all(), searchString, pagination, filter] as const,
  allFilters: () => [DocumentApiAction.FetchDocumentFilter] as const,
  filter: (searchString: string, knowledgeId?: string) =>
    [...DocumentKeys.allFilters(), searchString, knowledgeId] as const,
  thumbnails: (ids: string[]) =>
    [DocumentApiAction.FetchDocumentThumbnails, ids] as const,
  byIds: (ids: string[]) =>
    [DocumentApiAction.FetchDocumentList, 'byIds', ids] as const,
};
