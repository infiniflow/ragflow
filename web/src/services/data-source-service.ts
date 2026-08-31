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

const { dataSourceSet, dataSourceList } = api;
const methods = {
  dataSourceSet: {
    url: dataSourceSet,
    method: 'post',
  },
  dataSourceList: {
    url: dataSourceList,
    method: 'get',
  },
} as const;
const dataSourceService = registerServer<keyof typeof methods>(
  methods,
  request,
);

export const deleteDataSource = (id: string) =>
  request.delete(api.dataSourceDel(id));

export const dataSourceRebuild = (id: string, data: { kb_id: string }) => {
  return request.post(api.dataSourceRebuild(id), { data });
};

export const dataSourceUpdate = (id: string, data: Record<string, any>) => {
  return request.patch(api.dataSourceUpdate(id), { data });
};

export const getDataSourceLogs = (id: string, params?: any) =>
  request.get(api.dataSourceLogs(id), { params });
export const featchDataSourceDetail = (id: string) =>
  request.get(api.dataSourceDetail(id));

export const testDataSource = (
  id: string,
  data: { source: string; config?: Record<string, unknown> },
) => request.post(api.dataSourceTest(id), { data });

export const startGoogleDriveWebAuth = (payload: {
  credentials: string;
  redirect_uri?: string;
}) => request.post(api.googleWebAuthStart('google-drive'), { data: payload });

export const pollGoogleDriveWebAuthResult = (payload: { flow_id: string }) =>
  request.post(api.googleWebAuthResult('google-drive'), { data: payload });

// Gmail web auth follows the same pattern as Google Drive, but uses
// Gmail-specific endpoints and is consumed by the GmailTokenField UI.
export const startGmailWebAuth = (payload: {
  credentials: string;
  redirect_uri?: string;
}) => request.post(api.googleWebAuthStart('gmail'), { data: payload });

export const pollGmailWebAuthResult = (payload: { flow_id: string }) =>
  request.post(api.googleWebAuthResult('gmail'), { data: payload });

export const startBoxWebAuth = (payload: {
  client_id: string;
  client_secret: string;
  redirect_uri?: string;
}) => request.post(api.boxWebAuthStart(), { data: payload });

export const pollBoxWebAuthResult = (payload: { flow_id: string }) =>
  request.post(api.boxWebAuthResult(), { data: payload });

export default dataSourceService;
