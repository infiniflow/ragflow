import { useSetModalState } from '@/hooks/common-hooks';
import { useFetchFlow, useResetFlow, useSetFlow } from '@/hooks/flow-hooks';
import { IGraph } from '@/interfaces/database/flow';
import { useIsFetching } from '@tanstack/react-query';
import React, {
  ChangeEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from 'react';
import { Connection, Edge, Node, Position, ReactFlowInstance } from 'reactflow';
// import { shallow } from 'zustand/shallow';
import { variableEnabledFieldMap } from '@/constants/chat';
import { FileMimeType } from '@/constants/common';
import {
  ModelVariableType,
  settledModelVariableMap,
} from '@/constants/knowledge';
import { useFetchModelId } from '@/hooks/logic-hooks';
import { Variable } from '@/interfaces/database/chat';
import { downloadJsonFile } from '@/utils/file-util';
import { useDebounceEffect } from 'ahooks';
import { FormInstance, UploadFile, message } from 'antd';
import { DefaultOptionType } from 'antd/es/select';
import dayjs from 'dayjs';
import { humanId } from 'human-id';
import { get, isEmpty, lowerFirst, pick } from 'lodash';
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
import {
  BeginQuery,
  ICategorizeForm,
  IRelevantForm,
  ISwitchForm,
} from './interface';
import useGraphStore, { RFState } from './store';
import {
  buildDslComponentsByGraph,
  generateNodeNamesWithIncreasingIndex,
  generateSwitchHandleText,
  getNodeDragHandle,
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
      ev.dataTransfer.setData('application/reactflow', operatorId);
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
        newNode.style = {
          backgroundColor: 'rgba(255, 0, 0, 0.2)',
          width: 400,
          height: 300,
        };
        const iterationStartNode: Node<any> = {
          id: `${Operator.IterationStart}:${humanId()}`,
          type: 'iterationStartNode',
          position: { x: 50, y: 50 },
          draggable: false,
          data: {},
          parentId: newNode.id,
        };
        addNode(newNode);
        addNode(iterationStartNode);
      } else {
        addNode(newNode);
      }
    },
    [reactFlowInstance, getNodeName, nodes, initializeOperatorParams, addNode],
  );

  return { onDrop, onDragOver, setReactFlowInstance };
};

export const useShowFormDrawer = () => {
  const {
    clickedNodeId: clickNodeId,
    setClickedNodeId,
    getNode,
  } = useGraphStore((state) => state);
  const {
    visible: formDrawerVisible,
    hideModal: hideFormDrawer,
    showModal: showFormDrawer,
  } = useSetModalState();

  const handleShow = useCallback(
    (node: Node) => {
      setClickedNodeId(node.id);
      showFormDrawer();
    },
    [showFormDrawer, setClickedNodeId],
  );

  return {
    formDrawerVisible,
    hideFormDrawer,
    showFormDrawer: handleShow,
    clickedNode: getNode(clickNodeId),
  };
};

export const useBuildDslData = () => {
  const { data } = useFetchFlow();
  const { nodes, edges } = useGraphStore((state) => state);

  const buildDslData = useCallback(
    (currentNodes?: Node[]) => {
      const dslComponents = buildDslComponentsByGraph(
        currentNodes ?? nodes,
        edges,
        data.dsl.components,
      );

      return {
        ...data.dsl,
        graph: { nodes: currentNodes ?? nodes, edges },
        components: dslComponents,
      };
    },
    [data.dsl, edges, nodes],
  );

  return { buildDslData };
};

export const useSaveGraph = () => {
  const { data } = useFetchFlow();
  const { setFlow, loading } = useSetFlow();
  const { id } = useParams();
  const { buildDslData } = useBuildDslData();

  const saveGraph = useCallback(
    async (currentNodes?: Node[]) => {
      return setFlow({
        id,
        title: data.title,
        dsl: buildDslData(currentNodes),
      });
    },
    [setFlow, id, data.title, buildDslData],
  );

  return { saveGraph, loading };
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

  useEffect(() => {
    refetch();
  }, [refetch]);

  return { loading, flowDetail: data };
};

