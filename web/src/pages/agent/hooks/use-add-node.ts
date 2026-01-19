import { useFetchModelId } from '@/hooks/logic-hooks';
import { Connection, Node, Position, ReactFlowInstance } from '@xyflow/react';
import humanId from 'human-id';
import { t } from 'i18next';
import { lowerFirst } from 'lodash';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  NodeHandleId,
  NodeMap,
  Operator,
  initialAgentValues,
  initialArXivValues,
  initialBeginValues,
  initialBingValues,
  initialCategorizeValues,
  initialCodeValues,
  initialCrawlerValues,
  initialDataOperationsValues,
  initialDuckValues,
  initialEmailValues,
  initialExeSqlValues,
  initialExtractorValues,
  initialGithubValues,
  initialGoogleScholarValues,
  initialGoogleValues,
  initialHierarchicalMergerValues,
  initialInvokeValues,
  initialIterationStartValues,
  initialIterationValues,
  initialListOperationsValues,
  initialLoopValues,
  initialMessageValues,
  initialNoteValues,
  initialPDFGeneratorValues,
  initialParserValues,
  initialPubMedValues,
  initialRetrievalValues,
  initialRewriteQuestionValues,
  initialSearXNGValues,
  initialSplitterValues,
  initialStringTransformValues,
  initialSwitchValues,
  initialTavilyExtractValues,
  initialTavilyValues,
  initialTokenizerValues,
  initialUserFillUpValues,
  initialVariableAggregatorValues,
  initialVariableAssignerValues,
  initialWaitingDialogueValues,
  initialWenCaiValues,
  initialWikipediaValues,
  initialYahooFinanceValues,
} from '../constant';
import useGraphStore from '../store';
import {
  generateNodeNamesWithIncreasingIndex,
  getNodeDragHandle,
} from '../utils';

function isBottomSubAgent(type: string, position: Position) {
  return (
    (type === Operator.Agent && position === Position.Bottom) ||
    type === Operator.Tool
  );
}

const GroupStartNodeMap = {
  [Operator.Iteration]: {
    id: `${Operator.IterationStart}:${humanId()}`,
    type: 'iterationStartNode',
    position: { x: 50, y: 100 },
    data: {
      label: Operator.IterationStart,
      name: Operator.IterationStart,
      form: initialIterationStartValues,
    },
    extent: 'parent' as 'parent',
  },
  [Operator.Loop]: {
    id: `${Operator.LoopStart}:${humanId()}`,
    type: 'loopStartNode',
    position: { x: 50, y: 100 },
    data: {
      label: Operator.LoopStart,
      name: Operator.LoopStart,
      form: {},
    },
    extent: 'parent' as 'parent',
  },
};

