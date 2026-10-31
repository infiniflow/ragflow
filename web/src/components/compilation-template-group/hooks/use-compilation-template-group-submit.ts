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

import { useCallback } from 'react';

import { ICompilationTemplateGroup } from '@/interfaces/database/compilation-template';
import {
  ICreateCompilationTemplateGroupRequestBody,
  IUpdateCompilationTemplateGroupRequestBody,
} from '@/interfaces/request/compilation-template';

import { FormSchemaType } from '../schema';
import { transformFormToPayload } from '../utils';

export type CompilationTemplateGroupSaveResult = {
  code: number;
  data?: ICompilationTemplateGroup;
};

type UseCompilationTemplateGroupSubmitOptions = {
  isCreate: boolean;
  id?: string;
  createGroup: (
    params: ICreateCompilationTemplateGroupRequestBody,
  ) => Promise<CompilationTemplateGroupSaveResult>;
  updateGroup: (
    id: string,
    params: IUpdateCompilationTemplateGroupRequestBody,
  ) => Promise<CompilationTemplateGroupSaveResult>;
  onSuccess: (result: CompilationTemplateGroupSaveResult) => void;
};

export const useCompilationTemplateGroupSubmit = ({
  isCreate,
  id,
  createGroup,
  updateGroup,
  onSuccess,
}: UseCompilationTemplateGroupSubmitOptions) => {
  const onSubmit = useCallback(
    async (values: FormSchemaType) => {
      const payload = transformFormToPayload(values);
      let result;
      if (isCreate) {
        result = await createGroup(payload);
      } else if (id) {
        result = await updateGroup(id, payload);
      }
      if (result?.code === 0) {
        onSuccess(result);
      }
    },
    [createGroup, id, isCreate, onSuccess, updateGroup],
  );

  return { onSubmit };
};
