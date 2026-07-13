import { isEqual } from 'lodash';

import {
  ICompilationTemplate,
  ICompilationTemplateBuiltin,
  ICompilationTemplateGroup,
  ICompilationTemplateRaptorConfig,
  ICompilationTemplateSection,
} from '@/interfaces/database/compilation-template';
import { ICompilationTemplateConfigRequest } from '@/interfaces/request/compilation-template';

import { CompilationTemplateKind } from '@/constants/compilation';

import { FormSchemaType, TemplateSchemaType } from './schema';

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

export const DefaultTemplateValues: TemplateSchemaType = {
  id: undefined,
  name: '',
  description: '',
  llm_id: '',
  kind: '',
  config: {
    kind: '',
    llm_id: '',
    global_rules: '',
    example: '',
    instruction: '',
    page_example: '',
  },
};

export const DefaultValues: FormSchemaType = {
  name: '',
  description: '',
  avatar: '',
  templates: [DefaultTemplateValues],
};

export const isConfigMetaKey = (key: string) =>
  [
    'kind',
    'llm_id',
    'global_rules',
    'example',
    'instruction',
    'page_example',
    'synthesis',
  ].includes(key);

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

export const buildConfigFromBuiltin = (
  builtinTemplate: ICompilationTemplateBuiltin,
  kind: string,
  llmId: string,
): TemplateSchemaType['config'] => {
  const sections: TemplateSchemaType['config'] = {
    kind,
    llm_id: llmId,
    global_rules:
      typeof builtinTemplate.config?.global_rules === 'string'
        ? builtinTemplate.config.global_rules
        : '',
    example:
      typeof builtinTemplate.config?.example === 'string'
        ? builtinTemplate.config.example
        : '',
    ...(typeof builtinTemplate.config?.synthesis === 'object' &&
    builtinTemplate.config?.synthesis !== null
      ? {
          synthesis: builtinTemplate.config
            .synthesis as TemplateSchemaType['config']['synthesis'],
        }
      : {}),
  };

  if (kind === CompilationTemplateKind.Tree) {
    const builtinRaptor: ICompilationTemplateRaptorConfig =
      builtinTemplate.config?.raptor ?? {};
    return {
      ...sections,
      raptor: {
        prompt: builtinRaptor.prompt ?? '',
        max_token: builtinRaptor.max_token ?? 512,
        threshold: builtinRaptor.threshold ?? 0.1,
        rechunk: builtinRaptor.rechunk ?? false,
      },
    };
  }

  Object.entries(builtinTemplate.config ?? {}).forEach(([key, value]) => {
    if (isConfigMetaKey(key)) return;
    sections[key] = normalizeSection(
      value as ICompilationTemplateSection,
    ) as TemplateSchemaType['config'][string];
  });

  return sections;
};

export const transformDetailToForm = (
  detail: ICompilationTemplate,
): TemplateSchemaType => {
  const config = detail.config ?? {};
  const base: TemplateSchemaType['config'] = {
    kind: config.kind ?? '',
    llm_id: config.llm_id ?? '',
    global_rules: config.global_rules ?? '',
    example: typeof config.example === 'string' ? config.example : '',
    ...(typeof config.synthesis === 'object' && config.synthesis !== null
      ? {
          synthesis:
            config.synthesis as TemplateSchemaType['config']['synthesis'],
        }
      : {}),
  };

  if (detail.kind === CompilationTemplateKind.Tree) {
    const raptor: ICompilationTemplateRaptorConfig = config.raptor ?? {};
    return {
      id: detail.id,
      name: detail.name ?? '',
      description: detail.description ?? '',
      llm_id: config.llm_id ?? '',
      kind: detail.kind ?? '',
      config: {
        ...base,
        raptor: {
          prompt: raptor.prompt ?? '',
          max_token: raptor.max_token ?? 512,
          threshold: raptor.threshold ?? 0.1,
          rechunk: raptor.rechunk ?? false,
        },
      },
    };
  }

  Object.entries(config).forEach(([key, value]) => {
    if (isConfigMetaKey(key)) return;
    base[key] = normalizeSection(
      value as ICompilationTemplateSection,
    ) as TemplateSchemaType['config'][string];
  });

  return {
    id: detail.id,
    name: detail.name ?? '',
    description: detail.description ?? '',
    llm_id: config.llm_id ?? '',
    kind: detail.kind ?? '',
    config: base,
  };
};

export const transformGroupDetailToForm = (
  detail: ICompilationTemplateGroup,
): FormSchemaType => {
  const templates = (detail.templates ?? []).map((template) =>
    transformDetailToForm(template),
  );

  return {
    name: detail.name ?? '',
    description: detail.description ?? '',
    avatar: detail.avatar ?? '',
    templates: templates.length > 0 ? templates : [DefaultTemplateValues],
  };
};

export const transformTemplateToPayload = (template: TemplateSchemaType) => {
  const config: ICompilationTemplateConfigRequest = {
    kind: template.kind,
    llm_id: template.llm_id,
  };

  Object.entries(template.config).forEach(([key, value]) => {
    if (key === 'kind' || key === 'llm_id') return;
    if (key === 'instruction' || key === 'page_example') return;
    if (key === 'synthesis') {
      config[key] = value as ICompilationTemplateConfigRequest[string];
      return;
    }
    if (isConfigMetaKey(key)) {
      if (typeof value === 'string') config[key] = value;
    } else {
      config[key] = value as ICompilationTemplateConfigRequest[string];
    }
  });

  if (template.kind === CompilationTemplateKind.Artifacts) {
    const instruction = String(template.config.instruction ?? '').trim();
    const pageExample = String(template.config.page_example ?? '').trim();
    config.example = [instruction, pageExample].filter(Boolean).join('\n\n');
  }

  return {
    id: template.id,
    name: template.name,
    description: template.description,
    kind: template.kind,
    config,
  };
};

export const transformFormToPayload = (values: FormSchemaType) => {
  return {
    name: values.name,
    description: values.description,
    avatar: values.avatar || undefined,
    templates: values.templates.map((template) =>
      transformTemplateToPayload(template),
    ),
  };
};

export const SectionTitleKeyMap: Record<string, string> = {
  entity: 'setting.entitySpecification',
  relation: 'setting.relationSpecification',
  concept: 'setting.conceptSpecification',
  claim: 'setting.claimSpecification',
};

export const SectionPriority = ['entity', 'relation'];

export const sortSectionNames = (names: string[]): string[] => {
  const priority = SectionPriority.filter((name) => names.includes(name));
  const rest = names.filter((name) => !SectionPriority.includes(name));
  return [...priority, ...rest];
};

export const FieldLabelKeyMap: Record<string, string> = {
  type: 'setting.fieldType',
  description: 'setting.fieldDescription',
  rule: 'setting.fieldRule',
};

export const getTypeOptionsFromBuiltinSection = (
  builtinSection?: ICompilationTemplateSection,
) => {
  const typeSet = new Set<string>();
  builtinSection?.fields?.forEach((field) => {
    if (field.type) typeSet.add(field.type);
  });
  return Array.from(typeSet)
    .sort()
    .map((value) => ({ label: value, value }));
};
