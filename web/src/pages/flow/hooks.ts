import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchFlow, useResetFlow, useSetFlow } from '@/hooks/flow-hooks';
import { IGraph } from '@/interfaces/database/flow';
import { useIsFetching } from '@tanstack/react-query';
import React, {
  ChangeEvent,
  KeyboardEventHandler,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { Connection, Edge, Node, Position, ReactFlowInstance } from 'reactflow';
// import { shallow } from 'zustand/shallow';
import { variableEnabledFieldMap } from '@/constants/chat';
import {
  ModelVariableType,
  settledModelVariableMap,
} from '@/constants/knowledge';
import { useFetchModelId, useSendMessageWithSse } from '@/hooks/logic-hooks';
import { Variable } from '@/interfaces/database/chat';
import api from '@/utils/api';
import { useDebounceEffect } from 'ahooks';
import { FormInstance, message } from 'antd';
import { humanId } from 'human-id';
import { lowerFirst } from 'lodash';
import trim from 'lodash/trim';
import { useTranslation } from 'react-i18next';
import { useParams } from 'umi';
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
  initialExeSqlValues,
  initialGenerateValues,
  initialGithubValues,
  initialGoogleScholarValues,
  initialGoogleValues,
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
  initialTuShareValues,
  initialWenCaiValues,
  initialWikipediaValues,
  initialYahooFinanceValues,
} from './constant';
import { ICategorizeForm, IRelevantForm, ISwitchForm } from './interface';
import useGraphStore, { RFState } from './store';
import {
  buildDslComponentsByGraph,
  generateSwitchHandleText,
  receiveMessageError,
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
      [Operator.ExeSQL]: initialExeSqlValues,
      [Operator.Switch]: initialSwitchValues,
      [Operator.WenCai]: initialWenCaiValues,
      [Operator.AkShare]: initialAkShareValues,
      [Operator.YahooFinance]: initialYahooFinanceValues,
      [Operator.Jin10]: initialJin10Values,
      [Operator.Concentrator]: initialConcentratorValues,
      [Operator.TuShare]: initialTuShareValues,
      [Operator.Note]: initialNoteValues,
      [Operator.Crawler]: initialCrawlerValues,
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
      ev.dataTransfer.setData('application/reactflow', operatorId);
      ev.dataTransfer.effectAllowed = 'move';
    },
    [],
  );

  return { handleDragStart };
};

const splitName = (name: string) => {
  const names = name.split('_');
  const type = names.at(0);
  const index = Number(names.at(-1));

  return { type, index };
};

export const useHandleDrop = () => {
  const addNode = useGraphStore((state) => state.addNode);
  const nodes = useGraphStore((state) => state.nodes);
  const [reactFlowInstance, setReactFlowInstance] =
    useState<ReactFlowInstance<any, any>>();
  const initializeOperatorParams = useInitializeOperatorParams();
  const { t } = useTranslation();

  const onDragOver = useCallback((event: React.DragEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
  }, []);

  const generateNodeName = useCallback(
    (type: string) => {
      const name = t(`flow.${lowerFirst(type)}`);
      const templateNameList = nodes
        .filter((x) => {
          const temporaryName = x.data.name;

          const { type, index } = splitName(temporaryName);

          return (
            temporaryName.match(/_/g)?.length === 1 &&
            type === name &&
            !isNaN(index)
          );
        })
        .map((x) => {
          const temporaryName = x.data.name;
          const { index } = splitName(temporaryName);

          return {
            idx: index,
            name: temporaryName,
          };
        })
        .sort((a, b) => a.idx - b.idx);

      let index: number = 0;
      for (let i = 0; i < templateNameList.length; i++) {
        const idx = templateNameList[i]?.idx;
        const nextIdx = templateNameList[i + 1]?.idx;
        if (idx + 1 !== nextIdx) {
          index = idx + 1;
          break;
        }
      }

      return `${name}_${index}`;
    },
    [t, nodes],
  );

  const onDrop = useCallback(
    (event: React.DragEvent<HTMLDivElement>) => {
      event.preventDefault();

      const type = event.dataTransfer.getData('application/reactflow');

      // check if the dropped element is valid
      if (typeof type === 'undefined' || !type) {
        return;
      }

      // reactFlowInstance.project was renamed to reactFlowInstance.screenToFlowPosition
      // and you don't need to subtract the reactFlowBounds.left/top anymore
      // details: https://reactflow.dev/whats-new/2023-11-10
      const position = reactFlowInstance?.screenToFlowPosition({
        x: event.clientX,
        y: event.clientY,
      });
      const newNode = {
        id: `${type}:${humanId()}`,
        type: NodeMap[type as Operator] || 'ragNode',
        position: position || {
          x: 0,
          y: 0,
        },
        data: {
          label: `${type}`,
          name: generateNodeName(type),
          form: initializeOperatorParams(type as Operator),
        },
        sourcePosition: Position.Right,
        targetPosition: Position.Left,
      };

      addNode(newNode);
    },
    [reactFlowInstance, addNode, initializeOperatorParams, generateNodeName],
  );

  return { onDrop, onDragOver, setReactFlowInstance };
};

export const useShowDrawer = () => {
  const {
    clickedNodeId: clickNodeId,
    setClickedNodeId,
    getNode,
  } = useGraphStore((state) => state);
  const {
    visible: drawerVisible,
    hideModal: hideDrawer,
    showModal: showDrawer,
  } = useSetModalState();

  const handleShow = useCallback(
    (node: Node) => {
      setClickedNodeId(node.id);
      showDrawer();
    },
    [showDrawer, setClickedNodeId],
  );

  return {
    drawerVisible,
    hideDrawer,
    showDrawer: handleShow,
    clickedNode: getNode(clickNodeId),
  };
};

