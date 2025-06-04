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
import { useSetModalState } from '@/hooks/common-hooks';
import { useSetSelectedRecord } from '@/hooks/logic-hooks';
import { zodResolver } from '@hookform/resolvers/zod';
import { UploadChangeParam, UploadFile } from 'antd/es/upload';
import React, { useCallback, useMemo, useState } from 'react';
import { useForm } from 'react-hook-form';
import { useTranslation } from 'react-i18next';
import { z } from 'zod';
import { BeginQueryType } from '../constant';
import { BeginQuery } from '../interface';

interface IProps {
  parameters: BeginQuery[];
  ok(parameters: any[]): void;
  isNext?: boolean;
  loading?: boolean;
  submitButtonDisabled?: boolean;
}

const values = {};

const DebugContent = ({
  parameters,
  ok,
  isNext = true,
  loading = false,
  submitButtonDisabled = false,
}: IProps) => {
  const { t } = useTranslation();

  const FormSchema = useMemo(() => {
    const obj = parameters.reduce((pre, cur, idx) => {
      pre[idx] = z.string().optional();
      return pre;
    }, {});
    return z.object(obj);
  }, [parameters]);

  const form = useForm({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  const {
    visible,
    hideModal: hidePopover,
    switchVisible,
    showModal: showPopover,
  } = useSetModalState();
  const { setRecord, currentRecord } = useSetSelectedRecord<number>();
  // const { submittable } = useHandleSubmittable(form);
  const submittable = true;
  const [isUploading, setIsUploading] = useState(false);

  const handleShowPopover = useCallback(
    (idx: number) => () => {
      setRecord(idx);
      showPopover();
    },
    [setRecord, showPopover],
  );

  const normFile = (e: any) => {
    if (Array.isArray(e)) {
      return e;
    }
    return e?.fileList;
  };

  const onChange = useCallback(
    (optional: boolean) =>
      ({ fileList }: UploadChangeParam<UploadFile>) => {
        if (!optional) {
          setIsUploading(fileList.some((x) => x.status === 'uploading'));
        }
      },
    [],
  );

  const renderWidget = useCallback(
    (q: BeginQuery, idx: string) => {
      const props = {
        key: idx,
        label: q.name ?? q.key,
        name: idx,
      };
      if (q.optional === false) {
        props.rules = [{ required: true }];
      }

      // const urlList: { url: string; result: string }[] =
      //   form.getFieldValue(idx) || [];

      const urlList: { url: string; result: string }[] = [];

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
                      q.options?.map((x) => ({ label: x, value: x })) ?? []
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

  const onOk = useCallback(async () => {
    // const values = await form.validateFields();
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
  }, [ok, parameters]);

  const onSubmit = useCallback(
    (values: z.infer<typeof FormSchema>) => {
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
          <form onSubmit={form.handleSubmit(onSubmit)}>
            {parameters.map((x, idx) => {
              return <div key={idx}>{renderWidget(x, idx.toString())}</div>;
            })}
          </form>
        </Form>
      </section>
      <ButtonLoading
        onClick={onOk}
        loading={loading}
        disabled={!submittable || isUploading || submitButtonDisabled}
      >
        {t(isNext ? 'common.next' : 'flow.run')}
      </ButtonLoading>
    </>
  );
};

export default DebugContent;
