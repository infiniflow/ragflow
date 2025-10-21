import { AgentGlobals } from '@/constants/agent';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { buildNodeOutputOptions } from '@/utils/canvas-util';
import { DefaultOptionType } from 'antd/es/select';
import { t } from 'i18next';
import get from 'lodash/get';
import { useCallback, useContext, useEffect, useMemo, useState } from 'react';
import {
  AgentDialogueMode,
  BeginId,
  BeginQueryType,
  Operator,
  VariableType,
} from '../constant';
import { AgentFormContext } from '../context';
import { buildBeginInputListFromObject } from '../form/begin-form/utils';
import { BeginQuery } from '../interface';
import OperatorIcon from '../operator-icon';
import useGraphStore from '../store';

export function useSelectBeginNodeDataInputs() {
  const getNode = useGraphStore((state) => state.getNode);

  return buildBeginInputListFromObject(
    getNode(BeginId)?.data?.form?.inputs ?? {},
  );
}

export function useIsTaskMode(isTask?: boolean) {
  const getNode = useGraphStore((state) => state.getNode);

  return useMemo(() => {
    if (typeof isTask === 'boolean') {
      return isTask;
    }
    const node = getNode(BeginId);
    return node?.data?.form?.mode === AgentDialogueMode.Task;
  }, [getNode, isTask]);
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

export function useBuildNodeOutputOptions(nodeId?: string) {
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);

  return useMemo(() => {
    return buildNodeOutputOptions({
      nodes,
      edges,
      nodeId,
      Icon: ({ name }) => <OperatorIcon name={name as Operator}></OperatorIcon>,
    });
  }, [edges, nodeId, nodes]);
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
        title: t('flow.beginInput'),
        options: inputs.map((x) => ({
          label: x.name,
          parentLabel: <span>{t('flow.beginInput')}</span>,
          icon: <OperatorIcon name={Operator.Begin} className="block" />,
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
      icon: <OperatorIcon name={Operator.Begin} className="block" />,
      parentLabel: <span>{t('flow.beginInput')}</span>,
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
