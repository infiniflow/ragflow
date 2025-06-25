import { RAGFlowNodeType } from '@/interfaces/database/flow';
import { get, isEmpty, isPlainObject, omit } from 'lodash';
import { useMemo, useRef } from 'react';
import { Operator } from '../constant';
import { buildCategorizeListFromObject, convertToObjectArray } from '../utils';
import { useFormConfigMap } from './use-form-config-map';

export function useValues(node?: RAGFlowNodeType, isDirty?: boolean) {
  const operatorName: Operator = node?.data.label as Operator;
  const previousId = useRef<string | undefined>(node?.id);

  const FormConfigMap = useFormConfigMap();

  const currentFormMap = FormConfigMap[operatorName];

  const values = useMemo(() => {
    const formData = node?.data?.form;
    if (operatorName === Operator.Categorize) {
      const items = buildCategorizeListFromObject(
        get(node, 'data.form.category_description', {}),
      );
      if (isPlainObject(formData)) {
        console.info('xxx');
        const nextValues = {
          ...omit(formData, 'category_description'),
          items,
        };

        return nextValues;
      }
    } else if (operatorName === Operator.Message) {
      return {
        ...formData,
        content: convertToObjectArray(formData.content),
      };
    } else {
      return isEmpty(formData) ? currentFormMap : formData;
    }
  }, [currentFormMap, node, operatorName]);

  return values;
}
