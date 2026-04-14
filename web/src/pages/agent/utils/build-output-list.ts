import { OutputType } from '../form/components/output';

export function buildOutputList(outputs: Record<string, Record<string, any>>) {
  return Object.entries(outputs).reduce<OutputType[]>((pre, [key, val]) => {
    pre.push({ title: key, type: val.type });
    return pre;
  }, []);
}
