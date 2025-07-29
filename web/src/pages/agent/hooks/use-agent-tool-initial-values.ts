import { omit, pick } from 'lodash';
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
        case (Operator.TavilySearch, Operator.TavilyExtract, Operator.Google):
          return {
            api_key: '',
          };
        case Operator.ExeSQL:
          return omit(initialValues, 'sql');
        case Operator.Bing:
          return omit(initialValues, 'query');
        case Operator.YahooFinance:
          return omit(initialValues, 'stock_code');

        case Operator.Email:
          return pick(
            initialValues,
            'smtp_server',
            'smtp_port',
            'email',
            'password',
            'sender_name',
          );

        case Operator.DuckDuckGo:
          return pick(initialValues, 'top_n', 'channel');

        case Operator.Wikipedia:
          return pick(initialValues, 'top_n', 'language');

        default:
          return initialValues;
      }
    },
    [initialFormValuesMap],
  );

  return { initializeAgentToolValues };
}
