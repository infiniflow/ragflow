import MarkdownContent from '@/components/next-markdown-content';
import JsonEditor from '@/components/json-edit';
import { SelectWithSearch } from '@/components/originui/select-with-search';
import { RAGFlowFormItem } from '@/components/ragflow-form';
import { SwitchFormField } from '@/components/switch-form-field';
import { ButtonLoading } from '@/components/ui/button';
import { Form } from '@/components/ui/form';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { IMessage } from '@/interfaces/database/chat';
import { cn } from '@/lib/utils';
import { zodResolver } from '@hookform/resolvers/zod';
import { ReactNode, useCallback, useMemo } from 'react';
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
  initialValues?: Record<string, unknown>;
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
  initialValues,
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
        } else if (type === BeginQueryType.Object) {
          fieldSchema = z.union([z.record(z.any()), z.array(z.any())]);
          value = {};
        } else {
          fieldSchema = z.record(z.any());
        }

        if (cur.optional) {
          fieldSchema = z.preprocess(
            (val) => (val === '' ? undefined : val),
            fieldSchema.optional(),
          );
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
    values: initialValues
      ? Object.fromEntries(
          Object.entries(formSchemaValues.values).map(([key, value]) => [
            key,
            initialValues[key] ?? value,
          ]),
        )
      : undefined,
    resetOptions: { keepDirtyValues: true },
    resolver: zodResolver(formSchemaValues.schema),
  });

  const submittable = true;

  const renderWidget = useCallback((q: BeginQuery, idx: string) => {
    const label = q.name ?? q.key;

    const BeginQueryTypeMap = {
      [BeginQueryType.Line]: (
        <RAGFlowFormItem name={idx} label={label}>
          <Input />
        </RAGFlowFormItem>
      ),
      [BeginQueryType.Paragraph]: (
        <RAGFlowFormItem name={idx} label={label}>
          <Textarea rows={1} />
        </RAGFlowFormItem>
      ),
      [BeginQueryType.Options]: (
        <RAGFlowFormItem name={idx} label={label}>
          <SelectWithSearch
            allowClear
            options={
              q.options?.map((x) => ({
                label: x,
                value: x as string,
              })) ?? []
            }
          />
        </RAGFlowFormItem>
      ),
      [BeginQueryType.File]: (
        <RAGFlowFormItem name={idx} label={label}>
          {(field) => (
            <FileUploadDirectUpload
              value={field.value}
              onChange={field.onChange}
            />
          )}
        </RAGFlowFormItem>
      ),
      [BeginQueryType.Integer]: (
        <RAGFlowFormItem name={idx} label={label}>
          <Input type="number" />
        </RAGFlowFormItem>
      ),
      [BeginQueryType.Boolean]: <SwitchFormField name={idx} label={label} />,
      [BeginQueryType.Object]: (
        <RAGFlowFormItem name={idx} label={label}>
          {(field) => (
            <JsonEditor
              value={field.value ?? {}}
              onChange={field.onChange}
              height="200px"
            />
          )}
        </RAGFlowFormItem>
      ),
    };

    return (
      BeginQueryTypeMap[q.type as BeginQueryType] ??
      BeginQueryTypeMap[BeginQueryType.Paragraph]
    );
  }, []);

  const onSubmit = useCallback(
    (values: z.infer<typeof formSchemaValues.schema>) => {
      const nextValues = Object.entries(values).map(([key, value]) => {
        const item = parameters[Number(key)];
        return { ...item, value };
      });

      ok(nextValues);
    },
    [ok, parameters],
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
