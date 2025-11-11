import { FormFieldConfig, FormFieldType } from '@/components/dynamic-form';
import { buildSelectOptions } from '@/utils/component-util';
import { t } from 'i18next';
// const TypesWithoutArray = Object.values(JsonSchemaDataType).filter(
//   (item) => item !== JsonSchemaDataType.Array,
// );
// const TypesWithArray = [
//   ...TypesWithoutArray,
//   ...TypesWithoutArray.map((item) => `array<${item}>`),
// ];

export enum TypesWithArray {
  String = 'string',
  Number = 'number',
  Boolean = 'boolean',
  // Object = 'object',
  // ArrayString = 'array<string>',
  // ArrayNumber = 'array<number>',
  // ArrayBoolean = 'array<boolean>',
  // ArrayObject = 'array<object>',
}

export const GobalFormFields = [
  {
    label: t('flow.name'),
    name: 'name',
    placeholder: t('common.namePlaceholder'),
    required: true,
    validation: {
      pattern: /^[a-zA-Z_]+$/,
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
    options: buildSelectOptions(Object.values(TypesWithArray)),
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
    type: 'textarea',
  },
] as FormFieldConfig[];

export const GobalVariableFormDefaultValues = {
  name: '',
  type: TypesWithArray.String,
  value: '',
  description: '',
};

export const TypeMaps = {
  [TypesWithArray.String]: FormFieldType.Textarea,
  [TypesWithArray.Number]: FormFieldType.Number,
  [TypesWithArray.Boolean]: FormFieldType.Checkbox,
  // [TypesWithArray.Object]: FormFieldType.Textarea,
  // [TypesWithArray.ArrayString]: FormFieldType.Textarea,
  // [TypesWithArray.ArrayNumber]: FormFieldType.Textarea,
  // [TypesWithArray.ArrayBoolean]: FormFieldType.Textarea,
  // [TypesWithArray.ArrayObject]: FormFieldType.Textarea,
};
