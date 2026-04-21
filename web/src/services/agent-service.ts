import {
  IAgentLogsRequest,
  IPipeLineListRequest,
} from '@/interfaces/database/agent';
import { IAgentWebhookTraceRequest } from '@/interfaces/request/agent';
import api from '@/utils/api';
import { registerNextServer } from '@/utils/register-server';
import request from '@/utils/request';

const {
  createAgent,
  updateAgent: updateAgentApi,
  listAgents,
  deleteAgent,
  agentChatCompletion,
  resetAgent,
  listAgentTemplate,
  testDbConnect,
  getInputElements,
  debug,
  trace,
  fetchVersionList,
  fetchVersion,
  getAgent,
  fetchAgentLogs,
  fetchExternalAgentInputs,
  prompt,
  cancelDataflow,
  cancelCanvas,
} = api;

const methods = {
  getAgent: {
    url: getAgent,
    method: 'get',
  },
  createAgent: {
    url: createAgent,
    method: 'post',
  },
  fetchVersionList: {
    url: fetchVersionList,
    method: 'get',
  },
  fetchVersion: {
    url: fetchVersion,
    method: 'get',
  },
  listAgents: {
    url: listAgents,
    method: 'get',
  },
  resetAgent: {
    url: resetAgent,
    method: 'post',
  },
  deleteAgent: {
    url: deleteAgent,
    method: 'delete',
  },
  agentChatCompletion: {
    url: agentChatCompletion,
    method: 'post',
  },
  listAgentTemplate: {
    url: listAgentTemplate,
    method: 'get',
  },
  testDbConnect: {
    url: testDbConnect,
    method: 'post',
  },
  getInputElements: {
    url: getInputElements,
    method: 'get',
  },
  debugSingle: {
    url: debug,
    method: 'post',
  },
  uploadAgentFile: {
    url: (config: { agentId: string }) => api.uploadAgentFile(config.agentId),
    method: 'post',
  },
  trace: {
    url: trace,
    method: 'get',
  },
  inputForm: {
    url: (config: { agentId: string; componentId: string }) =>
      api.inputForm(config.agentId, config.componentId),
    method: 'get',
  },
  fetchAgentLogs: {
    url: fetchAgentLogs,
    method: 'get',
  },
  fetchExternalAgentInputs: {
    url: fetchExternalAgentInputs,
    method: 'get',
  },
  fetchPrompt: {
    url: prompt,
    method: 'get',
  },
  cancelDataflow: {
    url: cancelDataflow,
    method: 'put',
  },
  cancelCanvas: {
    url: cancelCanvas,
    method: 'put',
  },
  createAgentSession: {
    url: fetchAgentLogs,
    method: 'put',
  },
} as const;

const agentService = registerNextServer<keyof typeof methods>(methods);

export const updateAgent = (
  agentId: string,
  params: {
    title?: string;
    dsl?: Record<string, any>;
    avatar?: string;
    description?: string | null;
    permission?: string;
    release?: string;
  },
) => {
  return request(updateAgentApi(agentId), { method: 'put', data: params });
};

export const fetchTrace = (data: { canvas_id: string; message_id: string }) => {
  return request.get(methods.trace.url, { params: data });
};
export const fetchAgentLogsByCanvasId = (
  canvasId: string,
  params: IAgentLogsRequest,
) => {
  return request.get(methods.fetchAgentLogs.url(canvasId), { params: params });
};

export const fetchAgentLogsById = (canvasId: string, sessionId: string) => {
  return request.get(api.fetchAgentLogsById(canvasId, sessionId));
};

export const fetchPipeLineList = (params: IPipeLineListRequest) => {
  return request.get(api.listAgents, { params: params });
};

export const fetchWebhookTrace = (
  id: string,
  params: IAgentWebhookTraceRequest,
) => {
  return request.get(api.fetchWebhookTrace(id), { params: params });
};

export function createAgentSession({ id, name }: { id: string; name: string }) {
  return request.put(api.fetchAgentLogs(id), { data: { name } });
}

export const deleteAgentSession = (canvasId: string, sessionId: string) => {
  return request.delete(api.fetchAgentLogsById(canvasId, sessionId));
};

export const uploadAgentFile = (agentId: string, data: FormData) => {
  return request(api.uploadAgentFile(agentId), {
    method: 'post',
    data,
  });
};

export default agentService;