function useAddGroupNode() {
  const { addEdge, addNode } = useGraphStore((state) => state);

  const addGroupNode = useCallback(
    (operatorType: string, newNode: Node<any>, nodeId?: string) => {
      newNode.width = 500;
      newNode.height = 250;

      const startNode: Node<any> =
        GroupStartNodeMap[operatorType as keyof typeof GroupStartNodeMap];

      startNode.parentId = newNode.id;

      addNode(newNode);
      addNode(startNode);

      if (nodeId) {
        addEdge({
          source: nodeId,
          target: newNode.id,
          sourceHandle: NodeHandleId.Start,
          targetHandle: NodeHandleId.End,
        });
      }
      return newNode.id;
    },
    [addEdge, addNode],
  );

  return { addGroupNode };
}
export const useInitializeOperatorParams = () => {
  const llmId = useFetchModelId();

  const initialFormValuesMap = useMemo(() => {
    return {
      [Operator.Begin]: initialBeginValues,
      [Operator.Retrieval]: initialRetrievalValues,
      [Operator.Categorize]: { ...initialCategorizeValues, llm_id: llmId },
      [Operator.RewriteQuestion]: {
        ...initialRewriteQuestionValues,
        llm_id: llmId,
      },
      [Operator.Message]: initialMessageValues,
      [Operator.DuckDuckGo]: initialDuckValues,
      [Operator.Wikipedia]: initialWikipediaValues,
      [Operator.PubMed]: initialPubMedValues,
      [Operator.ArXiv]: initialArXivValues,
      [Operator.Google]: initialGoogleValues,
      [Operator.Bing]: initialBingValues,
      [Operator.GoogleScholar]: initialGoogleScholarValues,
      [Operator.SearXNG]: initialSearXNGValues,
      [Operator.GitHub]: initialGithubValues,
      [Operator.ExeSQL]: initialExeSqlValues,
      [Operator.Switch]: initialSwitchValues,
      [Operator.WenCai]: initialWenCaiValues,
      [Operator.YahooFinance]: initialYahooFinanceValues,
      [Operator.Note]: initialNoteValues,
      [Operator.Crawler]: initialCrawlerValues,
      [Operator.Invoke]: initialInvokeValues,
      [Operator.Email]: initialEmailValues,
      [Operator.Iteration]: initialIterationValues,
      [Operator.IterationStart]: initialIterationStartValues,
      [Operator.Code]: initialCodeValues,
      [Operator.WaitingDialogue]: initialWaitingDialogueValues,
      [Operator.Agent]: { ...initialAgentValues, llm_id: llmId },
      [Operator.Tool]: {},
      [Operator.TavilySearch]: initialTavilyValues,
      [Operator.UserFillUp]: initialUserFillUpValues,
      [Operator.StringTransform]: initialStringTransformValues,
      [Operator.TavilyExtract]: initialTavilyExtractValues,
      [Operator.Placeholder]: {},
      [Operator.File]: {},
      [Operator.Parser]: initialParserValues,
      [Operator.Tokenizer]: initialTokenizerValues,
      [Operator.Splitter]: initialSplitterValues,
      [Operator.HierarchicalMerger]: initialHierarchicalMergerValues,
      [Operator.Extractor]: {
        ...initialExtractorValues,
        llm_id: llmId,
        sys_prompt: t('flow.prompts.system.summary'),
        prompts: t('flow.prompts.user.summary'),
      },
      [Operator.DataOperations]: initialDataOperationsValues,
      [Operator.ListOperations]: initialListOperationsValues,
      [Operator.VariableAssigner]: initialVariableAssignerValues,
      [Operator.VariableAggregator]: initialVariableAggregatorValues,
      [Operator.Loop]: initialLoopValues,
      [Operator.LoopStart]: {},
      [Operator.ExitLoop]: {},
      [Operator.PDFGenerator]: initialPDFGeneratorValues,
      [Operator.ExcelProcessor]: {},
    };
  }, [llmId]);

  const initializeOperatorParams = useCallback(
    (operatorName: Operator, position: Position) => {
      const initialValues = initialFormValuesMap[operatorName];
      if (isBottomSubAgent(operatorName, position)) {
        return {
          ...initialValues,
          description: t('flow.descriptionMessage'),
          user_prompt: t('flow.userPromptDefaultValue'),
        };
      }

      return initialValues;
    },
    [initialFormValuesMap],
  );

  return { initializeOperatorParams, initialFormValuesMap };
};

export const useGetNodeName = () => {
  const { t } = useTranslation();

  return (type: string) => {
    const name = t(`flow.${lowerFirst(type)}`);
    return name;
  };
};

export function useCalculateNewlyChildPosition() {
  const getNode = useGraphStore((state) => state.getNode);
  const nodes = useGraphStore((state) => state.nodes);
  const edges = useGraphStore((state) => state.edges);

  const calculateNewlyBackChildPosition = useCallback(
    (id?: string, sourceHandle?: string) => {
      const parentNode = getNode(id);

      // Calculate the coordinates of child nodes to prevent newly added child nodes from covering other child nodes
      const allChildNodeIds = edges
        .filter((x) => x.source === id && x.sourceHandle === sourceHandle)
        .map((x) => x.target);

      const yAxises = nodes
        .filter((x) => allChildNodeIds.some((y) => y === x.id))
        .map((x) => x.position.y);

      const maxY = Math.max(...yAxises);

      const position = {
        y: yAxises.length > 0 ? maxY + 150 : parentNode?.position.y || 0,
        x: (parentNode?.position.x || 0) + 300,
      };

      return position;
    },
    [edges, getNode, nodes],
  );

  return { calculateNewlyBackChildPosition };
}

