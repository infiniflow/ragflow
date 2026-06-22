import { isEqual } from 'lodash';

import {
  ICompilationTemplate,
  ICompilationTemplateSection,
} from '@/interfaces/database/compilation-template';

import { FormSchemaType } from './schema';

export const DefaultFieldKeys = ['type', 'description', 'rule'];

export const FieldKeyOrders = [
  DefaultFieldKeys,
  ['statement', 'subject'],
  ['definition_excerpt', 'term'],
];

export const getFieldKeyOrder = (keys: string[]): string[] => {
  const sortedKeys = [...keys].sort();
  return (
    FieldKeyOrders.find((order) => isEqual([...order].sort(), sortedKeys)) ??
    keys
  );
};

export const DefaultValues: FormSchemaType = {
  name: '',
  description: '',
  llm_id: '',
  kind: '',
  config: {
    kind: '',
    llm_id: '',
    global_rules: '',
  },
};

export const isConfigMetaKey = (key: string) =>
  ['kind', 'llm_id', 'global_rules'].includes(key);

export const createEmptyField = (keys: string[]) =>
  Object.fromEntries(keys.map((key) => [key, '']));

export const normalizeSection = (
  section?: ICompilationTemplateSection,
): ICompilationTemplateSection => {
  const fields = section?.fields ?? [];
  return {
    description: section?.description ?? '',
    fields:
      fields.length > 0
        ? fields.map((field) =>
            Object.fromEntries(
              Object.entries(field).map(([key, value]) => [key, value ?? '']),
            ),
          )
        : [createEmptyField(DefaultFieldKeys)],
  };
};

export const transformDetailToForm = (
  detail: ICompilationTemplate,
): FormSchemaType => {
  const config = detail.config ?? {};
  const sections: FormSchemaType['config'] = {
    kind: config.kind ?? '',
    llm_id: config.llm_id ?? '',
    global_rules: config.global_rules ?? '',
  };

  Object.entries(config).forEach(([key, value]) => {
    if (isConfigMetaKey(key)) return;
    sections[key] = normalizeSection(
      value as ICompilationTemplateSection,
    ) as FormSchemaType['config'][string];
  });

  return {
    name: detail.name ?? '',
    description: detail.description ?? '',
    llm_id: config.llm_id ?? '',
    kind: detail.kind ?? '',
    config: sections,
  };
};

export const transformFormToPayload = (values: FormSchemaType) => {
  const config: Record<string, ICompilationTemplateSection | string> = {};
  Object.entries(values.config).forEach(([key, value]) => {
    if (isConfigMetaKey(key)) {
      if (typeof value === 'string') config[key] = value;
    } else if (typeof value !== 'string') {
      config[key] = value as ICompilationTemplateSection;
    }
  });

  return {
    name: values.name,
    description: values.description,
    kind: values.kind,
    config: {
      kind: values.kind,
      llm_id: values.llm_id,
      ...config,
    },
  };
};

export const SectionTitleKeyMap: Record<string, string> = {
  entity: 'setting.entitySpecification',
  relation: 'setting.relationSpecification',
  concept: 'setting.conceptSpecification',
  claim: 'setting.claimSpecification',
};

export const FieldLabelKeyMap: Record<string, string> = {
  type: 'setting.fieldType',
  description: 'setting.fieldDescription',
  rule: 'setting.fieldRule',
};
