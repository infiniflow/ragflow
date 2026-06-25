import {
  COMPILATION_TEMPLATE_KINDS,
  CompilationTemplateConfig,
  CompilationTemplateKind,
} from '@/interfaces/database/compilation-template';
import { z } from 'zod';

/**
 * Zod schema for the editor form. Mirrors {@link CompilationTemplateConfig}
 * one-for-one. RHF's zodResolver wires field-level error messages to the
 * matching input.
 *
 * Length caps follow the spec: descriptions / rules are 1024-character text,
 * type tokens are 64-character text. The cap on the number of repeater
 * entries is intentionally soft (32) — beyond that the LLM prompt becomes
 * unusable anyway.
 */
const TYPE_MAX = 64;
const TEXT_MAX = 1024;
const GLOBAL_RULES_MAX = 4096;
const FIELDS_MAX = 32;

const entityFieldSchema = z.object({
  type: z.string().trim().min(1, 'Type is required.').max(TYPE_MAX),
  description: z
    .string()
    .trim()
    .min(1, 'Field description is required.')
    .max(TEXT_MAX),
  rule: z.string().max(TEXT_MAX),
});

const relationFieldSchema = z.object({
  type: z.string().trim().min(1, 'Type is required.').max(TYPE_MAX),
  description: z
    .string()
    .trim()
    .min(1, 'Field description is required.')
    .max(TEXT_MAX),
  rule: z.string().max(TEXT_MAX),
});

const claimFieldSchema = z.object({
  statement: z.string().trim().min(1, 'Statement is required.').max(TEXT_MAX),
  subject: z.string().trim().min(1, 'Subject is required.').max(TEXT_MAX),
});

const conceptFieldSchema = z.object({
  term: z.string().trim().min(1, 'Term is required.').max(TEXT_MAX),
  definition_excerpt: z
    .string()
    .trim()
    .min(1, 'Definition excerpt is required.')
    .max(TEXT_MAX),
});

function addUniqueTypeIssue(
  ctx: z.RefinementCtx,
  fields: Array<{ type: string }>,
  pathPrefix: Array<string | number>,
) {
  const seen = new Map<string, number>();
  fields.forEach((field, index) => {
    const type = field.type.trim().toLowerCase();
    if (!type) {
      return;
    }
    const firstIndex = seen.get(type);
    if (firstIndex !== undefined) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: 'Type can not be duplicated.',
        path: [...pathPrefix, index, 'type'],
      });
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        message: 'Type can not be duplicated.',
        path: [...pathPrefix, firstIndex, 'type'],
      });
    } else {
      seen.set(type, index);
    }
  });
}

export const compilationTemplateFormSchema = z
  .object({
    name: z.string().trim().min(1, 'Template name is required.').max(128),
    description: z.string().max(TEXT_MAX).optional().default(''),
    kind: z.enum(
      COMPILATION_TEMPLATE_KINDS as readonly [
        CompilationTemplateKind,
        ...CompilationTemplateKind[],
      ],
    ),
    llm_id: z.string().trim().min(1, 'LLM is required.'),
    entity: z.object({
      description: z.string().max(TEXT_MAX),
      // ``min(1)`` enforced in superRefine for kinds that actually
      // render the entity/relation sections. ``tree`` hides them and
      // carries empty arrays — validating min length here would
      // silently block save with no visible error.
      fields: z.array(entityFieldSchema).max(FIELDS_MAX),
    }),
    relation: z.object({
      description: z.string().max(TEXT_MAX),
      fields: z.array(relationFieldSchema).max(FIELDS_MAX),
    }),
    claim: z
      .object({
        fields: z.array(claimFieldSchema).max(FIELDS_MAX),
      })
      .optional(),
    concept: z
      .object({
        fields: z.array(conceptFieldSchema).max(FIELDS_MAX),
      })
      .optional(),
    // Page-structure example for the REFINE writer (artifacts kind
    // only). Empty / whitespace-only saves as ``undefined`` so the
    // backend falls back to its built-in ARTIFACT_TEMPLATE_EXAMPLE.
    example: z.string().max(8000).optional(),
    raptor: z
      .object({
        prompt: z.string().max(TEXT_MAX),
        max_token: z.coerce.number().int().min(1).max(8192),
        threshold: z.coerce.number().min(0).max(1),
      })
      .optional(),
    global_rules: z.string().max(GLOBAL_RULES_MAX),
  })
  .superRefine((values, ctx) => {
    addUniqueTypeIssue(ctx, values.entity.fields, ['entity', 'fields']);
    addUniqueTypeIssue(ctx, values.relation.fields, ['relation', 'fields']);
    // Entity/relation length requirement is per-kind: tree-kind
    // templates hide those sections in the UI, so requiring at least
    // one field would block save with no visible error.
    if (values.kind !== 'tree') {
      if (values.entity.fields.length === 0) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: 'At least one entity field is required.',
          path: ['entity', 'fields'],
        });
      }
      if (values.relation.fields.length === 0) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: 'At least one relation field is required.',
          path: ['relation', 'fields'],
        });
      }
    }
    if (values.kind === 'artifacts') {
      if (!values.claim?.fields?.length) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: 'Claim specification is required.',
          path: ['claim', 'fields'],
        });
      }
      if (!values.concept?.fields?.length) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: 'Concept specification is required.',
          path: ['concept', 'fields'],
        });
      }
    }
    if (values.kind === 'tree') {
      if (!values.raptor?.prompt?.trim()) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: 'Summarization prompt is required.',
          path: ['raptor', 'prompt'],
        });
      }
    }
  });