function useAddChildEdge() {
  const addEdge = useGraphStore((state) => state.addEdge);

  const addChildEdge = useCallback(
    (position: Position = Position.Right, edge: Partial<Connection>) => {
      if (
        position === Position.Right &&
        edge.source &&
        edge.target &&
        edge.sourceHandle
      ) {
        addEdge({
          source: edge.source,
          target: edge.target,
          sourceHandle: edge.sourceHandle,
          targetHandle: NodeHandleId.End,
        });
      }
    },
    [addEdge],
  );

  return { addChildEdge };
}

function useAddToolNode() {
  const { nodes, edges, addEdge, getNode, addNode } = useGraphStore(
    (state) => state,
  );

  const addToolNode = useCallback(
    (newNode: Node<any>, nodeId?: string): boolean => {
      const agentNode = getNode(nodeId);

      if (agentNode) {
        const childToolNodeIds = edges
          .filter(
            (x) => x.source === nodeId && x.sourceHandle === NodeHandleId.Tool,
          )
          .map((x) => x.target);

        if (
          childToolNodeIds.length > 0 &&
          nodes.some((x) => x.id === childToolNodeIds[0])
        ) {
          return false;
        }

        newNode.position = {
          x: agentNode.position.x - 82,
          y: agentNode.position.y + 140,
        };

        addNode(newNode);
        if (nodeId) {
          addEdge({
            source: nodeId,
            target: newNode.id,
            sourceHandle: NodeHandleId.Tool,
            targetHandle: NodeHandleId.End,
          });
        }
        return true;
      }
      return false;
    },
    [addEdge, addNode, edges, getNode, nodes],
  );

  return { addToolNode };
}

function useResizeIterationNode() {
  const { getNode, nodes, updateNode } = useGraphStore((state) => state);

  const resizeIterationNode = useCallback(
    (type: string, position: Position, parentId?: string) => {
      const parentNode = getNode(parentId);
      if (parentNode && !isBottomSubAgent(type, position)) {
        const MoveRightDistance = 310;
        const childNodeList = nodes.filter((x) => x.parentId === parentId);
        const maxX = Math.max(...childNodeList.map((x) => x.position.x));
        if (maxX + MoveRightDistance > parentNode.position.x) {
          updateNode({
            ...parentNode,
            width: (parentNode.width || 0) + MoveRightDistance,
            position: {
              x: parentNode.position.x + MoveRightDistance / 2,
              y: parentNode.position.y,
            },
          });
        }
      }
    },
    [getNode, nodes, updateNode],
  );

  return { resizeIterationNode };
}
type CanvasMouseEvent = Pick<
  React.MouseEvent<HTMLElement>,
  'clientX' | 'clientY'
>;

