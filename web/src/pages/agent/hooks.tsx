import {
  Connection,
  Edge,
  getOutgoers,
  Node,
  Position,
  ReactFlowInstance,
} from '@xyflow/react';
import React, { useCallback, useEffect, useMemo, useState } from 'react';
// import { shallow } from 'zustand/shallow';
import { settledModelVariableMap } from '@/constants/knowledge';
import { useFetchModelId } from '@/hooks/logic-hooks';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { humanId } from 'human-id';
import { get, lowerFirst, omit } from 'lodash';
import { UseFormReturn } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import {
  initialAgentValues,
  initialAkShareValues,
  initialArXivValues,
  initialBaiduFanyiValues,
  initialBaiduValues,
  initialBeginValues,
  initialBingValues,
  initialCategorizeValues,
  initialCodeValues,
  initialConcentratorValues,
  initialCrawlerValues,
  initialDeepLValues,
  initialDuckValues,
  initialEmailValues,
  initialExeSqlValues,
  initialGithubValues,
  initialGoogleScholarValues,
  initialGoogleValues,
  initialInvokeValues,
  initialIterationValues,
  initialJin10Values,
  initialKeywordExtractValues,
  initialMessageValues,
  initialNoteValues,
  initialPubMedValues,
  initialQWeatherValues,
  initialRelevantValues,
  initialRetrievalValues,
  initialRewriteQuestionValues,
  initialStringTransformValues,
  initialSwitchValues,
  initialTavilyExtractValues,
  initialTavilyValues,
  initialTuShareValues,
  initialUserFillUpValues,
  initialWaitingDialogueValues,
  initialWenCaiValues,
  initialWikipediaValues,
  initialYahooFinanceValues,
  NodeMap,
  Operator,
  RestrictedUpstreamMap,
} from './constant';
import useGraphStore, { RFState } from './store';
import {
  buildCategorizeObjectFromList,
  generateNodeNamesWithIncreasingIndex,
  getNodeDragHandle,
  getRelativePositionToIterationNode,
  replaceIdWithText,
} from './utils';

const selector = (state: RFState) => ({
  nodes: state.nodes,
  edges: state.edges,
  onNodesChange: state.onNodesChange,
  onEdgesChange: state.onEdgesChange,
  onConnect: state.onConnect,
  setNodes: state.setNodes,
  onSelectionChange: state.onSelectionChange,
  onEdgeMouseEnter: state.onEdgeMouseEnter,
  onEdgeMouseLeave: state.onEdgeMouseLeave,
});

export const useSelectCanvasData = () => {
  // return useStore(useShallow(selector)); // throw error
  // return useStore(selector, shallow);
  return useGraphStore(selector);
};

export const useInitializeOperatorParams = () => {
  const llmId = useFetchModelId();

  const initialFormValuesMap = useMemo(() => {
    return {
      [Operator.Begin]: initialBeginValues,
      [Operator.Retrieval]: initialRetrievalValues,
      [Operator.Categorize]: { ...initialCategorizeValues, llm_id: llmId },
      [Operator.Relevant]: { ...initialRelevantValues, llm_id: llmId },
      [Operator.RewriteQuestion]: {
        ...initialRewriteQuestionValues,
        llm_id: llmId,
      },
      [Operator.Message]: initialMessageValues,
      [Operator.KeywordExtract]: {
        ...initialKeywordExtractValues,
        llm_id: llmId,
      },
      [Operator.DuckDuckGo]: initialDuckValues,
      [Operator.Baidu]: initialBaiduValues,
      [Operator.Wikipedia]: initialWikipediaValues,
      [Operator.PubMed]: initialPubMedValues,
      [Operator.ArXiv]: initialArXivValues,
      [Operator.Google]: initialGoogleValues,
      [Operator.Bing]: initialBingValues,
      [Operator.GoogleScholar]: initialGoogleScholarValues,
      [Operator.DeepL]: initialDeepLValues,
      [Operator.GitHub]: initialGithubValues,
      [Operator.BaiduFanyi]: initialBaiduFanyiValues,
      [Operator.QWeather]: initialQWeatherValues,
      [Operator.ExeSQL]: { ...initialExeSqlValues, llm_id: llmId },
      [Operator.Switch]: initialSwitchValues,
      [Operator.WenCai]: initialWenCaiValues,
      [Operator.AkShare]: initialAkShareValues,
      [Operator.YahooFinance]: initialYahooFinanceValues,
      [Operator.Jin10]: initialJin10Values,
      [Operator.Concentrator]: initialConcentratorValues,
      [Operator.TuShare]: initialTuShareValues,
      [Operator.Note]: initialNoteValues,
      [Operator.Crawler]: initialCrawlerValues,
      [Operator.Invoke]: initialInvokeValues,
      [Operator.Email]: initialEmailValues,
      [Operator.Iteration]: initialIterationValues,
      [Operator.IterationStart]: initialIterationValues,
      [Operator.Code]: initialCodeValues,
      [Operator.WaitingDialogue]: initialWaitingDialogueValues,
      [Operator.Agent]: { ...initialAgentValues, llm_id: llmId },
      [Operator.TavilySearch]: initialTavilyValues,
      [Operator.TavilyExtract]: initialTavilyExtractValues,
      [Operator.Tool]: {},
      [Operator.UserFillUp]: initialUserFillUpValues,
      [Operator.StringTransform]: initialStringTransformValues,
    };
  }, [llmId]);

  const initializeOperatorParams = useCallback(
    (operatorName: Operator) => {
      return initialFormValuesMap[operatorName];
    },
    [initialFormValuesMap],
  );

  return initializeOperatorParams;
};

