import { AgentGlobals } from '@/constants/agent';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { Edge } from '@xyflow/react';
import { DefaultOptionType } from 'antd/es/select';
import { t } from 'i18next';
import { isEmpty } from 'lodash';
import get from 'lodash/get';
import { useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { BeginId, BeginQueryType, Operator, VariableType } from '../constant';
import { AgentFormContext } from '../context';
import { buildBeginInputListFromObject } from '../form/begin-form/utils';
import { BeginQuery } from '../interface';
import useGraphStore from '../store';

export function useSelectBeginNodeDataInputs() {
  const getNode = useGraphStore((state) => state.getNode);

  return buildBeginInputListFromObject(
    getNode(BeginId)?.data?.form?.inputs ?? {},
  );
}

export const useGetBeginNodeDataQuery = () => {
  const getNode = useGraphStore((state) => state.getNode);

  const getBeginNodeDataQuery = useCallback(() => {
    return buildBeginInputListFromObject(
      get(getNode(BeginId), 'data.form.inputs', {}),
    );
  }, [getNode]);

  return getBeginNodeDataQuery;
};

export const useGetBeginNodeDataInputs = () => {
  const getNode = useGraphStore((state) => state.getNode);

  const inputs = get(getNode(BeginId), 'data.form.inputs', {});

  const beginNodeDataInputs = useMemo(() => {
    return buildBeginInputListFromObject(inputs);
  }, [inputs]);

  return beginNodeDataInputs;
};

export const useGetBeginNodeDataQueryIsSafe = () => {
  const [isBeginNodeDataQuerySafe, setIsBeginNodeDataQuerySafe] =
    useState(false);
  const inputs = useSelectBeginNodeDataInputs();
  const nodes = useGraphStore((state) => state.nodes);

  useEffect(() => {
    const query: BeginQuery[] = inputs;
    const isSafe = !query.some((q) => !q.optional && q.type === 'file');
    setIsBeginNodeDataQuerySafe(isSafe);
  }, [inputs, nodes]);

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

export function buildOutputOptions(
  outputs: Record<string, any> = {},
  nodeId?: string,
) {
  return Object.keys(outputs).map((x) => ({
    label: x,
    value: `${nodeId}@${x}`,
    type: outputs[x]?.type,
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

const StringList = [
  BeginQueryType.Line,
  BeginQueryType.Paragraph,
  BeginQueryType.Options,
];

function transferToVariableType(type: string) {
  if (StringList.some((x) => x === type)) {
    return VariableType.String;
  }
  return type;
}

export function useBuildBeginVariableOptions() {
  const inputs = useSelectBeginNodeDataInputs();

  const options = useMemo(() => {
    return [
      {
        label: <span>{t('flow.beginInput')}</span>,
        title: 'Begin Input',
        options: inputs.map((x) => ({
          label: x.name,
          value: `begin@${x.key}`,
          type: transferToVariableType(x.type),
        })),
      },
    ];
  }, [inputs]);

  return options;
}

export const useBuildVariableOptions = (nodeId?: string, parentId?: string) => {
  const nodeOutputOptions = useBuildNodeOutputOptions(nodeId);
  const parentNodeOutputOptions = useBuildNodeOutputOptions(parentId);
  const beginOptions = useBuildBeginVariableOptions();

  const options = useMemo(() => {
    return [...beginOptions, ...nodeOutputOptions, ...parentNodeOutputOptions];
  }, [beginOptions, nodeOutputOptions, parentNodeOutputOptions]);

  return options;
};

export function useBuildQueryVariableOptions(n?: RAGFlowNodeType) {
  const { data } = useFetchAgent();
  const node = useContext(AgentFormContext) || n;
  const options = useBuildVariableOptions(node?.id, node?.parentId);

  const nextOptions = useMemo(() => {
    const globals = data?.dsl?.globals ?? {};
    const globalOptions = Object.entries(globals).map(([key, value]) => ({
      label: key,
      value: key,
      type: Array.isArray(value)
        ? `${VariableType.Array}${key === AgentGlobals.SysFiles ? '<file>' : ''}`
        : typeof value,
    }));
    return [
      { ...options[0], options: [...options[0]?.options, ...globalOptions] },
      ...options.slice(1),
    ];
  }, [data.dsl?.globals, options]);

  return nextOptions;
}

export function useBuildComponentIdOptions(nodeId?: string, parentId?: string) {
  const nodes = useGraphStore((state) => state.nodes);

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

  return [
    {
      label: <span>Component Output</span>,
      title: 'Component Output',
      options: componentIdOptions,
    },
  ];
}

export function useBuildComponentIdAndBeginOptions(
  nodeId?: string,
  parentId?: string,
) {
  const componentIdOptions = useBuildComponentIdOptions(nodeId, parentId);
  const beginOptions = useBuildBeginVariableOptions();

  return [...beginOptions, ...componentIdOptions];
}

export const useGetComponentLabelByValue = (nodeId: string) => {
  const options = useBuildComponentIdAndBeginOptions(nodeId);

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

export function useGetVariableLabelByValue(nodeId: string) {
  const { getNode } = useGraphStore((state) => state);
  const nextOptions = useBuildQueryVariableOptions(getNode(nodeId));

  const flattenOptions = useMemo(() => {
    return nextOptions.reduce<DefaultOptionType[]>((pre, cur) => {
      return [...pre, ...cur.options];
    }, []);
  }, [nextOptions]);

  const getLabel = useCallback(
    (val?: string) => {
      return flattenOptions.find((x) => x.value === val)?.label;
    },
    [flattenOptions],
  );
  return getLabel;
}
