import React, { Dispatch, SetStateAction, useCallback } from 'react';
import { Node } from 'reactflow';

export const useHandleDrag = () => {
  const handleDrag = useCallback(
    (operatorId: string) => (ev: React.DragEvent<HTMLDivElement>) => {
      console.info(ev.clientX, ev.pageY);
      ev.dataTransfer.setData('operatorId', operatorId);
      ev.dataTransfer.setData('startClientX', ev.clientX.toString());
      ev.dataTransfer.setData('startClientY', ev.clientY.toString());
    },
    [],
  );

  return { handleDrag };
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
