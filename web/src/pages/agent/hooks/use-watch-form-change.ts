import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import { useCanvasReadonly } from '../context';
import useGraphStore from '../store';

export function useWatchFormChange(
  id?: string,
  form?: UseFormReturn<any>,
  enableReplacement = false,
) {
  let values = useWatch({ control: form?.control });
  const readOnly = useCanvasReadonly();
  const { updateNodeForm, replaceNodeForm, markNodeFormEdited } = useGraphStore(
    (state) => state,
  );

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas.
    // On a read-only canvas the form must never write back: this effect runs
    // once on mount without a dirty check, and disabled controls (e.g. the
    // Radix slider thumb) can still change local RHF state via the keyboard.
    if (readOnly) {
      return;
    }
    if (id) {
      if (form?.formState.isDirty) {
        markNodeFormEdited(id);
      }

      values = form?.getValues() || {};
      const nextValues: any = values;

      (enableReplacement ? replaceNodeForm : updateNodeForm)(id, nextValues);
    }
  }, [
    form?.formState.isDirty,
    id,
    markNodeFormEdited,
    readOnly,
    updateNodeForm,
    values,
  ]);
}
