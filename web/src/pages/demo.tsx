import { zodResolver } from '@hookform/resolvers/zod';
import { Form, useForm } from 'react-hook-form';
import { z } from 'zod';

import { Button } from '@/components/ui/button';
import { useCallback, useState } from 'react';
import DynamicCategorize from './agent/form/categorize-form/dynamic-categorize';

const formSchema = z.object({
  items: z
    .array(
      z
        .object({
          name: z.string().min(1, 'xxx').trim(),
          description: z.string().optional(),
          // examples: z
          //   .array(
          //     z.object({
          //       value: z.string(),
          //     }),
          //   )
          //   .optional(),
        })
        .optional(),
    )
    .optional(),
});

export function Demo() {
  const [flag, setFlag] = useState(false);

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      items: [],
    },
  });

  const handleReset = useCallback(() => {
    form?.reset();
  }, [form]);

  const handleSwitch = useCallback(() => {
    setFlag(true);
  }, []);

  return (
    <div>
      <Form {...form}>
        <DynamicCategorize></DynamicCategorize>
      </Form>
      <Button onClick={handleReset}>reset</Button>
      <Button onClick={handleSwitch}>switch</Button>
    </div>
  );
}
