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

export function clearSensitiveFields<T>(obj: T): T {
  return cloneDeepWith(obj, (value) => {
    if (!isPlainObject(value)) {
      return;
    }

    if (
      apiKeyOperators.includes(value.component_name) &&
      get(value, 'params.api_key')
    ) {
      return { ...value, params: { ...value.params, api_key: '' } };
    }

    if (
      get(value, 'data.label') === Operator.QueritSearch &&
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
