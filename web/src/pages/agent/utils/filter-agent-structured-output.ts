import { JSONSchema } from '@/components/jsonjoy-builder';
import { Operator } from '@/constants/agent';
import { isPlainObject } from 'lodash';

export function filterAgentStructuredOutput(
  structuredOutput: JSONSchema,
  operator?: string,
) {
  if (typeof structuredOutput === 'boolean') {
    return structuredOutput;
  }
  if (
    structuredOutput.properties &&
    isPlainObject(structuredOutput.properties)
  ) {
    const filterByPredicate = (predicate: (value: JSONSchema) => boolean) => {
      const properties = Object.entries({
        ...structuredOutput.properties,
      }).reduce(
        (pre, [key, value]) => {
          if (predicate(value)) {
            pre[key] = value;
          }
          return pre;
        },
        {} as Record<string, JSONSchema>,
      );

      return { ...structuredOutput, properties };
    };

    if (operator === Operator.Iteration) {
      return filterByPredicate(
        (value) => typeof value !== 'boolean' && value.type === 'array',
      );
    } else {
      return filterByPredicate(
        (value) => typeof value !== 'boolean' && value.type !== 'array',
      );
    }
  }

  return structuredOutput;
}
