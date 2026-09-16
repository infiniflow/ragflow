import { useEffect, useRef } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../../store';
import { validateQueritFormValuesForPersistence } from './utils';

export function useWatchFormChange(id?: string, form?: UseFormReturn<any>) {
  const values = useWatch({ control: form?.control });
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);
  const previousValues = useRef(values);

  useEffect(() => {
    if (Object.is(previousValues.current, values)) {
      return;
    }
    previousValues.current = values;
    if (id && form) {
      const nextValues = validateQueritFormValuesForPersistence(
        form.getValues(),
      );
      if (nextValues) {
        updateNodeForm(id, nextValues);
      }
    }
  }, [form, id, updateNodeForm, values]);
}