export const useHandleDrag = () => {
  const handleDragStart = useCallback(
    (operatorId: string) => (ev: React.DragEvent<HTMLDivElement>) => {
      ev.dataTransfer.setData('application/@xyflow/react', operatorId);
      ev.dataTransfer.effectAllowed = 'move';
    },
    [],
  );

  return { handleDragStart };
};

export const useGetNodeName = () => {
  const { t } = useTranslation();

  return (type: string) => {
    const name = t(`flow.${lowerFirst(type)}`);
    return name;
  };
};

export const useHandleDrop = () => {
  const addNode = useGraphStore((state) => state.addNode);
  const nodes = useGraphStore((state) => state.nodes);
  const [reactFlowInstance, setReactFlowInstance] =
    useState<ReactFlowInstance<any, any>>();
  const initializeOperatorParams = useInitializeOperatorParams();
  const getNodeName = useGetNodeName();

  const onDragOver = useCallback((event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
  }, []);

  const onDrop = useCallback(
    (event: React.DragEvent<HTMLDivElement>) => {
      event.preventDefault();

      const type = event.dataTransfer.getData('application/@xyflow/react');

      // check if the dropped element is valid
      if (typeof type === 'undefined' || !type) {
        return;
      }

      // reactFlowInstance.project was renamed to reactFlowInstance.screenToFlowPosition
      // and you don't need to subtract the reactFlowBounds.left/top anymore
      // details: https://@xyflow/react.dev/whats-new/2023-11-10
      const position = reactFlowInstance?.screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      });
      const newNode: Node<any> = {
        id: `${type}:${humanId()}`,
        type: NodeMap[type as Operator] || 'ragNode',
        position: position || {
          x: 0,
          y: 0,
        },
        data: {
          label: `${type}`,
          name: generateNodeNamesWithIncreasingIndex(getNodeName(type), nodes),
          form: initializeOperatorParams(type as Operator),
        },
        sourcePosition: Position.Right,
        targetPosition: Position.Left,
        dragHandle: getNodeDragHandle(type),
      };

      if (type === Operator.Iteration) {
        newNode.width = 500;
        newNode.height = 250;
        const iterationStartNode: Node<any> = {
          id: `${Operator.IterationStart}:${humanId()}`,
          type: 'iterationStartNode',
          position: { x: 50, y: 100 },
          // draggable: false,
          data: {
            label: Operator.IterationStart,
            name: Operator.IterationStart,
            form: {},
          },
          parentId: newNode.id,
          extent: 'parent',
        };
        addNode(newNode);
        addNode(iterationStartNode);
      } else {
        const subNodeOfIteration = getRelativePositionToIterationNode(
          nodes,
          position,
        );
        if (subNodeOfIteration) {
          newNode.parentId = subNodeOfIteration.parentId;
          newNode.position = subNodeOfIteration.position;
          newNode.extent = 'parent';
        }
        addNode(newNode);
      }
    },
    [reactFlowInstance, getNodeName, nodes, initializeOperatorParams, addNode],
  );

  return { onDrop, onDragOver, setReactFlowInstance, reactFlowInstance };
};

export const useHandleFormValuesChange = (
  operatorName: Operator,
  id?: string,
  form?: UseFormReturn,
) => {
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);
  const handleValuesChange = useCallback(
    (changedValues: any, values: any) => {
      let nextValues: any = values;
      // Fixed the issue that the related form value does not change after selecting the freedom field of the model
      if (
        Object.keys(changedValues).length === 1 &&
        'parameter' in changedValues &&
        changedValues['parameter'] in settledModelVariableMap
      ) {
        nextValues = {
          ...values,
          ...settledModelVariableMap[
            changedValues['parameter'] as keyof typeof settledModelVariableMap
          ],
        };
      }
      if (id) {
        updateNodeForm(id, nextValues);
      }
    },
    [updateNodeForm, id],
  );

  useEffect(() => {
    const subscription = form?.watch((value, { name, type, values }) => {
      if (id && name) {
        console.log(
          '🚀 ~ useEffect ~ value:',
          name,
          type,
          values,
          operatorName,
        );
        let nextValues: any = value;

        // Fixed the issue that the related form value does not change after selecting the freedom field of the model
        if (
          name === 'parameter' &&
          value['parameter'] in settledModelVariableMap
        ) {
          nextValues = {
            ...value,
            ...settledModelVariableMap[
              value['parameter'] as keyof typeof settledModelVariableMap
            ],
          };
        }

        const categoryDescriptionRegex = /items\.\d+\.name/g;
        if (
          operatorName === Operator.Categorize &&
          categoryDescriptionRegex.test(name)
        ) {
          nextValues = {
            ...omit(value, 'items'),
            category_description: buildCategorizeObjectFromList(value.items),
          };
        }
        // Manually triggered form updates are synchronized to the canvas
        if (type) {
          updateNodeForm(id, nextValues);
        }
      }
    });
    return () => subscription?.unsubscribe();
  }, [form, form?.watch, id, operatorName, updateNodeForm]);

  return { handleValuesChange };
};