export const useHandleKeyUp = () => {
  const deleteEdge = useGraphStore((state) => state.deleteEdge);
  const handleKeyUp: KeyboardEventHandler = useCallback(
    (e) => {
      if (e.code === 'Delete') {
        deleteEdge();
      }
    },
    [deleteEdge],
  );

  return { handleKeyUp };
};

export const useSaveGraph = () => {
  const { data } = useFetchFlow();
  const { setFlow } = useSetFlow();
  const { id } = useParams();
  const { nodes, edges } = useGraphStore((state) => state);
  const saveGraph = useCallback(async () => {
    const dslComponents = buildDslComponentsByGraph(nodes, edges);
    return setFlow({
      id,
      title: data.title,
      dsl: { ...data.dsl, graph: { nodes, edges }, components: dslComponents },
    });
  }, [nodes, edges, setFlow, id, data]);

  return { saveGraph };
};

export const useWatchGraphChange = () => {
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);
  useDebounceEffect(
    () => {
      // console.info('useDebounceEffect');
    },
    [nodes, edges],
    {
      wait: 1000,
    },
  );
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

const useSetGraphInfo = () => {
  const { setEdges, setNodes } = useGraphStore((state) => state);
  const setGraphInfo = useCallback(
    ({ nodes = [], edges = [] }: IGraph) => {
      if (nodes.length || edges.length) {
        setNodes(nodes);
        setEdges(edges);
      }
    },
    [setEdges, setNodes],
  );
  return setGraphInfo;
};

export const useFetchDataOnMount = () => {
  const { loading, data, refetch } = useFetchFlow();
  const setGraphInfo = useSetGraphInfo();

  useEffect(() => {
    setGraphInfo(data?.dsl?.graph ?? ({} as IGraph));
  }, [setGraphInfo, data]);

  useWatchGraphChange();

  useEffect(() => {
    refetch();
  }, [refetch]);

  return { loading, flowDetail: data };
};

export const useFlowIsFetching = () => {
  return useIsFetching({ queryKey: ['flowDetail'] }) > 0;
};

export const useSetLlmSetting = (form?: FormInstance) => {
  const initialLlmSetting = undefined;

  useEffect(() => {
    const switchBoxValues = Object.keys(variableEnabledFieldMap).reduce<
      Record<string, boolean>
    >((pre, field) => {
      pre[field] =
        initialLlmSetting === undefined
          ? true
          : !!initialLlmSetting[
              variableEnabledFieldMap[
                field as keyof typeof variableEnabledFieldMap
              ] as keyof Variable
            ];
      return pre;
    }, {});
    const otherValues = settledModelVariableMap[ModelVariableType.Precise];
    form?.setFieldsValue({
      ...switchBoxValues,
      ...otherValues,
    });
  }, [form, initialLlmSetting]);
};

export const useValidateConnection = () => {
  const { edges, getOperatorTypeFromId } = useGraphStore((state) => state);
  // restricted lines cannot be connected successfully.
  const isValidConnection = useCallback(
    (connection: Connection) => {
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
        ]?.every((x) => x !== getOperatorTypeFromId(connection.target));
      return ret;
    },
    [edges, getOperatorTypeFromId],
  );

  return isValidConnection;
};

export const useHandleNodeNameChange = (node?: Node) => {
  const [name, setName] = useState<string>('');
  const { updateNodeName, nodes } = useGraphStore((state) => state);
  const previousName = node?.data.name;
  const id = node?.id;

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

export const useSaveGraphBeforeOpeningDebugDrawer = (show: () => void) => {
  const { id } = useParams();
  const { saveGraph } = useSaveGraph();
  const { resetFlow } = useResetFlow();
  const { refetch } = useFetchFlow();
  const { send } = useSendMessageWithSse(api.runCanvas);
  const handleRun = useCallback(async () => {
    const saveRet = await saveGraph();
    if (saveRet?.retcode === 0) {
      // Call the reset api before opening the run drawer each time
      const resetRet = await resetFlow();
      // After resetting, all previous messages will be cleared.
      if (resetRet?.retcode === 0) {
        // fetch prologue
        const sendRet = await send({ id });
        if (receiveMessageError(sendRet)) {
          message.error(sendRet?.data?.retmsg);
        } else {
          refetch();
          show();
        }
      }
    }
  }, [saveGraph, resetFlow, id, send, show, refetch]);

  return handleRun;
};

export const useReplaceIdWithText = (output: unknown) => {
  const getNode = useGraphStore((state) => state.getNode);

  const getNameById = (id?: string) => {
    return getNode(id)?.data.name;
  };

  return replaceIdWithText(output, getNameById);
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

// exclude nodes with branches
const ExcludedNodes = [
  Operator.Categorize,
  Operator.Relevant,
  Operator.Begin,
  Operator.Answer,
];

export const useBuildComponentIdSelectOptions = (nodeId?: string) => {
  const nodes = useGraphStore((state) => state.nodes);

  const options = useMemo(() => {
    return nodes
      .filter(
        (x) =>
          x.id !== nodeId && !ExcludedNodes.some((y) => y === x.data.label),
      )
      .map((x) => ({ label: x.data.name, value: x.id }));
  }, [nodes, nodeId]);

  return options;
};
