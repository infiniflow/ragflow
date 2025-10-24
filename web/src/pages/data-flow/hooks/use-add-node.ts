import { useFetchModelId } from '@/hooks/logic-hooks';
import { Connection, Node, Position, ReactFlowInstance } from '@xyflow/react';
import humanId from 'human-id';
import { lowerFirst } from 'lodash';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  NodeHandleId,
  NodeMap,
  Operator,
  initialBeginValues,
  initialExtractorValues,
  initialHierarchicalMergerValues,
  initialNoteValues,
  initialParserValues,
  initialSplitterValues,
  initialTokenizerValues,
} from '../constant';
import useGraphStore from '../store';
import {
  generateNodeNamesWithIncreasingIndex,
  getNodeDragHandle,
} from '../utils';

export const useInitializeOperatorParams = () => {
  const llmId = useFetchModelId();
  const { t } = useTranslation();

  const initialFormValuesMap = useMemo(() => {
    return {
      [Operator.Begin]: initialBeginValues,
      [Operator.Note]: initialNoteValues,
      [Operator.Parser]: initialParserValues,
      [Operator.Tokenizer]: initialTokenizerValues,
      [Operator.Splitter]: initialSplitterValues,
      [Operator.HierarchicalMerger]: initialHierarchicalMergerValues,
      [Operator.Extractor]: {
        ...initialExtractorValues,
        llm_id: llmId,
        sys_prompt: t('dataflow.prompts.system.summary'),
        prompts: t('dataflow.prompts.user.summary'),
      },
    };
  }, [llmId, t]);

  const initializeOperatorParams = useCallback(
    (operatorName: Operator) => {
      const initialValues = initialFormValuesMap[operatorName];

      return initialValues;
    },
    [initialFormValuesMap],
  );

  return { initializeOperatorParams, initialFormValuesMap };
};

export const useGetNodeName = () => {
  const { t } = useTranslation();

  return (type: string) => {
    const name = t(`dataflow.${lowerFirst(type)}`);
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

type CanvasMouseEvent = Pick<
  React.MouseEvent<HTMLElement>,
  'clientX' | 'clientY'
>;

export function useAddNode(reactFlowInstance?: ReactFlowInstance<any, any>) {
  const { nodes, addNode } = useGraphStore((state) => state);
  const getNodeName = useGetNodeName();
  const { initializeOperatorParams } = useInitializeOperatorParams();
  const { calculateNewlyBackChildPosition } = useCalculateNewlyChildPosition();
  const { addChildEdge } = useAddChildEdge();
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
          data: {
            label: `${type}`,
            name: generateNodeNamesWithIncreasingIndex(
              getNodeName(type),
              nodes,
            ),
            form: initializeOperatorParams(type as Operator),
          },
          sourcePosition: Position.Right,
          targetPosition: Position.Left,
          dragHandle: getNodeDragHandle(type),
        };

        addNode(newNode);
        addChildEdge(params.position, {
          source: params.nodeId,
          target: newNode.id,
          sourceHandle: params.id,
        });

        return newNode.id;
      },
    [
      addChildEdge,
      addNode,
      calculateNewlyBackChildPosition,
      getNodeName,
      initializeOperatorParams,
      nodes,
      reactFlowInstance,
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
