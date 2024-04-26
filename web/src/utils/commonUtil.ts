import isObject from 'lodash/isObject';
import snakeCase from 'lodash/snakeCase';

export const isFormData = (data: unknown): data is FormData => {
  return data instanceof FormData;
};

const excludedFields = ['img2txt_id'];

const isExcludedField = (key: string) => {
  return excludedFields.includes(key);
};

export const convertTheKeysOfTheObjectToSnake = (data: unknown) => {
  if (isObject(data) && !isFormData(data)) {
    return Object.keys(data).reduce<Record<string, any>>((pre, cur) => {
      const value = (data as Record<string, any>)[cur];
      pre[isFormData(value) || isExcludedField(cur) ? cur : snakeCase(cur)] =
        value;
      return pre;
    }, {});
  }
  return data;
};

export const getSearchValue = (key: string) => {
  const params = new URL(document.location as any).searchParams;
  return params.get(key);
};

// Formatize numbers, add thousands of separators
export const formatNumberWithThousandsSeparator = (numberStr: string) => {
  const formattedNumber = numberStr.replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  return formattedNumber;
};
