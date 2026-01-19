import { CodeTemplateStrMap, ProgrammingLanguage } from '@/constants/agent';
import { ICodeForm } from '@/interfaces/database/agent';
import { isEmpty } from 'lodash';
import { useCallback, useEffect } from 'react';
import { UseFormReturn, useWatch } from 'react-hook-form';
import useGraphStore from '../../store';
import { FormSchemaType } from './schema';

function convertToObject(list: FormSchemaType['arguments'] = []) {
  return list.reduce<Record<string, string>>((pre, cur) => {
    pre[cur.name] = cur.type;

    return pre;
  }, {});
}

type ArrayOutputs = Extract<FormSchemaType['outputs'], Array<any>>;

type ObjectOutputs = Exclude<FormSchemaType['outputs'], Array<any>>;

function convertOutputsToObject({ lang, outputs }: FormSchemaType) {
  if (lang === ProgrammingLanguage.Python) {
    return (outputs as ArrayOutputs).reduce<ICodeForm['outputs']>(
      (pre, cur) => {
        pre[cur.name] = {
          value: '',
          type: cur.type,
        };

        return pre;
      },
      {},
    );
  }
  const outputsObject = outputs as ObjectOutputs;
  if (isEmpty(outputsObject)) {
    return {};
  }
  return {
    [outputsObject.name]: {
      value: '',
      type: outputsObject.type,
    },
  };
}

export function useWatchFormChange(
  id?: string,
  form?: UseFormReturn<FormSchemaType>,
) {
  let values = useWatch({ control: form?.control });
  const updateNodeForm = useGraphStore((state) => state.updateNodeForm);

  useEffect(() => {
    // Manually triggered form updates are synchronized to the canvas
    if (id) {
      values = form?.getValues() || {};
      let nextValues: any = {
        ...values,
        arguments: convertToObject(
          values?.arguments as FormSchemaType['arguments'],
        ),
        outputs: convertOutputsToObject(values as FormSchemaType),
      };

      updateNodeForm(id, nextValues);
    }
  }, [form?.formState.isDirty, id, updateNodeForm, values]);
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
        form?.setValue(
          'outputs',
          (lang === ProgrammingLanguage.Python
            ? []
            : {}) as FormSchemaType['outputs'],
        );
        updateNodeForm(id, script, ['script']);
      }
    },
    [form, id, updateNodeForm],
  );

  return handleLanguageChange;
}
