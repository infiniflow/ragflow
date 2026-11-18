import i18n from '@/locales/config';
import { z } from 'zod';
import {
  VariableAssignerLogicalArrayOperator,
  VariableAssignerLogicalOperator,
} from '../../constant';

// Operators that consume no parameter: the form hides the parameter input
// for them (renderParameter in dynamic-variables.tsx), so they are exempt
// from the required check.
export const ParameterlessOperators: string[] = [
  VariableAssignerLogicalOperator.Clear,
  VariableAssignerLogicalArrayOperator.RemoveFirst,
  VariableAssignerLogicalArrayOperator.RemoveLast,
];

// The parameter control switches with the variable type and operator: a
// variable-ref string (overwrite/append/extend), free text or a JSON string
// (set), a number-input string (arithmetic), or an EmptyValueMap default
// (0 / 'yes' / '{}' / []). Persisted DSLs may carry actual numbers, booleans,
// arrays, objects or null, so all of those parse; emptiness is judged by the
// refine below.
const ParameterSchema = z
  .union([
    z.string(),
    z.number(),
    z.boolean(),
    z.array(z.any()),
    z.record(z.any()),
    z.null(),
  ])
  .optional();

// 0, false and '{}' are legitimate parameter values; empty means "nothing
// entered" — unset, null, '' or the empty-array default an array-typed
// variable gets before a ref is picked.
function isEmptyParameter(value: unknown): boolean {
  return (
    value === undefined ||
    value === null ||
    value === '' ||
    (Array.isArray(value) && value.length === 0)
  );
}

// All emptiness checks live in the refine rather than on the fields: an
// object-level superRefine is skipped when the base shape fails, and a
// field-level check cannot see the sibling operator that decides whether
// parameter is required at all. Reporting every empty field of a row at
// once is also the better editing experience.
const VariableItemSchema = z
  .object({
    variable: z.string().optional(),
    operator: z.string().optional(),
    parameter: ParameterSchema,
  })
  .superRefine((item, ctx) => {
    if (!item.variable) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['variable'],
        message: i18n.t('flow.variableAssignerVariableMissing'),
      });
    }
    if (!item.operator) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['operator'],
        message: i18n.t('flow.variableAssignerOperatorMissing'),
      });
    }
    if (
      item.operator &&
      !ParameterlessOperators.includes(item.operator) &&
      isEmptyParameter(item.parameter)
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['parameter'],
        message: i18n.t('flow.variableAssignerParameterMissing'),
      });
    }
  });

export const FormSchema = z.object({
  variables: z.array(VariableItemSchema),
});

export type VariableAssignerFormSchemaType = z.infer<typeof FormSchema>;