export const useFlowIsFetching = () => {
  return useIsFetching({ queryKey: ['flowDetail'] }) > 0;
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

export const useGetBeginNodeDataQuery = () => {
  const getNode = useGraphStore((state) => state.getNode);

  const getBeginNodeDataQuery = useCallback(() => {
    return get(getNode('begin'), 'data.form.query', []);
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

export const useSaveGraphBeforeOpeningDebugDrawer = (show: () => void) => {
  const { saveGraph, loading } = useSaveGraph();
  const { resetFlow } = useResetFlow();

  const handleRun = useCallback(
    async (nextNodes?: Node[]) => {
      const saveRet = await saveGraph(nextNodes);
      if (saveRet?.code === 0) {
        // Call the reset api before opening the run drawer each time
        const resetRet = await resetFlow();
        // After resetting, all previous messages will be cleared.
        if (resetRet?.code === 0) {
          show();
        }
      }
    },
    [saveGraph, resetFlow, show],
  );

  return { handleRun, loading };
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
      ) as Node[] | undefined;

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

export const useWatchAgentChange = (chatDrawerVisible: boolean) => {
  const [time, setTime] = useState<string>();
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);
  const { saveGraph } = useSaveGraph();
  const { data: flowDetail } = useFetchFlow();

  const setSaveTime = useCallback((updateTime: number) => {
    setTime(dayjs(updateTime).format('YYYY-MM-DD HH:mm:ss'));
  }, []);

  useEffect(() => {
    setSaveTime(flowDetail?.update_time);
  }, [flowDetail, setSaveTime]);

  const saveAgent = useCallback(async () => {
    if (!chatDrawerVisible) {
      const ret = await saveGraph();
      setSaveTime(ret.data.update_time);
    }
  }, [chatDrawerVisible, saveGraph, setSaveTime]);

  useDebounceEffect(
    () => {
      saveAgent();
    },
    [nodes, edges],
    {
      wait: 1000 * 20,
    },
  );

  return time;
};

export const useHandleExportOrImportJsonFile = () => {
  const { buildDslData } = useBuildDslData();
  const {
    visible: fileUploadVisible,
    hideModal: hideFileUploadModal,
    showModal: showFileUploadModal,
  } = useSetModalState();
  const setGraphInfo = useSetGraphInfo();
  const { data } = useFetchFlow();
  const { t } = useTranslation();

  const onFileUploadOk = useCallback(
    async (fileList: UploadFile[]) => {
      if (fileList.length > 0) {
        const file: File = fileList[0] as unknown as File;
        if (file.type !== FileMimeType.Json) {
          message.error(t('flow.jsonUploadTypeErrorMessage'));
          return;
        }

        const graphStr = await file.text();
        const errorMessage = t('flow.jsonUploadContentErrorMessage');
        try {
          const graph = JSON.parse(graphStr);
          if (graphStr && !isEmpty(graph) && Array.isArray(graph?.nodes)) {
            setGraphInfo(graph ?? ({} as IGraph));
            hideFileUploadModal();
          } else {
            message.error(errorMessage);
          }
        } catch (error) {
          message.error(errorMessage);
        }
      }
    },
    [hideFileUploadModal, setGraphInfo, t],
  );

  const handleExportJson = useCallback(() => {
    downloadJsonFile(buildDslData().graph, `${data.title}.json`);
  }, [buildDslData, data.title]);

  return {
    fileUploadVisible,
    handleExportJson,
    handleImportJson: showFileUploadModal,
    hideFileUploadModal,
    onFileUploadOk,
  };
};

export const useShowSingleDebugDrawer = () => {
  const { visible, showModal, hideModal } = useSetModalState();
  const { saveGraph } = useSaveGraph();

  const showSingleDebugDrawer = useCallback(async () => {
    const saveRet = await saveGraph();
    if (saveRet?.code === 0) {
      showModal();
    }
  }, [saveGraph, showModal]);

  return {
    singleDebugDrawerVisible: visible,
    hideSingleDebugDrawer: hideModal,
    showSingleDebugDrawer,
  };
};
