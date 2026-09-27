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

import { IPaginationRequestBody } from '@/interfaces/request/base';
import api from '@/utils/api';
import request from '@/utils/request';

const mcpServerService = {
  get: (params: { mcp_id: string }) =>
    request.get(api.getMcpServer(params.mcp_id), {
      params: { mode: 'preview' },
    }),
  create: (params?: Record<string, any>) =>
    request.post(api.createMcpServer, { data: params }),
  update: ({ mcp_id, ...params }: Record<string, any>) =>
    request.put(api.updateMcpServer(mcp_id), { data: params }),
  delete: ({ mcp_id }: { mcp_id: string }) =>
    request.delete(api.deleteMcpServer(mcp_id)),
  import: (params?: Record<string, any>) =>
    request.post(api.importMcpServer, { data: params }),
  export: ({ mcp_id }: { mcp_id: string }) =>
    request.get(api.exportMcpServer(mcp_id)),
  test: (params: Record<string, any>) =>
    request.post(api.testMcpServer(params.name || 'preview'), { data: params }),
};

export default mcpServerService;

export const listMcpServers = (params?: IPaginationRequestBody, body?: any) =>
  request.get(api.listMcpServer, { params: { ...params, ...(body || {}) } });
