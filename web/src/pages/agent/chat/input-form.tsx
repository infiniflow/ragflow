'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { toast } from 'sonner';
import { z } from 'zod';

import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Message } from '@/interfaces/database/chat';
import { get } from 'lodash';
import { useParams } from 'umi';
import { useSendNextMessage } from './hooks';

const FormSchema = z.object({
  username: z.string().min(2, {
    message: 'Username must be at least 2 characters.',
  }),
});

type InputFormProps = Pick<ReturnType<typeof useSendNextMessage>, 'send'> & {
  message: Message;
};

export function InputForm({ send, message }: InputFormProps) {
  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      username: '',
    },
  });

  const { id: canvasId } = useParams();

  function onSubmit(data: z.infer<typeof FormSchema>) {
    const inputs = get(message, 'data.inputs', {});

    const nextInputs = Object.entries(inputs).reduce((pre, [key, val]) => {
      pre[key] = { ...val, value: data.username };

      return pre;
    }, {});

    send({
      inputs: nextInputs,
      id: canvasId,
    });

    toast('You submitted the following values', {
      description: (
        <pre className="mt-2 w-[320px] rounded-md bg-neutral-950 p-4">
          <code className="text-white">{JSON.stringify(data, null, 2)}</code>
        </pre>
      ),
    });
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="w-2/3 space-y-6">
        <FormField
          control={form.control}
          name="username"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Username</FormLabel>
              <FormControl>
                <Input placeholder="shadcn" {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <Button type="submit">Submit</Button>
      </form>
    </Form>
  );
}
