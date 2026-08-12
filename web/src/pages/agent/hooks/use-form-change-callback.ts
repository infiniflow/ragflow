import { useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';

export function useFormChangeCallback(
  form: UseFormReturn<any>,
  onValuesChange?: (values: any) => void,
) {
  const values = useWatch({ control: form.control });

  useEffect(() => {
    if (onValuesChange) {
      onValuesChange(values);
    }
  }, [onValuesChange, values]);
}
