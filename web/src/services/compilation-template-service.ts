/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

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
