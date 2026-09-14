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

const datasetNavService = {
  getNav: (params: { datasetId: string; keywords?: string }) =>
    request.get(api.getDatasetNav(params.datasetId), {
      params: { keywords: params.keywords || undefined },
    }),
  getNavChildren: (params: { datasetId: string; name: string }) =>
    request.get(api.getDatasetNavChildren(params.datasetId, params.name)),
  deleteNav: (params: { datasetId: string }) =>
    request.delete(api.deleteDatasetNav(params.datasetId)),
  deleteNavNode: (params: { datasetId: string; name: string }) =>
    request.delete(api.deleteDatasetNavNode(params.datasetId, params.name)),
};

export default datasetNavService;
