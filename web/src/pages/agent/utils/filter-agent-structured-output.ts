import { JSONSchema } from '@/components/jsonjoy-builder';
import { get, isPlainObject } from 'lodash';
import { JsonSchemaDataType } from '../constant';

// Loop operators can only accept variables of type list.

// Recursively traverse the JSON schema, keeping attributes with type "array" and discarding others.

export function filterLoopOperatorInput(
  structuredOutput: JSONSchema,
  type: string,
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
          (value.type === type || hasArrayChild(value))
        ) {
          pre[key] = filterLoopOperatorInput(value, type, path);
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
  type?: string,
) {
  if (typeof structuredOutput === 'boolean') {
    return structuredOutput;
  }
  if (
    structuredOutput.properties &&
    isPlainObject(structuredOutput.properties)
  ) {
    if (type) {
      return filterLoopOperatorInput(structuredOutput, type);
    }

    return structuredOutput;
  }

  return structuredOutput;
}

export function hasSpecificTypeChild(
  data: Record<string, any> | Array<any>,
  type: string,
) {
  if (Array.isArray(data)) {
    for (const value of data) {
      if (isPlainObject(value) && value.type === type) {
        return true;
      }
      if (hasSpecificTypeChild(value, type)) {
        return true;
      }
    }
  }

  if (isPlainObject(data)) {
    for (const value of Object.values(data)) {
      if (isPlainObject(value) && value.type === type) {
        return true;
      }

      if (hasSpecificTypeChild(value, type)) {
        return true;
      }
    }
  }

  return false;
}

export function hasArrayChild(data: Record<string, any> | Array<any>) {
  return hasSpecificTypeChild(data, JsonSchemaDataType.Array);
}

export function hasJsonSchemaChild(data: JSONSchema) {
  const properties = get(data, 'properties') ?? {};
  return isPlainObject(properties) && Object.keys(properties).length > 0;
}
