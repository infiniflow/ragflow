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
  trace,
  fetchVersionList,
  fetchVersion,
  getAgent,
  fetchAgentSessions,
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
    url: (config: { agentId: string; versionId: string }) =>
      fetchVersion(config.agentId, config.versionId),
    method: 'get',
  },
  listAgents: {
    url: listAgents,
    method: 'get',
  },
  listAgentTags: {
    url: api.listAgentTags,
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
    url: (config: { agentId: string; componentId: string }) =>
      api.debug(config.agentId, config.componentId),
    method: 'post',
  },
  uploadAgentFile: {
    url: (config: { agentId: string }) => api.uploadAgentFile(config.agentId),
    method: 'post',
  },
  trace: {
    url: (config: { agentId: string; messageId: string }) =>
      trace(config.agentId, config.messageId),
    method: 'get',
  },
  inputForm: {
    url: (config: { agentId: string; componentId: string }) =>
      api.inputForm(config.agentId, config.componentId),
    method: 'get',
  },
  fetchAgentLogs: {
    url: fetchAgentSessions,
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
    method: 'post',
  },
  cancelCanvas: {
    url: cancelCanvas,
    method: 'post',
  },
  createAgentSession: {
    url: api.createAgentSession,
    method: 'post',
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

export const updateAgentTags = (agentId: string, tags: string[]) => {
  return request(api.updateAgentTags(agentId), {
    method: 'put',
    data: { tags: tags.join(',') },
  });
};

export const fetchTrace = (data: { canvas_id: string; message_id: string }) => {
  return request.get(
    methods.trace.url({
      agentId: data.canvas_id,
      messageId: data.message_id,
    }),
  );
};
export const fetchAgentLogsByCanvasId = (
  canvasId: string,
  params: IAgentLogsRequest,
) => {
  return request.get(methods.fetchAgentLogs.url(canvasId), { params: params });
};

export const fetchAgentLogsById = (canvasId: string, sessionId: string) => {
  return request.get(api.fetchAgentSessionById(canvasId, sessionId));
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
  return request.post(api.createAgentSession(id), { data: { name } });
}

export const deleteAgentSession = (canvasId: string, sessionId: string) => {
  return request.delete(api.fetchAgentSessionById(canvasId, sessionId));
};

export const uploadAgentFile = (agentId: string, data: FormData) => {
  return request(api.uploadAgentFile(agentId), {
    method: 'post',
    data,
  });
};

export default agentService;
