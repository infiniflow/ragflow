import { z } from 'zod';

export const QueritTimeRangePattern =
  /^([dwmy][1-9][0-9]*|\d{4}-\d{2}-\d{2}to\d{4}-\d{2}-\d{2})$/;

const dynamicStringListSchema = z.array(
  z.object({
    value: z.string().trim().min(1),
  }),
);

export const QueritFormSchema = z.object({
  api_key: z.string(),
  query: z.string().trim().min(1),
  count: z.coerce.number().int().min(1),
  chunks_per_doc: z.coerce.number().int().min(1).max(3),
  site_include: dynamicStringListSchema,
  site_exclude: dynamicStringListSchema,
  time_range: z
    .string()
    .refine((value) => value === '' || QueritTimeRangePattern.test(value)),
  country_include: dynamicStringListSchema,
  language_include: dynamicStringListSchema,
});

type QueritFormRecord = Record<string, any>;

function toObjectArray(values: unknown) {
  return Array.isArray(values) ? values.map((value) => ({ value })) : [];
}

function toStringArray(values: unknown) {
  return Array.isArray(values)
    ? values.map((item) => item?.value as string)
    : [];
}

export function deserializeQueritFormValues(values: QueritFormRecord) {
  return {
    ...values,
    site_include: toObjectArray(values.site_include),
    site_exclude: toObjectArray(values.site_exclude),
    country_include: toObjectArray(values.country_include),
    language_include: toObjectArray(values.language_include),
  };
}

export function serializeQueritFormValues(values: QueritFormRecord) {
  return {
    ...values,
    count: Number(values.count),
    chunks_per_doc: Number(values.chunks_per_doc),
    site_include: toStringArray(values.site_include),
    site_exclude: toStringArray(values.site_exclude),
    country_include: toStringArray(values.country_include),
    language_include: toStringArray(values.language_include),
  };
}

export function validateQueritFormValuesForPersistence(
  values: QueritFormRecord,
) {
  const result = QueritFormSchema.safeParse(values);
  if (!result.success) {
    return undefined;
  }

  return serializeQueritFormValues({
    ...values,
    ...result.data,
  });
}
