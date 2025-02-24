'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { Button } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { RAGFlowSelect } from '@/components/ui/select';
import { FormSlider } from '@/components/ui/slider';
import { Textarea } from '@/components/ui/textarea';

const options = [
  { label: 'xx', value: 'xx' },
  { label: 'ii', value: 'ii' },
];

const groupOptions = [
  { label: 'scsdv', options },
  { label: 'thtyu', options: [{ label: 'jj', value: 'jj' }] },
];

const formSchema = z.object({
  username: z.number().min(2, {
    message: 'Username must be at least 2 characters.',
  }),
  a: z.number().min(2, {
    message: 'Username must be at least 2 characters.',
  }),
  b: z.string().min(2, {
    message: 'Username must be at least 2 characters.',
  }),
  c: z.number().min(2, {
    message: 'Username must be at least 2 characters.',
  }),
  d: z.string().min(2, {
    message: 'Username must be at least 2 characters.',
  }),
});

export default function TestingForm() {
  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      username: 0,
    },
  });

  function onSubmit(values: z.infer<typeof formSchema>) {
    console.log(values);
  }

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-8">
        <FormField
          control={form.control}
          name="username"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Username</FormLabel>
              <FormControl>
                <FormSlider {...field}></FormSlider>
              </FormControl>
              <FormDescription>
                This is your public display name.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="a"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Username</FormLabel>
              <FormControl>
                <FormSlider {...field}></FormSlider>
              </FormControl>
              <FormDescription>
                This is your public display name.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="b"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Username</FormLabel>
              <RAGFlowSelect
                value={field.value}
                onChange={field.onChange}
                FormControlComponent={FormControl}
                options={groupOptions}
              ></RAGFlowSelect>
              <FormDescription>
                This is your public display name.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="c"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Username</FormLabel>
              <FormControl>
                <FormSlider {...field}></FormSlider>
              </FormControl>
              <FormDescription>
                This is your public display name.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name="d"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Username</FormLabel>
              <FormControl>
                <Textarea
                  {...field}
                  className="bg-colors-background-inverse-weak"
                ></Textarea>
              </FormControl>
              <FormDescription>
                This is your public display name.
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
        <Button
          variant={'tertiary'}
          size={'sm'}
          type="submit"
          className="w-full"
        >
          Test
        </Button>
      </form>
    </Form>
  );
}
