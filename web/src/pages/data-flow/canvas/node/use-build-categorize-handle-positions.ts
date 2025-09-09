import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { useUpdateNodeInternals } from '@xyflow/react';
import { get } from 'lodash';
import { useEffect, useMemo } from 'react';
import { z } from 'zod';
import { useCreateCategorizeFormSchema } from '../../form/categorize-form/use-form-schema';

export const useBuildCategorizeHandlePositions = ({
  data,
  id,
}: {
  id: string;
  data: RAGFlowNodeType['data'];
}) => {
  const updateNodeInternals = useUpdateNodeInternals();

  const FormSchema = useCreateCategorizeFormSchema();

  type FormSchemaType = z.infer<typeof FormSchema>;

  const items: Required<FormSchemaType['items']> = useMemo(() => {
    return get(data, `form.items`, []);
  }, [data]);

  const positions = useMemo(() => {
    const list: Array<{
      top: number;
      name: string;
      uuid: string;
    }> &
      Required<FormSchemaType['items']> = [];

    items.forEach((x, idx) => {
      list.push({
        ...x,
        top: idx === 0 ? 86 : list[idx - 1].top + 8 + 24,
      });
    });

    return list;
  }, [items]);

  useEffect(() => {
    updateNodeInternals(id);
  }, [id, updateNodeInternals, items]);

  return { positions };
};
