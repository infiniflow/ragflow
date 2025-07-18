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
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import {
  TavilyExtractDepth,
  TavilyExtractFormat,
  initialTavilyExtractValues,
} from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { TavilyApiKeyField, TavilyFormSchema } from '../tavily-form';

const outputList = buildOutputList(initialTavilyExtractValues.outputs);

function TavilyExtractForm({ node }: INextOperatorForm) {
  const values = useFormValues(initialTavilyExtractValues, node);

  const FormSchema = z.object({
    ...TavilyFormSchema,
    urls: z.string(),
    extract_depth: z.enum([
      TavilyExtractDepth.Advanced,
      TavilyExtractDepth.Basic,
    ]),
    format: z.enum([TavilyExtractFormat.Text, TavilyExtractFormat.Markdown]),
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <TavilyApiKeyField></TavilyApiKeyField>
        </FormContainer>
        <FormContainer>
          <FormField
            control={form.control}
            name="urls"
            render={({ field }) => (
              <FormItem>
                <FormLabel>URL</FormLabel>
                <FormControl>
                  <Input {...field} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="extract_depth"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Extract Depth</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    placeholder="shadcn"
                    {...field}
                    options={buildOptions(TavilyExtractDepth)}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="format"
            render={({ field }) => (
              <FormItem>
                <FormLabel>Format</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    placeholder="shadcn"
                    {...field}
                    options={buildOptions(TavilyExtractFormat)}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
}

export default memo(TavilyExtractForm);