export type CompilationTemplateFormValues = z.infer<
  typeof compilationTemplateFormSchema
>;

/**
 * Translate a server-side template into the form's value shape. Same
 * structure today, kept as a function so future divergence (e.g. flattening
 * for nicer RHF defaults) is one edit.
 */
export function templateConfigToFormValues(
  name: string,
  description: string | undefined,
  config: CompilationTemplateConfig,
): CompilationTemplateFormValues {
  return {
    name,
    description: description ?? '',
    kind: config.kind,
    // ``llm_id`` is required on save; when the stored config predates the
    // field, server-side lazy-fill (tenant default) usually populates it
    // before this code runs. Defensive '' fallback keeps the form
    // controlled.
    llm_id: (config as { llm_id?: string }).llm_id ?? '',
    // ``tree`` kind hides the entity/relation sections in the editor.
    // Seed empty arrays so the per-field validators (which require
    // non-empty type/description) never fire on stub rows the user
    // can't see.
    entity: {
      description: config.entity?.description ?? '',
      fields:
        config.kind === 'tree'
          ? []
          : config.entity?.fields?.length
            ? config.entity.fields.map((f) => ({
                type: f.type ?? '',
                description: f.description ?? '',
                rule: f.rule ?? '',
              }))
            : [{ type: '', description: '', rule: '' }],
    },
    relation: {
      description: config.relation?.description ?? '',
      fields:
        config.kind === 'tree'
          ? []
          : config.relation?.fields?.length
            ? config.relation.fields.map((f) => ({
                type: f.type ?? '',
                description: f.description ?? '',
                rule: f.rule ?? '',
              }))
            : [{ type: '', description: '', rule: '' }],
    },
    claim:
      config.kind === 'artifacts'
        ? {
            fields: config.claim?.fields?.map((f) => ({
              statement: f.statement ?? '',
              subject: f.subject ?? '',
            })) ?? [{ statement: '', subject: '' }],
          }
        : undefined,
    concept:
      config.kind === 'artifacts'
        ? {
            fields: config.concept?.fields?.map((f) => ({
              term: f.term ?? '',
              definition_excerpt: f.definition_excerpt ?? '',
            })) ?? [{ term: '', definition_excerpt: '' }],
          }
        : undefined,
    // Artifact page-structure override. Empty for non-artifact kinds;
    // the form gates its render the same way as claim/concept above.
    example:
      config.kind === 'artifacts'
        ? ((config as { example?: string }).example ?? '')
        : undefined,
    raptor:
      config.kind === 'tree'
        ? {
            // Defaults mirror RAPTOR's per-doc parse-config so a fresh
            // template doesn't ship empty fields when the user clicks
            // "Add template" → kind "tree" without picking a built-in.
            prompt:
              config.raptor?.prompt ??
              'Please write a concise summary of the following texts:\n{cluster_content}',
            max_token: config.raptor?.max_token ?? 512,
            threshold: config.raptor?.threshold ?? 0.1,
          }
        : undefined,
    global_rules: config.global_rules ?? '',
  };
}

