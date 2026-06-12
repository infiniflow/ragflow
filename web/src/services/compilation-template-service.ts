import { IListCompilationTemplatesRequest } from '@/interfaces/request/compilation-template';
import api from '@/utils/api';
import request from '@/utils/request';

const compilationTemplateService = {
  list: (params?: IListCompilationTemplatesRequest) =>
    request.get(api.listCompilationTemplates, { params }),
  get: (params: { id: string }) =>
    request.get(api.getCompilationTemplate(params.id)),
  create: (params?: Record<string, any>) =>
    request.post(api.createCompilationTemplate, { data: params }),
  update: ({ id, ...params }: Record<string, any>) =>
    request.put(api.updateCompilationTemplate(id), { data: params }),
  delete: ({ id }: { id: string }) =>
    request.delete(api.deleteCompilationTemplate(id)),
  builtins: () => request.get(api.listBuiltinCompilationTemplates),
};

export default compilationTemplateService;
