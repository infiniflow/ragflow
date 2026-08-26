import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { TagRenameId } from '@/constants/knowledge';
import { IModalProps } from '@/interfaces/common';
import { useTranslation } from 'react-i18next';

import { zodResolver } from '@hookform/resolvers/zod';
import { useForm } from 'react-hook-form';
import { z } from 'zod';

import { ButtonLoading } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { IDocumentInfo } from '@/interfaces/database/document';
import Editor, { loader } from '@monaco-editor/react';
import DOMPurify from 'dompurify';
import { useEffect } from 'react';

loader.config({ paths: { vs: '/vs' } });

export function SetMetaDialog({
  hideModal,
  onOk,
  loading,
  initialMetaData,
}: IModalProps<any> & { initialMetaData?: IDocumentInfo['meta_fields'] }) {
  const { t } = useTranslation();

  const FormSchema = z.object({
    meta: z
      .string()
      .min(1, {
        message: t('knowledgeDetails.pleaseInputJson'),
      })
      .trim()
      .refine(
        (value) => {
          try {
            JSON.parse(value);
            return true;
          } catch (error) {
            return false;
          }
        },
        { message: t('knowledgeDetails.pleaseInputJson') },
      ),
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {},
  });

  async function onSubmit(data: z.infer<typeof FormSchema>) {
    const ret = await onOk?.(data.meta);
    if (ret) {
      hideModal?.();
    }
  }

  useEffect(() => {
    form.setValue('meta', JSON.stringify(initialMetaData, null, 4));
  }, [form, initialMetaData]);

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('knowledgeDetails.setMetaData')}</DialogTitle>
        </DialogHeader>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(onSubmit)}
            className="space-y-6"
            id={TagRenameId}
          >
            <FormField
              control={form.control}
              name="meta"
              render={({ field }) => (
                <FormItem>
                  <FormLabel
                    tooltip={
                      <div
                        dangerouslySetInnerHTML={{
                          __html: DOMPurify.sanitize(
                            t('knowledgeDetails.documentMetaTips'),
                          ),
                        }}
                      ></div>
                    }
                  >
                    {t('knowledgeDetails.metaData')}
                  </FormLabel>
                  <FormControl>
                    <Editor
                      height={200}
                      defaultLanguage="json"
                      theme="vs-dark"
                      {...field}
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          </form>
        </Form>
        <DialogFooter>
          <ButtonLoading type="submit" form={TagRenameId} loading={loading}>
            {t('common.save')}
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