export function formValuesToTemplateConfig(
  values: CompilationTemplateFormValues,
): CompilationTemplateConfig {
  return {
    kind: values.kind,
    llm_id: values.llm_id,
    entity: values.entity,
    relation: values.relation,
    claim: values.kind === 'artifacts' ? values.claim : undefined,
    concept: values.kind === 'artifacts' ? values.concept : undefined,
    // Trim → empty → undefined so an all-whitespace textarea means
    // "use backend default" rather than "send an empty override".
    example:
      values.kind === 'artifacts' && values.example?.trim()
        ? values.example.trim()
        : undefined,
    raptor: values.kind === 'tree' ? values.raptor : undefined,
    global_rules: values.global_rules,
  };
}

/**
 * Bootstrap an empty form when the user clicks "Add" without picking a
 * built-in template first. One blank entity field + one blank relation
 * field gives them a starting point for typing.
 */
export function emptyFormValues(): CompilationTemplateFormValues {
  return {
    name: '',
    description: '',
    kind: 'empty',
    // Seeded later by the form's defaultModelDict useEffect once the
    // /v1/models/default response lands.
    llm_id: '',
    entity: {
      description: '',
      fields: [{ type: '', description: '', rule: '' }],
    },
    relation: {
      description: '',
      fields: [{ type: '', description: '', rule: '' }],
    },
    claim: undefined,
    concept: undefined,
    example: undefined,
    global_rules: '',
  };
}

/**
 * Suggestion lists for the `type` selector inside the field repeater.
 * The combobox accepts free text too — these are just hints. The lists
 * are sourced from the spec on the original request.
 */
export const ENTITY_TYPE_SUGGESTIONS: Record<
  CompilationTemplateKind,
  string[]
> = {
  page_index: [],
  timeline: [],
  knowledge_graph: [],
  artifacts: [],
  tree: [],
  empty: [],
};

export const RELATION_TYPE_SUGGESTIONS: Record<
  CompilationTemplateKind,
  string[]
> = {
  page_index: [],
  timeline: [],
  knowledge_graph: [],
  artifacts: [],
  tree: [],
  empty: [],
};

export const ENTITY_TYPE_OPTIONS = [
  'title',
  'timestamp',
  'event',
  'person',
  'org',
  'product',
  'regulation',
  'location',
  'system',
  'equipment',
  'other',
] as const;

export const RELATION_TYPE_OPTIONS = [
  'include',
  'ordered',
  'owns',
  'part_of',
  'caused_by',
  'regulates',
  'uses',
  'located_in',
  'other',
] as const;

export type FieldTemplateValue = {
  description: string;
  rule: string;
};

export type FieldTemplateMap = Record<string, FieldTemplateValue>;

function mergeFieldTemplates(
  configs: CompilationTemplateConfig[],
  section: 'entity' | 'relation',
): FieldTemplateMap {
  return configs.reduce<FieldTemplateMap>((acc, config) => {
    config[section]?.fields?.forEach((field) => {
      if (field.type && !acc[field.type]) {
        acc[field.type] = {
          description: field.description ?? '',
          rule: field.rule ?? '',
        };
      }
    });
    return acc;
  }, {});
}

