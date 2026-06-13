import api from '@/utils/api';
import { registerNextServer } from '@/utils/register-server';

const { docAssistantAsk, docAssistantStatus } = api;

const methods = {
  ask: {
    url: docAssistantAsk,
    method: 'post',
  },
  status: {
    url: docAssistantStatus,
    method: 'get',
  },
} as const;

const docAssistantService = registerNextServer<keyof typeof methods>(methods);

export default docAssistantService;
