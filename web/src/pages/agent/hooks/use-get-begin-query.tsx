import { AgentGlobals, AgentStructuredOutputField } from '@/constants/agent';
import { useFetchAgent } from '@/hooks/use-agent-request';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import {
  buildNodeOutputOptions,
  buildOutputOptions,
  buildUpstreamNodeOutputOptions,
  isAgentStructured,
} from '@/utils/canvas-util';
import { DefaultOptionType } from 'antd/es/select';
import { t } from 'i18next';
import { flatten, isEmpty, toLower } from 'lodash';
import get from 'lodash/get';
import { MessageSquareCode } from 'lucide-react';
import { useCallback, useContext, useEffect, useMemo, useState } from 'react';
import {
  AgentDialogueMode,
  AgentVariableType,
  BeginId,
  BeginQueryType,
  BeginQueryTypeMap,
  JsonSchemaDataType,
  Operator,
  VariableType,
} from '../constant';
import { AgentFormContext } from '../context';
import { buildBeginInputListFromObject } from '../form/begin-form/utils';
import { BeginQuery } from '../interface';
import OperatorIcon from '../operator-icon';
import useGraphStore from '../store';
import {
  useFindAgentStructuredOutputLabelByValue,
  useFindAgentStructuredOutputTypeByValue,
} from './use-build-structured-output';

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

export function useBuildUpstreamNodeOutputOptions(nodeId?: string) {
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);

  return useMemo(() => {
    return buildUpstreamNodeOutputOptions({
      nodes,
      edges,
      nodeId,
    });
  }, [edges, nodeId, nodes]);
}

export function useBuildParentOutputOptions(parentId?: string) {
  const { getNode, getOperatorTypeFromId } = useGraphStore((state) => state);
  const parentNode = getNode(parentId);

  const parentType = getOperatorTypeFromId(parentId);

  if (
    parentType &&
    [Operator.Loop].includes(parentType as Operator) &&
    parentNode
  ) {
    const options = buildOutputOptions(parentNode);
    if (options) {
      return [options];
    }
  }

  return [];
}

// exclude nodes with branches
const ExcludedNodes = [Operator.Categorize, Operator.Begin, Operator.Note];

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

