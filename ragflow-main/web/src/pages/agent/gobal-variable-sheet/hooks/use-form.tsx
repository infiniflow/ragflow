import { useFetchAgent } from '@/hooks/use-agent-request';
import { GlobalVariableType } from '@/interfaces/database/agent';
import { useCallback } from 'react';
import { FieldValues } from 'react-hook-form';
import { useSaveGraph } from '../../hooks/use-save-graph';
import { TypesWithArray } from '../constant';

export const useHandleForm = () => {
  const { data, refetch } = useFetchAgent();
  const { saveGraph, loading } = useSaveGraph();
  const handleObjectData = (value: any) => {
    try {
      return JSON.parse(value);
    } catch (error) {
      return value;
    }
  };
  const handleSubmit = useCallback(
    async (fieldValue: FieldValues) => {
      const param = {
        ...(data.dsl?.variables || {}),
        [fieldValue.name]: {
          ...fieldValue,
          value:
            fieldValue.type === TypesWithArray.Object ||
            fieldValue.type === TypesWithArray.ArrayObject
              ? handleObjectData(fieldValue.value)
              : fieldValue.value,
        },
      } as Record<string, GlobalVariableType>;

      const res = await saveGraph(undefined, {
        globalVariables: param,
      });

      if (res.code === 0) {
        refetch();
      }
    },
    [data.dsl?.variables, refetch, saveGraph],
  );

  return { handleSubmit, loading };
};
