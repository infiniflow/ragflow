import React, { useCallback } from 'react';

export const useHandleDrop = () => {
  const allowDrop = (ev: React.DragEvent<HTMLDivElement>) => {
    ev.preventDefault();
  };

  const handleDrop = useCallback((ev: React.DragEvent<HTMLDivElement>) => {
    ev.preventDefault();
    const operatorId = ev.dataTransfer.getData('operatorId');
    console.info(operatorId);
  }, []);

  return { handleDrop, allowDrop };
};

export const useHandleDrag = () => {
  const handleDrag = useCallback(
    (operatorId: string) => (ev: React.DragEvent<HTMLDivElement>) => {
      ev.dataTransfer.setData('operatorId', operatorId);
    },
    [],
  );

  return { handleDrag };
};
