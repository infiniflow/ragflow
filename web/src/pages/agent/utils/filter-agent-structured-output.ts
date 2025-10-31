import { JSONSchema } from '@/components/jsonjoy-builder';
import { Operator } from '@/constants/agent';
import { isPlainObject } from 'lodash';

// Loop operators can only accept variables of type list.

// Recursively traverse the JSON schema, keeping attributes with type "array" and discarding others.

export function filterLoopOperatorInput(
  structuredOutput: JSONSchema,
  path = [],
) {
  if (typeof structuredOutput === 'boolean') {
    return structuredOutput;
  }
  if (
    structuredOutput.properties &&
    isPlainObject(structuredOutput.properties)
  ) {
    const properties = Object.entries({
      ...structuredOutput.properties,
    }).reduce(
      (pre, [key, value]) => {
        if (
          typeof value !== 'boolean' &&
          (value.type === 'array' || hasArrayChild(value))
        ) {
          pre[key] = filterLoopOperatorInput(value, path);
        }
        return pre;
      },
      {} as Record<string, JSONSchema>,
    );

    return { ...structuredOutput, properties };
  }

  return structuredOutput;
}

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
    if (operator === Operator.Iteration) {
      return filterLoopOperatorInput(structuredOutput);
    }

    return structuredOutput;
  }

  return structuredOutput;
}

export function hasArrayChild(data: Record<string, any> | Array<any>) {
  if (Array.isArray(data)) {
    for (const value of data) {
      if (isPlainObject(value) && value.type === 'array') {
        return true;
      }
      if (hasArrayChild(value)) {
        return true;
      }
    }
  }

  if (isPlainObject(data)) {
    for (const value of Object.values(data)) {
      if (isPlainObject(value) && value.type === 'array') {
        return true;
      }

      if (hasArrayChild(value)) {
        return true;
      }
    }
  }

  return false;
}
