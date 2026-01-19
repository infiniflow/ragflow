'use client';

import { z } from 'zod';

import { RAGFlowFormItem } from '@/components/ragflow-form';
import { ButtonLoading } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { FileUploadDirectUpload } from '@/pages/agent/debug-content/uploader';
import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';

const formSchema = z.object({
  file: z.record(z.any()),
});

export type FormSchemaType = z.infer<typeof formSchema>;

type UploaderFormProps = {
  ok: (values: FormSchemaType) => void;
  loading: boolean;
};

export function UploaderForm({ ok, loading }: UploaderFormProps) {
  const { t } = useTranslation();
  const form = useForm<FormSchemaType>({
    resolver: zodResolver(formSchema),
    defaultValues: {},
  });

  return (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(ok)} className="space-y-8">
        <RAGFlowFormItem name="file">
          {(field) => {
            return (
              <FileUploadDirectUpload
                value={field.value}
                onChange={field.onChange}
              ></FileUploadDirectUpload>
            );
          }}
        </RAGFlowFormItem>

        <div>
          <ButtonLoading
            type="submit"
            loading={loading}
            className="w-full mt-1"
          >
            {t('flow.run')}
          </ButtonLoading>
        </div>
      </form>
    </Form>
  );
}
