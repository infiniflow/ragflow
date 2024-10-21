import get from 'lodash/get';
import { useEffect, useMemo } from 'react';
import { useUpdateNodeInternals } from 'reactflow';
import { Operator } from '../../constant';
import { NodeData } from '../../interface';
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
