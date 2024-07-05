import { variableEnabledFieldMap } from '@/constants/chat';
import omit from 'lodash/omit';

// chat model setting and generate operator
export const excludeUnEnabledVariables = (
  values: any = {},
  prefix = 'llm_setting.',
) => {
  const unEnabledFields: Array<keyof typeof variableEnabledFieldMap> =
    Object.keys(variableEnabledFieldMap).filter((key) => !values[key]) as Array<
      keyof typeof variableEnabledFieldMap
    >;

  return unEnabledFields.map(
    (key) => `${prefix}${variableEnabledFieldMap[key]}`,
  );
};

// chat model setting and generate operator
export const removeUselessFieldsFromValues = (values: any, prefix?: string) => {
  const nextValues: any = omit(values, [
    ...Object.keys(variableEnabledFieldMap),
    'parameter',
    ...excludeUnEnabledVariables(values, prefix),
  ]);

  return nextValues;
};
