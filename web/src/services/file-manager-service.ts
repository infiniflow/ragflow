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
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const {
  listFile,
  removeFile,
  uploadFile,
  getAllParentFolder,
  createFolder,
  connectFileToKnowledge,
  getDocumentFile,
  getFile,
  moveFile,
  getDatasetDocumentFileDownload,
  getAttachmentFileDownload,
} = api;

const methods = {
  listFile: {
    url: listFile,
    method: 'get',
  },
  removeFile: {
    url: removeFile,
    method: 'delete',
  },
  uploadFile: {
    url: uploadFile,
    method: 'post',
  },
  getAllParentFolder: {
    url: getAllParentFolder,
    method: 'get',
  },
  createFolder: {
    url: createFolder,
    method: 'post',
  },
  connectFileToKnowledge: {
    url: connectFileToKnowledge,
    method: 'post',
  },
  getFile: {
    url: getFile,
    method: 'get',
    responseType: 'blob',
  },
  getDocumentFile: {
    url: getDocumentFile,
    method: 'get',
    responseType: 'blob',
  },
  moveFile: {
    url: moveFile,
    method: 'post',
  },
} as const;

const fileManagerService = registerServer<keyof typeof methods>(
  methods,
  request,
);

export const downloadAgentFile = (data: { docId: string; ext: string }) => {
  return request.get(getAttachmentFileDownload(data.docId), {
    params: { ext: data.ext },
    responseType: 'blob',
  });
};

export const downloadDatasetDocument = (data: {
  datasetId: string;
  docId: string;
  ext: string;
}) => {
  return request.get(
    getDatasetDocumentFileDownload(data.datasetId, data.docId),
    {
      params: { ext: data.ext },
      responseType: 'blob',
    },
  );
};
export default fileManagerService;
