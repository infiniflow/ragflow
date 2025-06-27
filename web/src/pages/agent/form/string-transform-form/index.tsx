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
import { buildOptions } from '@/utils/form';
import { zodResolver } from '@hookform/resolvers/zod';
import { useCallback, useMemo } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import { z } from 'zod';
import {
  StringTransformDelimiter,
  StringTransformMethod,
  initialStringTransformValues,
} from '../../constant';
import { INextOperatorForm } from '../../interface';
import { Output, transferOutputs } from '../components/output';
import { PromptEditor } from '../components/prompt-editor';
import { QueryVariable } from '../components/query-variable';
import { useValues } from './use-values';
import { useWatchFormChange } from './use-watch-form-change';

const DelimiterOptions = Object.entries(StringTransformDelimiter).map(
  ([key, val]) => ({ label: key, value: val }),
);

export const StringTransformForm = ({ node }: INextOperatorForm) => {
  const values = useValues(node);

  const FormSchema = z.object({
    method: z.string(),
    split_ref: z.string().optional(),
    script: z.string().optional(),
    delimiters: z.array(z.string()),
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
      const outputs = {
        ...initialStringTransformValues.outputs,
        result: {
          type:
            value === StringTransformMethod.Merge ? 'string' : 'Array<string>',
        },
      };
      form.setValue('outputs', outputs);
    },
    [form],
  );

  useWatchFormChange(node?.id, form);

  return (
    <Form {...form}>
      <form
        className="space-y-5 px-5 "
        autoComplete="off"
        onSubmit={(e) => {
          e.preventDefault();
        }}
      >
        <FormContainer>
          <FormField
            control={form.control}
            name="method"
            render={({ field }) => (
              <FormItem>
                <FormLabel>method</FormLabel>
                <FormControl>
                  <RAGFlowSelect
                    {...field}
                    options={buildOptions(StringTransformMethod)}
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
                  <FormLabel>script</FormLabel>
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
                <FormLabel>delimiters</FormLabel>
                <FormControl>
                  {isSplit ? (
                    <MultiSelect
                      options={DelimiterOptions}
                      onValueChange={field.onChange}
                      variant="inverted"
                      {...field}
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
      </form>
      <div className="p-5">
        <Output list={outputList}></Output>
      </div>
    </Form>
  );
};
