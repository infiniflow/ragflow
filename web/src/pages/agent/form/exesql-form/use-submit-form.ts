import { useTestDbConnect } from '@/hooks/use-agent-request';
import { useCallback } from 'react';
import { z } from 'zod';

export const ExeSQLFormSchema = {
  db_type: z.string().min(1),
  google_application_credentials_source: z
    .enum(['adc', 'json'])
    .optional()
    .default('json'),
  database: z.string(),
  username: z.string(),
  host: z.string(),
  port: z.number(),
  password: z.string().optional().or(z.literal('')),
  service_account_credentials_json: z.string().optional().or(z.literal('')),
  max_records: z.number(),
};

export const FormSchema = z
  .object({
    sql: z.string().optional(),
    ...ExeSQLFormSchema,
  })
  .superRefine((v, ctx) => {
    if (v.db_type === 'BigQuery') {
      const credentialSource =
        v.google_application_credentials_source?.toLowerCase() ?? 'json';
      if (credentialSource === 'adc') {
        return;
      }

      if (
        !(
          v.google_application_credentials_json &&
          v.google_application_credentials_json.trim().length > 0
        )
      ) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['google_application_credentials_json'],
          message: 'String must contain at least 1 character(s)',
        });
      } else {
        try {
          JSON.parse(v.google_application_credentials_json);
        } catch {
          ctx.addIssue({
            code: z.ZodIssueCode.custom,
            path: ['google_application_credentials_json'],
            message: 'Invalid JSON file content',
          });
        }
      }
      return;
    }

    if (!v.database.trim().length) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['database'],
        message: 'String must contain at least 1 character(s)',
      });
    }
    if (!v.username.trim().length) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['username'],
        message: 'String must contain at least 1 character(s)',
      });
    }
    if (!v.host.trim().length) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['host'],
        message: 'String must contain at least 1 character(s)',
      });
    }

    if (
      v.db_type !== 'trino' &&
      !(v.password && v.password.trim().length > 0)
    ) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['password'],
        message: 'String must contain at least 1 character(s)',
      });
    }
  });

export function useSubmitForm() {
  const { testDbConnect, loading } = useTestDbConnect();

  const onSubmit = useCallback(
    async (data: z.infer<typeof FormSchema>) => {
      testDbConnect(data);
    },
    [testDbConnect],
  );

  return { loading, onSubmit };
}
