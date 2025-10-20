import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../store';

export function useWatchFormChange(id?: string, form?: UseFormReturn<any>) {
  let values = useWatch({ control: form?.control });

  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  useEffect(() => {
    if (id) {
      updateNodeForm(id, values);
    }
  }, [id, updateNodeForm, values]);
}