export function useBuildBeginDynamicVariableOptions() {
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

const Env = 'env.';

export function useBuildGlobalWithBeginVariableOptions() {
  const { data } = useFetchAgent();
  const dynamicBeginOptions = useBuildBeginDynamicVariableOptions();
  const globals = data?.dsl?.globals ?? {};
  const globalOptions = Object.entries(globals)
    .filter(([key]) => !key.startsWith(Env))
    .map(([key, value]) => ({
      label: key,
      value: key,
      icon: <OperatorIcon name={Operator.Begin} className="block" />,
      parentLabel: <span>{t('flow.beginInput')}</span>,
      type: Array.isArray(value)
        ? `${VariableType.Array}${key === AgentGlobals.SysFiles ? '<file>' : ''}`
        : typeof value,
    }));

  return [
    {
      ...dynamicBeginOptions[0],
      options: [...(dynamicBeginOptions[0]?.options ?? []), ...globalOptions],
    },
  ];
}

export function useBuildConversationVariableOptions() {
  const { data } = useFetchAgent();

  const conversationVariables = useMemo(
    () => data?.dsl?.variables ?? {},
    [data?.dsl?.variables],
  );

  const options = useMemo(() => {
    return [
      {
        label: <span>{t('flow.conversationVariable')}</span>,
        title: t('flow.conversationVariable'),
        options: Object.entries(conversationVariables).map(([key, value]) => {
          const keyWithPrefix = `${Env}${key}`;
          return {
            label: keyWithPrefix,
            parentLabel: <span>{t('flow.conversationVariable')}</span>,
            icon: <MessageSquareCode className="size-3" />,
            value: keyWithPrefix,
            type: value.type,
          };
        }),
      },
    ];
  }, [conversationVariables]);

  return options;
}

export const useBuildVariableOptions = (nodeId?: string, parentId?: string) => {
  const upstreamNodeOutputOptions = useBuildUpstreamNodeOutputOptions(nodeId);
  const parentNodeOutputOptions = useBuildParentOutputOptions(parentId);
  const parentUpstreamNodeOutputOptions =
    useBuildUpstreamNodeOutputOptions(parentId);

  const options = useMemo(() => {
    return [
      ...upstreamNodeOutputOptions,
      ...parentNodeOutputOptions,
      ...parentUpstreamNodeOutputOptions,
    ];
  }, [
    upstreamNodeOutputOptions,
    parentNodeOutputOptions,
    parentUpstreamNodeOutputOptions,
  ]);

  return options;
};

export type BuildQueryVariableOptions = {
  nodeIds?: string[];
  variablesExceptOperatorOutputs?: AgentVariableType[];
};

export function useBuildQueryVariableOptions({
  n,
  nodeIds = [],
  variablesExceptOperatorOutputs, // Variables other than operator output variables
}: {
  n?: RAGFlowNodeType;
} & BuildQueryVariableOptions = {}) {
  const node = useContext(AgentFormContext) || n;
  const nodes = useGraphStore((state) => state.nodes);

  const options = useBuildVariableOptions(node?.id, node?.parentId);

  const conversationOptions = useBuildConversationVariableOptions();

  const globalWithBeginVariableOptions =
    useBuildGlobalWithBeginVariableOptions();

  const AgentVariableOptionsMap = {
    [AgentVariableType.Begin]: globalWithBeginVariableOptions,
    [AgentVariableType.Conversation]: conversationOptions,
  };

  const nextOptions = useMemo(() => {
    return [
      ...globalWithBeginVariableOptions,
      ...conversationOptions,
      ...options,
    ];
  }, [conversationOptions, globalWithBeginVariableOptions, options]);

  // Which options are entirely under external control?
  if (!isEmpty(nodeIds) || !isEmpty(variablesExceptOperatorOutputs)) {
    const nodeOutputOptions = buildNodeOutputOptions({ nodes, nodeIds });

    const variablesExceptOperatorOutputsOptions =
      variablesExceptOperatorOutputs?.map((x) => AgentVariableOptionsMap[x]) ??
      [];

    return [
      ...flatten(variablesExceptOperatorOutputsOptions),
      ...nodeOutputOptions,
    ];
  }
  return nextOptions;
}

export function useFilterQueryVariableOptionsByTypes({
  types,
  nodeIds = [],
  variablesExceptOperatorOutputs,
}: {
  types?: JsonSchemaDataType[];
} & BuildQueryVariableOptions) {
  const nextOptions = useBuildQueryVariableOptions({
    nodeIds,
    variablesExceptOperatorOutputs,
  });

  const filteredOptions = useMemo(() => {
    return !isEmpty(types)
      ? nextOptions.map((x) => {
          return {
            ...x,
            options: x.options.filter(
              (y) =>
                types?.some((x) =>
                  toLower(x).startsWith('array')
                    ? toLower(y.type).includes(toLower(x))
                    : toLower(y.type) === toLower(x),
                ) ||
                // agent structured output
                isAgentStructured(
                  y.value,
                  y.value.slice(-AgentStructuredOutputField.length),
                ),
            ),
          };
        })
      : nextOptions;
  }, [nextOptions, types]);

  return filteredOptions;
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
  const beginOptions = useBuildBeginDynamicVariableOptions();

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

export function flatOptions(options: DefaultOptionType[]) {
  return options.reduce<DefaultOptionType[]>((pre, cur) => {
    return [...pre, ...cur.options];
  }, []);
}

export function useFlattenQueryVariableOptions({
  nodeId,
  nodeIds = [],
  variablesExceptOperatorOutputs,
}: {
  nodeId?: string;
} & BuildQueryVariableOptions = {}) {
  const { getNode } = useGraphStore((state) => state);
  const nextOptions = useBuildQueryVariableOptions({
    n: getNode(nodeId),
    nodeIds,
    variablesExceptOperatorOutputs,
  });

  const flattenOptions = useMemo(() => {
    return flatOptions(nextOptions);
  }, [nextOptions]);

  return flattenOptions;
}

export function useGetVariableLabelOrTypeByValue({
  nodeId,
  nodeIds = [],
  variablesExceptOperatorOutputs,
}: {
  nodeId?: string;
} & BuildQueryVariableOptions = {}) {
  const flattenOptions = useFlattenQueryVariableOptions({
    nodeId,
    nodeIds,
    variablesExceptOperatorOutputs,
  });
  const findAgentStructuredOutputTypeByValue =
    useFindAgentStructuredOutputTypeByValue();
  const findAgentStructuredOutputLabel =
    useFindAgentStructuredOutputLabelByValue();

  const getItem = useCallback(
    (val?: string) => {
      return flattenOptions.find((x) => x.value === val);
    },
    [flattenOptions],
  );

  const getLabel = useCallback(
    (val?: string) => {
      const item = getItem(val);
      if (item) {
        return (
          <div>
            {item.parentLabel} / {item.label}
          </div>
        );
      }
      return getItem(val)?.label || findAgentStructuredOutputLabel(val);
    },
    [findAgentStructuredOutputLabel, getItem],
  );

  const getType = useCallback(
    (val?: string) => {
      const currentType =
        getItem(val)?.type || findAgentStructuredOutputTypeByValue(val);

      if (currentType && currentType in BeginQueryTypeMap) {
        return BeginQueryTypeMap[currentType as BeginQueryType];
      }

      return currentType;
    },
    [findAgentStructuredOutputTypeByValue, getItem],
  );

  return { getLabel, getType };
}
