import { variableEnabledFieldMap } from '@/constants/chat';
import { TFunction } from 'i18next';
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

export function buildOptions(
  data: Record<string, any>,
  t?: TFunction<['translation', ...string[]], undefined>,
  prefix?: string,
) {
  if (t) {
    return Object.values(data).map((val) => ({
      label: t(`${prefix ? prefix + '.' : ''}${val.toLowerCase()}`),
      value: val,
    }));
  }
  return Object.values(data).map((val) => ({ label: val, value: val }));
}

export function setLLMSettingEnabledValues(
  initialLlmSetting?: Record<string, any>,
) {
  const values = Object.keys(variableEnabledFieldMap).reduce<
    Record<string, boolean>
  >((pre, field) => {
    pre[field] =
      initialLlmSetting === undefined
        ? false
        : !!initialLlmSetting[
            variableEnabledFieldMap[
              field as keyof typeof variableEnabledFieldMap
            ]
          ];
    return pre;
  }, {});
  return values;
}
