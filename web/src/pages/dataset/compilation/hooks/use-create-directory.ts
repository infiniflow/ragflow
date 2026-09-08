import { zodResolver } from '@hookform/resolvers/zod';
import { useCallback, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import type { CreateDirectoryFormValues } from '../interface';

export function useCreateDirectory() {
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);

  const FormSchema = useMemo(
    () =>
      z.object({
        name: z.string().min(1).trim(),
        rule: z.string().optional(),
      }),
    [],
  );

  const form = useForm<CreateDirectoryFormValues>({
    resolver: zodResolver(FormSchema),
    defaultValues: { name: '', rule: '' },
  });

  const showModal = useCallback(() => {
    form.reset({ name: '', rule: '' });
    setOpen(true);
  }, [form]);

  const hideModal = useCallback(() => {
    setOpen(false);
    form.reset({ name: '', rule: '' });
  }, [form]);

  const handleOk = useCallback(
    async (values: CreateDirectoryFormValues) => {
      setLoading(true);
      try {
        // TODO: Replace with real API call
        console.log('Create directory:', values);
        setOpen(false);
        form.reset({ name: '', rule: '' });
      } finally {
        setLoading(false);
      }
    },
    [form],
  );

  return {
    open,
    loading,
    form,
    showModal,
    hideModal,
    handleOk,
  };
}
