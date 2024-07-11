import type {} from '@redux-devtools/extension';
import { humanId } from 'human-id';
import differenceWith from 'lodash/differenceWith';
import intersectionWith from 'lodash/intersectionWith';
import lodashSet from 'lodash/set';
import {
  Connection,
  Edge,
  EdgeChange,
  Node,
  NodeChange,
  OnConnect,
  OnEdgesChange,
  OnNodesChange,
  OnSelectionChangeFunc,
  OnSelectionChangeParams,
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
} from 'reactflow';
import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import { immer } from 'zustand/middleware/immer';
import { Operator } from './constant';
import { NodeData } from './interface';
import { isEdgeEqual } from './utils';

export type RFState = {
  nodes: Node<NodeData>[];
  edges: Edge[];
  selectedNodeIds: string[];
  selectedEdgeIds: string[];
  clickedNodeId: string; // currently selected node
  onNodesChange: OnNodesChange;
  onEdgesChange: OnEdgesChange;
  onConnect: OnConnect;
  setNodes: (nodes: Node[]) => void;
  setEdges: (edges: Edge[]) => void;
  setEdgesByNodeId: (nodeId: string, edges: Edge[]) => void;
  updateNodeForm: (nodeId: string, values: any, path?: string[]) => void;
  onSelectionChange: OnSelectionChangeFunc;
  addNode: (nodes: Node) => void;
  getNode: (id?: string | null) => Node<NodeData> | undefined;
  addEdge: (connection: Connection) => void;
  getEdge: (id: string) => Edge | undefined;
  updateFormDataOnConnect: (connection: Connection) => void;
  deletePreviousEdgeOfClassificationNode: (connection: Connection) => void;
  duplicateNode: (id: string) => void;
  deleteEdge: () => void;
  deleteEdgeById: (id: string) => void;
  deleteNodeById: (id: string) => void;
  deleteEdgeBySourceAndSourceHandle: (connection: Partial<Connection>) => void;
  findNodeByName: (operatorName: Operator) => Node | undefined;
  updateMutableNodeFormItem: (id: string, field: string, value: any) => void;
  getOperatorTypeFromId: (id?: string | null) => string | undefined;
  updateNodeName: (id: string, name: string) => void;
  setClickedNodeId: (id?: string) => void;
};

// this is our useStore hook that we can use in our components to get parts of the store and call actions
const useGraphStore = create<RFState>()(
  devtools(
    immer((set, get) => ({
      nodes: [] as Node[],
      edges: [] as Edge[],
      selectedNodeIds: [] as string[],
      selectedEdgeIds: [] as string[],
      clickedNodeId: '',
      onNodesChange: (changes: NodeChange[]) => {
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
      setNodes: (nodes: Node[]) => {
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
          // the abandoned edges
          const selfAbandonedEdges = [];
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

      addNode: (node: Node) => {
        set({ nodes: get().nodes.concat(node) });
      },
      getNode: (id?: string | null) => {
        return get().nodes.find((x) => x.id === id);
      },
      getOperatorTypeFromId: (id?: string | null) => {
        return get().getNode(id)?.data?.label;
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
        const { getOperatorTypeFromId, updateNodeForm } = get();
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
            default:
              break;
          }
        }
      },
      deletePreviousEdgeOfClassificationNode: (connection: Connection) => {
        // Delete the edge on the classification node or relevant node anchor when the anchor is connected to other nodes
        const { edges, getOperatorTypeFromId, deleteEdgeById } = get();
        // the node containing the anchor
        const anchoredNodes = [Operator.Categorize, Operator.Relevant];
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
      duplicateNode: (id: string) => {
        const { getNode, addNode } = get();
        const node = getNode(id);
        const position = {
          x: (node?.position?.x || 0) + 30,
          y: (node?.position?.y || 0) + 20,
        };

        addNode({
          ...(node || {}),
          data: node?.data,
          selected: false,
          dragging: false,
          id: `${node?.data?.label}:${humanId()}`,
          position,
        });
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
        const { edges, updateNodeForm, getOperatorTypeFromId } = get();
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
      findNodeByName: (name: Operator) => {
        return get().nodes.find((x) => x.data.label === name);
      },
      updateNodeForm: (nodeId: string, values: any, path: string[] = []) => {
        set({
          nodes: get().nodes.map((node) => {
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
          }),
        });
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
    })),
    { name: 'graph' },
  ),
);

export default useGraphStore;
