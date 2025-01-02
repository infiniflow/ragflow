import {
  Connection,
  Edge,
  Node,
  Position,
  ReactFlowInstance,
} from '@xyflow/react';
import React, {
  ChangeEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';
// import { shallow } from 'zustand/shallow';
import { variableEnabledFieldMap } from '@/constants/chat';
import {
  ModelVariableType,
  settledModelVariableMap,
} from '@/constants/knowledge';
import { useFetchModelId } from '@/hooks/logic-hooks';
import { Variable } from '@/interfaces/database/chat';
import {
  ICategorizeForm,
  IRelevantForm,
  ISwitchForm,
  RAGFlowNodeType,
} from '@/interfaces/database/flow';
import { FormInstance, message } from 'antd';
import { humanId } from 'human-id';
import { get, isEmpty, lowerFirst, pick } from 'lodash';
import trim from 'lodash/trim';
import { useTranslation } from 'react-i18next';
import { v4 as uuid } from 'uuid';
import {
  NodeMap,
  Operator,
  RestrictedUpstreamMap,
  SwitchElseTo,
  initialAkShareValues,
  initialArXivValues,
  initialBaiduFanyiValues,
  initialBaiduValues,
  initialBeginValues,
  initialBingValues,
  initialCategorizeValues,
  initialConcentratorValues,
  initialCrawlerValues,
  initialDeepLValues,
  initialDuckValues,
  initialEmailValues,
  initialExeSqlValues,
  initialGenerateValues,
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
  initialSwitchValues,
  initialTemplateValues,
  initialTuShareValues,
  initialWenCaiValues,
  initialWikipediaValues,
  initialYahooFinanceValues,
} from './constant';
import useGraphStore, { RFState } from './store';
import {
  generateNodeNamesWithIncreasingIndex,
  generateSwitchHandleText,
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
      [Operator.Generate]: { ...initialGenerateValues, llm_id: llmId },
      [Operator.Answer]: {},
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
      [Operator.Template]: initialTemplateValues,
      [Operator.Email]: initialEmailValues,
      [Operator.Iteration]: initialIterationValues,
      [Operator.IterationStart]: initialIterationValues,
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

  return { onDrop, onDragOver, setReactFlowInstance };
};

export const useHandleFormValuesChange = (id?: string) => {
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

  return { handleValuesChange };
};

export const useSetLlmSetting = (
  form?: FormInstance,
  formData?: Record<string, any>,
) => {
  const initialLlmSetting = pick(
    formData,
    Object.values(variableEnabledFieldMap),
  );
  useEffect(() => {
    const switchBoxValues = Object.keys(variableEnabledFieldMap).reduce<
      Record<string, boolean>
    >((pre, field) => {
      pre[field] = isEmpty(initialLlmSetting)
        ? true
        : !!initialLlmSetting[
            variableEnabledFieldMap[
              field as keyof typeof variableEnabledFieldMap
            ] as keyof Variable
          ];
      return pre;
    }, {});
    let otherValues = settledModelVariableMap[ModelVariableType.Precise];
    if (!isEmpty(initialLlmSetting)) {
      otherValues = initialLlmSetting;
    }
    form?.setFieldsValue({
      ...switchBoxValues,
      ...otherValues,
    });
  }, [form, initialLlmSetting]);
};

export const useValidateConnection = () => {
  const { edges, getOperatorTypeFromId, getParentIdById } = useGraphStore(
    (state) => state,
  );

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

  // restricted lines cannot be connected successfully.
  const isValidConnection = useCallback(
    (connection: Connection | Edge) => {
      // node cannot connect to itself
      const isSelfConnected = connection.target === connection.source;

      // limit the connection between two nodes to only one connection line in one direction
      const hasLine = edges.some(
        (x) => x.source === connection.source && x.target === connection.target,
      );

      const ret =
        !isSelfConnected &&
        !hasLine &&
        RestrictedUpstreamMap[
          getOperatorTypeFromId(connection.source) as Operator
        ]?.every((x) => x !== getOperatorTypeFromId(connection.target)) &&
        isSameNodeChild(connection);
      return ret;
    },
    [edges, getOperatorTypeFromId, isSameNodeChild],
  );

  return isValidConnection;
};

export const useHandleNodeNameChange = ({
  id,
  data,
}: {
  id?: string;
  data: any;
}) => {
  const [name, setName] = useState<string>('');
  const { updateNodeName, nodes } = useGraphStore((state) => state);
  const previousName = data?.name;

  const handleNameBlur = useCallback(() => {
    const existsSameName = nodes.some((x) => x.data.name === name);
    if (trim(name) === '' || existsSameName) {
      if (existsSameName && previousName !== name) {
        message.error('The name cannot be repeated');
      }
      setName(previousName);
      return;
    }

    if (id) {
      updateNodeName(id, name);
    }
  }, [name, id, updateNodeName, previousName, nodes]);

  const handleNameChange = useCallback((e: ChangeEvent<any>) => {
    setName(e.target.value);
  }, []);

  useEffect(() => {
    setName(previousName);
  }, [previousName]);

  return { name, handleNameBlur, handleNameChange };
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

/**
 *  monitor changes in the data.form field of the categorize and relevant operators
 *  and then synchronize them to the edge
 */
export const useWatchNodeFormDataChange = () => {
  const { getNode, nodes, setEdgesByNodeId } = useGraphStore((state) => state);

  const buildCategorizeEdgesByFormData = useCallback(
    (nodeId: string, form: ICategorizeForm) => {
      // add
      // delete
      // edit
      const categoryDescription = form.category_description;
      const downstreamEdges = Object.keys(categoryDescription).reduce<Edge[]>(
        (pre, sourceHandle) => {
          const target = categoryDescription[sourceHandle]?.to;
          if (target) {
            pre.push({
              id: uuid(),
              source: nodeId,
              target,
              sourceHandle,
            });
          }

          return pre;
        },
        [],
      );

      setEdgesByNodeId(nodeId, downstreamEdges);
    },
    [setEdgesByNodeId],
  );

  const buildRelevantEdgesByFormData = useCallback(
    (nodeId: string, form: IRelevantForm) => {
      const downstreamEdges = ['yes', 'no'].reduce<Edge[]>((pre, cur) => {
        const target = form[cur as keyof IRelevantForm] as string;
        if (target) {
          pre.push({ id: uuid(), source: nodeId, target, sourceHandle: cur });
        }

        return pre;
      }, []);

      setEdgesByNodeId(nodeId, downstreamEdges);
    },
    [setEdgesByNodeId],
  );

  const buildSwitchEdgesByFormData = useCallback(
    (nodeId: string, form: ISwitchForm) => {
      // add
      // delete
      // edit
      const conditions = form.conditions;
      const downstreamEdges = conditions.reduce<Edge[]>((pre, _, idx) => {
        const target = conditions[idx]?.to;
        if (target) {
          pre.push({
            id: uuid(),
            source: nodeId,
            target,
            sourceHandle: generateSwitchHandleText(idx),
          });
        }

        return pre;
      }, []);

      // Splice the else condition of the conditional judgment to the edge list
      const elseTo = form[SwitchElseTo];
      if (elseTo) {
        downstreamEdges.push({
          id: uuid(),
          source: nodeId,
          target: elseTo,
          sourceHandle: SwitchElseTo,
        });
      }

      setEdgesByNodeId(nodeId, downstreamEdges);
    },
    [setEdgesByNodeId],
  );

  useEffect(() => {
    nodes.forEach((node) => {
      const currentNode = getNode(node.id);
      const form = currentNode?.data.form ?? {};
      const operatorType = currentNode?.data.label;
      switch (operatorType) {
        case Operator.Relevant:
          buildRelevantEdgesByFormData(node.id, form as IRelevantForm);
          break;
        case Operator.Categorize:
          buildCategorizeEdgesByFormData(node.id, form as ICategorizeForm);
          break;
        case Operator.Switch:
          buildSwitchEdgesByFormData(node.id, form as ISwitchForm);
          break;
        default:
          break;
      }
    });
  }, [
    nodes,
    buildCategorizeEdgesByFormData,
    getNode,
    buildRelevantEdgesByFormData,
    buildSwitchEdgesByFormData,
  ]);
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
