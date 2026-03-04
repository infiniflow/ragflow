import MarkdownContent from '@/components/next-markdown-content';
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
import { IMessage } from '@/interfaces/database/chat';
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import React, { ReactNode, useCallback, useMemo } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { BeginQueryType } from '../constant';
import { BeginQuery } from '../interface';
import { FileUploadDirectUpload } from './uploader';

const StringFields = [
  BeginQueryType.Line,
  BeginQueryType.Paragraph,
  BeginQueryType.Options,
];

interface IProps {
  parameters: BeginQuery[];
  message?: IMessage;
  ok(parameters: any[]): void;
  isNext?: boolean;
  loading?: boolean;
  submitButtonDisabled?: boolean;
  btnText?: ReactNode;
  className?: string;
  maxHeight?: string;
}

const DebugContent = ({
  parameters,
  message,
  ok,
  isNext = true,
  loading = false,
  submitButtonDisabled = false,
  btnText,
  className,
  maxHeight,
}: IProps) => {
  const { t } = useTranslation();

  const formSchemaValues = useMemo(() => {
    const obj = parameters.reduce<{
      schema: Record<string, z.ZodType>;
      values: Record<string, any>;
    }>(
      (pre, cur, idx) => {
        const type = cur.type;
        let fieldSchema;
        let value;
        if (StringFields.some((x) => x === type)) {
          fieldSchema = z.string().trim().min(1);
        } else if (type === BeginQueryType.Boolean) {
          fieldSchema = z.boolean();
          value = false;
        } else if (type === BeginQueryType.Integer || type === 'float') {
          fieldSchema = z.coerce.number();
        } else if (type === BeginQueryType.File) {
          fieldSchema = z.array(z.record(z.any())).min(1);
        } else {
          fieldSchema = z.record(z.any());
        }

        if (cur.optional) {
          fieldSchema = fieldSchema.optional();
        }

        const index = idx.toString();

        pre.schema[index] = fieldSchema;
        pre.values[index] = value;

        return pre;
      },
      { schema: {}, values: {} },
    );

    return { schema: z.object(obj.schema), values: obj.values };
  }, [parameters]);

  const form = useForm<z.infer<typeof formSchemaValues.schema>>({
    defaultValues: formSchemaValues.values,
    resolver: zodResolver(formSchemaValues.schema),
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
              name={props.name}
              render={({ field }) => (
                <div className="space-y-6">
                  <FormItem className="w-full">
                    <FormLabel>{props.label}</FormLabel>
                    <FormControl>
                      <FileUploadDirectUpload
                        value={field.value}
                        onChange={field.onChange}
                      ></FileUploadDirectUpload>
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
    [form],
  );

  const onSubmit = useCallback(
    (values: z.infer<typeof formSchemaValues.schema>) => {
      const nextValues = Object.entries(values).map(([key, value]) => {
        const item = parameters[Number(key)];
        return { ...item, value };
      });

      ok(nextValues);
    },
    [formSchemaValues, ok, parameters],
  );
  return (
    <>
      <section className={className}>
        {message?.data?.tips && (
          <div className="mb-2">
            <MarkdownContent
              content={message?.data?.tips}
              loading={false}
            ></MarkdownContent>
          </div>
        )}
        <Form {...form}>
          <form onSubmit={form.handleSubmit(onSubmit)}>
            <section
              className={cn('overflow-auto px-2 space-y-4 pb-4', maxHeight)}
            >
              {parameters.map((x, idx) => {
                return <div key={idx}>{renderWidget(x, idx.toString())}</div>;
              })}
            </section>
            <div className="px-2">
              <ButtonLoading
                type="submit"
                loading={loading}
                disabled={!submittable || submitButtonDisabled}
                className="w-full mt-1"
              >
                {btnText || t(isNext ? 'common.next' : 'flow.run')}
              </ButtonLoading>
            </div>
          </form>
        </Form>
      </section>
    </>
  );
};

export default DebugContent;
