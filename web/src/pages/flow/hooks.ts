import { useSetModalState } from '@/hooks/commonHooks';
import React, {
  Dispatch,
  KeyboardEventHandler,
  SetStateAction,
  useCallback,
  useState,
} from 'react';
import {
  Node,
  Position,
  ReactFlowInstance,
  useOnSelectionChange,
  useReactFlow,
} from 'reactflow';
import { v4 as uuidv4 } from 'uuid';

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

export const useHandleDrop = (setNodes: Dispatch<SetStateAction<Node[]>>) => {
  const [reactFlowInstance, setReactFlowInstance] =
    useState<ReactFlowInstance<any, any>>();

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
      const newNode = {
        id: uuidv4(),
        type: 'textUpdater',
        position: position || {
          x: 0,
          y: 0,
        },
        data: { label: `${type}` },
        sourcePosition: Position.Right,
        targetPosition: Position.Left,
      };

      setNodes((nds) => nds.concat(newNode));
    },
    [reactFlowInstance, setNodes],
  );

  return { onDrop, onDragOver, setReactFlowInstance };
};

export const useShowDrawer = () => {
  const {
    visible: drawerVisible,
    hideModal: hideDrawer,
    showModal: showDrawer,
  } = useSetModalState();

  return {
    drawerVisible,
    hideDrawer,
    showDrawer,
  };
};

export const useHandleSelectionChange = () => {
  const [selectedNodes, setSelectedNodes] = useState<string[]>([]);
  const [selectedEdges, setSelectedEdges] = useState<string[]>([]);

  useOnSelectionChange({
    onChange: ({ nodes, edges }) => {
      setSelectedNodes(nodes.map((node) => node.id));
      setSelectedEdges(edges.map((edge) => edge.id));
    },
  });

  return { selectedEdges, selectedNodes };
};

export const useDeleteEdge = (selectedEdges: string[]) => {
  const { setEdges } = useReactFlow();

  const deleteEdge = useCallback(() => {
    setEdges((edges) =>
      edges.filter((edge) => selectedEdges.every((x) => x !== edge.id)),
    );
  }, [setEdges, selectedEdges]);

  return deleteEdge;
};

export const useHandleKeyUp = (
  selectedEdges: string[],
  selectedNodes: string[],
) => {
  const deleteEdge = useDeleteEdge(selectedEdges);
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
  const saveGraph = useCallback(() => {}, []);

  return { saveGraph };
};
