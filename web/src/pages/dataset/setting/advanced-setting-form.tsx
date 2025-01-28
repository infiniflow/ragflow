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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { FormSlider } from '@/components/ui/slider';
import { Textarea } from '@/components/ui/textarea';
import ChunkMethodCard from './chunk-method-card';

const formSchema = z.object({
  parser_id: z.string().min(1, {
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

export default function AdvancedSettingForm() {
  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      parser_id: '',
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
          name="a"
          render={({ field }) => (
            <FormItem className="w-2/5">
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
        <ChunkMethodCard></ChunkMethodCard>
        <FormField
          control={form.control}
          name="a"
          render={({ field }) => (
            <FormItem className="w-2/5">
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
            <FormItem className="w-2/5">
              <FormLabel>Username</FormLabel>
              <Select onValueChange={field.onChange} defaultValue={field.value}>
                <FormControl>
                  <SelectTrigger>
                    <SelectValue placeholder="Select a verified email to display" />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectItem value="m@example.com">m@example.com</SelectItem>
                  <SelectItem value="m@google.com">m@google.com</SelectItem>
                  <SelectItem value="m@support.com">m@support.com</SelectItem>
                </SelectContent>
              </Select>
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
            <FormItem className="w-2/5">
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
            <FormItem className="w-2/5">
              <FormLabel>Username</FormLabel>
              <FormControl>
                <Textarea {...field}></Textarea>
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
          className="w-2/5"
        >
          Test
        </Button>
      </form>
    </Form>
  );
}
