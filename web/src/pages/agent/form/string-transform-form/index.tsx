import { FormContainer } from '@/components/form-container';
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { MultiSelect } from '@/components/ui/multi-select';
import { RAGFlowSelect } from '@/components/ui/select';
import { zodResolver } from '@hookform/resolvers/zod';
import { t } from 'i18next';
import { toLower } from 'lodash';
import { memo, useCallback, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';
import {
  StringTransformDelimiter,
  StringTransformMethod,
  initialStringTransformValues,
} from '../../constant';
import { INextOperatorForm } from '../../interface';
import { FormWrapper } from '../components/form-wrapper';
import { Output, transferOutputs } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { QueryVariable } from '../components/query-variable';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-form-change';

const DelimiterOptions = Object.entries(StringTransformDelimiter).map(
  ([key, val]) => ({ label: t('flow.' + toLower(key)), value: val }),
);

function StringTransformForm({ node }: INextOperatorForm) {
  const values = useValues(node);

  const FormSchema = z.object({
    method: z.string(),
    split_ref: z.string().optional(),
    script: z.string().optional(),
    delimiters: z.array(z.string()).or(z.string()),
    outputs: z.object({ result: z.object({ type: z.string() }) }).optional(),
  });

  const form = useForm<z.infer<typeof FormSchema>>({
    defaultValues: values,
    resolver: zodResolver(FormSchema),
  });

  const method = useWatch({ control: form.control, name: 'method' });

  const isSplit = method === StringTransformMethod.Split;

  const outputList = useMemo(() => {
    return transferOutputs(values.outputs);
  }, [values.outputs]);

  const handleMethodChange = useCallback(
    (value: StringTransformMethod) => {
      const isMerge = value === StringTransformMethod.Merge;
      const outputs = {
        ...initialStringTransformValues.outputs,
        result: {
          type: isMerge ? 'string' : 'Array<string>',
        },
      };
      form.setValue('outputs', outputs);
      form.setValue(
        'delimiters',
        isMerge ? StringTransformDelimiter.Comma : [],
      );
    },
    [form],
  );

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <FormWrapper>
        <FormContainer>
          <FormField
            control={form.control}
            name="method"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.method')}</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    {...field}
                    options={Object.values(StringTransformMethod).map(
                      (val) => ({ label: t('flow.' + val), value: val }),
                    )}
                    onChange={(value) => {
                      handleMethodChange(value);
                      field.onChange(value);
                    }}
                  ></RAGFlowSelect>
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          {isSplit && (
            <QueryVariable
              label={<FormLabel>split_ref</FormLabel>}
              name="split_ref"
            ></QueryVariable>
          )}
          {isSplit || (
            <FormField
              control={form.control}
              name="script"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('flow.script')}</FormLabel>
                  <FormControl>
                    <PromptEditor {...field} showToolbar={false}></PromptEditor>
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}
          <FormField
            control={form.control}
            name="delimiters"
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('flow.delimiters')}</FormLabel>
                <FormControl>
                  {isSplit ? (
                    <MultiSelect
                      options={DelimiterOptions}
                      onValueChange={field.onChange}
                      defaultValue={field.value as string[]}
                      variant="inverted"
                      // {...field}
                    />
                  ) : (
                    <RAGFlowSelect
                      {...field}
                      options={DelimiterOptions}
                    ></RAGFlowSelect>
                  )}
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name="outputs"
            render={() => <div></div>}
          />
        </FormContainer>
      </FormWrapper>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
}

export default memo(StringTransformForm);
