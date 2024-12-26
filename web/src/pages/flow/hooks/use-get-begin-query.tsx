import { DefaultOptionType } from 'antd/es/select';
import get from 'lodash/get';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { BeginId, Operator } from '../constant';
import { BeginQuery } from '../interface';
import useGraphStore from '../store';

export const useGetBeginNodeDataQuery = () => {
  const getNode = useGraphStore((state) => state.getNode);

  const getBeginNodeDataQuery = useCallback(() => {
    return get(getNode(BeginId), 'data.form.query', []);
  }, [getNode]);

  return getBeginNodeDataQuery;
};

export const useGetBeginNodeDataQueryIsEmpty = () => {
  const [isBeginNodeDataQueryEmpty, setIsBeginNodeDataQueryEmpty] =
    useState(false);
  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();
  const nodes = useGraphStore((state) => state.nodes);

  useEffect(() => {
    const query: BeginQuery[] = getBeginNodeDataQuery();
    setIsBeginNodeDataQueryEmpty(query.length === 0);
  }, [getBeginNodeDataQuery, nodes]);

  return isBeginNodeDataQueryEmpty;
};

// exclude nodes with branches
const ExcludedNodes = [
  Operator.Categorize,
  Operator.Relevant,
  Operator.Begin,
  Operator.Note,
];

export const useBuildComponentIdSelectOptions = (nodeId?: string) => {
  const nodes = useGraphStore((state) => state.nodes);
  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();
  const query: BeginQuery[] = getBeginNodeDataQuery();

  const componentIdOptions = useMemo(() => {
    return nodes
      .filter(
        (x) =>
          x.id !== nodeId && !ExcludedNodes.some((y) => y === x.data.label),
      )
      .map((x) => ({ label: x.data.name, value: x.id }));
  }, [nodes, nodeId]);

  const groupedOptions = [
    {
      label: <span>Component Output</span>,
      title: 'Component Output',
      options: componentIdOptions,
    },
    {
      label: <span>Begin Input</span>,
      title: 'Begin Input',
      options: query.map((x) => ({
        label: x.name,
        value: `begin@${x.key}`,
      })),
    },
  ];

  return groupedOptions;
};

export const useGetComponentLabelByValue = (nodeId: string) => {
  const options = useBuildComponentIdSelectOptions(nodeId);
  const flattenOptions = useMemo(
    () =>
      options.reduce<DefaultOptionType[]>((pre, cur) => {
        return [...pre, ...cur.options];
      }, []),
    [options],
  );

  const getLabel = useCallback(
    (val?: string) => {
      return flattenOptions.find((x) => x.value === val)?.label;
    },
    [flattenOptions],
  );
  return getLabel;
};
