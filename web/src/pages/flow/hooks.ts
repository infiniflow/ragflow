import React, { Dispatch, SetStateAction, useCallback, useState } from 'react';
import { Node, OnInit } from 'reactflow';
import { v4 as uuidv4 } from 'uuid';

export const useHandleDrag = () => {
  const handleDragStart = useCallback(
    (operatorId: string) => (ev: React.DragEvent<HTMLDivElement>) => {
      console.info(ev.clientX, ev.pageY);
      ev.dataTransfer.setData('operatorId', operatorId);
      ev.dataTransfer.setData('startClientX', ev.clientX.toString());
      ev.dataTransfer.setData('startClientY', ev.clientY.toString());
    },
    [],
  );

  return { handleDragStart };
};

export const useHandleDrop = (setNodes: Dispatch<SetStateAction<Node[]>>) => {
  const allowDrop = (ev: React.DragEvent<HTMLDivElement>) => {
    ev.preventDefault();
  };

  const handleDrop = useCallback(
    (ev: React.DragEvent<HTMLDivElement>) => {
      ev.preventDefault();
      const operatorId = ev.dataTransfer.getData('operatorId');
      const startClientX = ev.dataTransfer.getData('startClientX');
      const startClientY = ev.dataTransfer.getData('startClientY');
      console.info(operatorId);
      console.info(ev.pageX, ev.pageY);
      console.info(ev.clientX, ev.clientY);
      console.info(ev.movementX, ev.movementY);
      const x = ev.clientX - 200;
      const y = ev.clientY - 72;
      setNodes((pre) => {
        return pre.concat({
          id: operatorId,
          position: { x, y },
          data: { label: operatorId },
        });
      });
    },
    [setNodes],
  );

  return { handleDrop, allowDrop };
};

export const useHandleDragNext = () => {
  const handleDragStart = useCallback(
    (operatorId: string) => (ev: React.DragEvent<HTMLDivElement>) => {
      ev.dataTransfer.setData('application/reactflow', operatorId);
      ev.dataTransfer.effectAllowed = 'move';
    },
    [],
  );

  return { handleDragStart };
};

export const useHandleDropNext = (
  setNodes: Dispatch<SetStateAction<Node[]>>,
) => {
  const [reactFlowInstance, setReactFlowInstance] = useState<
    OnInit<any | any> | undefined
  >();

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
        position,
        data: { label: `${type} node` },
      };

      setNodes((nds) => nds.concat(newNode));
    },
    [reactFlowInstance, setNodes],
  );

  return { onDrop, onDragOver, setReactFlowInstance };
};
