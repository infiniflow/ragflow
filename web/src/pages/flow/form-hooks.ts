import { useCallback } from 'react';
import { Operator } from './constant';
import useGraphStore from './store';

const ExcludedNodesMap = {
  // exclude some nodes downstream of the classification node
  [Operator.Categorize]: [
    Operator.Categorize,
    Operator.Answer,
    Operator.Begin,
    Operator.Relevant,
  ],
  [Operator.Relevant]: [Operator.Begin, Operator.Answer, Operator.Relevant],
  [Operator.Generate]: [Operator.Begin],
};

export const useBuildFormSelectOptions = (
  operatorName: Operator,
  selfId?: string, // exclude the current node
) => {
  const nodes = useGraphStore((state) => state.nodes);

  const buildCategorizeToOptions = useCallback(
    (toList: string[]) => {
      const excludedNodes: Operator[] = ExcludedNodesMap[operatorName] ?? [];
      return nodes
        .filter(
          (x) =>
            excludedNodes.every((y) => y !== x.data.label) &&
            x.id !== selfId &&
            !toList.some((y) => y === x.id), // filter out selected values ​​in other to fields from the current drop-down box options
        )
        .map((x) => ({ label: x.data.name, value: x.id }));
    },
    [nodes, operatorName, selfId],
  );

  return buildCategorizeToOptions;
};

/**
 * dumped
 * @param nodeId
 * @returns
 */
export const useHandleFormSelectChange = (nodeId?: string) => {
  const { addEdge, deleteEdgeBySourceAndSourceHandle } = useGraphStore(
    (state) => state,
  );
  const handleSelectChange = useCallback(
    (name?: string) => (value?: string) => {
      if (nodeId && name) {
        if (value) {
          addEdge({
            source: nodeId,
            target: value,
            sourceHandle: name,
            targetHandle: null,
          });
        } else {
          // clear selected value
          deleteEdgeBySourceAndSourceHandle({
            source: nodeId,
            sourceHandle: name,
          });
        }
      }
    },
    [addEdge, nodeId, deleteEdgeBySourceAndSourceHandle],
  );

  return { handleSelectChange };
};
