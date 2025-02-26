import { LLMFactory } from '@/constants/llm';
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
  LLMFactory.OpenAI,
  LLMFactory.Moonshot,
  LLMFactory.PPIO,
  LLMFactory.ZhipuAI,
  LLMFactory.Ollama,
  LLMFactory.Xinference,
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

export const stringToUint8Array = (str: string) => {
  // const byteString = str.replace(/b'|'/g, '');
  const byteString = str.slice(2, -1);

  const uint8Array = new Uint8Array(byteString.length);
  for (let i = 0; i < byteString.length; i++) {
    uint8Array[i] = byteString.charCodeAt(i);
  }

  return uint8Array;
};

export const hexStringToUint8Array = (hex: string) => {
  const arr = hex.match(/[\da-f]{2}/gi);
  if (Array.isArray(arr)) {
    return new Uint8Array(
      arr.map(function (h) {
        return parseInt(h, 16);
      }),
    );
  }
};

export function hexToArrayBuffer(input: string) {
  if (typeof input !== 'string') {
    throw new TypeError('Expected input to be a string');
  }

  if (input.length % 2 !== 0) {
    throw new RangeError('Expected string to be an even number of characters');
  }

  const view = new Uint8Array(input.length / 2);

  for (let i = 0; i < input.length; i += 2) {
    view[i / 2] = parseInt(input.substring(i, i + 2), 16);
  }

  return view.buffer;
}

export function formatFileSize(bytes: number, si = true, dp = 1) {
  let nextBytes = bytes;
  const thresh = si ? 1000 : 1024;

  if (Math.abs(bytes) < thresh) {
    return nextBytes + ' B';
  }

  const units = si
    ? ['kB', 'MB', 'GB', 'TB', 'PB', 'EB', 'ZB', 'YB']
    : ['KiB', 'MiB', 'GiB', 'TiB', 'PiB', 'EiB', 'ZiB', 'YiB'];
  let u = -1;
  const r = 10 ** dp;

  do {
    nextBytes /= thresh;
    ++u;
  } while (
    Math.round(Math.abs(nextBytes) * r) / r >= thresh &&
    u < units.length - 1
  );

  return nextBytes.toFixed(dp) + ' ' + units[u];
}
