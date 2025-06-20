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
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { QueryVariable } from '../components/query-variable';
import { SearchDepth, Topic, useValues } from './use-values';
import { useWatchFormChange } from './use-watch-change';

const TavilyForm = () => {
  const values = useValues();

  const FormSchema = z.object({
    query: z.string(),
    search_depth: z.enum([SearchDepth.Advanced, SearchDepth.Basic]),
    topic: z.enum([Topic.News, Topic.General]),
    max_results: z.coerce.number(),
    days: z.coerce.number(),
    include_answer: z.boolean(),
    include_raw_content: z.boolean(),
    include_images: z.boolean(),
    include_image_descriptions: z.boolean(),
    include_domains: z.array(z.string()),
    exclude_domains: z.array(z.string()),
  });

  const form = useForm({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

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
        </FormContainer>
      </form>
    </Form>
  );
};

export default TavilyForm;
