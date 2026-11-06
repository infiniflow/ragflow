import { FormContainer } from '@/components/form-container';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { useTranslate } from '@/hooks/common-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { z } from 'zod';
import { initialSearch1APICrawlValues } from '../../constant';
import { useFormValues } from '../../hooks/use-form-values';
import { useWatchFormChange } from '../../hooks/use-watch-form-change';
import { INextOperatorForm } from '../../interface';
import { buildOutputList } from '../../utils/build-output-list';
import { ApiKeyField } from '../components/api-key-field';
import { FormWrapper } from '../components/form-wrapper';
import { Output } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';

const outputList = buildOutputList(initialSearch1APICrawlValues.outputs);

const FormSchema = z.object({
  api_key: z.string(),
  url: z.string().min(1),
});

function Search1APICrawlForm({ node }: INextOperatorForm) {
  const { t } = useTranslate('flow');
  const values = useFormValues(initialSearch1APICrawlValues, node);
  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
  });

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <ApiKeyField placeholder={t('search1APIApiKeyTip')} />
          <FormField
            control={form.control}
            name="url"
            render={({ field }) => (
              <FormItem>
                <FormLabel tooltip={t('search1APICrawlUrlTip')}>
                  {t('search1APICrawlUrl')}
                </FormLabel>
                <FormControl>
                  <PromptEditor
                    {...field}
                    multiLine={false}
                    showToolbar={false}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList} />
      </div>
    </Form>
  );
}

export default memo(Search1APICrawlForm);
