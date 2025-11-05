import { RAGFlowNodeType } from '@/interfaces/database/flow';
import type {} from '@redux-devtools/extension';
import {
  Connection,
  Edge,
  EdgeChange,
  EdgeMouseHandler,
  OnConnect,
  OnEdgesChange,
  OnNodesChange,
  OnSelectionChangeFunc,
  OnSelectionChangeParams,
  addEdge,
  applyEdgeChanges,
  applyNodeChanges,
} from '@xyflow/react';
import { cloneDeep, omit } from 'lodash';
import differenceWith from 'lodash/differenceWith';
import intersectionWith from 'lodash/intersectionWith';
import lodashSet from 'lodash/set';
import { create } from 'zustand';
import { devtools } from 'zustand/middleware';
import { immer } from 'zustand/middleware/immer';
import { NodeHandleId, Operator, SwitchElseTo } from './constant';
import {
  duplicateNodeForm,
  generateDuplicateNode,
  generateNodeNamesWithIncreasingIndex,
  getOperatorIndex,
  isEdgeEqual,
  mapEdgeMouseEvent,
} from './utils';
import { deleteAllDownstreamAgentsAndTool } from './utils/delete-node';

export type RFState = {
  nodes: RAGFlowNodeType[];
  edges: Edge[];
  selectedNodeIds: string[];
  selectedEdgeIds: string[];
  clickedNodeId: string; // currently selected node
  clickedToolId: string; // currently selected tool id
  onNodesChange: OnNodesChange<RAGFlowNodeType>;
  onEdgesChange: OnEdgesChange;
  onEdgeMouseEnter?: EdgeMouseHandler<Edge>;
  /** This event handler is called when mouse of a user leaves an edge */
  onEdgeMouseLeave?: EdgeMouseHandler<Edge>;
  onConnect: OnConnect;
  setNodes: (nodes: RAGFlowNodeType[]) => void;
  setEdges: (edges: Edge[]) => void;
  setEdgesByNodeId: (nodeId: string, edges: Edge[]) => void;
  updateNodeForm: (
    nodeId: string,
    values: any,
    path?: (string | number)[],
  ) => RAGFlowNodeType[];
  replaceNodeForm: (nodeId: string, values: any) => void;
  onSelectionChange: OnSelectionChangeFunc;
  addNode: (nodes: RAGFlowNodeType) => void;
  getNode: (id?: string | null) => RAGFlowNodeType | undefined;
  updateNode: (node: RAGFlowNodeType) => void;
  addEdge: (connection: Connection) => void;
  getEdge: (id: string) => Edge | undefined;
  updateFormDataOnConnect: (connection: Connection) => void;
  updateSwitchFormData: (
    source: string,
    sourceHandle?: string | null,
    target?: string | null,
    isConnecting?: boolean,
  ) => void;
  duplicateNode: (id: string, name: string) => void;
  duplicateIterationNode: (id: string, name: string) => void;
  deleteEdge: () => void;
  deleteEdgeById: (id: string) => void;
  deleteNodeById: (id: string) => void;
  deleteAgentDownstreamNodesById: (id: string) => void;
  deleteAgentToolNodeById: (id: string) => void;
  deleteIterationNodeById: (id: string) => void;
  findNodeByName: (operatorName: Operator) => RAGFlowNodeType | undefined;
  updateMutableNodeFormItem: (id: string, field: string, value: any) => void;
  getOperatorTypeFromId: (id?: string | null) => string | undefined;
  getParentIdById: (id?: string | null) => string | undefined;
  updateNodeName: (id: string, name: string) => void;
  generateNodeName: (name: string) => string;
  setClickedNodeId: (id?: string) => void;
  setClickedToolId: (id?: string) => void;
  findUpstreamNodeById: (id?: string | null) => RAGFlowNodeType | undefined;
  deleteEdgesBySourceAndSourceHandle: (
    source: string,
    sourceHandle: string,
  ) => void; // Deleting a condition of a classification operator will delete the related edge
  findAgentToolNodeById: (id: string | null) => string | undefined;
  selectNodeIds: (nodeIds: string[]) => void;
  hasChildNode: (nodeId: string) => boolean;
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
      clickedToolId: '',
      onNodesChange: (changes) => {
        set({
          nodes: applyNodeChanges(
            changes, // The issue of errors when using templates was resolved by using cloneDeep.
            cloneDeep(get().nodes) as RAGFlowNodeType[], //   Cannot assign to read only property 'width' of object '#<Object>'
          ),
        });
      },
      onEdgesChange: (changes: EdgeChange[]) => {
        set({
          edges: applyEdgeChanges(changes, get().edges),
        });
      },
      onEdgeMouseEnter: (event, edge) => {
        const { edges, setEdges } = get();
        const edgeId = edge.id;

        // Updates edge
        setEdges(mapEdgeMouseEvent(edges, edgeId, true));
      },
      onEdgeMouseLeave: (event, edge) => {
        const { edges, setEdges } = get();
        const edgeId = edge.id;

        // Updates edge
        setEdges(mapEdgeMouseEvent(edges, edgeId, false));
      },
      onConnect: (connection: Connection) => {
        const { updateFormDataOnConnect } = get();
        const newEdges = addEdge(connection, get().edges);
        set({
          edges: newEdges,
        });
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
      updateNode: (node) => {
        const { nodes } = get();
        const nextNodes = nodes.map((x) => {
          if (x.id === node.id) {
            return node;
          }
          return x;
        });
        set({ nodes: nextNodes });
      },
      getNode: (id?: string | null) => {
        // console.log('getNode', id, get().nodes);
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
        //  TODO: This may not be reasonable. You need to choose between listening to changes in the form.
        get().updateFormDataOnConnect(connection);
      },
      getEdge: (id: string) => {
        return get().edges.find((x) => x.id === id);
      },
      updateFormDataOnConnect: (connection: Connection) => {
        const { getOperatorTypeFromId, updateSwitchFormData } = get();
        const { source, target, sourceHandle } = connection;
        const operatorType = getOperatorTypeFromId(source);
        if (source) {
          switch (operatorType) {
            case Operator.Switch: {
              updateSwitchFormData(source, sourceHandle, target, true);
              break;
            }
            default:
              break;
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
          const { source, sourceHandle, target } = currentEdge;
          const operatorType = getOperatorTypeFromId(source);
          // After deleting the edge, set the corresponding field in the node's form field to undefined
          switch (operatorType) {
            case Operator.Relevant:
              updateNodeForm(source, {
                [sourceHandle as string]: undefined,
              });
              break;
            // case Operator.Categorize:
            //   if (sourceHandle)
            //     updateNodeForm(source, undefined, [
            //       'category_description',
            //       sourceHandle,
            //       'to',
            //     ]);
            //   break;
            case Operator.Switch: {
              updateSwitchFormData(source, sourceHandle, target, false);
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
      deleteNodeById: (id: string) => {
        const {
          nodes,
          edges,
          getOperatorTypeFromId,
          deleteAgentDownstreamNodesById,
        } = get();
        if (getOperatorTypeFromId(id) === Operator.Agent) {
          deleteAgentDownstreamNodesById(id);
          return;
        }
        set({
          nodes: nodes.filter((node) => node.id !== id),
          edges: edges
            .filter((edge) => edge.source !== id)
            .filter((edge) => edge.target !== id),
        });
      },
      deleteAgentDownstreamNodesById: (id) => {
        const { edges, nodes } = get();

        const { downstreamAgentAndToolNodeIds, downstreamAgentAndToolEdges } =
          deleteAllDownstreamAgentsAndTool(id, edges);

        set({
          nodes: nodes.filter(
            (node) =>
              !downstreamAgentAndToolNodeIds.some((x) => x === node.id) &&
              node.id !== id,
          ),
          edges: edges.filter(
            (edge) =>
              edge.source !== id &&
              edge.target !== id &&
              !downstreamAgentAndToolEdges.some((x) => x.id === edge.id),
          ),
        });
      },
      deleteAgentToolNodeById: (id) => {
        const { edges, deleteEdgeById, deleteNodeById } = get();

        const edge = edges.find(
          (x) => x.source === id && x.sourceHandle === NodeHandleId.Tool,
        );

        if (edge) {
          deleteEdgeById(edge.id);
          deleteNodeById(edge.target);
        }
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
      replaceNodeForm(nodeId, values) {
        if (nodeId) {
          set((state) => {
            for (const node of state.nodes) {
              if (node.id === nodeId) {
                //cloneDeep Solving the issue of react-hook-form errors
                node.data.form = cloneDeep(values); // TypeError: Cannot assign to read only property '0' of object '[object Array]'
                break;
              }
            }
          });
        }
      },
      updateSwitchFormData: (source, sourceHandle, target, isConnecting) => {
        const { updateNodeForm, edges } = get();
        if (sourceHandle) {
          // A handle will connect to multiple downstream nodes
          let currentHandleTargets = edges
            .filter(
              (x) =>
                x.source === source &&
                x.sourceHandle === sourceHandle &&
                typeof x.target === 'string',
            )
            .map((x) => x.target);

          let targets: string[] = currentHandleTargets;
          if (target) {
            if (!isConnecting) {
              targets = currentHandleTargets.filter((x) => x !== target);
            }
          }

          if (sourceHandle === SwitchElseTo) {
            updateNodeForm(source, targets, [SwitchElseTo]);
          } else {
            const operatorIndex = getOperatorIndex(sourceHandle);
            if (operatorIndex) {
              updateNodeForm(source, targets, [
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
          set((state) => {
            for (const node of state.nodes) {
              if (node.id === id) {
                node.data.name = name;
                break;
              }
            }
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
      setClickedToolId: (id?: string) => {
        set({ clickedToolId: id });
      },
      findUpstreamNodeById: (id) => {
        const { edges, getNode } = get();
        const edge = edges.find((x) => x.target === id);
        return getNode(edge?.source);
      },
      deleteEdgesBySourceAndSourceHandle: (source, sourceHandle) => {
        const { edges, setEdges } = get();
        setEdges(
          edges.filter(
            (edge) =>
              !(edge.source === source && edge.sourceHandle === sourceHandle),
          ),
        );
      },
      findAgentToolNodeById: (id) => {
        const { edges } = get();
        return edges.find(
          (edge) =>
            edge.source === id && edge.sourceHandle === NodeHandleId.Tool,
        )?.target;
      },
      selectNodeIds: (nodeIds) => {
        const { nodes, setNodes } = get();
        setNodes(
          nodes.map((node) => ({
            ...node,
            selected: nodeIds.includes(node.id),
          })),
        );
      },
      hasChildNode: (nodeId) => {
        const { edges } = get();
        return edges.some((edge) => edge.source === nodeId);
      },
    })),
    { name: 'graph', trace: true },
  ),
);

export default useGraphStore;
