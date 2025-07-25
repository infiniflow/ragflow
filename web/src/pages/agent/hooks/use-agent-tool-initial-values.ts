import { omit } from 'lodash';
import { useCallback } from 'react';
import { Operator } from '../constant';
import { useInitializeOperatorParams } from './use-add-node';

export function useAgentToolInitialValues() {
  const { initialFormValuesMap } = useInitializeOperatorParams();

  const initializeAgentToolValues = useCallback(
    (operatorName: Operator) => {
      const initialValues = initialFormValuesMap[operatorName];

      switch (operatorName) {
        case Operator.Retrieval:
          return {
            ...omit(initialValues, 'query'),
            description: '',
          };
        case (Operator.TavilySearch, Operator.TavilyExtract):
          return {
            api_key: '',
          };
        case Operator.ExeSQL:
          return omit(initialValues, 'sql');
        case Operator.Bing:
          return omit(initialValues, 'query');

        default:
          return initialValues;
      }
    },
    [initialFormValuesMap],
  );

  return { initializeAgentToolValues };
}
