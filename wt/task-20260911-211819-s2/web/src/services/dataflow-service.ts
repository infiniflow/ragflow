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
  listDataflow,
  removeDataflow,
  fetchDataflow,
  runDataflow,
  setDataflow,
} = api;

const methods = {
  listDataflow: {
    url: listDataflow,
    method: 'get',
  },
  removeDataflow: {
    url: removeDataflow,
    method: 'post',
  },
  fetchDataflow: {
    url: fetchDataflow,
    method: 'get',
  },
  runDataflow: {
    url: runDataflow,
    method: 'post',
  },
  setDataflow: {
    url: setDataflow,
    method: 'post',
  },
} as const;

const dataflowService = registerNextServer<keyof typeof methods>(methods);

export default dataflowService;
