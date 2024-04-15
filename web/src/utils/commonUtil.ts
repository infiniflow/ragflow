import isObject from 'lodash/isObject';
import snakeCase from 'lodash/snakeCase';

export const convertTheKeysOfTheObjectToSnake = (data: unknown) => {
  if (isObject(data)) {
    return Object.keys(data).reduce<Record<string, any>>((pre, cur) => {
      pre[snakeCase(cur)] = (data as Record<string, any>)[cur];
      return pre;
    }, {});
  }
  return data;
};
