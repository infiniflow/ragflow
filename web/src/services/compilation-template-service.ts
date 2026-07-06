import {
  ICreateCompilationTemplateRequestBody,
  IUpdateCompilationTemplateRequestBody,
} from '@/interfaces/request/compilation-template';
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

export const getCompilationTemplate = (id: string) =>
  request.get(api.compilationTemplate(id));

export const createCompilationTemplate = (
  data: ICreateCompilationTemplateRequestBody,
) => request.post(api.compilationTemplates, data);

export const updateCompilationTemplate = (
  id: string,
  data: IUpdateCompilationTemplateRequestBody,
) => request.put(api.compilationTemplate(id), data);

export const listBuiltinCompilationTemplates = () =>
  request.get(`${api.compilationTemplates}/builtins`);

export const listWikiPresets = () => request.get(api.wikiPresets);

export default compilationTemplateService;
