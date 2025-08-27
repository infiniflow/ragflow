import { Button } from '@/components/ui/button';
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
import { Input } from '@/components/ui/input';
import { IModalProps } from '@/interfaces/common';
import { zodResolver } from '@hookform/resolvers/zod';
import { isEmpty } from 'lodash';
import { useEffect } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { QueryVariable } from '../components/query-variable';
import { VariableFormSchemaType } from './schema';

type ModalFormProps = {
  initialValue: VariableFormSchemaType;
  otherThanCurrentQuery: VariableFormSchemaType[];
  submit(values: any): void;
};

const FormId = 'BeginParameterForm';

function VariableForm({
  initialValue,
  otherThanCurrentQuery,
  submit,
}: ModalFormProps) {
  const { t } = useTranslation();
  const FormSchema = z.object({
    key: z
      .string()
      .trim()
      .min(1)
      .refine(
        (value) =>
          !value || !otherThanCurrentQuery.some((x) => x.key === value),
        { message: 'The key cannot be repeated!' },
      ),
    ref: z.string(),
    value: z.string(),
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
    defaultValues: {
      key: '',
      value: '',
      ref: '',
    },
  });

  useEffect(() => {
    if (!isEmpty(initialValue)) {
      form.reset(initialValue);
    }
  }, [form, initialValue]);

  function onSubmit(data: z.infer<typeof FormSchema>) {
    submit(data);
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        id={FormId}
        className="space-y-5"
        autoComplete="off"
      >
        <FormField
          name="key"
          control={form.control}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('flow.key')}</FormLabel>
              <FormControl>
                <Input {...field} autoComplete="off" />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <QueryVariable name="ref" label={t('flow.ref')}></QueryVariable>
        <FormField
          name="value"
          control={form.control}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('flow.value')}</FormLabel>
              <FormControl>
                <Input {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </form>
    </Form>
  );
}

export function VariableDialog({
  initialValue,
  hideModal,
  otherThanCurrentQuery,
  submit,
}: ModalFormProps & IModalProps<VariableFormSchemaType>) {
  const { t } = useTranslation();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('flow.variableSettings')}</DialogTitle>
        </DialogHeader>
        <VariableForm
          initialValue={initialValue}
          otherThanCurrentQuery={otherThanCurrentQuery}
          submit={submit}
        ></VariableForm>
        <DialogFooter>
          <Button type="submit" form={FormId}>
            {t('modal.okText')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
