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
