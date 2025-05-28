import { CodeTemplateStrMap, ProgrammingLanguage } from '@/constants/agent';
import { settledModelVariableMap } from '@/constants/knowledge';
import { omit } from 'lodash';
import { useCallback, useEffect } from 'react';
import { UseFormReturn } from 'react-hook-form';
import { Operator } from '../constant';
import useGraphStore from '../store';
import { buildCategorizeObjectFromList, convertToStringArray } from '../utils';

export const useHandleFormValuesChange = (
  operatorName: Operator,
  id?: string,
  form?: UseFormReturn,
) => {
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  const handleValuesChange = useCallback(
    (changedValues: any, values: any) => {
      let nextValues: any = values;
      // Fixed the issue that the related form value does not change after selecting the freedom field of the model
      if (
        Object.keys(changedValues).length === 1 &&
        'parameter' in changedValues &&
        changedValues['parameter'] in settledModelVariableMap
      ) {
        nextValues = {
          ...values,
          ...settledModelVariableMap[
            changedValues['parameter'] as keyof typeof settledModelVariableMap
          ],
        };
      }
      if (id) {
        updateNodeForm(id, nextValues);
      }
    },
    [updateNodeForm, id],
  );

  useEffect(() => {
    const subscription = form?.watch((value, { name, type, values }) => {
      if (id && name) {
        console.log(
          '🚀 ~ useEffect ~ value:',
          name,
          type,
          values,
          operatorName,
        );
        let nextValues: any = value;

        // Fixed the issue that the related form value does not change after selecting the freedom field of the model
        if (
          name === 'parameter' &&
          value['parameter'] in settledModelVariableMap
        ) {
          nextValues = {
            ...value,
            ...settledModelVariableMap[
              value['parameter'] as keyof typeof settledModelVariableMap
            ],
          };
        }

        const categoryDescriptionRegex = /items\.\d+\.name/g;
        if (
          operatorName === Operator.Categorize &&
          categoryDescriptionRegex.test(name)
        ) {
          nextValues = {
            ...omit(value, 'items'),
            category_description: buildCategorizeObjectFromList(value.items),
          };
        }

        if (
          operatorName === Operator.Code &&
          type === 'change' &&
          name === 'lang'
        ) {
          nextValues = {
            ...value,
            script: CodeTemplateStrMap[value.lang as ProgrammingLanguage],
          };
        }

        if (operatorName === Operator.Message) {
          nextValues = {
            ...value,
            content: convertToStringArray(value.content),
          };
        }

        // Manually triggered form updates are synchronized to the canvas
        if (form.formState.isDirty) {
          // run(id, nextValues);
          updateNodeForm(id, nextValues);
        }
      }
    });
    return () => subscription?.unsubscribe();
  }, [form, form?.watch, id, operatorName, updateNodeForm]);

  return { handleValuesChange };
};