const FALLBACK_FIELD_TEMPLATE_CONFIGS: CompilationTemplateConfig[] = [
  {
    kind: 'page_index',
    entity: {
      description: '',
      fields: [
        {
          type: 'title',
          description: 'A concise page or section title.',
          rule: 'Use the source heading when available; otherwise generate a short descriptive title.',
        },
      ],
    },
    relation: {
      description: '',
      fields: [
        {
          type: 'include',
          description:
            'Containment relation between an index item and its covered content.',
          rule: 'Direction from parent item to included child item or page range.',
        },
      ],
    },
    global_rules: '',
  },
  {
    kind: 'timeline',
    entity: {
      description: '',
      fields: [
        {
          type: 'timestamp',
          description: 'Date or time expression for an event.',
          rule: 'Normalize when possible; preserve the original expression when uncertain.',
        },
        {
          type: 'event',
          description: 'A notable occurrence or milestone.',
          rule: 'Use a concise factual phrase tied to the timestamp.',
        },
      ],
    },
    relation: {
      description: '',
      fields: [
        {
          type: 'ordered',
          description: 'Chronological ordering between timeline events.',
          rule: 'Direction from earlier event to later event.',
        },
      ],
    },
    global_rules: '',
  },
  {
    kind: 'artifacts',
    entity: {
      description: '',
      fields: [
        {
          type: 'person',
          description: 'A natural person (individual human).',
          rule: '- Full name preferred (e.g., "Elon Musk", not "Musk" alone if ambiguous).\n- Include titles only if integral to identity (e.g., "Dr. Smith").\n- Max length: 60 characters.',
        },
        {
          type: 'org',
          description:
            'Organization, company, institution, agency, or any collective group.',
          rule: '- Use the official name when possible (e.g., "United Nations").\n- Abbreviations accepted if widely known (e.g., "UN", "NASA").\n- Max length: 80 characters.',
        },
        {
          type: 'product',
          description:
            'Tangible or intangible product, service, software, or offering.',
          rule: '- Include version numbers if relevant (e.g., "iPhone 14").\n- Generic categories only if no specific name is given.\n- Max length: 100 characters.',
        },
        {
          type: 'regulation',
          description:
            'Law, policy, standard, guideline, or regulatory document.',
          rule: '- Use official title or identifier (e.g., "GDPR", "OSHA standard 1910").\n- Include jurisdiction if known.\n- Max length: 120 characters.',
        },
        {
          type: 'location',
          description:
            'Geographic place (country, city, address, region, natural feature).',
          rule: '- Hierarchical format allowed (e.g., "Paris, France").\n- Avoid vague terms unless resolved.\n- Max length: 80 characters.',
        },
        {
          type: 'system',
          description:
            'Technical system, platform, framework, or infrastructure.',
          rule: '- Distinct from product: system implies integrated environment.\n- Use proper naming.\n- Max length: 100 characters.',
        },
        {
          type: 'equipment',
          description: 'Physical device, machinery, hardware, or tool.',
          rule: '- Specific model preferred.\n- Generic allowed only if precise type.\n- Max length: 80 characters.',
        },
        {
          type: 'other',
          description: 'Entities that do not fit any above category.',
          rule: '- Use sparingly; prefer mapping to a defined type when possible.\n- Still provide a meaningful label.\n- Max length: 80 characters.',
        },
      ],
    },
    relation: {
      description: '',
      fields: [
        {
          type: 'owns',
          description: 'Ownership or possession (legal or de facto).',
          rule: '- Direction from owner to owned: (A owns B).',
        },
        {
          type: 'part_of',
          description: 'Mereological relation - component to whole.',
          rule: '- Direction from part to whole: (A part_of B).',
        },
        {
          type: 'caused_by',
          description:
            'Causal relation - event, action, or state leads to another.',
          rule: '- Direction from effect to cause: (A caused_by B) meaning B causes A.',
        },
        {
          type: 'regulates',
          description:
            'Regulatory or governing relation (law/standard controls entity).',
          rule: '- Direction from regulator to regulated: (A regulates B).',
        },
        {
          type: 'uses',
          description:
            'Utilization - an entity employs or consumes another entity.',
          rule: '- Direction from user to used: (A uses B).',
        },
        {
          type: 'located_in',
          description:
            'Spatial containment - entity situated inside a location.',
          rule: '- Direction from located entity to containing location: (A located_in B).',
        },
        {
          type: 'other',
          description:
            'Any meaningful relation not covered by the above types.',
          rule: '- Provide an explicit label in a "relation_label" field.\n- Direction must be clear.',
        },
      ],
    },
    global_rules: '',
  },
];

export const FALLBACK_ENTITY_FIELD_TEMPLATES = mergeFieldTemplates(
  FALLBACK_FIELD_TEMPLATE_CONFIGS,
  'entity',
);

export const FALLBACK_RELATION_FIELD_TEMPLATES = mergeFieldTemplates(
  FALLBACK_FIELD_TEMPLATE_CONFIGS,
  'relation',
);

export function buildFieldTemplateMaps(configs: CompilationTemplateConfig[]): {
  entity: FieldTemplateMap;
  relation: FieldTemplateMap;
} {
  return {
    entity: {
      ...FALLBACK_ENTITY_FIELD_TEMPLATES,
      ...mergeFieldTemplates(configs, 'entity'),
    },
    relation: {
      ...FALLBACK_RELATION_FIELD_TEMPLATES,
      ...mergeFieldTemplates(configs, 'relation'),
    },
  };
}

export const TYPE_FIELD_MAX = TYPE_MAX;
export const TEXT_FIELD_MAX = TEXT_MAX;
export const GLOBAL_RULES_FIELD_MAX = GLOBAL_RULES_MAX;
