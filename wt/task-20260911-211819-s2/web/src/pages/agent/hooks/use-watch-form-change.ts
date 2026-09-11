import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../store';

export function useWatchFormChange(
  id?: string,
  form?: UseFormReturn<any>,
  enableReplacement = false,
) {
  let values = useWatch({ control: form?.control });
  const { updateNodeForm, replaceNodeForm, markNodeFormEdited } = useGraphStore(
    (state) => state,
  );

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas
    if (id) {
      if (form?.formState.isDirty) {
        markNodeFormEdited(id);
      }

      values = form?.getValues() || {};
      const nextValues: any = values;

      (enableReplacement ? replaceNodeForm : updateNodeForm)(id, nextValues);
    }
  }, [form?.formState.isDirty, id, markNodeFormEdited, updateNodeForm, values]);
}