export const useValidateConnection = () => {
  const { getOperatorTypeFromId, getParentIdById, edges, nodes } =
    useGraphStore((state) => state);

  const isSameNodeChild = useCallback(
    (connection: Connection | Edge) => {
      const sourceParentId = getParentIdById(connection.source);
      const targetParentId = getParentIdById(connection.target);
      if (sourceParentId || targetParentId) {
        return sourceParentId === targetParentId;
      }
      return true;
    },
    [getParentIdById],
  );

  const hasCanvasCycle = useCallback(
    (connection: Connection | Edge) => {
      const target = nodes.find((node) => node.id === connection.target);
      const hasCycle = (node: RAGFlowNodeType, visited = new Set()) => {
        if (visited.has(node.id)) return false;

        visited.add(node.id);

        for (const outgoer of getOutgoers(node, nodes, edges)) {
          if (outgoer.id === connection.source) return true;
          if (hasCycle(outgoer, visited)) return true;
        }
      };

      if (target?.id === connection.source) return false;

      return target ? !hasCycle(target) : false;
    },
    [edges, nodes],
  );

  // restricted lines cannot be connected successfully.
  const isValidConnection = useCallback(
    (connection: Connection | Edge) => {
      // node cannot connect to itself
      const isSelfConnected = connection.target === connection.source;

      // limit the connection between two nodes to only one connection line in one direction
      // const hasLine = edges.some(
      //   (x) => x.source === connection.source && x.target === connection.target,
      // );

      const ret =
        !isSelfConnected &&
        RestrictedUpstreamMap[
          getOperatorTypeFromId(connection.source) as Operator
        ]?.every((x) => x !== getOperatorTypeFromId(connection.target)) &&
        isSameNodeChild(connection) &&
        hasCanvasCycle(connection);
      return ret;
    },
    [getOperatorTypeFromId, hasCanvasCycle, isSameNodeChild],
  );

  return isValidConnection;
};

export const useReplaceIdWithName = () => {
  const getNode = useGraphStore((state) => state.getNode);

  const replaceIdWithName = useCallback(
    (id?: string) => {
      return getNode(id)?.data.name;
    },
    [getNode],
  );

  return replaceIdWithName;
};

export const useReplaceIdWithText = (output: unknown) => {
  const getNameById = useReplaceIdWithName();

  return {
    replacedOutput: replaceIdWithText(output, getNameById),
    getNameById,
  };
};

export const useDuplicateNode = () => {
  const duplicateNodeById = useGraphStore((store) => store.duplicateNode);
  const getNodeName = useGetNodeName();

  const duplicateNode = useCallback(
    (id: string, label: string) => {
      duplicateNodeById(id, getNodeName(label));
    },
    [duplicateNodeById, getNodeName],
  );

  return duplicateNode;
};

export const useCopyPaste = () => {
  const nodes = useGraphStore((state) => state.nodes);
  const duplicateNode = useDuplicateNode();

  const onCopyCapture = useCallback(
    (event: ClipboardEvent) => {
      if (get(event, 'srcElement.tagName') !== 'BODY') return;

      event.preventDefault();
      const nodesStr = JSON.stringify(
        nodes.filter((n) => n.selected && n.data.label !== Operator.Begin),
      );

      event.clipboardData?.setData('agent:nodes', nodesStr);
    },
    [nodes],
  );

  const onPasteCapture = useCallback(
    (event: ClipboardEvent) => {
      const nodes = JSON.parse(
        event.clipboardData?.getData('agent:nodes') || '[]',
      ) as RAGFlowNodeType[] | undefined;

      if (Array.isArray(nodes) && nodes.length) {
        event.preventDefault();
        nodes.forEach((n) => {
          duplicateNode(n.id, n.data.label);
        });
      }
    },
    [duplicateNode],
  );

  useEffect(() => {
    window.addEventListener('copy', onCopyCapture);
    return () => {
      window.removeEventListener('copy', onCopyCapture);
    };
  }, [onCopyCapture]);

  useEffect(() => {
    window.addEventListener('paste', onPasteCapture);
    return () => {
      window.removeEventListener('paste', onPasteCapture);
    };
  }, [onPasteCapture]);
};
