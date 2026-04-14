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
import { useCallback, useId, useMemo, useRef } from 'react';
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
  const filePickerRef = useRef<HTMLInputElement | null>(null);

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
                <button
                  type="button"
                  className="
                    group
                    w-full h-10 px-2 py-1 flex items-center gap-4 bg-bg-input
                    rounded-md border-0.5 border-border-button"
                  onClick={() => {
                    filePickerRef.current?.click();
                  }}
                >
                  <div
                    className="
                    h-6 px-2 py-1 flex items-center justify-center text-sm text-text-primary rounded
                    bg-bg-input border-0.5 border-border-default transition-colors
                    group-hover:bg-border-button group-focus-visible:bg-border-button"
                  >
                    {t('admin.selectFile')}
                  </div>

                  <span className="text-sm text-text-secondary">
                    {value ? value.name : t('admin.noFileSelected')}
                  </span>
                </button>
              </FormControl>

              <Input
                type="file"
                accept=".xlsx"
                className="hidden"
                tabIndex={-1}
                onChange={(e) => {
                  const files = e.target.files;
                  onChange(files?.[0]);
                }}
                {...field}
                ref={(ref) => {
                  filePickerRef.current = ref;
                  field.ref(ref);
                }}
              />

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
