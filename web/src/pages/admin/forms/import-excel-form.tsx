import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { zodResolver } from '@hookform/resolvers/zod';
import { useCallback, useId, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { Trans, useTranslation } from 'react-i18next';
import { z } from 'zod';

export interface ImportExcelFormData {
  file: File;
  overwriteExisting: boolean;
}

interface ImportExcelFormProps {
  id: string;
  form: ReturnType<typeof useForm<ImportExcelFormData>>;
  onSubmit?: (data: ImportExcelFormData) => void;
}

export const ImportExcelForm = ({
  id,
  form,
  onSubmit = () => {},
}: ImportExcelFormProps) => {
  const { t } = useTranslation();

  return (
    <Form {...form}>
      <form
        id={id}
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
      >
        {/* File input field */}
        <FormField
          control={form.control}
          name="file"
          // eslint-disable-next-line @typescript-eslint/no-unused-vars
          render={({ field: { onChange, value, ...field } }) => (
            <FormItem>
              <FormLabel className="text-sm font-medium">
                {t('admin.importSelectExcelFile')}
              </FormLabel>

              <FormControl>
                <Input
                  type="file"
                  accept=".xlsx"
                  className="mt-2 px-3 h-10 bg-bg-input border-border-button file:mr-4 file:py-2 file:px-4 file:rounded-full file:border-0 file:text-sm file:font-semibold file:bg-bg-accent file:text-text-primary hover:file:bg-bg-accent/80"
                  onChange={(e) => {
                    const files = e.target.files;
                    onChange(files?.[0]);
                  }}
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />

        <p className="text-sm text-text-secondary">
          <Trans
            i18nKey="admin.importFileTips"
            components={{ code: <code /> }}
          />
        </p>
      </form>
    </Form>
  );
};

// Export the form validation state for parent component
function useImportExcelForm() {
  const { t } = useTranslation();
  const id = useId();

  const schema = useMemo(() => {
    return z.object({
      file: z
        .instanceof(File, { message: t('admin.importFileRequired') })
        .refine(
          (file) => {
            return (
              file.type ===
                'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' ||
              file.name.endsWith('.xlsx')
            );
          },
          {
            message: t('admin.invalidExcelFile'),
          },
        ),
      overwriteExisting: z.boolean().optional(),
    });
  }, [t]);

  const form = useForm<ImportExcelFormData>({
    defaultValues: {
      file: undefined,
      overwriteExisting: false,
    },
    resolver: zodResolver(schema),
  });

  const FormComponent = useCallback(
    (props: Partial<ImportExcelFormProps>) => (
      <ImportExcelForm id={id} form={form} {...props} />
    ),
    [id, form],
  );

  return {
    schema,
    id,
    form,
    FormComponent,
  };
}

export default useImportExcelForm;
