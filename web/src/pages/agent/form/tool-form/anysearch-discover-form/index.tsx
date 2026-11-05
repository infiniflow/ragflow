import { FormContainer } from '@/components/form-container';
import { Form } from '@/components/ui/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { memo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { ApiKeyField } from '../../components/api-key-field';
import { FormWrapper } from '../../components/form-wrapper';
import { useValues } from '../use-values';
import { useWatchFormChange } from '../use-watch-change';

const FormSchema = z.object({
  api_key: z.string().optional(),
});

function AnySearchDiscoverForm() {
  const { t } = useTranslation();
  const values = useValues();

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  useWatchFormChange(form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <ApiKeyField placeholder={t('flow.anySearchApiKeyTip')}></ApiKeyField>
        </FormContainer>
      </FormWrapper>
    </Form>
  );
}

export default memo(AnySearchDiscoverForm);
