import { useSetModalState } from '@/hooks/commonHooks';
import React, { Dispatch, SetStateAction, useCallback, useState } from 'react';
import { Node, ReactFlowInstance } from 'reactflow';
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
        type,
        position: position || {
          x: 0,
          y: 0,
        },
        data: { label: `${type} node` },
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
