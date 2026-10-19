import { useTranslation } from 'react-i18next';
import { z } from 'zod';

export function useCreateScoredSwitchFormSchema() {
  const { t } = useTranslation();

  return z.object({
    query: z.string().optional(),
    image: z.string().optional(),
    llm_id: z.string().optional(),
    llm_filter: z.string().optional(),
    instructions: z.string().optional(),
    request_format: z.enum(['content_parts', 'assets']),
    escalate_enabled: z.boolean(),
    margin_threshold: z.coerce.number().min(0).max(1),
    min_top_score: z.coerce.number().min(0).max(1),
    items: z
      .array(
        z.object({
          name: z.string().min(1, t('flow.nameRequiredMsg')).trim(),
          description: z.string().optional(),
          uuid: z.string(),
        }),
      )
      .min(2, t('flow.scoredSwitchOptionMinimum'))
      .superRefine((items, ctx) => {
        const seen = new Set<string>();
        items.forEach((item, index) => {
          const name = item.name.trim();
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
        z
          .object({
            id: z.string(),
            type: z.enum(['noul', 'score', 'box']),
            instructions: z.string().trim().min(1, t('flow.scoredSwitchQuestionRequired')),
            true_text: z.string().optional(),
            false_text: z.string().optional(),
            levels: z.array(z.string()).optional(),
          })
          .superRefine((task, ctx) => {
            if (task.type !== 'score') {
              return;
            }
            const levels = (task.levels ?? []).map((level) => level.trim()).filter(Boolean);
            if (levels.length < 2) {
              ctx.addIssue({
                code: z.ZodIssueCode.custom,
                message: t('flow.scoredSwitchLevelMinimum'),
                path: ['levels'],
              });
            }
          }),
      )
      .optional(),
  });
}
