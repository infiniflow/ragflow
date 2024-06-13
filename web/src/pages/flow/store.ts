import type {} from '@redux-devtools/extension';
import { humanId } from 'human-id';
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
import { Operator } from './constant';
import { NodeData } from './interface';

export type RFState = {
  nodes: Node<NodeData>[];
  edges: Edge[];
  selectedNodeIds: string[];
  selectedEdgeIds: string[];
  onNodesChange: OnNodesChange;
  onEdgesChange: OnEdgesChange;
  onConnect: OnConnect;
  setNodes: (nodes: Node[]) => void;
  setEdges: (edges: Edge[]) => void;
  updateNodeForm: (nodeId: string, values: any) => void;
  onSelectionChange: OnSelectionChangeFunc;
  addNode: (nodes: Node) => void;
  getNode: (id: string) => Node | undefined;
  duplicateNode: (id: string) => void;
  deleteEdge: () => void;
  deleteEdgeById: (id: string) => void;
  deleteNodeById: (id: string) => void;
  findNodeByName: (operatorName: Operator) => Node | undefined;
};

// this is our useStore hook that we can use in our components to get parts of the store and call actions
const useGraphStore = create<RFState>()(
  devtools(
    (set, get) => ({
      nodes: [] as Node[],
      edges: [] as Edge[],
      selectedNodeIds: [] as string[],
      selectedEdgeIds: [] as string[],
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
        set({
          edges: addEdge(connection, get().edges),
        });
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
      addNode: (node: Node) => {
        set({ nodes: get().nodes.concat(node) });
      },
      getNode: (id: string) => {
        return get().nodes.find((x) => x.id === id);
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
        const { edges } = get();
        set({
          edges: edges.filter((edge) => edge.id !== id),
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
      updateNodeForm: (nodeId: string, values: any) => {
        set({
          nodes: get().nodes.map((node) => {
            if (node.id === nodeId) {
              node.data = { ...node.data, form: values };
            }

            return node;
          }),
        });
      },
    }),
    { name: 'graph' },
  ),
);

export default useGraphStore;
