import {
  ICreateCompilationTemplateGroupRequestBody,
  IUpdateCompilationTemplateGroupRequestBody,
} from '@/interfaces/request/compilation-template';
import api from '@/utils/api';
import request from '@/utils/next-request';
import { registerNextServer } from '@/utils/register-server';

const methods = {
  listGroups: {
    url: api.compilationTemplateGroups,
    method: 'get',
  },
} as const;

const compilationTemplateGroupService =
  registerNextServer<keyof typeof methods>(methods);

export const createCompilationTemplateGroup = (
  data: ICreateCompilationTemplateGroupRequestBody,
) => request.post(api.compilationTemplateGroups, data);

export const updateCompilationTemplateGroup = (
  id: string,
  data: IUpdateCompilationTemplateGroupRequestBody,
) => request.put(api.compilationTemplateGroup(id), data);

export const getCompilationTemplateGroup = (id: string) =>
  request.get(api.compilationTemplateGroup(id));

export const deleteCompilationTemplateGroup = (id: string) =>
  request.delete(api.compilationTemplateGroup(id));

export { compilationTemplateGroupService };
export default compilationTemplateGroupService;
