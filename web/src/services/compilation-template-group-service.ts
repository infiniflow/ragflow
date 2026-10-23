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
  ICreateCompilationTemplateGroupRequestBody,
  IOntologyFixBatchRequestBody,
  IOntologyFixRequestBody,
  IOntologyFixResponse,
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

/**
 * Applies one confirmed edit to an ontology template. The write is validated
 * server-side through the same path the editor save uses; re-compiling the
 * documents that produced the finding is a separate, explicit step.
 */
/**
 * Applies the edits a reader selected, as one write, and resolves with the
 * backend envelope (`{ code, message, data }`).
 *
 * The envelope is on `.data` of what this axios instance resolves with, so it is
 * unwrapped here on purpose: a caller reading `code` off the response itself
 * gets `undefined`, treats every write — including a successful one — as
 * refused, and then skips both the re-compile and the refresh that would have
 * shown the change.
 */
export const applyOntologyFixes = async (
  templateId: string,
  fixes: IOntologyFixRequestBody[],
): Promise<IOntologyFixResponse> => {
  const { data } = await request.post(
    api.compilationTemplateOntologyFix(templateId),
    { fixes } satisfies IOntologyFixBatchRequestBody,
  );
  return data as IOntologyFixResponse;
};

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
