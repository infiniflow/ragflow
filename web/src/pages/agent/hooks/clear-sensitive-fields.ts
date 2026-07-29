import { Operator } from '@/constants/agent';
import { cloneDeepWith, get, isPlainObject } from 'lodash';

const apiKeyOperators = [
  Operator.TavilySearch,
  Operator.TavilyExtract,
  Operator.Google,
  Operator.KeenableSearch,
  Operator.BGPT,
  Operator.QueritSearch,
];

function isQueritOperator(value: unknown) {
  if (typeof value !== 'string') {
    return false;
  }

  return ['querit', 'queritsearch'].includes(
    value.replace(/_/g, '').toLowerCase(),
  );
}

export function clearSensitiveFields<T>(obj: T): T {
  return cloneDeepWith(obj, (value) => {
    if (!isPlainObject(value)) {
      return;
    }

    if (
      (apiKeyOperators.includes(value.component_name) ||
        isQueritOperator(value.component_name)) &&
      get(value, 'params.api_key')
    ) {
      return { ...value, params: { ...value.params, api_key: '' } };
    }

    if (
      isQueritOperator(get(value, 'data.label')) &&
      get(value, 'data.form.api_key')
    ) {
      return {
        ...value,
        data: {
          ...value.data,
          form: {
            ...value.data.form,
            api_key: '',
          },
        },
      };
    }
  });
}
