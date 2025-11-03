import { JSONSchema } from '@/components/jsonjoy-builder';
import { get, isPlainObject } from 'lodash';
import { JsonSchemaDataType } from '../constant';

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
