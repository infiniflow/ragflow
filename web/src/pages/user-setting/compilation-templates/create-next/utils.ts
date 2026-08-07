import { humanId } from 'human-id';
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
import {
  DefaultFieldKeys,
  DefaultTemplateValues,
  FieldKeyOrders,
  SectionPriority,
} from './constant';

export const getFieldKeyOrder = (keys: string[]): string[] => {
  const sortedKeys = [...keys].sort();
  return (
    FieldKeyOrders.find((order) => isEqual([...order].sort(), sortedKeys)) ??
    keys
  );
};

export const generateTemplateName = () =>
  humanId({ separator: ' ', capitalize: true });

export const isConfigMetaKey = (key: string) =>
  [
    'kind',
    'llm_id',
    'global_rules',
    'example',
    'instruction',
    'synthesis',
    'use_blueprint',
    'plan',
    'rechunk',
    'rechunk_rules',
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
  const instruction =
    typeof builtinTemplate.config?.instruction === 'string'
      ? builtinTemplate.config.instruction
      : '';
  const example =
    typeof builtinTemplate.config?.example === 'string'
      ? builtinTemplate.config.example
      : '';
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
    use_blueprint:
      kind === CompilationTemplateKind.Artifacts &&
      (instruction.length > 0 || example.length > 0),
    plan:
      typeof builtinTemplate.config?.plan === 'boolean'
        ? builtinTemplate.config.plan
        : true,
    ...(kind !== CompilationTemplateKind.Tree
      ? {
          rechunk: builtinTemplate.config?.rechunk === true,
          rechunk_rules:
            typeof builtinTemplate.config?.rechunk_rules === 'string'
              ? builtinTemplate.config.rechunk_rules
              : '',
        }
      : {}),
  };

  if (
    kind === CompilationTemplateKind.Artifacts &&
    (instruction.length > 0 || example.length > 0)
  ) {
    sections.instruction = instruction;
    sections.example = example;
  }

  if (kind === CompilationTemplateKind.Tree) {
    const builtinRaptor: ICompilationTemplateRaptorConfig =
      builtinTemplate.config?.raptor ?? {};
    return {
      ...sections,
      raptor: {
        prompt: builtinRaptor.prompt ?? '',
        max_token: builtinRaptor.max_token ?? 512,
        clustering_threshold: builtinRaptor.clustering_threshold ?? 0.3,
        clustering_ratio: builtinRaptor.clustering_ratio ?? 0.5,
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
  const storedInstruction =
    typeof config.instruction === 'string' ? config.instruction : '';
  const storedExample =
    typeof config.example === 'string' ? config.example : '';
  const hasBlueprintContent = Boolean(
    storedInstruction.trim() || storedExample.trim(),
  );
  const base: TemplateSchemaType['config'] = {
    kind: config.kind ?? '',
    llm_id: config.llm_id ?? '',
    global_rules: config.global_rules ?? '',
    example: storedExample,
    ...(typeof config.synthesis === 'object' && config.synthesis !== null
      ? {
          synthesis:
            config.synthesis as TemplateSchemaType['config']['synthesis'],
        }
      : {}),
    use_blueprint:
      detail.kind === CompilationTemplateKind.Artifacts && hasBlueprintContent,
    plan: typeof config.plan === 'boolean' ? config.plan : true,
    ...(detail.kind !== CompilationTemplateKind.Tree
      ? {
          rechunk: config.rechunk === true,
          rechunk_rules:
            typeof config.rechunk_rules === 'string'
              ? config.rechunk_rules
              : '',
        }
      : {}),
  };

  if (
    detail.kind === CompilationTemplateKind.Artifacts &&
    hasBlueprintContent
  ) {
    base.instruction = storedInstruction;
    base.example = storedExample;
  }

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
          clustering_threshold: raptor.clustering_threshold ?? 0.3,
          clustering_ratio: raptor.clustering_ratio ?? 0.5,
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
    if (key === 'example' || key === 'instruction') return;
    if (key === 'synthesis') {
      config[key] = value as ICompilationTemplateConfigRequest[string];
      return;
    }
    if (key === 'plan') {
      config[key] = value as ICompilationTemplateConfigRequest[string];
      return;
    }
    if (isConfigMetaKey(key)) {
      if (typeof value === 'string' || typeof value === 'boolean')
        config[key] = value;
    } else {
      config[key] = value as ICompilationTemplateConfigRequest[string];
    }
  });

  if (template.kind === CompilationTemplateKind.Artifacts) {
    if (template.config.use_blueprint) {
      const instruction = String(template.config.instruction ?? '').trim();
      const example = String(template.config.example ?? '').trim();
      config.instruction = instruction;
      config.example = example;
    } else {
      config.instruction = '';
      config.example = '';
    }
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

export const sortSectionNames = (names: string[]): string[] => {
  const priority = SectionPriority.filter((name) => names.includes(name));
  const rest = names.filter((name) => !SectionPriority.includes(name));
  return [...priority, ...rest];
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

export const getRequiredFieldKeys = (fieldKeys: string[]) =>
  fieldKeys.includes('type') ? ['type', 'description'] : fieldKeys;

export const getAvailableTypeOptions = (
  builtinSection: ICompilationTemplateSection | undefined,
  existingFields: Record<string, string>[] | undefined,
  editingType?: string,
) => {
  const usedTypes = new Set(
    (existingFields ?? []).map((field) => field.type).filter(Boolean),
  );
  if (editingType) usedTypes.delete(editingType);
  return getTypeOptionsFromBuiltinSection(builtinSection).filter(
    (option) => !usedTypes.has(option.value),
  );
};