export function useAddNode(reactFlowInstance?: ReactFlowInstance<any, any>) {
  const { edges, nodes, addEdge, addNode, getNode } = useGraphStore(
    (state) => state,
  );
  const getNodeName = useGetNodeName();
  const { initializeOperatorParams } = useInitializeOperatorParams();
  const { calculateNewlyBackChildPosition } = useCalculateNewlyChildPosition();
  const { addChildEdge } = useAddChildEdge();
  const { addToolNode } = useAddToolNode();
  const { resizeIterationNode } = useResizeIterationNode();
  const { addGroupNode } = useAddGroupNode();
  //   const [reactFlowInstance, setReactFlowInstance] =
  //     useState<ReactFlowInstance<any, any>>();

  const addCanvasNode = useCallback(
    (
      type: string,
      params: {
        nodeId?: string;
        position: Position;
        id?: string;
        isFromConnectionDrag?: boolean;
      } = {
        position: Position.Right,
      },
    ) =>
      (event?: CanvasMouseEvent): string | undefined => {
        const nodeId = params.nodeId;
        const node = getNode(nodeId);

        // reactFlowInstance.project was renamed to reactFlowInstance.screenToFlowPosition
        // and you don't need to subtract the reactFlowBounds.left/top anymore
        // details: https://@xyflow/react.dev/whats-new/2023-11-10
        let position = reactFlowInstance?.screenToFlowPosition({
          x: event?.clientX || 0,
          y: event?.clientY || 0,
        });

        if (
          params.position === Position.Right &&
          type !== Operator.Note &&
          !params.isFromConnectionDrag
        ) {
          position = calculateNewlyBackChildPosition(nodeId, params.id);
        }

        const newNode: Node<any> = {
          id: `${type}:${humanId()}`,
          type: NodeMap[type as Operator] || 'ragNode',
          position: position || {
            x: 0,
            y: 0,
          },
          draggable: type === Operator.Placeholder ? false : undefined,
          data: {
            label: `${type}`,
            name: generateNodeNamesWithIncreasingIndex(
              getNodeName(type),
              nodes,
            ),
            form: initializeOperatorParams(type as Operator, params.position),
          },
          sourcePosition: Position.Right,
          targetPosition: Position.Left,
          dragHandle: getNodeDragHandle(type),
        };

        if (node && node.parentId) {
          newNode.parentId = node.parentId;
          newNode.extent = 'parent';
          const parentNode = getNode(node.parentId);
          if (parentNode && !isBottomSubAgent(type, params.position)) {
            resizeIterationNode(type, params.position, node.parentId);
          }
        }

        if ([Operator.Iteration, Operator.Loop].includes(type as Operator)) {
          return addGroupNode(type, newNode, nodeId);
        } else if (
          type === Operator.Agent &&
          params.position === Position.Bottom
        ) {
          const agentNode = getNode(nodeId);
          if (agentNode) {
            // Calculate the coordinates of child nodes to prevent newly added child nodes from covering other child nodes
            const allChildAgentNodeIds = edges
              .filter(
                (x) =>
                  x.source === nodeId &&
                  x.sourceHandle === NodeHandleId.AgentBottom,
              )
              .map((x) => x.target);

            const xAxises = nodes
              .filter((x) => allChildAgentNodeIds.some((y) => y === x.id))
              .map((x) => x.position.x);

            const maxX = Math.max(...xAxises);

            newNode.position = {
              x: xAxises.length > 0 ? maxX + 262 : agentNode.position.x + 82,
              y: agentNode.position.y + 140,
            };
          }
          addNode(newNode);
          if (nodeId) {
            addEdge({
              source: nodeId,
              target: newNode.id,
              sourceHandle: NodeHandleId.AgentBottom,
              targetHandle: NodeHandleId.AgentTop,
            });
          }
          return newNode.id;
        } else if (type === Operator.Tool) {
          const toolNodeAdded = addToolNode(newNode, params.nodeId);
          return toolNodeAdded ? newNode.id : undefined;
        } else {
          addNode(newNode);
          addChildEdge(params.position, {
            source: params.nodeId,
            target: newNode.id,
            sourceHandle: params.id,
          });
        }

        return newNode.id;
      },
    [
      addChildEdge,
      addEdge,
      addGroupNode,
      addNode,
      addToolNode,
      calculateNewlyBackChildPosition,
      edges,
      getNode,
      getNodeName,
      initializeOperatorParams,
      nodes,
      reactFlowInstance,
      resizeIterationNode,
    ],
  );

  const addNoteNode = useCallback(
    (e: CanvasMouseEvent) => {
      addCanvasNode(Operator.Note)(e);
    },
    [addCanvasNode],
  );

  return { addCanvasNode, addNoteNode };
}
