import { useCallback } from 'react';

import {
  ICreateCompilationTemplateRequestBody,
  IUpdateCompilationTemplateRequestBody,
} from '@/interfaces/request/compilation-template';

import { FormSchemaType } from '../schema';
import { transformFormToPayload } from '../utils';

type UseCompilationTemplateSubmitOptions = {
  isCreate: boolean;
  id?: string;
  createTemplate: (
    params: ICreateCompilationTemplateRequestBody,
  ) => Promise<{ code: number } & Record<string, unknown>>;
  updateTemplate: (
    id: string,
    params: IUpdateCompilationTemplateRequestBody,
  ) => Promise<{ code: number } & Record<string, unknown>>;
  onSuccess: () => void;
};

export const useCompilationTemplateSubmit = ({
  isCreate,
  id,
  createTemplate,
  updateTemplate,
  onSuccess,
}: UseCompilationTemplateSubmitOptions) => {
  const onSubmit = useCallback(
    async (values: FormSchemaType) => {
      const payload = transformFormToPayload(values);
      let result;
      if (isCreate) {
        result = await createTemplate(payload);
      } else if (id) {
        result = await updateTemplate(id, payload);
      }
      if (result?.code === 0) {
        onSuccess();
      }
    },
    [createTemplate, id, isCreate, onSuccess, updateTemplate],
  );

  return { onSubmit };
};
