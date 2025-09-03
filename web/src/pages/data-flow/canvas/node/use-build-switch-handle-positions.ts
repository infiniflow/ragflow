import { ISwitchCondition, RAGFlowNodeType } from '@/interfaces/database/flow';
import { useUpdateNodeInternals } from '@xyflow/react';
import get from 'lodash/get';
import { useEffect, useMemo } from 'react';
import { SwitchElseTo } from '../../constant';
import { generateSwitchHandleText } from '../../utils';

export const useBuildSwitchHandlePositions = ({
  data,
  id,
}: {
  id: string;
  data: RAGFlowNodeType['data'];
}) => {
  const updateNodeInternals = useUpdateNodeInternals();

  const conditions: ISwitchCondition[] = useMemo(() => {
    return get(data, 'form.conditions', []);
  }, [data]);

  const positions = useMemo(() => {
    const list: Array<{
      text: string;
      top: number;
      idx: number;
      condition?: ISwitchCondition;
    }> = [];

    [...conditions, ''].forEach((x, idx) => {
      let top = idx === 0 ? 53 : list[idx - 1].top + 10 + 14 + 16 + 16; // case number (Case 1) height + flex gap
      if (idx >= 1) {
        const previousItems = conditions[idx - 1]?.items ?? [];
        if (previousItems.length > 0) {
          // top += 12; // ConditionBlock padding
          top += previousItems.length * 26; // condition variable height
          // top += (previousItems.length - 1) * 25; // operator height
        }
      }

      list.push({
        text:
          idx < conditions.length
            ? generateSwitchHandleText(idx)
            : SwitchElseTo,
        idx,
        top,
        condition: typeof x === 'string' ? undefined : x,
      });
    });

    return list;
  }, [conditions]);

  useEffect(() => {
    updateNodeInternals(id);
  }, [id, updateNodeInternals, conditions]);

  return { positions };
};
