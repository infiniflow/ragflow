import get from 'lodash/get';
import intersectionWith from 'lodash/intersectionWith';
import isEqual from 'lodash/isEqual';
import pick from 'lodash/pick';
import { useEffect, useMemo, useState } from 'react';
import { useUpdateNodeInternals } from 'reactflow';
import { IPosition, NodeData } from '../../interface';
import { buildNewPositionMap } from '../../utils';

export const useBuildCategorizeHandlePositions = ({
  data,
  id,
}: {
  id: string;
  data: NodeData;
}) => {
  const updateNodeInternals = useUpdateNodeInternals();
  const [positionMap, setPositionMap] = useState<Record<string, IPosition>>({});
  const categoryData = useMemo(
    () => get(data, 'form.category_description') ?? {},
    [data],
  );

  const positions = useMemo(() => {
    return Object.keys(categoryData)
      .map((x) => {
        const position = positionMap[x];
        return { text: x, ...position };
      })
      .filter((x) => typeof x?.right === 'number');
  }, [categoryData, positionMap]);

  useEffect(() => {
    // Cache used coordinates
    setPositionMap((state) => {
      // index in use
      const indexesInUse = Object.values(state).map((x) => x.idx);
      const categoryDataKeys = Object.keys(categoryData);
      const stateKeys = Object.keys(state);
      if (!isEqual(categoryDataKeys.sort(), stateKeys.sort())) {
        const intersectionKeys = intersectionWith(
          stateKeys,
          categoryDataKeys,
          (categoryDataKey, postionMapKey) => categoryDataKey === postionMapKey,
        );
        const newPositionMap = buildNewPositionMap(
          categoryDataKeys.filter(
            (x) => !intersectionKeys.some((y) => y === x),
          ),
          indexesInUse,
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

export const useBuildSwitchHandlePositions = ({
  data,
  id,
}: {
  id: string;
  data: NodeData;
}) => {};
