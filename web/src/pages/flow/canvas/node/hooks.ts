import get from 'lodash/get';
import pick from 'lodash/pick';
import { useEffect, useMemo, useState } from 'react';
import { useUpdateNodeInternals } from 'reactflow';
import { Operator } from '../../constant';
import { IPosition, NodeData } from '../../interface';
import {
  buildNewPositionMap,
  generateSwitchHandleText,
  isKeysEqual,
} from '../../utils';

export const useBuildCategorizeHandlePositions = ({
  data,
  id,
}: {
  id: string;
  data: NodeData;
}) => {
  const operatorName = data.label as Operator;
  const updateNodeInternals = useUpdateNodeInternals();
  const [positionMap, setPositionMap] = useState<Record<string, IPosition>>({});

  const categoryData = useMemo(() => {
    if (operatorName === Operator.Categorize) {
      return get(data, `form.category_description`, {});
    } else if (operatorName === Operator.Switch) {
      return get(data, 'form.conditions', []);
    }
    return {};
  }, [data, operatorName]);

  const positions = useMemo(() => {
    return Object.keys(categoryData)
      .map((x, idx) => {
        const position = positionMap[x];
        let text = x;
        if (operatorName === Operator.Switch) {
          text = generateSwitchHandleText(idx);
        }
        return { text, ...position };
      })
      .filter((x) => typeof x?.right === 'number');
  }, [categoryData, positionMap, operatorName]);

  useEffect(() => {
    // Cache used coordinates
    setPositionMap((state) => {
      const categoryDataKeys = Object.keys(categoryData);
      const stateKeys = Object.keys(state);
      if (!isKeysEqual(categoryDataKeys, stateKeys)) {
        const { newPositionMap, intersectionKeys } = buildNewPositionMap(
          categoryDataKeys,
          state,
        );

        const nextPositionMap = {
          ...pick(state, intersectionKeys),
          ...newPositionMap,
        };

        return nextPositionMap;
      }
      return state;
    });
  }, [categoryData]);

  useEffect(() => {
    updateNodeInternals(id);
  }, [id, updateNodeInternals, positionMap]);

  return { positions };
};

// export const useBuildSwitchHandlePositions = ({
//   data,
//   id,
// }: {
//   id: string;
//   data: NodeData;
// }) => {
//   const [positionMap, setPositionMap] = useState<Record<string, IPosition>>({});
//   const conditions = useMemo(() => get(data, 'form.conditions', []), [data]);
//   const updateNodeInternals = useUpdateNodeInternals();

//   const positions = useMemo(() => {
//     return conditions
//       .map((x, idx) => {
//         const text = `Item ${idx}`;
//         const position = positionMap[text];
//         return { text: text, ...position };
//       })
//       .filter((x) => typeof x?.right === 'number');
//   }, [conditions, positionMap]);

//   useEffect(() => {
//     updateNodeInternals(id);
//   }, [id, updateNodeInternals, positionMap]);

//   return { positions };
// };
