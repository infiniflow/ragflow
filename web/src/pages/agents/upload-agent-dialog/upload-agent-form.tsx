'use client';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { FileUploader } from '@/components/file-uploader';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { FileMimeType } from '@/constants/common';
import { TagRenameId } from '@/constants/knowledge';
import { IModalProps } from '@/interfaces/common';
import { NameFormField, NameFormSchema } from '../name-form-field';

export const FormSchema = z.object({
  fileList: z.array(z.instanceof(File)),
  ...NameFormSchema,
});

export type FormSchemaType = z.infer<typeof FormSchema>;
export function UploadAgentForm({ hideModal, onOk }: IModalProps<any>) {
  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: { name: '' },
  });

  async function onSubmit(data: FormSchemaType) {
    const ret = await onOk?.(data);
    if (ret) {
      hideModal?.();
    }
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
        id={TagRenameId}
      >
        <NameFormField></NameFormField>
        <FormField
          control={form.control}
          name="fileList"
          render={({ field }) => (
            <FormItem>
              <FormLabel required>DSL</FormLabel>
              <FormControl>
                <FileUploader
                  className="text-ellipsis overflow-hidden"
                  value={field.value}
                  onValueChange={field.onChange}
                  maxFileCount={1}
                  accept={{ '*.json': [FileMimeType.Json] }}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  );
}
