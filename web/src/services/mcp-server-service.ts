import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const {
  getMcpServerList,
  getMultipleMcpServers,
  createMcpServer,
  updateMcpServer,
  deleteMcpServer,
} = api;

const methods = {
  get_list: {
    url: getMcpServerList,
    method: 'get',
  },
  get_multiple: {
    url: getMultipleMcpServers,
    method: 'post',
  },
  add: {
    url: createMcpServer,
    method: 'post'
  },
  update: {
    url: updateMcpServer,
    method: 'post'
  },
  rm: {
    url: deleteMcpServer,
    method: 'post'
  },
} as const;

const mcpServerService = registerServer<keyof typeof methods>(methods, request);

export const getMcpServer = (serverId: string) =>
  request.get(api.getMcpServer(serverId));

export default mcpServerService;
