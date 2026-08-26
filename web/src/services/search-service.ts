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
import { registerNextServer } from '@/utils/register-server';

const {
  createSearch,
  getSearchList,
  deleteSearch,
  getSearchDetail,
  updateSearchSetting,
  askShare,
  mindmapShare,
  getRelatedQuestionsShare,
  getSearchDetailShare,
} = api;

const methods = {
  createSearch: {
    url: createSearch,
    method: 'post',
  },
  getSearchList: {
    url: getSearchList,
    method: 'get',
  },
  deleteSearch: { url: deleteSearch, method: 'delete' },
  getSearchDetail: {
    url: getSearchDetail,
    method: 'get',
  },
  updateSearchSetting: {
    url: updateSearchSetting,
    method: 'put',
  },
  askShare: {
    url: askShare,
    method: 'post',
  },
  mindmapShare: {
    url: mindmapShare,
    method: 'post',
  },
  getRelatedQuestionsShare: {
    url: getRelatedQuestionsShare,
    method: 'post',
  },
  getSearchDetailShare: {
    url: getSearchDetailShare,
    method: 'get',
  },
} as const;

const searchService = registerNextServer<keyof typeof methods>(methods);
export const searchServiceNext = searchService;

export default searchService;
