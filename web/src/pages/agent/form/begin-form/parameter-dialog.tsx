import { KeyInput } from '@/components/key-input';
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
import { RAGFlowSelect, RAGFlowSelectOptionType } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { useTranslate } from '@/hooks/common-hooks';
import { IModalProps } from '@/interfaces/common';
import { zodResolver } from '@hookform/resolvers/zod';
import { isEmpty } from 'lodash';
import { ChangeEvent, useEffect, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { BeginQueryType, BeginQueryTypeIconMap } from '../../constant';
import { BeginQuery } from '../../interface';
import { BeginDynamicOptions } from './begin-dynamic-options';

type ModalFormProps = {
  initialValue: BeginQuery;
  otherThanCurrentQuery: BeginQuery[];
  submit(values: any): void;
};

const FormId = 'BeginParameterForm';

function ParameterForm({
  initialValue,
  otherThanCurrentQuery,
  submit,
}: ModalFormProps) {
  const { t } = useTranslate('flow');
  const FormSchema = z.object({
    type: z.string(),
    key: z
      .string()
      .trim()
      .min(1)
      .refine(
        (value) =>
          !value || !otherThanCurrentQuery.some((x) => x.key === value),
        { message: 'The key cannot be repeated!' },
      ),
    optional: z.boolean(),
    name: z.string().trim().min(1),
    options: z
      .array(z.object({ value: z.string().or(z.boolean()).or(z.number()) }))
      .optional(),
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    resolver: zodResolver(FormSchema),
    mode: 'onChange',
    defaultValues: {
      type: BeginQueryType.Line,
      optional: false,
      key: '',
      name: '',
      options: [],
    },
  });

  const options = useMemo(() => {
    return Object.values(BeginQueryType).reduce<RAGFlowSelectOptionType[]>(
      (pre, cur) => {
        const Icon = BeginQueryTypeIconMap[cur];

        return [
          ...pre,
          {
            label: (
              <div className="flex items-center gap-2">
                <Icon
                  className={`size-${cur === BeginQueryType.Options ? 4 : 5}`}
                ></Icon>
                {t(cur.toLowerCase())}
              </div>
            ),
            value: cur,
          },
        ];
      },
      [],
    );
  }, [t]);

  const type = useWatch({
    control: form.control,
    name: 'type',
  });

  useEffect(() => {
    if (!isEmpty(initialValue)) {
      form.reset({
        ...initialValue,
        options: initialValue.options?.map((x) => ({ value: x })),
      });
    }
  }, [form, initialValue]);

  function onSubmit(data: z.infer<typeof FormSchema>) {
    const values = { ...data, options: data.options?.map((x) => x.value) };

    submit(values);
  }

  const handleKeyChange = (e: ChangeEvent<HTMLInputElement>) => {
    const name = form.getValues().name || '';
    form.setValue('key', e.target.value.trim());
    if (!name) {
      form.setValue('name', e.target.value.trim());
    }
  };
  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        id={FormId}
        className="space-y-5"
        autoComplete="off"
      >
        <FormField
          name="type"
          control={form.control}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('type')}</FormLabel>
              <FormControl>
                <RAGFlowSelect {...field} options={options} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          name="key"
          control={form.control}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('key')}</FormLabel>
              <FormControl>
                <KeyInput
                  {...field}
                  autoComplete="off"
                  onBlur={handleKeyChange}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          name="name"
          control={form.control}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('name')}</FormLabel>
              <FormControl>
                <Input {...field} />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          name="optional"
          control={form.control}
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('optional')}</FormLabel>
              <FormControl>
                <Switch
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        {type === BeginQueryType.Options && (
          <BeginDynamicOptions></BeginDynamicOptions>
        )}
      </form>
    </Form>
  );
}

export function ParameterDialog({
  initialValue,
  hideModal,
  otherThanCurrentQuery,
  submit,
}: ModalFormProps & IModalProps<BeginQuery>) {
  const { t } = useTranslation();

  return (
    <Dialog open onOpenChange={hideModal}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('flow.variableSettings')}</DialogTitle>
        </DialogHeader>
        <ParameterForm
          initialValue={initialValue}
          otherThanCurrentQuery={otherThanCurrentQuery}
          submit={submit}
        ></ParameterForm>
        <DialogFooter>
          <Button type="submit" form={FormId}>
            {t('modal.okText')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
