import { Operator } from '@/constants/agent';
import { cloneDeepWith, get, isPlainObject } from 'lodash';

const apiKeyOperators = [
  Operator.TavilySearch,
  Operator.TavilyExtract,
  Operator.Google,
  Operator.KeenableSearch,
  Operator.YouComSearch,
  Operator.BGPT,
  Operator.QueritContents,
  Operator.QueritSearch,
];

// Canvas nodes carry the operator under `data.label` and the key under
// `data.form.api_key`, so a graph export has to be sanitized separately from the
// agent-tool records above.
const nodeLabelApiKeyOperators: string[] = [Operator.YouComSearch];

function isQueritOperator(value: unknown) {
  if (typeof value !== 'string') {
    return false;
  }

  return ['querit', 'queritcontents', 'queritsearch'].includes(
    value.replace(/_/g, '').toLowerCase(),
  );
}

function isNodeLabelApiKeyOperator(value: unknown) {
  if (typeof value !== 'string') {
    return false;
  }

  return isQueritOperator(value) || nodeLabelApiKeyOperators.includes(value);
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
      isNodeLabelApiKeyOperator(get(value, 'data.label')) &&
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
