import { FormContainer } from '@/components/form-container';
import { TopNFormField } from '@/components/top-n-item';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { initialSearXNGValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { QueryVariable } from '../components/query-variable';

const FormSchema = z.object({
  query: z.string(),
  searxng_url: z.string().min(1),
  top_n: z.string(),
});

const outputList = buildOutputList(initialSearXNGValues.outputs);

function SearXNGForm({ node }: INextOperatorForm) {
  const { t } = useTranslate('flow');
  const defaultValues = useFormValues(initialSearXNGValues, node);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <QueryVariable></QueryVariable>
          <TopNFormField></TopNFormField>
          <FormField
            control={form.control}
            name="searxng_url"
            render={({ field }) => (
              <FormItem>
                <FormLabel>SearXNG URL</FormLabel>
                <FormControl>
                  <Input {...field} placeholder="http://localhost:4000" />
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

export default memo(SearXNGForm);
