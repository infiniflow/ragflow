import { getMonospaceLength } from '@/utils/helper';

export const monospace = (rule, value, callback) => {
  const length = getMonospaceLength(value);
  if (rule.max > 0 && length > rule.max) {
    callback(`长度不能超过${rule.max}!`);
  } else if (rule.min > 0 && length < rule.min) {
    callback(`长度不能小于${rule.min}!`);
  } else {
    callback();
  }
};
