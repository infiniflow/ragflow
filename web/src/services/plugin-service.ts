import api from '@/utils/api';
import registerServer from '@/utils/register-server';
import request from '@/utils/request';

const {
  llm_tools
} = api;

const methods = {
  getLlmTools: {
    url: llm_tools,
    method: 'get',
  },
} as const;

const pluginService = registerServer<keyof typeof methods>(methods, request);

export default pluginService;
