import api from '@/utils/api';
import request from '@/utils/next-request';
import { registerNextServer } from '@/utils/register-server';

const methods = {
  listTemplates: {
    url: api.compilationTemplates,
    method: 'get',
  },
} as const;

const compilationTemplateService =
  registerNextServer<keyof typeof methods>(methods);

export const deleteCompilationTemplate = (id: string) =>
  request.delete(api.compilationTemplate(id));

export default compilationTemplateService;
