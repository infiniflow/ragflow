import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Popover, PopoverContent } from '@/components/ui/popover';
import { useParseDocument } from '@/hooks/document-hooks';
import { IModalProps } from '@/interfaces/common';
import { zodResolver } from '@hookform/resolvers/zod';
import { PropsWithChildren } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';

const reg =
  /^(((ht|f)tps?):\/\/)?([^!@#$%^&*?.\s-]([^!@#$%^&*?.\s]{0,63}[^!@#$%^&*?.\s])?\.)+[a-z]{2,6}\/?/;

const FormSchema = z.object({
  url: z.string(),
  result: z.any(),
});

const values = {
  url: '',
  result: null,
};

export const PopoverForm = ({
  children,
  visible,
  switchVisible,
}: PropsWithChildren<IModalProps<any>>) => {
  const form = useForm({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });
  const { parseDocument, loading } = useParseDocument();
  const { t } = useTranslation();

  // useResetFormOnCloseModal({
  //   form,
  //   visible,
  // });

  async function onSubmit(values: z.infer<typeof FormSchema>) {
    const val = values.url;

    if (reg.test(val)) {
      const ret = await parseDocument(val);
      if (ret?.data?.code === 0) {
        form.setValue('result', ret?.data?.data);
      }
    }
  }

  const content = (
    <Form {...form}>
      <form onSubmit={form.handleSubmit(onSubmit)}>
        <FormField
          control={form.control}
          name={`url`}
          render={({ field }) => (
            <FormItem className="flex-1">
              <FormControl>
                <Input
                  {...field}
                  // onPressEnter={(e) => e.preventDefault()}
                  placeholder={t('flow.pasteFileLink')}
                  // suffix={
                  //   <Button
                  //     type="primary"
                  //     onClick={onOk}
                  //     size={'small'}
                  //     loading={loading}
                  //   >
                  //     {t('common.submit')}
                  //   </Button>
                  // }
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name={`result`}
          render={() => <></>}
        />
      </form>
    </Form>
  );

  return (
    <Popover open={visible} onOpenChange={switchVisible}>
      {children}
      <PopoverContent>{content}</PopoverContent>
    </Popover>
  );
};
