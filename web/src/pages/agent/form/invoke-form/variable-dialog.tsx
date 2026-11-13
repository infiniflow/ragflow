import { Button } from '@/components/ui/button';
import { SelectWithSearch } from '@/components/originui/select-with-search';
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
import { Textarea } from '@/components/ui/textarea';
import { IModalProps } from '@/interfaces/common';
import { zodResolver } from '@hookform/resolvers/zod';
import { isEmpty } from 'lodash';
import { useEffect } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { InvokeParameterTypeOptions } from '../../constant';
import { QueryVariable } from '../components/query-variable';
import { VariableFormSchema, VariableFormSchemaType } from './schema';

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
  const FormSchema = VariableFormSchema.superRefine(({ key }, context) => {
    if (
      !key.trim() ||
      otherThanCurrentQuery.some((x) => x.key === key.trim())
    ) {
      context.addIssue({
        code: z.ZodIssueCode.custom,
        path: ['key'],
        message: !key.trim() ? 'Required' : 'The key cannot be repeated!',
      });
    }
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
    defaultValues: {
      key: '',
      value: '',
      ref: '',
      type: 'string',
    },
  });

  useEffect(() => {
    if (!isEmpty(initialValue)) {
      form.reset({ type: 'string', ...initialValue });
    }
  }, [form, initialValue]);

  function onSubmit(data: z.infer<typeof FormSchema>) {
    submit({ ...data, key: data.key.trim() });
  }

  const parameterType = useWatch({ control: form.control, name: 'type' });

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
          name="type"
          control={form.control}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('flow.type')}</FormLabel>
              <FormControl>
                <SelectWithSearch
                  value={field.value ?? 'string'}
                  onChange={field.onChange}
                  options={InvokeParameterTypeOptions}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          name="value"
          control={form.control}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('flow.value')}</FormLabel>
              <FormControl>
                {parameterType === 'object' ? (
                  <Textarea {...field} className="font-mono" rows={5} />
                ) : (
                  <Input {...field} />
                )}
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
