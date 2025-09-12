import { ButtonLoading } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { useSelectKnowledgeOptions } from '@/hooks/knowledge-hooks';
import { IModalProps } from '@/interfaces/common';
import { zodResolver } from '@hookform/resolvers/zod';
import { Link2 } from 'lucide-react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { UseHandleConnectToKnowledgeReturnType } from './hooks';

const FormId = 'LinkToDatasetForm';

const FormSchema = z.object({
  knowledgeIds: z.array(z.string()).min(0, {
    message: 'Username must be at least 1 characters.',
  }),
});

function LinkToDatasetForm({
  initialConnectedIds,
  onConnectToKnowledgeOk,
}: Pick<
  UseHandleConnectToKnowledgeReturnType,
  'initialConnectedIds' | 'onConnectToKnowledgeOk'
>) {
  const { t } = useTranslation();
  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    defaultValues: {
      knowledgeIds: initialConnectedIds,
    },
  });

  const options = useSelectKnowledgeOptions();

  function onSubmit(data: z.infer<typeof FormSchema>) {
    onConnectToKnowledgeOk(data.knowledgeIds);
  }

  //   useEffect(() => {
  //     form.setValue('knowledgeIds', initialConnectedIds); // this is invalid
  //   }, [form, initialConnectedIds]);

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className="space-y-6"
        id={FormId}
      >
        <FormField
          control={form.control}
          name="knowledgeIds"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('common.name')}</FormLabel>
              <FormControl>
                <MultiSelect
                  options={options}
                  onValueChange={field.onChange}
                  defaultValue={field.value}
                  placeholder={t('fileManager.pleaseSelect')}
                  maxCount={100}
                  //   {...field}
                  modalPopover
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

export function LinkToDatasetDialog({
  hideModal,
  initialConnectedIds,
  onConnectToKnowledgeOk,
  loading,
}: IModalProps<any> &
  Pick<
    UseHandleConnectToKnowledgeReturnType,
    'initialConnectedIds' | 'onConnectToKnowledgeOk'
  >) {
  const { t } = useTranslation();
  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>{t('fileManager.addToKnowledge')}</DialogTitle>
        </DialogHeader>
        <LinkToDatasetForm
          initialConnectedIds={initialConnectedIds}
          onConnectToKnowledgeOk={onConnectToKnowledgeOk}
        ></LinkToDatasetForm>
        <DialogFooter>
          <ButtonLoading type="submit" form={FormId} loading={loading}>
            <div className="flex gap-2 items-center">
              <Link2 /> Save
            </div>
          </ButtonLoading>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
