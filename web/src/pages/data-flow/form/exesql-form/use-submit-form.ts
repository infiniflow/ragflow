import { useTestDbConnect } from '@/hooks/use-agent-request';
import { useCallback } from 'react';
import { z } from 'zod';

export const ExeSQLFormSchema = {
  db_type: z.string().min(1),
  database: z.string().min(1),
  username: z.string().min(1),
  host: z.string().min(1),
  port: z.number(),
  password: z.string().min(1),
  max_records: z.number(),
};

export const FormSchema = z.object({
  sql: z.string().optional(),
  ...ExeSQLFormSchema,
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
