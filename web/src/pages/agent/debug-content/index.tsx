import { FileUploader } from '@/components/file-uploader';
import { ButtonLoading } from '@/components/ui/button';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { RAGFlowSelect } from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { zodResolver } from '@hookform/resolvers/zod';
import React, { ReactNode, useCallback, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { BeginQueryType } from '../constant';
import { BeginQuery } from '../interface';

export const BeginQueryComponentMap = {
  [BeginQueryType.Line]: 'string',
  [BeginQueryType.Paragraph]: 'string',
  [BeginQueryType.Options]: 'string',
  [BeginQueryType.File]: 'file',
  [BeginQueryType.Integer]: 'number',
  [BeginQueryType.Boolean]: 'boolean',
};

const StringFields = [
  BeginQueryType.Line,
  BeginQueryType.Paragraph,
  BeginQueryType.Options,
];

interface IProps {
  parameters: BeginQuery[];
  ok(parameters: any[]): void;
  isNext?: boolean;
  loading?: boolean;
  submitButtonDisabled?: boolean;
  btnText?: ReactNode;
}

const values = {};

const DebugContent = ({
  parameters,
  ok,
  isNext = true,
  loading = false,
  submitButtonDisabled = false,
  btnText,
}: IProps) => {
  const { t } = useTranslation();

  const FormSchema = useMemo(() => {
    const obj = parameters.reduce<Record<string, z.ZodType>>(
      (pre, cur, idx) => {
        const type = cur.type;
        let fieldSchema;
        if (StringFields.some((x) => x === type)) {
          fieldSchema = z.string();
        } else if (type === BeginQueryType.Boolean) {
          fieldSchema = z.boolean();
        } else if (type === BeginQueryType.Integer) {
          fieldSchema = z.coerce.number();
        } else {
          fieldSchema = z.instanceof(File);
        }

        if (cur.optional) {
          fieldSchema.optional();
        }

        pre[idx.toString()] = fieldSchema;

        return pre;
      },
      {},
    );

    return z.object(obj);
  }, [parameters]);

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  const submittable = true;

  const renderWidget = useCallback(
    (q: BeginQuery, idx: string) => {
      const props = {
        key: idx,
        label: q.name ?? q.key,
        name: idx,
      };

      const BeginQueryTypeMap = {
        [BeginQueryType.Line]: (
          <FormField
            control={form.control}
            name={props.name}
            render={({ field }) => (
              <FormItem className="flex-1">
                <FormLabel>{props.label}</FormLabel>
                <FormControl>
                  <Input {...field}></Input>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        ),
        [BeginQueryType.Paragraph]: (
          <FormField
            control={form.control}
            name={props.name}
            render={({ field }) => (
              <FormItem className="flex-1">
                <FormLabel>{props.label}</FormLabel>
                <FormControl>
                  <Textarea rows={1} {...field}></Textarea>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        ),
        [BeginQueryType.Options]: (
          <FormField
            control={form.control}
            name={props.name}
            render={({ field }) => (
              <FormItem className="flex-1">
                <FormLabel>{props.label}</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    allowClear
                    options={
                      q.options?.map((x) => ({
                        label: x,
                        value: x as string,
                      })) ?? []
                    }
                    {...field}
                  ></RAGFlowSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        ),
        [BeginQueryType.File]: (
          <React.Fragment key={idx}>
            <FormField
              control={form.control}
              name={'file'}
              render={({ field }) => (
                <div className="space-y-6">
                  <FormItem className="w-full">
                    <FormLabel>{t('assistantAvatar')}</FormLabel>
                    <FormControl>
                      <FileUploader
                        value={field.value}
                        onValueChange={field.onChange}
                        maxFileCount={1}
                        maxSize={4 * 1024 * 1024}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                </div>
              )}
            />
          </React.Fragment>
        ),
        [BeginQueryType.Integer]: (
          <FormField
            control={form.control}
            name={props.name}
            render={({ field }) => (
              <FormItem className="flex-1">
                <FormLabel>{props.label}</FormLabel>
                <FormControl>
                  <Input type="number" {...field}></Input>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        ),
        [BeginQueryType.Boolean]: (
          <FormField
            control={form.control}
            name={props.name}
            render={({ field }) => (
              <FormItem className="flex-1">
                <FormLabel>{props.label}</FormLabel>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  ></Switch>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        ),
      };

      return (
        BeginQueryTypeMap[q.type as BeginQueryType] ??
        BeginQueryTypeMap[BeginQueryType.Paragraph]
      );
    },
    [form, t],
  );

  const onSubmit = useCallback(
    (values: z.infer<typeof FormSchema>) => {
      console.log('🚀 ~ values:', values);
      const nextValues = Object.entries(values).map(([key, value]) => {
        const item = parameters[Number(key)];
        let nextValue = value;
        if (Array.isArray(value)) {
          nextValue = ``;

          value.forEach((x) => {
            nextValue +=
              x?.originFileObj instanceof File
                ? `${x.name}\n${x.response?.data}\n----\n`
                : `${x.url}\n${x.result}\n----\n`;
          });
        }
        return { ...item, value: nextValue };
      });

      ok(nextValues);
    },
    [ok, parameters],
  );

  return (
    <>
      <section>
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
            {parameters.map((x, idx) => {
              return <div key={idx}>{renderWidget(x, idx.toString())}</div>;
            })}
            <ButtonLoading
              type="submit"
              loading={loading}
              disabled={!submittable || submitButtonDisabled}
              className="w-full"
            >
              {btnText || t(isNext ? 'common.next' : 'flow.run')}
            </ButtonLoading>
          </form>
        </Form>
      </section>
    </>
  );
};

export default DebugContent;
