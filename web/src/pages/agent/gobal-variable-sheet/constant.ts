import { FormFieldConfig, FormFieldType } from '@/components/dynamic-form';
import { t } from 'i18next';
import { TypesWithArray } from '../constant';
import { buildConversationVariableSelectOptions } from '../utils';
export { TypesWithArray } from '../constant';
// const TypesWithoutArray = Object.values(JsonSchemaDataType).filter(
//   (item) => item !== JsonSchemaDataType.Array,
// );
// const TypesWithArray = [
//   ...TypesWithoutArray,
//   ...TypesWithoutArray.map((item) => `array<${item}>`),
// ];

export const GlobalFormFields = [
  {
    label: t('flow.name'),
    name: 'name',
    placeholder: t('common.namePlaceholder'),
    required: true,
    validation: {
      pattern: /^[a-zA-Z_0-9]+$/,
      message: t('flow.variableNameMessage'),
    },
    type: FormFieldType.Text,
  },
  {
    label: t('flow.type'),
    name: 'type',
    placeholder: '',
    required: true,
    type: FormFieldType.Select,
    options: buildConversationVariableSelectOptions(),
  },
  {
    label: t('flow.defaultValue'),
    name: 'value',
    placeholder: '',
    type: FormFieldType.Textarea,
  },
  {
    label: t('flow.description'),
    name: 'description',
    placeholder: t('flow.variableDescription'),
    type: FormFieldType.Textarea,
  },
] as FormFieldConfig[];

export const GlobalVariableFormDefaultValues = {
  name: '',
  type: TypesWithArray.String,
  value: '',
  description: '',
};

export const TypeMaps = {
  [TypesWithArray.String]: FormFieldType.Textarea,
  [TypesWithArray.Number]: FormFieldType.Number,
  [TypesWithArray.Boolean]: FormFieldType.Checkbox,
  [TypesWithArray.Object]: FormFieldType.Textarea,
  [TypesWithArray.ArrayString]: FormFieldType.Textarea,
  [TypesWithArray.ArrayNumber]: FormFieldType.Textarea,
  [TypesWithArray.ArrayBoolean]: FormFieldType.Textarea,
  [TypesWithArray.ArrayObject]: FormFieldType.Textarea,
};
