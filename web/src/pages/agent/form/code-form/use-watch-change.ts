import { CodeTemplateStrMap, ProgrammingLanguage } from '@/constants/agent';
import { useCallback, useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../../store';

function convertToObject(list: Array<{ name: string; component_id: string }>) {
  return list.reduce<Record<string, string>>((pre, cur) => {
    pre[cur.name] = cur.component_id;

    return pre;
  }, {});
}

export function useWatchFormChange(id?: string, form?: UseFormReturn) {
  let values = useWatch({ control: form?.control });
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas
    if (id) {
      values = form?.getValues() || {};
      let nextValues: any = {
        ...values,
        arguments: convertToObject(values.arguments),
      };

      updateNodeForm(id, nextValues);
    }
  }, [form?.formState.isDirty, id, updateNodeForm, values]);
}

export function useHandleLanguageChange(id?: string, form?: UseFormReturn) {
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  const handleLanguageChange = useCallback(
    (lang: string) => {
      if (id) {
        const script = CodeTemplateStrMap[lang as ProgrammingLanguage];
        form?.setValue('script', script);
        updateNodeForm(id, script, ['script']);
      }
    },
    [form, id, updateNodeForm],
  );

  return handleLanguageChange;
}
