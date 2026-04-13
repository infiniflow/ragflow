import { BeginQuery } from '../../interface';

export function buildBeginInputListFromObject(
  inputs: Record<string, Omit<BeginQuery, 'key'>>,
) {
  return Object.entries(inputs || {}).reduce<BeginQuery[]>(
    (pre, [key, value]) => {
      pre.push({ ...(value || {}), key });

      return pre;
    },
    [],
  );
}
