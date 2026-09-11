import { useTestDbConnect } from '@/hooks/use-agent-request';
import { useCallback } from 'react';
import { z } from 'zod';

// The backend only checks for a positive integer, so an unbounded value lets a
// single query pull an entire table into memory. Cap it an order of magnitude
// above the 1024 default.
export const MinMaxRecords = 1;
export const MaxMaxRecords = 10000;

// TCP port range.
export const MinPort = 1;
export const MaxPort = 65535;

export const ExeSQLFormSchema = {
  db_type: z.string().min(1),
  database: z.string().min(1),
  username: z.string().min(1),
  host: z.string().min(1),
  port: z.number().int().min(MinPort).max(MaxPort),
  password: z.string().optional().or(z.literal('')),
  max_records: z.number().int().min(MinMaxRecords).max(MaxMaxRecords),
};

export const FormSchema = z
  .object({
    sql: z.string().optional(),
    ...ExeSQLFormSchema,
  })
  .superRefine((v, ctx) => {
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
