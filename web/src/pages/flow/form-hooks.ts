import { useTranslate } from '@/hooks/common-hooks';
import { useCallback, useMemo } from 'react';
import { Operator, RestrictedUpstreamMap } from './constant';
import useGraphStore from './store';

export const useBuildFormSelectOptions = (
  operatorName: Operator,
  selfId?: string, // exclude the current node
) => {
  const nodes = useGraphStore((state) => state.nodes);

  const buildCategorizeToOptions = useCallback(
    (toList: string[]) => {
      const excludedNodes: Operator[] = [
        Operator.Note,
        ...(RestrictedUpstreamMap[operatorName] ?? []),
      ];
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

export const useBuildSortOptions = () => {
  const { t } = useTranslate('flow');

  const options = useMemo(() => {
    return ['data', 'relevance'].map((x) => ({
      value: x,
      label: t(x),
    }));
  }, [t]);
  return options;
};
