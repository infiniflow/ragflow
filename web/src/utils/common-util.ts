import { LLMFactory } from '@/constants/llm';
import { IFactory } from '@/interfaces/database/llm';
import isObject from 'lodash/isObject';
import snakeCase from 'lodash/snakeCase';

export const isFormData = (data: unknown): data is FormData => {
  return data instanceof FormData;
};

const excludedFields = ['img2txt_id', 'mcpServers'];

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
  LLMFactory.Ai302,
  LLMFactory.CometAPI,
  LLMFactory.DeerAPI,
  LLMFactory.JiekouAI,
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

// Get the actual color value of a CSS variable
function getCSSVariableValue(variableName: string): string {
  const computedStyle = getComputedStyle(document.documentElement);
  const value = computedStyle.getPropertyValue(variableName).trim();
  if (!value) {
    throw new Error(`CSS variable ${variableName} is not defined`);
  }
  return value;
}

/**Parse the color and convert to RGB,
 * #fff -> [255, 255, 255]
 * var(--text-primary) -> [var(--text-primary-r), var(--text-primary-g), var(--text-primary-b)]
 * */
export function parseColorToRGB(color: string): [number, number, number] {
  // Handling CSS variables (e.g. var(--accent-primary))
  let colorStr = color;
  if (colorStr.startsWith('var(')) {
    const varMatch = color.match(/var\(([^)]+)\)/);
    if (!varMatch) {
      console.error(`Invalid CSS variable: ${color}`);
      return [0, 0, 0];
    }
    const varName = varMatch[1];
    if (!varName) {
      console.error(`Invalid CSS variable: ${colorStr}`);
      return [0, 0, 0];
    }
    colorStr = getCSSVariableValue(varName);
  }

  // Handle rgb(var(--accent-primary)) format
  if (colorStr.startsWith('rgb(var(')) {
    const varMatch = colorStr.match(/rgb\(var\(([^)]+)\)\)/);
    if (!varMatch) {
      console.error(`Invalid nested CSS variable: ${color}`);
      return [0, 0, 0];
    }
    const varName = varMatch[1];
    if (!varName) {
      console.error(`Invalid nested CSS variable: ${colorStr}`);
      return [0, 0, 0];
    }
    // Get the CSS variable value which should be in format "r, g, b"
    const rgbValues = getCSSVariableValue(varName);
    const rgbMatch = rgbValues.match(/^(\d+),?\s*(\d+),?\s*(\d+)$/);
    if (rgbMatch) {
      return [
        parseInt(rgbMatch[1]),
        parseInt(rgbMatch[2]),
        parseInt(rgbMatch[3]),
      ];
    }
    console.error(`Unsupported RGB CSS variable format: ${rgbValues}`);
    return [0, 0, 0];
  }

  // Handles hexadecimal colors (e.g. #FF5733)
  if (colorStr.startsWith('#')) {
    const cleanedHex = colorStr.replace(/^#/, '');
    if (cleanedHex.length === 3) {
      return [
        parseInt(cleanedHex[0] + cleanedHex[0], 16),
        parseInt(cleanedHex[1] + cleanedHex[1], 16),
        parseInt(cleanedHex[2] + cleanedHex[2], 16),
      ];
    }
    return [
      parseInt(cleanedHex.slice(0, 2), 16),
      parseInt(cleanedHex.slice(2, 4), 16),
      parseInt(cleanedHex.slice(4, 6), 16),
    ];
  }

  // Handling RGB colors (e.g., rgb(255, 87, 51))
  if (colorStr.startsWith('rgb')) {
    const rgbMatch = colorStr.match(/rgb\((\d+),\s*(\d+),\s*(\d+)\)/);
    if (rgbMatch) {
      return [
        parseInt(rgbMatch[1]),
        parseInt(rgbMatch[2]),
        parseInt(rgbMatch[3]),
      ];
    }
    console.error(`Unsupported RGB format: ${colorStr}`);
    return [0, 0, 0];
  }
  console.error(`Unsupported colorStr format: ${colorStr}`);
  return [0, 0, 0];
}

/**
 *
 * @param color eg: #fff, or var(--color-text-primary)
 * @param opcity 0~1
 * @return rgba(r,g,b,opcity)
 */
export function parseColorToRGBA(color: string, opcity = 1): string {
  const [r, g, b] = parseColorToRGB(color);
  return `rgba(${r},${g},${b},${opcity})`;
}
