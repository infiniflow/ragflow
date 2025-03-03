import { RAGFlowNodeType } from '@/interfaces/database/flow';
import type {} from '@redux-devtools/extension';
import {
  Connection,
  Edge,
  EdgeChange,
  OnConnect,
  OnEdgesChange,
  OnNodesChange,
  OnSelectionChangeFunc,
  OnSelectionChangeParams,
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
} from '@xyflow/react';
import { omit } from 'lodash';
import differenceWith from 'lodash/differenceWith';
import intersectionWith from 'lodash/intersectionWith';
import lodashSet from 'lodash/set';
import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import { immer } from 'zustand/middleware/immer';
import { Operator, SwitchElseTo } from './constant';
import {
  duplicateNodeForm,
  generateDuplicateNode,
  generateNodeNamesWithIncreasingIndex,
  getOperatorIndex,
  isEdgeEqual,
} from './utils';

export type RFState = {
  nodes: RAGFlowNodeType[];
  edges: Edge[];
  selectedNodeIds: string[];
  selectedEdgeIds: string[];
  clickedNodeId: string; // currently selected node
  onNodesChange: OnNodesChange<RAGFlowNodeType>;
  onEdgesChange: OnEdgesChange;
  onConnect: OnConnect;
  setNodes: (nodes: RAGFlowNodeType[]) => void;
  setEdges: (edges: Edge[]) => void;
  setEdgesByNodeId: (nodeId: string, edges: Edge[]) => void;
  updateNodeForm: (
    nodeId: string,
    values: any,
    path?: (string | number)[],
  ) => RAGFlowNodeType[];
  onSelectionChange: OnSelectionChangeFunc;
  addNode: (nodes: RAGFlowNodeType) => void;
  getNode: (id?: string | null) => RAGFlowNodeType | undefined;
  addEdge: (connection: Connection) => void;
  getEdge: (id: string) => Edge | undefined;
  updateFormDataOnConnect: (connection: Connection) => void;
  updateSwitchFormData: (
    source: string,
    sourceHandle?: string | null,
    target?: string | null,
  ) => void;
  deletePreviousEdgeOfClassificationNode: (connection: Connection) => void;
  duplicateNode: (id: string, name: string) => void;
  duplicateIterationNode: (id: string, name: string) => void;
  deleteEdge: () => void;
  deleteEdgeById: (id: string) => void;
  deleteNodeById: (id: string) => void;
  deleteIterationNodeById: (id: string) => void;
  deleteEdgeBySourceAndSourceHandle: (connection: Partial<Connection>) => void;
  findNodeByName: (operatorName: Operator) => RAGFlowNodeType | undefined;
  updateMutableNodeFormItem: (id: string, field: string, value: any) => void;
  getOperatorTypeFromId: (id?: string | null) => string | undefined;
  getParentIdById: (id?: string | null) => string | undefined;
  updateNodeName: (id: string, name: string) => void;
  generateNodeName: (name: string) => string;
  setClickedNodeId: (id?: string) => void;
};

