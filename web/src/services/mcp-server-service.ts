import { IPaginationRequestBody } from '@/interfaces/request/base';
import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const {
  listMcpServer,
  createMcpServer,
  updateMcpServer,
  deleteMcpServer,
  getMcpServer,
  importMcpServer,
  exportMcpServer,
  listMcpServerTools,
  testMcpServerTool,
  cacheMcpServerTool,
  testMcpServer,
} = api;

const methods = {
  list: {
    url: listMcpServer,
    method: 'post',
  },
  get: {
    url: getMcpServer,
    method: 'get',
  },
  create: {
    url: createMcpServer,
    method: 'post',
  },
  update: {
    url: updateMcpServer,
    method: 'post',
  },
  delete: {
    url: deleteMcpServer,
    method: 'post',
  },
  import: {
    url: importMcpServer,
    method: 'post',
  },
  export: {
    url: exportMcpServer,
    method: 'post',
  },
  listTools: {
    url: listMcpServerTools,
    method: 'post',
  },
  testTool: {
    url: testMcpServerTool,
    method: 'post',
  },
  cacheTool: {
    url: cacheMcpServerTool,
    method: 'post',
  },
  test: {
    url: testMcpServer,
    method: 'post',
  },
} as const;

const mcpServerService = registerServer<keyof typeof methods>(methods, request);

export default mcpServerService;

export const listMcpServers = (params?: IPaginationRequestBody, body?: any) =>
  request.post(api.listMcpServer, { data: body || {}, params });
