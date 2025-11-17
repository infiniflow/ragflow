import { JSONSchema } from '@/components/jsonjoy-builder';
import { get, isPlainObject, toLower } from 'lodash';
import { JsonSchemaDataType } from '../constant';

function predicate(types: string[], type: string) {
  return types.some((x) => toLower(x) === toLower(type));
}

export function hasSpecificTypeChild(
  data: Record<string, any> | Array<any>,
  types: string[] = [],
) {
  if (Array.isArray(data)) {
    for (const value of data) {
      if (isPlainObject(value) && predicate(types, value.type)) {
        return true;
      }
      if (hasSpecificTypeChild(value, types)) {
        return true;
      }
    }
  }

  if (isPlainObject(data)) {
    for (const value of Object.values(data)) {
      if (isPlainObject(value) && predicate(types, value.type)) {
        return true;
      }

      if (hasSpecificTypeChild(value, types)) {
        return true;
      }
    }
  }

  return false;
}

export function hasArrayChild(data: Record<string, any> | Array<any>) {
  return hasSpecificTypeChild(data, [JsonSchemaDataType.Array]);
}

export function hasJsonSchemaChild(data: JSONSchema) {
  const properties = get(data, 'properties') ?? {};
  return isPlainObject(properties) && Object.keys(properties).length > 0;
}
