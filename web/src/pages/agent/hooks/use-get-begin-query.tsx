import { useFetchAgent } from '@/hooks/use-agent-request';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { Edge } from '@xyflow/react';
import { DefaultOptionType } from 'antd/es/select';
import { isEmpty } from 'lodash';
import get from 'lodash/get';
import { useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { BeginId, Operator } from '../constant';
import { AgentFormContext } from '../context';
import { buildBeginInputListFromObject } from '../form/begin-form/utils';
import { BeginQuery } from '../interface';
import useGraphStore from '../store';

export const useGetBeginNodeDataQuery = () => {
  const getNode = useGraphStore((state) => state.getNode);

  const getBeginNodeDataQuery = useCallback(() => {
    return buildBeginInputListFromObject(
      get(getNode(BeginId), 'data.form.inputs', {}),
    );
  }, [getNode]);

  return getBeginNodeDataQuery;
};

export const useGetBeginNodeDataQueryIsSafe = () => {
  const [isBeginNodeDataQuerySafe, setIsBeginNodeDataQuerySafe] =
    useState(false);
  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();
  const nodes = useGraphStore((state) => state.nodes);

  useEffect(() => {
    const query: BeginQuery[] = getBeginNodeDataQuery();
    const isSafe = !query.some((q) => !q.optional && q.type === 'file');
    setIsBeginNodeDataQuerySafe(isSafe);
  }, [getBeginNodeDataQuery, nodes]);

  return isBeginNodeDataQuerySafe;
};

function filterAllUpstreamNodeIds(edges: Edge[], nodeIds: string[]) {
  return nodeIds.reduce<string[]>((pre, nodeId) => {
    const currentEdges = edges.filter((x) => x.target === nodeId);

    const upstreamNodeIds: string[] = currentEdges.map((x) => x.source);

    const ids = upstreamNodeIds.concat(
      filterAllUpstreamNodeIds(edges, upstreamNodeIds),
    );

    ids.forEach((x) => {
      if (pre.every((y) => y !== x)) {
        pre.push(x);
      }
    });

    return pre;
  }, []);
}

function buildOutputOptions(
  outputs: Record<string, any> = {},
  nodeId?: string,
) {
  return Object.keys(outputs).map((x) => ({
    label: x,
    value: `${nodeId}@${x}`,
  }));
}

export function useBuildNodeOutputOptions(nodeId?: string) {
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);

  const nodeOutputOptions = useMemo(() => {
    if (!nodeId) {
      return [];
    }
    const upstreamIds = filterAllUpstreamNodeIds(edges, [nodeId]);

    const nodeWithOutputList = nodes.filter(
      (x) =>
        upstreamIds.some((y) => y === x.id) && !isEmpty(x.data?.form?.outputs),
    );

    return nodeWithOutputList
      .filter((x) => x.id !== nodeId)
      .map((x) => ({
        label: x.data.name,
        value: x.id,
        title: x.data.name,
        options: buildOutputOptions(x.data.form.outputs, x.id),
      }));
  }, [edges, nodeId, nodes]);

  return nodeOutputOptions;
}

// exclude nodes with branches
const ExcludedNodes = [
  Operator.Categorize,
  Operator.Relevant,
  Operator.Begin,
  Operator.Note,
];

export const useBuildComponentIdSelectOptions = (
  nodeId?: string,
  parentId?: string,
) => {
  const nodes = useGraphStore((state) => state.nodes);
  const getBeginNodeDataQuery = useGetBeginNodeDataQuery();

  const nodeOutputOptions = useBuildNodeOutputOptions(nodeId);

  // Limit the nodes inside iteration to only reference peer nodes with the same parentId and other external nodes other than their parent nodes
  const filterChildNodesToSameParentOrExternal = useCallback(
    (node: RAGFlowNodeType) => {
      // Node inside iteration
      if (parentId) {
        return (
          (node.parentId === parentId || node.parentId === undefined) &&
          node.id !== parentId
        );
      }

      return node.parentId === undefined; // The outermost node
    },
    [parentId],
  );

  const componentIdOptions = useMemo(() => {
    return nodes
      .filter(
        (x) =>
          x.id !== nodeId &&
          !ExcludedNodes.some((y) => y === x.data.label) &&
          filterChildNodesToSameParentOrExternal(x),
      )
      .map((x) => ({ label: x.data.name, value: x.id }));
  }, [nodes, nodeId, filterChildNodesToSameParentOrExternal]);

  const options = useMemo(() => {
    const query: BeginQuery[] = getBeginNodeDataQuery();
    return [
      {
        label: <span>Begin Input</span>,
        title: 'Begin Input',
        options: query.map((x) => ({
          label: x.name,
          value: `begin@${x.key}`,
        })),
      },
      ...nodeOutputOptions,
    ];
  }, [getBeginNodeDataQuery, nodeOutputOptions]);

  return options;
};

export const useGetComponentLabelByValue = (nodeId: string) => {
  const options = useBuildComponentIdSelectOptions(nodeId);

  const flattenOptions = useMemo(() => {
    return options.reduce<DefaultOptionType[]>((pre, cur) => {
      return [...pre, ...cur.options];
    }, []);
  }, [options]);

  const getLabel = useCallback(
    (val?: string) => {
      return flattenOptions.find((x) => x.value === val)?.label;
    },
    [flattenOptions],
  );
  return getLabel;
};

export function useBuildQueryVariableOptions() {
  const { data } = useFetchAgent();
  const node = useContext(AgentFormContext);
  const options = useBuildComponentIdSelectOptions(node?.id, node?.parentId);

  const nextOptions = useMemo(() => {
    const globalOptions = Object.keys(data?.dsl?.globals ?? {}).map((x) => ({
      label: x,
      value: x,
    }));
    return [
      { ...options[0], options: [...options[0]?.options, ...globalOptions] },
      ...options.slice(1),
    ];
  }, [data.dsl.globals, options]);

  return nextOptions;
}
