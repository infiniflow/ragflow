import { useTranslation } from 'react-i18next';
import { z } from 'zod';

const fieldNamePattern = /^[A-Za-z_][A-Za-z0-9_.-]{0,63}$/;
const questionTypes = new Set(['noul', 'score', 'box']);
const fieldTypes = new Set([
  'text',
  'number',
  'date',
  'time',
  'datetime',
  'duration',
  'list',
  'estimate',
  'forecast',
]);
const listItems = new Set(['text', 'number', 'date']);
const forecastIntervals = new Set([
  'step',
  'minute',
  'hour',
  'day',
  'week',
  'month',
  'quarter',
  'year',
]);

export function useCreateScoredSwitchFormSchema() {
  const { t } = useTranslation();

  return z
    .object({
      query: z.string().optional(),
      image: z.string().optional(),
      llm_id: z.string().optional(),
      llm_filter: z.string().optional(),
      instructions: z.string().optional(),
      request_format: z.enum(['content_parts', 'assets']),
      escalate_enabled: z.boolean(),
      margin_threshold: z.coerce.number().min(0).max(1),
      min_top_score: z.coerce.number().min(0).max(1),
      field_mode: z.boolean().optional(),
      reference_date: z.string().optional(),
      include_quantiles: z.boolean().optional(),
      return_state_text: z.boolean().optional(),
      items: z
        .array(
          z.object({
            name: z.string().trim(),
            description: z.string().optional(),
            uuid: z.string(),
          }),
        )
        .superRefine((items, ctx) => {
          const seen = new Set<string>();
          items.forEach((item, index) => {
            const name = item.name.trim();
            if (!name) {
              return;
            }
            if (seen.has(name)) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: t('flow.nameRepeatedMsg'),
                path: [index, 'name'],
              });
            }
            seen.add(name);
          });
        }),
      tasks: z
        .array(
          z.object({
            _key: z.string().optional(),
            id: z.string(),
            type: z.enum([
              'noul',
              'score',
              'box',
              'text',
              'number',
              'date',
              'time',
              'datetime',
              'duration',
              'list',
              'estimate',
              'forecast',
            ]),
            instructions: z.string().optional(),
            true_text: z.string().optional(),
            false_text: z.string().optional(),
            levels: z.array(z.string()).optional(),
            unit: z.string().optional(),
            items: z.string().optional(),
            integer: z.boolean().optional(),
            decimals: z.union([z.literal(''), z.number()]).optional(),
            min: z.string().optional(),
            max: z.string().optional(),
            step: z.string().optional(),
            scale: z.string().optional(),
            horizon: z.union([z.literal(''), z.number()]).optional(),
            interval: z.string().optional(),
          }),
        )
        .optional(),
    })
    .superRefine((value, ctx) => {
      const fieldMode = value.field_mode === true;
      if (!fieldMode) {
        const named = value.items.filter((item) => item.name.trim() !== '');
        if (named.length < 2) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('flow.scoredSwitchOptionMinimum'),
            path: ['items'],
          });
        }
        value.items.forEach((item, index) => {
          if (!item.name.trim()) {
            ctx.addIssue({
              code: z.ZodIssueCode.custom,
              message: t('flow.nameRequiredMsg'),
              path: ['items', index, 'name'],
            });
          }
        });
      }
      const tasks = value.tasks ?? [];
      const seenFieldNames = new Set<string>();
      if (fieldMode && !tasks.some((task) => fieldTypes.has(task.type))) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: t('flow.scoredSwitchFieldMinimum'),
          path: ['tasks'],
        });
      }
      tasks.forEach((task, index) => {
        if (!fieldMode && questionTypes.has(task.type)) {
          if (!task.instructions?.trim()) {
            ctx.addIssue({
              code: z.ZodIssueCode.custom,
              message: t('flow.scoredSwitchQuestionRequired'),
              path: ['tasks', index, 'instructions'],
            });
          }
          if (task.type === 'score') {
            const levels = (task.levels ?? [])
              .map((level) => level.trim())
              .filter(Boolean);
            if (levels.length < 2) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: t('flow.scoredSwitchLevelMinimum'),
                path: ['tasks', index, 'levels'],
              });
            }
          }
        }
        if (!fieldMode || !fieldTypes.has(task.type)) {
          return;
        }
        const fieldName = task.id.trim();
        if (fieldName && seenFieldNames.has(fieldName)) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('flow.nameRepeatedMsg'),
            path: ['tasks', index, 'id'],
          });
        }
        if (fieldName) {
          seenFieldNames.add(fieldName);
        }
        if (!fieldNamePattern.test(fieldName)) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('flow.scoredSwitchFieldNameInvalid'),
            path: ['tasks', index, 'id'],
          });
        }
        if (task.type === 'list' && !listItems.has(task.items ?? '')) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('flow.scoredSwitchListItemsRequired'),
            path: ['tasks', index, 'items'],
          });
        }
        if (
          (task.type === 'estimate' || task.type === 'forecast') &&
          (!task.min?.trim() || !task.max?.trim())
        ) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('flow.scoredSwitchEstimateBoundsRequired'),
            path: ['tasks', index, 'min'],
          });
        }
        if (task.type === 'forecast') {
          const horizon =
            typeof task.horizon === 'number' ? task.horizon : Number.NaN;
          if (
            !forecastIntervals.has(task.interval ?? '') ||
            horizon < 1 ||
            horizon > 64
          ) {
            ctx.addIssue({
              code: z.ZodIssueCode.custom,
              message: t('flow.scoredSwitchForecastRequired'),
              path: ['tasks', index, 'horizon'],
            });
          }
        }
        if (
          task.decimals !== '' &&
          task.decimals !== undefined &&
          (task.decimals < 0 || task.decimals > 6)
        ) {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            message: t('flow.scoredSwitchDecimalsRange'),
            path: ['tasks', index, 'decimals'],
          });
        }
      });
    });
}
