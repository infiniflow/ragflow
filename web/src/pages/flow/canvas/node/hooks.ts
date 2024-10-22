import get from 'lodash/get';
import { useEffect, useMemo } from 'react';
import { useUpdateNodeInternals } from 'reactflow';
import { Operator, SwitchElseTo } from '../../constant';
import { ISwitchCondition, NodeData } from '../../interface';
import { generateSwitchHandleText } from '../../utils';

export const useBuildCategorizeHandlePositions = ({
  data,
  id,
}: {
  id: string;
  data: NodeData;
}) => {
  const operatorName = data.label as Operator;
  const updateNodeInternals = useUpdateNodeInternals();

  const categoryData = useMemo(() => {
    if (operatorName === Operator.Categorize) {
      return get(data, `form.category_description`, {});
    } else if (operatorName === Operator.Switch) {
      return get(data, 'form.conditions', []);
    }
    return {};
  }, [data, operatorName]);

  const positions = useMemo(() => {
    return Object.keys(categoryData).map((x, idx) => {
      let text = x;
      if (operatorName === Operator.Switch) {
        text = generateSwitchHandleText(idx);
      }
      return { text, idx, top: 44 + (idx + 1) * 16 };
    });
  }, [categoryData, operatorName]);

  useEffect(() => {
    updateNodeInternals(id);
  }, [id, updateNodeInternals, categoryData]);

  return { positions };
};

export const useBuildSwitchHandlePositions = ({
  data,
  id,
}: {
  id: string;
  data: NodeData;
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
      let top = idx === 0 ? 58 : list[idx - 1].top + 32; // case number (Case 1) height + flex gap
      if (idx - 1 >= 0) {
        const previousItems = conditions[idx - 1]?.items ?? [];
        if (previousItems.length > 0) {
          top += 12; // ConditionBlock padding
          top += previousItems.length * 22; // condition variable height
          top += (previousItems.length - 1) * 25; // operator height
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
