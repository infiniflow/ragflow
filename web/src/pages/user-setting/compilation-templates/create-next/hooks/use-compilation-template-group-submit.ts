import { useCallback } from 'react';

import {
  ICreateCompilationTemplateGroupRequestBody,
  IUpdateCompilationTemplateGroupRequestBody,
} from '@/interfaces/request/compilation-template';

import { FormSchemaType } from '../schema';
import { transformFormToPayload } from '../utils';

type UseCompilationTemplateGroupSubmitOptions = {
  isCreate: boolean;
  id?: string;
  createGroup: (
    params: ICreateCompilationTemplateGroupRequestBody,
  ) => Promise<{ code: number } & Record<string, unknown>>;
  updateGroup: (
    id: string,
    params: IUpdateCompilationTemplateGroupRequestBody,
  ) => Promise<{ code: number } & Record<string, unknown>>;
  onSuccess: () => void;
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
        onSuccess();
      }
    },
    [createGroup, id, isCreate, onSuccess, updateGroup],
  );

  return { onSubmit };
};
