import { FormContainer } from '@/components/form-container';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { Output, OutputType } from '../components/output';
import { QueryVariable } from '../components/query-variable';
import { DynamicDomain } from './dynamic-domain';
import { SearchDepth, Topic, defaultValues, useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

const TavilyForm = () => {
  const values = useValues();

  const FormSchema = z.object({
    api_key: z.string(),
    query: z.string(),
    search_depth: z.enum([SearchDepth.Advanced, SearchDepth.Basic]),
    topic: z.enum([Topic.News, Topic.General]),
    max_results: z.coerce.number(),
    days: z.coerce.number(),
    include_answer: z.boolean(),
    include_raw_content: z.boolean(),
    include_images: z.boolean(),
    include_image_descriptions: z.boolean(),
    include_domains: z.array(z.object({ value: z.any() })), // TODO: z.string should be used, but an error will be reported
    exclude_domains: z.array(z.object({ value: z.any() })),
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  const outputList = useMemo(() => {
    return Object.entries(defaultValues.outputs).reduce<OutputType[]>(
      (pre, [key, val]) => {
        pre.push({ title: key, type: val.type });
        return pre;
      },
      [],
    );
  }, []);

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <form
        className="space-y-5 px-5 "
        autoComplete="off"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormContainer>
          <FormField
            control={form.control}
            name="api_key"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Api Key</FormLabel>
                <FormControl>
                  <Input type="password" {...field}></Input>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </FormContainer>
        <FormContainer>
          <QueryVariable></QueryVariable>
          <FormField
            control={form.control}
            name="search_depth"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Search Depth</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    placeholder="shadcn"
                    {...field}
                    options={buildOptions(SearchDepth)}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="topic"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Topic</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    placeholder="shadcn"
                    {...field}
                    options={buildOptions(Topic)}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="max_results"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Max Results</FormLabel>
                <FormControl>
                  <Input type={'number'} {...field}></Input>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="days"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Days</FormLabel>
                <FormControl>
                  <Input type={'number'} {...field}></Input>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="include_answer"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Include Answer</FormLabel>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  ></Switch>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="include_raw_content"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Include Raw Content</FormLabel>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  ></Switch>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="include_images"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Include Images</FormLabel>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  ></Switch>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="include_image_descriptions"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Include Image Descriptions</FormLabel>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  ></Switch>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <DynamicDomain
            name="include_domains"
            label={'Include Domains'}
          ></DynamicDomain>
          <DynamicDomain
            name="exclude_domains"
            label={'Exclude Domains'}
          ></DynamicDomain>
        </FormContainer>
      </form>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};

export default TavilyForm;
