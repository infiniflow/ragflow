import {
  QueritFormSchema,
  deserializeQueritFormValues,
  serializeQueritFormValues,
} from '../../querit-form/utils';

export const QueritAgentFormSchema = QueritFormSchema.omit({ query: true });

export function deserializeQueritAgentFormValues(values: Record<string, any>) {
  return deserializeQueritFormValues(values);
}

export function validateQueritAgentFormValuesForPersistence(
  values: Record<string, any>,
) {
  const result = QueritAgentFormSchema.safeParse(values);
  if (!result.success) {
    return undefined;
  }

  return serializeQueritFormValues(result.data);
}
