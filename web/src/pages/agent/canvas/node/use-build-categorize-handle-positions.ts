import { ICategorizeItemResult } from '@/interfaces/database/agent';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { useUpdateNodeInternals } from '@xyflow/react';
import { get } from 'lodash';
import { useEffect, useMemo } from 'react';

export const useBuildCategorizeHandlePositions = ({
  data,
  id,
}: {
  id: string;
  data: RAGFlowNodeType['data'];
}) => {
  const updateNodeInternals = useUpdateNodeInternals();

  const categoryData: ICategorizeItemResult = useMemo(() => {
    return get(data, `form.category_description`, {});
  }, [data]);

  const positions = useMemo(() => {
    const list: Array<{
      text: string;
      top: number;
      idx: number;
    }> = [];

    Object.keys(categoryData)
      .sort((a, b) => categoryData[a].index - categoryData[b].index)
      .forEach((x, idx) => {
        list.push({
          text: x,
          idx,
          top: idx === 0 ? 86 : list[idx - 1].top + 8 + 24,
        });
      });

    return list;
  }, [categoryData]);

  useEffect(() => {
    updateNodeInternals(id);
  }, [id, updateNodeInternals, categoryData]);

  return { positions };
};
