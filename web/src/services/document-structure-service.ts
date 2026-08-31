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

import api from '@/utils/api';
import request from '@/utils/next-request';

export const getDocumentStructureGraph = (
  datasetId: string,
  documentId: string,
  keywords?: string,
) =>
  request.get(api.documentStructureGraph(datasetId, documentId), {
    params: keywords ? { keywords } : undefined,
  });

export const deleteDocumentStructureGraph = (
  datasetId: string,
  documentId: string,
  templateId: string,
) =>
  request.delete(api.documentStructureGraph(datasetId, documentId), {
    data: { template_id: templateId },
  });

const documentStructureService = {
  getDocumentStructureGraph,
  deleteDocumentStructureGraph,
};

export default documentStructureService;
