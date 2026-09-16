import { CodeTemplateStrMap, ProgrammingLanguage } from '@/constants/agent';
import { isEmpty } from 'lodash';
import { useCallback, useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../../store';
import { FormSchemaType } from './schema';
import {
  buildDefaultCodeOutput,
  hasLegacyMultiOutputs,
  serializeCodeOutputContract,
} from './utils';

function convertToObject(list: FormSchemaType['arguments'] = []) {
  return list.reduce<Record<string, string>>((pre, cur) => {
    pre[cur.name] = cur.type;

    return pre;
  }, {});
}

export function useWatchFormChange(
  id?: string,
  form?: UseFormReturn<FormSchemaType>,
) {
  const watchedValues = useWatch({ control: form?.control });
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);
  const getNode = useGraphStore((state) => state.getNode);

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas
    if (id) {
      const values = form?.getValues() || watchedValues || {};
      const currentOutputs = getNode(id)?.data?.form?.outputs;
      const shouldPreserveLegacyOutputs =
        hasLegacyMultiOutputs(currentOutputs) &&
        isEmpty(form?.formState.dirtyFields?.output);
      const hasCompleteOutputContract =
        !!values?.output?.name?.trim() && !!values?.output?.type?.trim();
      const nextValues: any = {
        ...values,
        arguments: convertToObject(
          values?.arguments as FormSchemaType['arguments'],
        ),
        outputs: shouldPreserveLegacyOutputs
          ? currentOutputs
          : hasCompleteOutputContract
            ? serializeCodeOutputContract({
                name: values.output?.name?.trim() ?? '',
                type: values.output?.type?.trim() ?? '',
              })
            : (currentOutputs ??
              serializeCodeOutputContract(buildDefaultCodeOutput())),
      };
      delete nextValues.output;

      updateNodeForm(id, nextValues);
    }
  }, [
    form?.formState.dirtyFields?.output,
    form?.formState.isDirty,
    form,
    getNode,
    id,
    updateNodeForm,
    watchedValues,
  ]);
}

export function useHandleLanguageChange(
  id?: string,
  form?: UseFormReturn<FormSchemaType>,
) {
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  const handleLanguageChange = useCallback(
    (lang: string) => {
      if (id) {
        const script = CodeTemplateStrMap[lang as ProgrammingLanguage];
        form?.setValue('script', script);
        if (
          !form?.getValues('output')?.name ||
          !form?.getValues('output')?.type
        ) {
          form?.setValue('output', buildDefaultCodeOutput(), {
            shouldDirty: true,
          });
        }
        updateNodeForm(id, script, ['script']);
      }
    },
    [form, id, updateNodeForm],
  );

  return handleLanguageChange;
}
