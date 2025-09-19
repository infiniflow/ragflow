import { Connection, Edge, getOutgoers } from '@xyflow/react';
import { useCallback } from 'react';
// import { shallow } from 'zustand/shallow';
import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { lowerFirst } from 'lodash';
import { useTranslation } from 'react-i18next';
import { Operator, RestrictedUpstreamMap } from './constant';
import useGraphStore, { RFState } from './store';
import { replaceIdWithText } from './utils';

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

export const useGetNodeName = () => {
  const { t } = useTranslation();

  return (type: string) => {
    const name = t(`dataflow.${lowerFirst(type)}`);
    return name;
  };
};

export const useGetNodeDescription = () => {
  const { t } = useTranslation();

  return (type: string) => {
    const name = t(`dataflow.${lowerFirst(type)}Description`);
    return name;
  };
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
