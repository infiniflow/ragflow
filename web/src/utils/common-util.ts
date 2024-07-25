import { IFactory } from '@/interfaces/database/llm';
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

const orderFactoryList = [
  'OpenAI',
  'Moonshot',
  'ZHIPU-AI',
  'Ollama',
  'Xinference',
];

export const sortLLmFactoryListBySpecifiedOrder = (list: IFactory[]) => {
  const finalList: IFactory[] = [];
  orderFactoryList.forEach((orderItem) => {
    const index = list.findIndex((item) => item.name === orderItem);
    if (index !== -1) {
      finalList.push(list[index]);
    }
  });

  list.forEach((item) => {
    if (finalList.every((x) => x.name !== item.name)) {
      finalList.push(item);
    }
  });

  return finalList;
};

export const filterOptionsByInput = (
  input: string,
  option: { label: string; value: string } | undefined,
) => (option?.label ?? '').toLowerCase().includes(input.toLowerCase());

export const toFixed = (value: unknown, fixed = 2) => {
  if (typeof value === 'number') {
    return value.toFixed(fixed);
  }
  return value;
};