// this is our useStore hook that we can use in our components to get parts of the store and call actions
const useGraphStore = create<RFState>()(
  devtools(
    immer((set, get) => ({
      nodes: [] as RAGFlowNodeType[],
      edges: [] as Edge[],
      selectedNodeIds: [] as string[],
      selectedEdgeIds: [] as string[],
      clickedNodeId: '',
      onNodesChange: (changes) => {
        set({
          nodes: applyNodeChanges(changes, get().nodes),
        });
      },
      onEdgesChange: (changes: EdgeChange[]) => {
        set({
          edges: applyEdgeChanges(changes, get().edges),
        });
      },
      onConnect: (connection: Connection) => {
        const {
          deletePreviousEdgeOfClassificationNode,
          updateFormDataOnConnect,
        } = get();
        set({
          edges: addEdge(connection, get().edges),
        });
        deletePreviousEdgeOfClassificationNode(connection);
        updateFormDataOnConnect(connection);
      },
      onSelectionChange: ({ nodes, edges }: OnSelectionChangeParams) => {
        set({
          selectedEdgeIds: edges.map((x) => x.id),
          selectedNodeIds: nodes.map((x) => x.id),
        });
      },
      setNodes: (nodes: RAGFlowNodeType[]) => {
        set({ nodes });
      },
      setEdges: (edges: Edge[]) => {
        set({ edges });
      },
      setEdgesByNodeId: (nodeId: string, currentDownstreamEdges: Edge[]) => {
        const { edges, setEdges } = get();
        // the previous downstream edge of this node
        const previousDownstreamEdges = edges.filter(
          (x) => x.source === nodeId,
        );
        const isDifferent =
          previousDownstreamEdges.length !== currentDownstreamEdges.length ||
          !previousDownstreamEdges.every((x) =>
            currentDownstreamEdges.some(
              (y) =>
                y.source === x.source &&
                y.target === x.target &&
                y.sourceHandle === x.sourceHandle,
            ),
          ) ||
          !currentDownstreamEdges.every((x) =>
            previousDownstreamEdges.some(
              (y) =>
                y.source === x.source &&
                y.target === x.target &&
                y.sourceHandle === x.sourceHandle,
            ),
          );

        const intersectionDownstreamEdges = intersectionWith(
          previousDownstreamEdges,
          currentDownstreamEdges,
          isEdgeEqual,
        );
        if (isDifferent) {
          // other operator's edges
          const irrelevantEdges = edges.filter((x) => x.source !== nodeId);
          // the added downstream edges
          const selfAddedDownstreamEdges = differenceWith(
            currentDownstreamEdges,
            intersectionDownstreamEdges,
            isEdgeEqual,
          );
          setEdges([
            ...irrelevantEdges,
            ...intersectionDownstreamEdges,
            ...selfAddedDownstreamEdges,
          ]);
        }
      },
      addNode: (node: RAGFlowNodeType) => {
        set({ nodes: get().nodes.concat(node) });
      },
      getNode: (id?: string | null) => {
        return get().nodes.find((x) => x.id === id);
      },
      getOperatorTypeFromId: (id?: string | null) => {
        return get().getNode(id)?.data?.label;
      },
      getParentIdById: (id?: string | null) => {
        return get().getNode(id)?.parentId;
      },
      addEdge: (connection: Connection) => {
        set({
          edges: addEdge(connection, get().edges),
        });
        get().deletePreviousEdgeOfClassificationNode(connection);
        //  TODO: This may not be reasonable. You need to choose between listening to changes in the form.
        get().updateFormDataOnConnect(connection);
      },
      getEdge: (id: string) => {
        return get().edges.find((x) => x.id === id);
      },
      updateFormDataOnConnect: (connection: Connection) => {
        const { getOperatorTypeFromId, updateNodeForm, updateSwitchFormData } =
          get();
        const { source, target, sourceHandle } = connection;
        const operatorType = getOperatorTypeFromId(source);
        if (source) {
          switch (operatorType) {
            case Operator.Relevant:
              updateNodeForm(source, { [sourceHandle as string]: target });
              break;
            case Operator.Categorize:
              if (sourceHandle)
                updateNodeForm(source, target, [
                  'category_description',
                  sourceHandle,
                  'to',
                ]);
              break;
            case Operator.Switch: {
              updateSwitchFormData(source, sourceHandle, target);
              break;
            }
            default:
              break;
          }
        }
      },
      deletePreviousEdgeOfClassificationNode: (connection: Connection) => {
        // Delete the edge on the classification node or relevant node anchor when the anchor is connected to other nodes
        const { edges, getOperatorTypeFromId, deleteEdgeById } = get();
        // the node containing the anchor
        const anchoredNodes = [
          Operator.Categorize,
          Operator.Relevant,
          Operator.Switch,
        ];
        if (
          anchoredNodes.some(
            (x) => x === getOperatorTypeFromId(connection.source),
          )
        ) {
          const previousEdge = edges.find(
            (x) =>
              x.source === connection.source &&
              x.sourceHandle === connection.sourceHandle &&
              x.target !== connection.target,
          );
          if (previousEdge) {
            deleteEdgeById(previousEdge.id);
          }
        }
      },
      duplicateNode: (id: string, name: string) => {
        const { getNode, addNode, generateNodeName, duplicateIterationNode } =
          get();
        const node = getNode(id);

        if (node?.data.label === Operator.Iteration) {
          duplicateIterationNode(id, name);
          return;
        }

        addNode({
          ...(node || {}),
          data: {
            ...duplicateNodeForm(node?.data),
            name: generateNodeName(name),
          },
          ...generateDuplicateNode(node?.position, node?.data?.label),
        });
      },
      duplicateIterationNode: (id: string, name: string) => {
        const { getNode, generateNodeName, nodes } = get();
        const node = getNode(id);

        const iterationNode: RAGFlowNodeType = {
          ...(node || {}),
          data: {
            ...(node?.data || { label: Operator.Iteration, form: {} }),
            name: generateNodeName(name),
          },
          ...generateDuplicateNode(node?.position, node?.data?.label),
        };

        const children = nodes
          .filter((x) => x.parentId === node?.id)
          .map((x) => ({
            ...(x || {}),
            data: {
              ...duplicateNodeForm(x?.data),
              name: generateNodeName(x.data.name),
            },
            ...omit(generateDuplicateNode(x?.position, x?.data?.label), [
              'position',
            ]),
            parentId: iterationNode.id,
          }));

        set({ nodes: nodes.concat(iterationNode, ...children) });
      },
      deleteEdge: () => {
        const { edges, selectedEdgeIds } = get();
        set({
          edges: edges.filter((edge) =>
            selectedEdgeIds.every((x) => x !== edge.id),
          ),
        });
      },
      deleteEdgeById: (id: string) => {
        const {
          edges,
          updateNodeForm,
          getOperatorTypeFromId,
          updateSwitchFormData,
        } = get();
        const currentEdge = edges.find((x) => x.id === id);

        if (currentEdge) {
          const { source, sourceHandle } = currentEdge;
          const operatorType = getOperatorTypeFromId(source);
          // After deleting the edge, set the corresponding field in the node's form field to undefined
          switch (operatorType) {
            case Operator.Relevant:
              updateNodeForm(source, {
                [sourceHandle as string]: undefined,
              });
              break;
            case Operator.Categorize:
              if (sourceHandle)
                updateNodeForm(source, undefined, [
                  'category_description',
                  sourceHandle,
                  'to',
                ]);
              break;
            case Operator.Switch: {
              updateSwitchFormData(source, sourceHandle, undefined);
              break;
            }
            default:
              break;
          }
        }
        set({
          edges: edges.filter((edge) => edge.id !== id),
        });
      },
      deleteEdgeBySourceAndSourceHandle: ({
        source,
        sourceHandle,
      }: Partial<Connection>) => {
        const { edges } = get();
        const nextEdges = edges.filter(
          (edge) =>
            edge.source !== source || edge.sourceHandle !== sourceHandle,
        );
        set({
          edges: nextEdges,
        });
      },
      deleteNodeById: (id: string) => {
        const { nodes, edges } = get();
        set({
          nodes: nodes.filter((node) => node.id !== id),
          edges: edges
            .filter((edge) => edge.source !== id)
            .filter((edge) => edge.target !== id),
        });
      },
      deleteIterationNodeById: (id: string) => {
        const { nodes, edges } = get();
        const children = nodes.filter((node) => node.parentId === id);
        set({
          nodes: nodes.filter((node) => node.id !== id && node.parentId !== id),
          edges: edges.filter(
            (edge) =>
              edge.source !== id &&
              edge.target !== id &&
              !children.some(
                (child) => edge.source === child.id && edge.target === child.id,
              ),
          ),
        });
      },
      findNodeByName: (name: Operator) => {
        return get().nodes.find((x) => x.data.label === name);
      },
      updateNodeForm: (
        nodeId: string,
        values: any,
        path: (string | number)[] = [],
      ) => {
        const nextNodes = get().nodes.map((node) => {
          if (node.id === nodeId) {
            let nextForm: Record<string, unknown> = { ...node.data.form };
            if (path.length === 0) {
              nextForm = Object.assign(nextForm, values);
            } else {
              lodashSet(nextForm, path, values);
            }
            return {
              ...node,
              data: {
                ...node.data,
                form: nextForm,
              },
            } as any;
          }

          return node;
        });
        set({
          nodes: nextNodes,
        });

        return nextNodes;
      },
      updateSwitchFormData: (source, sourceHandle, target) => {
        const { updateNodeForm } = get();
        if (sourceHandle) {
          if (sourceHandle === SwitchElseTo) {
            updateNodeForm(source, target, [SwitchElseTo]);
          } else {
            const operatorIndex = getOperatorIndex(sourceHandle);
            if (operatorIndex) {
              updateNodeForm(source, target, [
                'conditions',
                Number(operatorIndex) - 1, // The index is the conditions form index
                'to',
              ]);
            }
          }
        }
      },
      updateMutableNodeFormItem: (id: string, field: string, value: any) => {
        const { nodes } = get();
        const idx = nodes.findIndex((x) => x.id === id);
        if (idx) {
          lodashSet(nodes, [idx, 'data', 'form', field], value);
        }
      },
      updateNodeName: (id, name) => {
        if (id) {
          set({
            nodes: get().nodes.map((node) => {
              if (node.id === id) {
                node.data.name = name;
              }

              return node;
            }),
          });
        }
      },
      setClickedNodeId: (id?: string) => {
        set({ clickedNodeId: id });
      },
      generateNodeName: (name: string) => {
        const { nodes } = get();

        return generateNodeNamesWithIncreasingIndex(name, nodes);
      },
    })),
    { name: 'graph' },
  ),
);

export default useGraphStore;
