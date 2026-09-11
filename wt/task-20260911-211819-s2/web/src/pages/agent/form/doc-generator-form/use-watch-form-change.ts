import { useEffect } from 'react';
import { UseFormReturn } from 'react-hook-form';
import useGraphStore from '../../store';

export const useWatchFormChange = (
  nodeId: string | undefined,
  form: UseFormReturn<any>,
) => {
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  useEffect(() => {
    const { unsubscribe } = form.watch((value) => {
      if (nodeId) {
        updateNodeForm(nodeId, value);
      }
    });
    return () => unsubscribe();
  }, [form, nodeId, updateNodeForm]);
};
