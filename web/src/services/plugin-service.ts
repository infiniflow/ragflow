import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const { llmTools } = api;

const methods = {
  getLlmTools: {
    url: llmTools,
    method: 'get',
  },
} as const;

const pluginService = registerServer<keyof typeof methods>(methods, request);

export default pluginService;
